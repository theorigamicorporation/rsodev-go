package platform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestReadsAreRetriedAndWritesAreNot(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if hits.Add(1) < 3 {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte("upstream is restarting"))
			return
		}
		_, _ = w.Write([]byte(`{"data":{"buckets":[{"name":"assets"}]}}`))
	}))
	defer server.Close()
	c, err := New(WithAccessToken("token"), WithEndpoint(server.URL), WithRetry(RetryPolicy{Attempts: 3, Base: time.Millisecond}))
	if err != nil {
		t.Fatal(err)
	}
	buckets, err := c.ListBuckets(context.Background())
	if err != nil || len(buckets) != 1 || hits.Load() != 3 {
		t.Fatalf("buckets %v, err %v after %d requests", buckets, err, hits.Load())
	}

	hits.Store(0)
	_, err = c.CreateBucket(context.Background(), "assets")
	if err == nil || hits.Load() != 1 || KindOf(err) != KindUnavailable {
		t.Fatalf("a write was sent %d times, err %v", hits.Load(), err)
	}
}

func TestClientWithoutTenantSendsNoTenantHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, set := r.Header["X-Tenant"]; set {
			t.Errorf("X-Tenant was sent: %v", r.Header)
		}
		if agent := r.Header.Get("User-Agent"); agent != "tool/1 rsodev-go/"+Version {
			t.Errorf("User-Agent %q", agent)
		}
		_, _ = w.Write([]byte(`{"data":{"myUser":{"id":"1","username":"ana","tenants":[{"id":"acme","name":"Acme","region":"EU","myRoles":["ADMIN"]}]}}}`))
	}))
	defer server.Close()
	c, err := New(WithAccessToken("token"), WithEndpoint(server.URL), WithUserAgent("tool/1"))
	if err != nil {
		t.Fatal(err)
	}
	me, err := c.Me(context.Background())
	if err != nil || me.Username != "ana" || len(me.Tenants) != 1 || me.Tenants[0].Roles[0] != "ADMIN" {
		t.Fatalf("%+v, %v", me, err)
	}
	if err := c.DeleteApp(context.Background(), "web"); err == nil {
		t.Error("deleting an app without a tenant should be refused before any request")
	}
}

func TestErrorKinds(t *testing.T) {
	cases := []struct {
		code   string
		status int
		kind   Kind
	}{
		{"FORBIDDEN", 200, KindForbidden},
		{"AUTHORIZATION_ERROR", 200, KindForbidden},
		{"AUTHENTICATION_ERROR", 200, KindUnauthenticated},
		{"NOT_FOUND", 200, KindNotFound},
		{"CONFLICT", 200, KindConflict},
		{"QUOTA_EXCEEDED", 200, KindQuota},
		{"insufficient_quota", 429, KindQuota},
		{"BAD_USER_INPUT", 400, KindInvalid},
		{"", 503, KindUnavailable},
		{"", 418, KindOther},
	}
	for _, tc := range cases {
		err := fmt.Errorf("wrapped: %w", &Error{StatusCode: tc.status, Message: "no", code: tc.code})
		if got := KindOf(err); got != tc.kind {
			t.Errorf("code %q status %d: kind %d, want %d", tc.code, tc.status, got, tc.kind)
		}
	}
	if !IsNotFound(notFound("app", "web")) || IsNotFound(errors.New("other")) {
		t.Error("IsNotFound")
	}
	if KindOf(ErrSessionExpired) != KindUnauthenticated || KindOf(&DataError{StatusCode: 404}) != KindNotFound {
		t.Error("sign-in and data API errors are not classified")
	}
}

func TestAllCollectsEveryPage(t *testing.T) {
	g := newGateway(t)
	g.handle = func(_ string, variables map[string]any) (any, string, string) {
		list, _ := variables["list"].(map[string]any)
		skip, _ := list["skip"].(float64)
		limit, _ := list["limit"].(float64)
		items := []map[string]any{}
		for i := int(skip); i < int(skip+limit) && i < 5; i++ {
			items = append(items, map[string]any{"name": fmt.Sprintf("registry-%d", i), "domain": "r.example.com", "status": "READY"})
		}
		return map[string]any{"dockerRegistries": map[string]any{"total": 5, "items": items}}, "", ""
	}
	registries, err := All(context.Background(), 2, g.client().RegistriesPage)
	if err != nil || len(registries) != 5 || registries[4].Name != "registry-4" || len(g.queries) != 3 {
		t.Fatalf("%d registries in %d requests, err %v", len(registries), len(g.queries), err)
	}
}

func TestIngressRouteRoundTrip(t *testing.T) {
	g := newGateway(t)
	g.handle = func(_ string, variables map[string]any) (any, string, string) {
		input := variables["input"].(map[string]any)
		rule := input["routes"].([]any)[0].(map[string]any)
		service := rule["services"].([]any)[0].(map[string]any)
		if service["externalServiceId"] != "vm" || rule["middlewares"].([]any)[0].(map[string]any)["name"] != "auth" {
			t.Errorf("unexpected input: %v", input)
		}
		return map[string]any{"createIngressRoute": map[string]any{"name": "shop", "routes": []map[string]any{{
			"match": rule["match"], "middlewares": []map[string]any{{"name": "auth"}}, "services": []map[string]any{{"name": "vm", "port": 80}},
		}}}}, "", ""
	}
	route, err := g.client().CreateIngressRoute(context.Background(), IngressRoute{Name: "shop", Routes: []RouteRule{{
		Match: "Host(`shop.example.com`)", Middlewares: []string{"auth"}, Services: []RouteService{{Name: "vm", Port: 80}},
	}}})
	if err != nil || route.Routes[0].Middlewares[0] != "auth" || route.Routes[0].Services[0].Port != 80 {
		t.Fatalf("%+v, %v", route, err)
	}
}

func TestWaitAppReady(t *testing.T) {
	g := newGateway(t)
	reads := 0
	g.handle = func(string, map[string]any) (any, string, string) {
		reads++
		status := map[string]any{}
		if reads > 1 {
			status = map[string]any{"url": "https://web.example.com", "latestReadyRevisionName": "web-00001"}
		}
		return map[string]any{"app": map[string]any{"name": "web", "status": status, "template": map[string]any{"spec": map[string]any{"containers": []any{}}}}}, "", ""
	}
	app, err := g.client().WaitAppReady(context.Background(), "web", WaitOptions{Interval: time.Millisecond})
	if err != nil || !app.Ready() || *app.Status.URL != "https://web.example.com" || reads != 2 {
		t.Fatalf("%+v, %v after %d reads", app, err, reads)
	}
}

// identity is a stand-in for the identity provider.
func identity(t *testing.T, pending *int) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/o/device/", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"device_code": "dev", "user_code": "ABCD-EFGH", "verification_uri": "https://id.example.com/device",
			"verification_uri_complete": "https://id.example.com/device?code=ABCD-EFGH", "expires_in": 600, "interval": 1})
	})
	mux.HandleFunc("/o/token/", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		switch r.Form.Get("grant_type") {
		case "refresh_token":
			if r.Form.Get("refresh_token") != "refresh" {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": "invalid_grant"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "renewed", "expires_in": 300})
		default:
			if *pending > 0 {
				*pending--
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": "authorization_pending"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "access", "refresh_token": "refresh", "expires_in": 300})
		}
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func TestDeviceSignInAndRenewal(t *testing.T) {
	pending := 1
	server := identity(t, &pending)
	oauth := OAuth{TokenURL: server.URL + "/o/token/"}
	ctx := context.Background()

	code, err := oauth.StartDevice(ctx)
	if err != nil || code.UserCode == "" || !strings.Contains(code.VerificationURIComplete, code.UserCode) {
		t.Fatalf("%+v, %v", code, err)
	}
	set, err := oauth.WaitDevice(ctx, code)
	if err != nil || set.AccessToken != "access" || set.RefreshToken != "refresh" || set.Expiry.IsZero() {
		t.Fatalf("%+v, %v", set, err)
	}

	var saved TokenSet
	set.Expiry = time.Now().Add(-time.Minute)
	source := &RefreshingSource{OAuth: oauth, Set: set, Save: func(s TokenSet) error { saved = s; return nil }}
	token, err := source.Token(ctx)
	if err != nil || token != "renewed" || saved.AccessToken != "renewed" || saved.RefreshToken != "refresh" {
		t.Fatalf("token %q, saved %+v, err %v", token, saved, err)
	}

	revoked := &RefreshingSource{OAuth: oauth, Set: TokenSet{AccessToken: "old", RefreshToken: "revoked", Expiry: time.Now().Add(-time.Minute)}}
	if _, err := revoked.Token(ctx); !errors.Is(err, ErrSessionExpired) || !errors.Is(err, ErrNotSignedIn) {
		t.Errorf("a refused renewal: %v", err)
	}

	pending = 1000
	short, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	if _, err := oauth.WaitDevice(short, code); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("a cancelled wait: %v", err)
	}
}

func TestChatStreamsAndReportsUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ai/v1/chat/completions" {
			t.Errorf("path %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(": waiting\n\ndata: {\"choices\":[{\"delta\":{\"content\":\"Hel\"}}]}\n\ndata: {\"choices\":[{\"delta\":{\"content\":\"lo\"}}],\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":2}}\n\ndata: [DONE]\n\n"))
	}))
	defer server.Close()
	c, err := New(WithTenant("acme"), WithAccessToken("token"), WithEndpoint(server.URL+"/graphql"))
	if err != nil {
		t.Fatal(err)
	}
	var text strings.Builder
	usage, err := c.Chat(context.Background(), "model", []ChatMessage{{Role: "user", Content: "hi"}}, func(piece string) { text.WriteString(piece) })
	if err != nil || text.String() != "Hello" || usage == nil || usage.CompletionTokens != 2 {
		t.Fatalf("%q, %+v, %v", text.String(), usage, err)
	}
}

func TestIngressRouteWithoutCertificateSendsAnExplicitNull(t *testing.T) {
	var input map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Query     string `json:"query"`
			Variables struct {
				Input map[string]any `json:"input"`
			} `json:"variables"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if strings.Contains(body.Query, "createIngressRoute") {
			input = body.Variables.Input
			_, _ = w.Write([]byte(`{"data":{"createIngressRoute":{"name":"plain","entryPoints":["e"],"certResolver":null,"routes":[]}}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":{"ingressRoute":{"name":"","routes":[]}}}`))
	}))
	defer server.Close()
	c, err := New(WithAccessToken("token"), WithEndpoint(server.URL))
	if err != nil {
		t.Fatal(err)
	}
	transport := "slow"
	route, err := c.CreateIngressRoute(context.Background(), IngressRoute{Name: "plain", NoTLS: true,
		Routes: []RouteRule{{Match: "Host(`plain.example.com`)", Services: []RouteService{{Name: "vm", Port: 80, ServersTransport: &transport}}}}})
	if err != nil || !route.NoTLS {
		t.Fatalf("%+v, %v", route, err)
	}
	if value, present := input["certResolver"]; !present || value != nil {
		t.Fatalf("certResolver must be an explicit null, got %v (present %v)", value, present)
	}
	service := input["routes"].([]any)[0].(map[string]any)["services"].([]any)[0].(map[string]any)
	if service["serversTransport"] != "slow" {
		t.Fatalf("serversTransport not sent: %v", service)
	}
	if _, err := c.GetIngressRoute(context.Background(), "gone"); !IsNotFound(err) {
		t.Fatalf("a missing route must be not found, got %v", err)
	}
}

func TestCostEstimateSendsOnlyWhatIsAsked(t *testing.T) {
	var input map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Variables struct {
				Input map[string]any `json:"input"`
			} `json:"variables"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		input = body.Variables.Input
		_, _ = w.Write([]byte(`{"data":{"costEstimate":{"currency":"EUR","hourlyTotal":0.5,"monthlyTotal":365,"hoursPerMonth":730,"lines":[{"sku":"vcpu","qty":2}]}}}`))
	}))
	defer server.Close()
	c, err := New(WithAccessToken("token"), WithEndpoint(server.URL))
	if err != nil {
		t.Fatal(err)
	}
	estimate, err := c.CostEstimate(context.Background(), EstimateInput{Instances: []EstimateItem{{VCPU: 2, RAMGB: 4, DiskGB: 20}}})
	if err != nil || estimate.Currency != "EUR" || estimate.MonthlyTotal != 365 || len(estimate.Lines) != 1 {
		t.Fatalf("%+v, %v", estimate, err)
	}
	if _, sent := input["apps"]; sent || len(input["instances"].([]any)) != 1 {
		t.Fatalf("input: %v", input)
	}
	if _, sent := input["instances"].([]any)[0].(map[string]any)["count"]; sent {
		t.Fatalf("a zero count must be left out: %v", input)
	}
}

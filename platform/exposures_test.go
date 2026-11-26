package platform

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestExposeHostSendsOnlyWhatIsSet(t *testing.T) {
	g := newGateway(t)
	var inputs []map[string]any
	var namespaces []any
	g.handle = func(_ string, variables map[string]any) (any, string, string) {
		input := variables["input"].(map[string]any)
		inputs = append(inputs, input)
		namespaces = append(namespaces, variables["namespace"])
		return map[string]any{"exposeHost": map[string]any{"host": input["host"], "namespace": "acme", "status": "PENDING", "tls": "AUTO",
			"target": input["target"], "dnsTarget": []string{}}}, "", ""
	}
	c := g.client()
	target := ExposureTarget{Address: "198.51.100.10", Port: 8443, Scheme: ExposureHTTPS}

	got, err := c.ExposeHost(context.Background(), ExposeHostInput{Host: "shop.example.com", Target: target})
	if err != nil || got.Host != "shop.example.com" || got.Status != ExposurePending || got.Target == nil || got.Target.Port != 8443 {
		t.Fatalf("%+v, %v", got, err)
	}
	if len(inputs[0]) != 2 || namespaces[0] != nil {
		t.Errorf("a plain request carried more than host and target: %v, namespace %v", inputs[0], namespaces[0])
	}

	yes := true
	_, err = c.ExposeHost(context.Background(), ExposeHostInput{Host: "shop.example.com", Target: target, TLS: ExposureTLSNone,
		BasicAuth: &ExposureBasicAuth{Username: "u", Password: "long enough"}, Replace: true, Namespace: "team", Proxied: &yes, TakeOver: true})
	if err != nil {
		t.Fatal(err)
	}
	full := inputs[1]
	if full["tls"] != "NONE" || full["replace"] != true || full["proxied"] != true || full["takeOver"] != true ||
		full["basicAuth"].(map[string]any)["username"] != "u" || namespaces[1] != "team" {
		t.Errorf("unexpected input: %v, namespace %v", full, namespaces[1])
	}

	if _, err = c.ExposeHost(context.Background(), ExposeHostInput{Host: "shop.example.com", Target: target, RemoveBasicAuth: true}); err != nil {
		t.Fatal(err)
	}
	if value, present := inputs[2]["basicAuth"]; !present || value != nil {
		t.Errorf("removing the sign-in must send an explicit null: %v", inputs[2])
	}
}

func TestExposuresArePagedPerNamespace(t *testing.T) {
	g := newGateway(t)
	g.handle = func(query string, variables map[string]any) (any, string, string) {
		switch {
		case strings.Contains(query, "unexposeHost"):
			if variables["host"] != "a.example.com" || variables["namespace"] != "team" {
				t.Errorf("unexpected variables: %v", variables)
			}
			return map[string]any{"unexposeHost": true}, "", ""
		case strings.Contains(query, "exposures("):
			if variables["namespace"] != "team" {
				t.Errorf("unexpected namespace: %v", variables["namespace"])
			}
			list := variables["list"].(map[string]any)
			if list["skip"] == nil {
				return map[string]any{"exposures": map[string]any{"total": 2, "items": []map[string]any{{"host": "a.example.com"}}}}, "", ""
			}
			return map[string]any{"exposures": map[string]any{"total": 2, "items": []map[string]any{{"host": "b.example.com"}}}}, "", ""
		}
		return nil, CodeNotFound, "no such host"
	}
	c := g.client()
	all, err := c.ListExposures(context.Background(), "team")
	if err != nil || len(all) != 2 || all[1].Host != "b.example.com" {
		t.Fatalf("%+v, %v", all, err)
	}
	if err := c.UnexposeHost(context.Background(), "a.example.com", "team"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.GetExposure(context.Background(), "c.example.com", ""); !IsNotFound(err) {
		t.Fatalf("want a not-found error, got %v", err)
	}
}

func TestDomainVerificationReportsItsCode(t *testing.T) {
	g := newGateway(t)
	g.handle = func(query string, variables map[string]any) (any, string, string) {
		if variables["domain"] != "example.com" {
			t.Errorf("unexpected variables: %v", variables)
		}
		if strings.Contains(query, "verifyDomain") {
			return nil, CodeVerificationFailed, "the TXT record was not found"
		}
		return map[string]any{"registerDomain": map[string]any{"domain": "example.com", "status": "PENDING",
			"challengeName": "_challenge.example.com", "challengeValue": "value"}}, "", ""
	}
	c := g.client()
	domain, err := c.RegisterDomain(context.Background(), "example.com")
	if err != nil || domain.Status != DomainPending || domain.ChallengeName != "_challenge.example.com" || domain.VerifiedAt != nil {
		t.Fatalf("%+v, %v", domain, err)
	}
	if _, err := c.VerifyDomain(context.Background(), "example.com"); !HasCode(err, CodeVerificationFailed) {
		t.Fatalf("want %s, got %v", CodeVerificationFailed, err)
	}
}

func TestDNSZoneRoundTrip(t *testing.T) {
	g := newGateway(t)
	g.handle = func(query string, variables map[string]any) (any, string, string) {
		if strings.Contains(query, "deleteDnsZone") {
			if variables["name"] != "example.com" || variables["confirm"] != "example.com" {
				t.Errorf("unexpected variables: %v", variables)
			}
			return map[string]any{"deleteDnsZone": true}, "", ""
		}
		return map[string]any{"createDnsZone": map[string]any{"name": variables["domain"], "status": "PENDING", "delegated": true,
			"nameservers": []string{"ns1.example.net", "ns2.example.net"}}}, "", ""
	}
	c := g.client()
	zone, err := c.CreateDNSZone(context.Background(), "example.com")
	if err != nil || zone.Status != DNSZonePending || !zone.Delegated || len(zone.Nameservers) != 2 {
		t.Fatalf("%+v, %v", zone, err)
	}
	if err := c.DeleteDNSZone(context.Background(), "example.com"); err != nil {
		t.Fatal(err)
	}
}

func TestWorkloadIdentityReadsTheFileForEveryExchange(t *testing.T) {
	file := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(file, []byte("first\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var assertions []string
	token := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("grant_type") != "client_credentials" || r.Form.Get("client_id") != "workload" ||
			r.Form.Get("client_assertion_type") != "urn:ietf:params:oauth:client-assertion-type:jwt-bearer" || r.Form.Get("password") != "" {
			t.Errorf("unexpected token form: %v", r.Form)
		}
		assertions = append(assertions, r.Form.Get("client_assertion"))
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "token", "expires_in": 600})
	}))
	defer token.Close()
	g := newGateway(t)
	g.handle = func(string, map[string]any) (any, string, string) { return map[string]any{"ready": true}, "", "" }

	c, err := New(WithTenant("acme"), WithWorkloadIdentity(file), WithEndpoint(g.URL+"/graphql"), WithTokenURL(token.URL), WithClientID("workload"))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := c.Do(context.Background(), `query { ready }`, nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	if len(assertions) != 1 || assertions[0] != "first" {
		t.Fatalf("want one exchange with the trimmed token, got %v", assertions)
	}

	// The file is rotated and the access token is about to expire.
	if err := os.WriteFile(file, []byte("second"), 0o600); err != nil {
		t.Fatal(err)
	}
	source := c.tokens.(*WorkloadIdentitySource)
	source.mu.Lock()
	source.current.Expiry = time.Now().Add(5 * time.Second)
	source.mu.Unlock()
	if err := c.Do(context.Background(), `query { ready }`, nil, nil); err != nil {
		t.Fatal(err)
	}
	if len(assertions) != 2 || assertions[1] != "second" {
		t.Fatalf("the rotated token was not used: %v", assertions)
	}

	missing := &WorkloadIdentitySource{TokenFile: filepath.Join(t.TempDir(), "absent")}
	if _, err := missing.Token(context.Background()); err == nil {
		t.Fatal("a missing token file must fail")
	}
}

func TestExposeHostTargetCertificateSetting(t *testing.T) {
	g := newGateway(t)
	var targets []map[string]any
	g.handle = func(_ string, variables map[string]any) (any, string, string) {
		input := variables["input"].(map[string]any)
		targets = append(targets, input["target"].(map[string]any))
		return map[string]any{"exposeHost": map[string]any{"host": input["host"], "namespace": "acme", "status": "ACTIVE", "tls": "AUTO",
			"target":  map[string]any{"address": "198.51.100.10", "port": 8443, "scheme": "HTTPS", "insecureSkipVerify": true},
			"proxied": false, "dnsTarget": []string{},
			"replacedRecords": []map[string]any{{"type": "A", "name": "shop.example.com", "content": "203.0.113.7", "removedAt": "2026-01-01T00:00:00Z"}}}}, "", ""
	}
	c := g.client()
	target := ExposureTarget{Address: "198.51.100.10", Port: 8443, Scheme: ExposureHTTPS}

	got, err := c.ExposeHost(context.Background(), ExposeHostInput{Host: "shop.example.com", Target: target})
	if err != nil {
		t.Fatal(err)
	}
	if _, present := targets[0]["insecureSkipVerify"]; present {
		t.Errorf("a nil setting must be left out so that a host keeps what it has: %v", targets[0])
	}
	if got.Target.InsecureSkipVerify == nil || !*got.Target.InsecureSkipVerify || got.Proxied == nil || *got.Proxied ||
		len(got.ReplacedRecords) != 1 || got.ReplacedRecords[0].Content != "203.0.113.7" {
		t.Errorf("unexpected exposure: %+v", got)
	}

	for _, want := range []bool{true, false} {
		value := want
		target.InsecureSkipVerify = &value
		if _, err := c.ExposeHost(context.Background(), ExposeHostInput{Host: "shop.example.com", Target: target}); err != nil {
			t.Fatal(err)
		}
		if sent, present := targets[len(targets)-1]["insecureSkipVerify"]; !present || sent != want {
			t.Errorf("want insecureSkipVerify %v sent, got %v", want, targets[len(targets)-1])
		}
	}
}

func TestWorkloadIdentityScope(t *testing.T) {
	file := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(file, []byte("signed"), 0o600); err != nil {
		t.Fatal(err)
	}
	var scopes []string
	var present []bool
	token := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		scopes = append(scopes, r.Form.Get("scope"))
		present = append(present, r.Form.Has("scope"))
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "token", "expires_in": 600})
	}))
	defer token.Close()
	g := newGateway(t)
	g.handle = func(string, map[string]any) (any, string, string) { return map[string]any{"ready": true}, "", "" }

	for _, opts := range [][]Option{{}, {WithWorkloadIdentityScope("profile api")}} {
		all := append([]Option{WithTenant("acme"), WithWorkloadIdentity(file), WithEndpoint(g.URL + "/graphql"), WithTokenURL(token.URL), WithClientID("workload")}, opts...)
		c, err := New(all...)
		if err != nil {
			t.Fatal(err)
		}
		if err := c.Do(context.Background(), `query { ready }`, nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	if len(scopes) != 2 || present[0] || scopes[1] != "profile api" {
		t.Fatalf("want no scope, then the one set: %v %v", scopes, present)
	}
}

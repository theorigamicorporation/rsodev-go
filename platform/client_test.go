package platform

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// gateway is a scripted stand-in for the platform API: it answers each
// GraphQL operation with what handle returns for its query text.
type gateway struct {
	*httptest.Server
	t       *testing.T
	queries []string
	handle  func(query string, variables map[string]any) (data any, code, message string)
	rest    func(path string, body map[string]any) (status int, answer any)
}

func newGateway(t *testing.T) *gateway {
	t.Helper()
	g := &gateway{t: t}
	g.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer token" || r.Header.Get("X-Tenant") != "acme" {
			t.Errorf("unexpected headers: %v", r.Header)
		}
		raw, _ := io.ReadAll(r.Body)
		if r.URL.Path != "/graphql" {
			var body map[string]any
			_ = json.Unmarshal(raw, &body)
			status, answer := g.rest(r.URL.Path, body)
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(answer)
			return
		}
		var req struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		_ = json.Unmarshal(raw, &req)
		g.queries = append(g.queries, req.Query)
		data, code, message := g.handle(req.Query, req.Variables)
		if code != "" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": nil, "errors": []map[string]any{{"message": message, "extensions": map[string]any{"code": code}}},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
	}))
	t.Cleanup(g.Close)
	return g
}

func (g *gateway) client(opts ...Option) *Client {
	g.t.Helper()
	c, err := New(append([]Option{WithTenant("acme"), WithAccessToken("token"), WithEndpoint(g.URL + "/graphql")}, opts...)...)
	if err != nil {
		g.t.Fatal(err)
	}
	return c
}

func TestNewRequiresCredentials(t *testing.T) {
	if _, err := New(WithAccessToken("x")); err != nil {
		t.Fatalf("a client without a tenant is allowed: %v", err)
	}
	if _, err := New(WithTenant("acme")); err == nil {
		t.Fatal("expected an error without credentials")
	}
	if _, err := New(WithTenant("acme"), WithServiceAccount("ci", "secret")); err != nil {
		t.Fatal(err)
	}
}

func TestServiceAccountSignInIsCachedAndRenewed(t *testing.T) {
	var signIns int
	token := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		signIns++
		_ = r.ParseForm()
		if r.Form.Get("grant_type") != "client_credentials" || r.Form.Get("username") != "ci" || r.Form.Get("password") != "secret" || r.Form.Get("client_id") != "cid" {
			t.Errorf("unexpected token form: %v", r.Form)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "token", "expires_in": 3600})
	}))
	defer token.Close()
	g := newGateway(t)
	g.handle = func(string, map[string]any) (any, string, string) { return map[string]any{"ready": true}, "", "" }

	c, err := New(WithTenant("acme"), WithServiceAccount("ci", "secret"), WithEndpoint(g.URL+"/graphql"),
		WithTokenURL(token.URL), WithClientID("cid"), WithRegion("EU"), WithUserAgent("test/1"))
	if err != nil {
		t.Fatal(err)
	}
	var out struct{ Ready bool }
	for i := 0; i < 2; i++ {
		if err := c.Do(context.Background(), `query { ready }`, nil, &out); err != nil {
			t.Fatal(err)
		}
	}
	if !out.Ready || signIns != 1 {
		t.Fatalf("ready=%v after %d sign-ins, want one", out.Ready, signIns)
	}
	source := c.tokens.(*ServiceAccountSource)
	source.mu.Lock()
	source.current.Expiry = time.Now().Add(5 * time.Second) // about to expire
	source.mu.Unlock()
	if err := c.Do(context.Background(), `query { ready }`, nil, nil); err != nil {
		t.Fatal(err)
	}
	if signIns != 2 {
		t.Fatalf("token was not renewed: %d sign-ins", signIns)
	}
}

func TestErrorsCarryTheirCode(t *testing.T) {
	g := newGateway(t)
	g.handle = func(query string, _ map[string]any) (any, string, string) {
		switch {
		case strings.Contains(query, "queue("):
			return nil, CodeNotFound, "Queue orders not found"
		case strings.Contains(query, "createQueue("):
			return nil, CodeConflict, "orders already exists"
		}
		return nil, CodeQuotaExceeded, "Quota exceeded for queues"
	}
	c := g.client()
	_, err := c.GetQueue(context.Background(), "orders")
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Code() != CodeNotFound || !IsNotFound(err) {
		t.Fatalf("want NOT_FOUND, got %v", err)
	}
	if _, err = c.CreateQueue(context.Background(), "orders"); !IsConflict(err) {
		t.Fatalf("want CONFLICT, got %v", err)
	}
	if _, err = c.CreateTopic(context.Background(), "events"); !IsQuotaExceeded(err) || !strings.Contains(err.Error(), "QUOTA_EXCEEDED") {
		t.Fatalf("want QUOTA_EXCEEDED, got %v", err)
	}
	if IsNotFound(errors.New("plain")) {
		t.Fatal("a plain error is not a not-found")
	}
}

func TestWaitForTask(t *testing.T) {
	g := newGateway(t)
	calls := 0
	g.handle = func(string, map[string]any) (any, string, string) {
		calls++
		status := "RUNNING"
		if calls >= 3 {
			status = "COMPLETED"
		}
		return map[string]any{"taskProgress": map[string]any{"taskId": "t1", "status": status, "progress": calls * 30}}, "", ""
	}
	task, err := g.client().WaitForTask(context.Background(), "t1", time.Millisecond)
	if err != nil || task.Status != "COMPLETED" || calls != 3 {
		t.Fatalf("task=%+v err=%v calls=%d", task, err, calls)
	}
}

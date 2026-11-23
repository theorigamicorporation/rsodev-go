package platform

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// discoveryServer serves a configuration document that points sign-in back
// at itself, and counts how often each route is asked.
func discoveryServer(t *testing.T, doc func(base string) map[string]string) (*httptest.Server, *atomic.Int32, *atomic.Value) {
	t.Helper()
	var documents atomic.Int32
	var lastClient atomic.Value
	lastClient.Store("")
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	mux.HandleFunc(discoveryPath, func(w http.ResponseWriter, _ *http.Request) {
		documents.Add(1)
		if doc == nil {
			http.NotFound(w, nil)
			return
		}
		_ = json.NewEncoder(w).Encode(doc(server.URL))
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		lastClient.Store(r.PostForm.Get("client_id"))
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "t", "expires_in": 600})
	})
	return server, &documents, &lastClient
}

func TestDiscoverReadsTheDocument(t *testing.T) {
	server, _, _ := discoveryServer(t, func(base string) map[string]string {
		return map[string]string{"api_version": "1", "token_endpoint": base + "/token", "client_id": "public-client", "device_authorization_endpoint": base + "/device"}
	})
	doc, err := Discover(context.Background(), server.URL+"/graphql", nil)
	if err != nil {
		t.Fatal(err)
	}
	if doc.ClientID != "public-client" || doc.TokenEndpoint != server.URL+"/token" || doc.DeviceAuthorizationEndpoint != server.URL+"/device" {
		t.Fatalf("unexpected document: %+v", doc)
	}
}

func TestDiscoverDropsPlainHTTPSignInAddresses(t *testing.T) {
	server, _, _ := discoveryServer(t, func(string) map[string]string {
		return map[string]string{"token_endpoint": "http://sign-in.example.com/token", "client_id": "public-client"}
	})
	doc, err := Discover(context.Background(), server.URL+"/graphql", nil)
	if err != nil {
		t.Fatal(err)
	}
	if doc.TokenEndpoint != "" {
		t.Fatalf("a plain http token endpoint was kept: %q", doc.TokenEndpoint)
	}
}

func TestSignInUsesTheDocumentWhenNothingIsSet(t *testing.T) {
	server, documents, lastClient := discoveryServer(t, func(base string) map[string]string {
		return map[string]string{"token_endpoint": base + "/token", "client_id": "public-client"}
	})
	oauth := OAuth{Endpoint: server.URL + "/graphql"}
	for range 3 {
		if _, err := oauth.ServiceAccount(context.Background(), "name", "secret"); err != nil {
			t.Fatal(err)
		}
	}
	if got := lastClient.Load(); got != "public-client" {
		t.Fatalf("signed in with client %q", got)
	}
	if got := documents.Load(); got != 1 {
		t.Fatalf("the document was read %d times, want once", got)
	}
}

func TestSignInKeepsWhatTheCallerSet(t *testing.T) {
	server, _, lastClient := discoveryServer(t, func(base string) map[string]string {
		return map[string]string{"token_endpoint": "https://elsewhere.example.com/token", "client_id": "public-client"}
	})
	oauth := OAuth{Endpoint: server.URL + "/graphql", TokenURL: server.URL + "/token", ClientID: "mine"}
	if _, err := oauth.ServiceAccount(context.Background(), "name", "secret"); err != nil {
		t.Fatal(err)
	}
	if got := lastClient.Load(); got != "mine" {
		t.Fatalf("signed in with client %q", got)
	}
}

func TestSignInFallsBackWithoutADocument(t *testing.T) {
	server, documents, _ := discoveryServer(t, nil)
	oauth := OAuth{Endpoint: server.URL + "/graphql"}
	resolved := oauth.resolve(context.Background())
	_ = oauth.resolve(context.Background())
	if resolved.TokenURL != DefaultTokenURL || resolved.ClientID != DefaultClientID {
		t.Fatalf("no fallback to the defaults: %+v", resolved)
	}
	if got := documents.Load(); got != 1 {
		t.Fatalf("a missing document was asked for %d times, want once", got)
	}
}

func TestDiscoveryURLRefusesNonsense(t *testing.T) {
	if _, err := DiscoveryURL("not an address"); err == nil {
		t.Fatal("expected an error")
	}
}

package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Discovery is the platform's configuration document: where to sign in and
// with which public client. The gateway serves it without credentials at
// /.well-known/rso-configuration, so a client only has to know the gateway.
type Discovery struct {
	APIVersion                  string `json:"api_version"`
	GraphQLEndpoint             string `json:"graphql_endpoint"`
	Issuer                      string `json:"issuer"`
	TokenEndpoint               string `json:"token_endpoint"`
	DeviceAuthorizationEndpoint string `json:"device_authorization_endpoint"`
	ClientID                    string `json:"client_id"`
}

// discoveryPath is where the gateway serves the document.
const discoveryPath = "/.well-known/rso-configuration"

// DiscoveryURL returns the address of the configuration document of the
// gateway whose GraphQL endpoint is given.
func DiscoveryURL(endpoint string) (string, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("platform: %q is not an endpoint address", endpoint)
	}
	return u.Scheme + "://" + u.Host + discoveryPath, nil
}

// Discover reads the configuration document of the gateway whose GraphQL
// endpoint is given. A nil HTTP client means a 10 second default.
func Discover(ctx context.Context, endpoint string, h *http.Client) (Discovery, error) {
	target, err := DiscoveryURL(endpoint)
	if err != nil {
		return Discovery{}, err
	}
	if h == nil {
		h = &http.Client{Timeout: 10 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return Discovery{}, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := h.Do(req)
	if err != nil {
		return Discovery{}, fmt.Errorf("reaching the platform: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return Discovery{}, fmt.Errorf("platform: the configuration document answered %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Discovery{}, err
	}
	var out Discovery
	if err := json.Unmarshal(body, &out); err != nil {
		return Discovery{}, fmt.Errorf("platform: the configuration document is not JSON: %w", err)
	}
	for _, address := range []*string{&out.TokenEndpoint, &out.DeviceAuthorizationEndpoint} {
		if !strings.HasPrefix(*address, "https://") && !isLoopback(*address) {
			*address = "" // never send credentials anywhere but over TLS
		}
	}
	return out, nil
}

func isLoopback(address string) bool {
	u, err := url.Parse(address)
	if err != nil || u.Scheme != "http" {
		return false
	}
	host := u.Hostname()
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

// discoveryTTL is how long an answer, or a failure, is remembered.
const (
	discoveryTTL        = 5 * time.Minute
	discoveryFailureTTL = time.Minute
)

type discoveryEntry struct {
	doc     Discovery
	ok      bool
	expires time.Time
}

var discoveryCache = struct {
	sync.Mutex
	entries map[string]discoveryEntry
}{entries: map[string]discoveryEntry{}}

// cachedDiscovery returns the document for an endpoint, reading it at most
// once per discoveryTTL. A failure is remembered briefly and reported as
// "nothing known", so a gateway without the document costs one request.
func cachedDiscovery(ctx context.Context, endpoint string, h *http.Client) (Discovery, bool) {
	now := time.Now()
	discoveryCache.Lock()
	entry, found := discoveryCache.entries[endpoint]
	discoveryCache.Unlock()
	if found && now.Before(entry.expires) {
		return entry.doc, entry.ok
	}
	doc, err := Discover(ctx, endpoint, h)
	entry = discoveryEntry{doc: doc, ok: err == nil, expires: now.Add(discoveryTTL)}
	if err != nil {
		entry.expires = now.Add(discoveryFailureTTL)
	}
	discoveryCache.Lock()
	discoveryCache.entries[endpoint] = entry
	discoveryCache.Unlock()
	return entry.doc, entry.ok
}

// resolve fills what the caller left unset: first from the configuration
// document of Endpoint, then from the defaults of the public cloud. Values
// the caller set are never replaced.
func (o OAuth) resolve(ctx context.Context) OAuth {
	// The client id and the endpoints belong together: the document is used
	// only when the caller chose neither.
	if o.Endpoint != "" && o.TokenURL == "" && o.ClientID == "" {
		if doc, ok := cachedDiscovery(ctx, o.Endpoint, o.HTTPClient); ok && doc.TokenEndpoint != "" && doc.ClientID != "" {
			o.TokenURL, o.ClientID = doc.TokenEndpoint, doc.ClientID
			if o.DeviceURL == "" {
				o.DeviceURL = doc.DeviceAuthorizationEndpoint
			}
		}
	}
	return o.withDefaults()
}

package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Defaults of the public RSO Cloud.
const (
	DefaultEndpoint = "https://gateway.cloud.rso.dev/graphql"
	DefaultTokenURL = "https://auth.theorigamicorporation.com/application/o/token/" //nolint:gosec // an address, not a credential
	// DefaultClientID is the public OAuth client that service accounts sign in through.
	DefaultClientID = "FdPqdka22aPG9AnWuVMpFhsz7BGsgJCbxTCTkXrg"
)

// Version is the version of this SDK, sent in the User-Agent header.
const Version = "0.2.0"

type config struct {
	endpoint, tokenURL, clientID  string
	tenant, region                string
	accessToken                   string
	serviceAccount, serviceSecret string
	tokens                        TokenSource
	userAgent                     string
	httpClient                    *http.Client
	retry                         RetryPolicy
}

// Option configures a Client.
type Option func(*config)

// WithTenant sets the tenant every request is scoped to. Leave it out only
// for calls about the caller itself, such as Me.
func WithTenant(tenant string) Option { return func(c *config) { c.tenant = tenant } }

// WithServiceAccount signs in with a service account's name and secret.
func WithServiceAccount(name, secret string) Option {
	return func(c *config) { c.serviceAccount, c.serviceSecret = name, secret }
}

// WithAccessToken uses a ready access token instead of a service account. The
// client cannot renew it.
func WithAccessToken(token string) Option { return func(c *config) { c.accessToken = token } }

// WithTokenSource takes access tokens from any TokenSource, for example a
// RefreshingSource holding a person's sign-in.
func WithTokenSource(tokens TokenSource) Option { return func(c *config) { c.tokens = tokens } }

// WithRetry replaces the retry policy for read operations.
func WithRetry(policy RetryPolicy) Option { return func(c *config) { c.retry = policy } }

// WithEndpoint sets the GraphQL endpoint of the gateway.
func WithEndpoint(endpoint string) Option { return func(c *config) { c.endpoint = endpoint } }

// WithTokenURL sets the OAuth2 token endpoint service accounts sign in at.
func WithTokenURL(tokenURL string) Option { return func(c *config) { c.tokenURL = tokenURL } }

// WithClientID sets the OAuth2 client id used for the sign-in.
func WithClientID(clientID string) Option { return func(c *config) { c.clientID = clientID } }

// WithRegion selects a region of the tenant; the gateway's default applies
// when unset.
func WithRegion(region string) Option { return func(c *config) { c.region = region } }

// WithUserAgent puts a product token in front of the SDK's own User-Agent.
func WithUserAgent(userAgent string) Option { return func(c *config) { c.userAgent = userAgent } }

// WithHTTPClient replaces the HTTP client (60 second timeout by default).
func WithHTTPClient(h *http.Client) Option { return func(c *config) { c.httpClient = h } }

// Client talks to the platform API for one tenant. It is safe for concurrent use.
type Client struct {
	cfg    config
	tokens TokenSource
}

// New builds a Client. It fails when no credentials are given.
func New(opts ...Option) (*Client, error) {
	cfg := config{endpoint: DefaultEndpoint, tokenURL: DefaultTokenURL, clientID: DefaultClientID, retry: DefaultRetry}
	for _, opt := range opts {
		opt(&cfg)
	}
	if cfg.httpClient == nil {
		cfg.httpClient = &http.Client{Timeout: 60 * time.Second}
	}
	c := &Client{cfg: cfg}
	oauth := OAuth{TokenURL: cfg.tokenURL, ClientID: cfg.clientID, HTTPClient: cfg.httpClient}
	switch {
	case cfg.tokens != nil:
		c.tokens = cfg.tokens
	case cfg.serviceAccount != "" && cfg.serviceSecret != "":
		source := &ServiceAccountSource{OAuth: oauth, Name: cfg.serviceAccount, Secret: cfg.serviceSecret}
		if cfg.accessToken != "" {
			source.current = TokenSet{AccessToken: cfg.accessToken}
		}
		c.tokens = source
	case cfg.accessToken != "":
		c.tokens = StaticToken(cfg.accessToken)
	default:
		return nil, errors.New("platform: credentials are required (WithServiceAccount, WithAccessToken or WithTokenSource)")
	}
	return c, nil
}

// Tenant returns the tenant the client is scoped to.
func (c *Client) Tenant() string { return c.cfg.tenant }

// Endpoint returns the GraphQL endpoint.
func (c *Client) Endpoint() string { return c.cfg.endpoint }

// namespace is how the tenant is named in the few operations that still take
// one; null lets the platform fill it in.
func (c *Client) namespace() any { return optional(c.cfg.tenant) }

// baseURL is the gateway's address without the GraphQL path, for its REST routes.
func (c *Client) baseURL() string {
	return strings.TrimSuffix(strings.TrimSuffix(c.cfg.endpoint, "/"), "/graphql")
}

// Token returns a valid access token, signing in or renewing as needed.
func (c *Client) Token(ctx context.Context) (string, error) { return c.tokens.Token(ctx) }

// userAgent is the SDK's product token, after the caller's own when set.
func (c *Client) userAgent() string {
	own := "rsodev-go/" + Version
	if c.cfg.userAgent == "" {
		return own
	}
	return c.cfg.userAgent + " " + own
}

// authorize puts the token, the tenant and the optional headers on a request.
func (c *Client) authorize(ctx context.Context, req *http.Request) error {
	token, err := c.Token(ctx)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if c.cfg.tenant != "" {
		req.Header.Set("X-Tenant", c.cfg.tenant)
	}
	if c.cfg.region != "" {
		req.Header.Set("X-Region", c.cfg.region)
	}
	req.Header.Set("User-Agent", c.userAgent())
	return nil
}

// Do runs one GraphQL operation and decodes its data into out (which may be
// nil). It is the escape hatch for anything without a typed operation.
// Refusals come back as *Error. Queries are retried on transient failures
// according to the retry policy; mutations are sent once.
func (c *Client) Do(ctx context.Context, query string, variables map[string]any, out any) error {
	payload, err := json.Marshal(map[string]any{"query": query, "variables": variables})
	if err != nil {
		return err
	}
	attempts := 1
	if isQuery(query) && c.cfg.retry.Attempts > 1 {
		attempts = c.cfg.retry.Attempts
	}
	for attempt := 1; ; attempt++ {
		retryable, err := c.do(ctx, payload, out)
		if err == nil || !retryable || attempt >= attempts {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(c.cfg.retry.delay(attempt)):
		}
	}
}

// do sends the operation once. The first result says whether trying again
// could help: the request never arrived or the gateway was not available.
func (c *Client) do(ctx context.Context, payload []byte, out any) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.endpoint, bytes.NewReader(payload))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/json")
	if err := c.authorize(ctx, req); err != nil {
		return false, err
	}
	resp, err := c.cfg.httpClient.Do(req)
	if err != nil {
		return ctx.Err() == nil, fmt.Errorf("platform: calling the API: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return ctx.Err() == nil, err
	}
	transient := resp.StatusCode == http.StatusBadGateway || resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusGatewayTimeout
	var envelope struct {
		Data   json.RawMessage `json:"data"`
		Errors []GraphQLError  `json:"errors"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return transient, &Error{StatusCode: resp.StatusCode, Message: fmt.Sprintf("the API answered %d with a body that is not JSON: %s", resp.StatusCode, truncate(string(body), 300))}
	}
	if len(envelope.Errors) > 0 {
		return transient, &Error{Errors: envelope.Errors, StatusCode: resp.StatusCode}
	}
	if resp.StatusCode >= 400 {
		return transient, &Error{StatusCode: resp.StatusCode, Message: fmt.Sprintf("the API answered %d: %s", resp.StatusCode, truncate(string(body), 300))}
	}
	if out == nil || len(envelope.Data) == 0 {
		return false, nil
	}
	return false, json.Unmarshal(envelope.Data, out)
}

// isQuery reports whether a document is a read: it does not start with
// "mutation" or "subscription".
func isQuery(document string) bool {
	document = strings.TrimSpace(document)
	return !strings.HasPrefix(document, "mutation") && !strings.HasPrefix(document, "subscription")
}

// RetryPolicy says how often and how patiently reads are retried.
type RetryPolicy struct {
	// Attempts is the total number of tries; 1 or less disables retries.
	Attempts int
	// Base is the wait before the second try; it doubles each time up to Max.
	Base, Max time.Duration
}

// DefaultRetry tries a read three times, waiting 200ms and then 400ms.
var DefaultRetry = RetryPolicy{Attempts: 3, Base: 200 * time.Millisecond, Max: 2 * time.Second}

func (p RetryPolicy) delay(attempt int) time.Duration {
	base := p.Base
	if base <= 0 {
		base = 200 * time.Millisecond
	}
	wait := base << (attempt - 1)
	if p.Max > 0 && wait > p.Max {
		wait = p.Max
	}
	return wait
}

// optional turns an empty string into a GraphQL null.
func optional(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

package platform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// TokenSource hands out an access token, renewing it when needed.
type TokenSource interface {
	Token(ctx context.Context) (string, error)
}

// ErrNotSignedIn means no usable credentials were configured.
var ErrNotSignedIn = errors.New("platform: not signed in")

// OAuth describes the identity provider the platform signs in with.
type OAuth struct {
	TokenURL string
	// DeviceURL starts the device sign-in. Empty derives it from TokenURL.
	DeviceURL  string
	ClientID   string
	HTTPClient *http.Client
}

func (o OAuth) withDefaults() OAuth {
	if o.TokenURL == "" {
		o.TokenURL = DefaultTokenURL
	}
	if o.ClientID == "" {
		o.ClientID = DefaultClientID
	}
	if o.DeviceURL == "" {
		o.DeviceURL = strings.TrimSuffix(strings.TrimSuffix(o.TokenURL, "/"), "/token") + "/device/"
	}
	if o.HTTPClient == nil {
		o.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}
	return o
}

// TokenSet is what the identity provider hands out.
type TokenSet struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	Expiry       time.Time `json:"expiry,omitempty"`
}

// Valid reports whether the access token can still be used for a request.
func (t TokenSet) Valid(now time.Time) bool {
	return t.AccessToken != "" && (t.Expiry.IsZero() || t.Expiry.Sub(now) > 30*time.Second)
}

// OAuthError is the identity provider refusing a token request.
type OAuthError struct {
	Status      int
	Code        string
	Description string
}

// Error implements error.
func (e *OAuthError) Error() string {
	switch {
	case e.Description != "":
		return fmt.Sprintf("sign-in refused: %s", e.Description)
	case e.Code != "":
		return fmt.Sprintf("sign-in refused: %s", e.Code)
	}
	return fmt.Sprintf("sign-in refused: the identity provider answered %d", e.Status)
}

func (o OAuth) post(ctx context.Context, target string, form url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := o.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("reaching the identity provider: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		refused := &OAuthError{Status: resp.StatusCode}
		var parsed struct {
			Error       string `json:"error"`
			Description string `json:"error_description"`
		}
		if json.Unmarshal(body, &parsed) == nil {
			refused.Code, refused.Description = parsed.Error, parsed.Description
		}
		return refused
	}
	return json.Unmarshal(body, out)
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
}

func (r tokenResponse) set(now time.Time) (TokenSet, error) {
	if r.AccessToken == "" {
		return TokenSet{}, errors.New("the identity provider returned no access token")
	}
	set := TokenSet{AccessToken: r.AccessToken, RefreshToken: r.RefreshToken}
	if r.ExpiresIn > 0 {
		set.Expiry = now.Add(time.Duration(r.ExpiresIn) * time.Second)
	}
	return set, nil
}

// ServiceAccount exchanges a service account's name and secret for a token.
func (o OAuth) ServiceAccount(ctx context.Context, name, secret string) (TokenSet, error) {
	o = o.withDefaults()
	var out tokenResponse
	err := o.post(ctx, o.TokenURL, url.Values{
		"grant_type": {"client_credentials"},
		"client_id":  {o.ClientID},
		"username":   {name},
		"password":   {secret},
		"scope":      {"profile"},
	}, &out)
	if err != nil {
		return TokenSet{}, err
	}
	return out.set(time.Now())
}

// Refresh trades a refresh token for a new token set.
func (o OAuth) Refresh(ctx context.Context, refreshToken string) (TokenSet, error) {
	o = o.withDefaults()
	var out tokenResponse
	err := o.post(ctx, o.TokenURL, url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {o.ClientID},
		"refresh_token": {refreshToken},
	}, &out)
	if err != nil {
		return TokenSet{}, err
	}
	set, err := out.set(time.Now())
	if err == nil && set.RefreshToken == "" {
		set.RefreshToken = refreshToken
	}
	return set, err
}

// DeviceCode is a pending device sign-in: the person opens VerificationURI
// in a browser and confirms UserCode there.
type DeviceCode struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

// deviceScopes asks for the profile the platform reads and for a refresh
// token, so a person signs in once and not every few minutes.
const deviceScopes = "openid profile email offline_access"

// StartDevice begins a device sign-in.
func (o OAuth) StartDevice(ctx context.Context) (DeviceCode, error) {
	o = o.withDefaults()
	var out DeviceCode
	err := o.post(ctx, o.DeviceURL, url.Values{"client_id": {o.ClientID}, "scope": {deviceScopes}}, &out)
	if err != nil {
		return DeviceCode{}, err
	}
	if out.DeviceCode == "" || out.VerificationURI == "" {
		return DeviceCode{}, errors.New("the identity provider does not offer device sign-in")
	}
	return out, nil
}

// WaitDevice polls until the person has confirmed the sign-in, refused it,
// or the code has expired.
func (o OAuth) WaitDevice(ctx context.Context, code DeviceCode) (TokenSet, error) {
	o = o.withDefaults()
	interval := time.Duration(code.Interval) * time.Second
	if interval <= 0 {
		interval = 5 * time.Second
	}
	deadline := time.Now().Add(time.Duration(code.ExpiresIn) * time.Second)
	if code.ExpiresIn <= 0 {
		deadline = time.Now().Add(10 * time.Minute)
	}
	for {
		var out tokenResponse
		err := o.post(ctx, o.TokenURL, url.Values{
			"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
			"client_id":   {o.ClientID},
			"device_code": {code.DeviceCode},
		}, &out)
		if err == nil {
			return out.set(time.Now())
		}
		var refused *OAuthError
		if !errors.As(err, &refused) {
			return TokenSet{}, err
		}
		switch refused.Code {
		case "authorization_pending":
		case "slow_down":
			interval += 5 * time.Second
		case "expired_token":
			return TokenSet{}, errors.New("the sign-in code expired before it was confirmed")
		case "access_denied":
			return TokenSet{}, errors.New("the sign-in was refused in the browser")
		default:
			return TokenSet{}, err
		}
		if time.Now().Add(interval).After(deadline) {
			return TokenSet{}, errors.New("the sign-in code expired before it was confirmed")
		}
		select {
		case <-ctx.Done():
			return TokenSet{}, ctx.Err()
		case <-time.After(interval):
		}
	}
}

// StaticToken is a ready access token that is used until it stops working.
type StaticToken string

// Token implements TokenSource.
func (t StaticToken) Token(context.Context) (string, error) {
	if t == "" {
		return "", ErrNotSignedIn
	}
	return string(t), nil
}

// ServiceAccountSource signs in as a service account and renews the token
// shortly before it expires.
type ServiceAccountSource struct {
	OAuth  OAuth
	Name   string
	Secret string

	mu      sync.Mutex
	current TokenSet
}

// Token implements TokenSource.
func (s *ServiceAccountSource) Token(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.current.Valid(time.Now()) {
		return s.current.AccessToken, nil
	}
	set, err := s.OAuth.ServiceAccount(ctx, s.Name, s.Secret)
	if err != nil {
		return "", err
	}
	s.current = set
	return set.AccessToken, nil
}

// RefreshingSource uses a stored token set and renews it with its refresh
// token. Save, when set, is told about every renewed set so it can be kept.
type RefreshingSource struct {
	OAuth OAuth
	Set   TokenSet
	Save  func(TokenSet) error

	mu sync.Mutex
}

// Token implements TokenSource.
func (s *RefreshingSource) Token(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Set.Valid(time.Now()) {
		return s.Set.AccessToken, nil
	}
	if s.Set.RefreshToken == "" {
		if s.Set.AccessToken == "" {
			return "", ErrNotSignedIn
		}
		return "", ErrSessionExpired
	}
	set, err := s.OAuth.Refresh(ctx, s.Set.RefreshToken)
	if err != nil {
		var refused *OAuthError
		if errors.As(err, &refused) {
			return "", ErrSessionExpired
		}
		return "", err
	}
	s.Set = set
	if s.Save != nil {
		// A token that could not be saved still works for this run.
		_ = s.Save(set)
	}
	return set.AccessToken, nil
}

// ErrSessionExpired means the stored sign-in can no longer be renewed.
var ErrSessionExpired = fmt.Errorf("%w: the session has expired", ErrNotSignedIn)

// Package doauth is the DigitalOcean OAuth2 client: authorize URL, code exchange, refresh and revoke.
package doauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultBase is the DigitalOcean cloud host that serves the OAuth endpoints.
const DefaultBase = "https://cloud.digitalocean.com"

// Scope asks for read and write so the control plane can create and delete droplets.
const Scope = "read write"

const maxBody = 1 << 20

// ErrRejected means DigitalOcean refused the grant (bad code, spent or revoked refresh token).
var ErrRejected = errors.New("doauth: provider rejected the grant")

// Client talks to one DigitalOcean OAuth host. Zero Base uses DefaultBase.
type Client struct {
	Base    string
	HTTP    *http.Client
	Timeout time.Duration
}

// Tokens is one token response. ExpiresIn is zero when the provider sent none.
type Tokens struct {
	Access    string
	Refresh   string
	ExpiresIn time.Duration
}

func (c Client) base() string {
	if c.Base == "" {
		return DefaultBase
	}
	return strings.TrimRight(c.Base, "/")
}

// NewVerifier returns an RFC 7636 verifier from 32 random bytes.
func NewVerifier() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("doauth: pkce verifier: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// Challenge is the S256 challenge of a verifier.
func Challenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// AuthorizeURL is the page the operator's browser must open.
func (c Client) AuthorizeURL(clientID, redirectURI, state, challenge string) string {
	q := url.Values{
		"client_id": {clientID}, "redirect_uri": {redirectURI}, "response_type": {"code"},
		"scope": {Scope}, "state": {state}, "code_challenge": {challenge}, "code_challenge_method": {"S256"},
	}
	return c.base() + "/v1/oauth/authorize?" + q.Encode()
}

// Exchange trades an authorization code for tokens.
func (c Client) Exchange(ctx context.Context, clientID, secret, redirectURI, code, verifier string) (Tokens, error) {
	return c.token(ctx, url.Values{
		"grant_type": {"authorization_code"}, "code": {code}, "client_id": {clientID}, "client_secret": {secret},
		"redirect_uri": {redirectURI}, "code_verifier": {verifier},
	})
}

// Refresh spends a refresh token; DigitalOcean rotates it, so the caller must store the new one.
func (c Client) Refresh(ctx context.Context, clientID, secret, refresh string) (Tokens, error) {
	return c.token(ctx, url.Values{
		"grant_type": {"refresh_token"}, "refresh_token": {refresh}, "client_id": {clientID}, "client_secret": {secret},
	})
}

// Revoke invalidates an access token on DigitalOcean's side.
func (c Client) Revoke(ctx context.Context, token string) error {
	_, status, err := c.post(ctx, "/v1/oauth/revoke", url.Values{"token": {token}})
	if err != nil {
		return err
	}
	if status/100 != 2 {
		return fmt.Errorf("doauth: revoke returned status %d", status)
	}
	return nil
}

func (c Client) token(ctx context.Context, form url.Values) (Tokens, error) {
	body, status, err := c.post(ctx, "/v1/oauth/token", form)
	if err != nil {
		return Tokens{}, err
	}
	if status == http.StatusBadRequest || status == http.StatusUnauthorized {
		return Tokens{}, ErrRejected
	}
	if status/100 != 2 {
		return Tokens{}, fmt.Errorf("doauth: token endpoint returned status %d", status)
	}
	var out struct {
		Access    string `json:"access_token"`
		Refresh   string `json:"refresh_token"`
		ExpiresIn int64  `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &out); err != nil || out.Access == "" {
		return Tokens{}, errors.New("doauth: token response is not usable")
	}
	return Tokens{Access: out.Access, Refresh: out.Refresh, ExpiresIn: time.Duration(out.ExpiresIn) * time.Second}, nil
}

func (c Client) post(ctx context.Context, path string, form url.Values) ([]byte, int, error) {
	if c.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.Timeout)
		defer cancel()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base()+path, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, 0, fmt.Errorf("doauth: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: time.Minute}
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, 0, errors.New("doauth: request failed")
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return nil, 0, errors.New("doauth: read response failed")
	}
	return body, resp.StatusCode, nil
}

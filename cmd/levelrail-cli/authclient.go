package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"time"
)

// authSessionClient is a minimal HTTP client for the session-cookie-only
// routes internal/api/router.go deliberately never accepts a bearer
// token on: POST /api/v1/auth/login, GET /api/v1/auth/session, and
// every /api/v1/auth/tokens route ("session-only, deliberately never
// bearer-token authenticated... only an interactive human session can
// manage the token set", router.go's own comment on that registration).
//
// This is a separate type from client.go's own Client on purpose:
// Client's entire contract is "one bearer token, no session state" (see
// its own doc comment), the right shape for every other command in this
// CLI, and bolting a cookie jar onto it for the sake of the two commands
// that need the opposite (auth login, tokens *) would blur that
// contract for everyone else. A cookiejar.Jar is what makes the second
// request in a login-then-mint-token sequence actually carry the first
// request's Set-Cookie response: net/http.Client does nothing with a
// response's cookies unless a Jar is attached.
//
// Real, load-bearing gap this inherits rather than works around: the
// login response's session cookie is Secure (auth.go's handleLogin, by
// design: "a session cookie is exactly the kind of value that must
// never be sent back over a plain connection"), and Go's cookiejar
// correctly refuses to attach a Secure cookie to a plain-http request.
// Against the common local-dev target (defaultAPIURL, http://
// localhost:8080, no TLS-terminating Caddy in front yet), the cookie
// jar will accept the cookie but never send it back, so the
// token-minting step of "auth login" fails with a 401 there. That
// matches handleLogin's own documented caveat for direct plain-HTTP
// access, not a bug in this client: working around it by manually
// resending the cookie over http would defeat the Secure flag's actual
// purpose, so this client doesn't do that.
type authSessionClient struct {
	baseURL string
	hc      *http.Client
}

// newAuthSessionClient builds an authSessionClient with its own cookie
// jar, scoped to one call site's lifetime (one login, immediately
// followed by whatever session-only calls that login was for). Never
// shared across CLI invocations: this project's credentials file only
// ever persists a bearer token (config.go's writeCredentialsFile), never
// a session cookie, so there is nothing to reuse a jar across runs for.
func newAuthSessionClient(baseURL string) (*authSessionClient, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("create cookie jar: %w", err)
	}
	return &authSessionClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		hc:      &http.Client{Timeout: 30 * time.Second, Jar: jar},
	}, nil
}

func (c *authSessionClient) do(ctx context.Context, method, path string, body, out any) error {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode request body: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reqBody) //nolint:gosec // same operator-supplied target client.go's own do() already accepts, not attacker-controlled input
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.hc.Do(req) //nolint:gosec // same target as above
	if err != nil {
		return fmt.Errorf("request %s %s: %w", method, c.baseURL+path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &apiError{StatusCode: resp.StatusCode, Message: extractErrorMessage(data)}
	}

	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("decode response body: %w", err)
		}
	}
	return nil
}

// loginRequest mirrors internal/api's loginRequest (internal/api/auth.go).
type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// loginResponse mirrors internal/api's loginResponse.
type loginResponse struct {
	Username string `json:"username"`
}

// Login calls POST /api/v1/auth/login. On success, this client's cookie
// jar now carries the session cookie the response set; every subsequent
// call through this same authSessionClient sends it automatically.
func (c *authSessionClient) Login(ctx context.Context, username, password string) (loginResponse, error) {
	var out loginResponse
	err := c.do(ctx, http.MethodPost, "/api/v1/auth/login", loginRequest{Username: username, Password: password}, &out)
	return out, err
}

// tokenResource mirrors internal/api's tokenResource (internal/api/tokens.go).
type tokenResource struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Abilities  []string   `json:"abilities"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
}

// createTokenRequest mirrors internal/api's createTokenRequest.
type createTokenRequest struct {
	Name          string   `json:"name"`
	Abilities     []string `json:"abilities"`
	ExpiresInDays int      `json:"expires_in_days,omitempty"`
}

// createTokenResponse mirrors internal/api's createTokenResponse: Token
// is the plaintext credential, present in this one response only, GitHub
// PAT convention, never recoverable again.
type createTokenResponse struct {
	tokenResource
	Token string `json:"token"`
}

// CreateToken calls POST /api/v1/auth/tokens: requires a live session
// (this client must have already called Login successfully), never a
// bearer token, per this route's own server-side gate.
func (c *authSessionClient) CreateToken(ctx context.Context, req createTokenRequest) (createTokenResponse, error) {
	var out createTokenResponse
	err := c.do(ctx, http.MethodPost, "/api/v1/auth/tokens", req, &out)
	return out, err
}

// ListTokens calls GET /api/v1/auth/tokens: never returns a token
// secret, including for already-revoked rows.
func (c *authSessionClient) ListTokens(ctx context.Context) ([]tokenResource, error) {
	var out []tokenResource
	err := c.do(ctx, http.MethodGet, "/api/v1/auth/tokens", nil, &out)
	return out, err
}

// RevokeToken calls DELETE /api/v1/auth/tokens/{id}.
func (c *authSessionClient) RevokeToken(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/auth/tokens/"+pathEscape(id), nil, nil)
}

// sessionInfoResponse mirrors internal/api's sessionInfoResponse
// (internal/api/account.go).
type sessionInfoResponse struct {
	Username  string `json:"username"`
	ExpiresAt string `json:"expires_at"`
}

// GetSession calls GET /api/v1/auth/session: requires a live session,
// same as CreateToken/ListTokens/RevokeToken above.
func (c *authSessionClient) GetSession(ctx context.Context) (sessionInfoResponse, error) {
	var out sessionInfoResponse
	err := c.do(ctx, http.MethodGet, "/api/v1/auth/session", nil, &out)
	return out, err
}

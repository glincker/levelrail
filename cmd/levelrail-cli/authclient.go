package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// Session-cookie-only client, kept apart from the bearer-token Client.
// Secure login cookies are never sent over plain http, so token minting on local dev fails with 401 by design.
type authSessionClient struct {
	baseURL string
	hc      *http.Client
}

// Jar is scoped to one call: credentials store only a bearer token, never a session cookie.
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

const (
	trustedDeviceCookie = "trusted_device"
	authCookiePath      = "/api/v1/auth"
)

func (c *authSessionClient) authCookieURL() *url.URL {
	u, err := url.Parse(c.baseURL + authCookiePath + "/")
	if err != nil {
		return nil
	}
	return u
}

// seedTrustedDevice replays this machine's saved device cookie so the server
// recognises it and skips new-device approval.
func (c *authSessionClient) seedTrustedDevice(value string) {
	u := c.authCookieURL()
	if value == "" || u == nil {
		return
	}
	c.hc.Jar.SetCookies(u, []*http.Cookie{{Name: trustedDeviceCookie, Value: value, Path: authCookiePath, HttpOnly: true, Secure: u.Scheme == "https", SameSite: http.SameSiteStrictMode}}) //nolint:gosec // Secure follows the scheme, plain http only on loopback
}

// trustedDevice is the device cookie the server issued or extended during this login.
func (c *authSessionClient) trustedDevice() string {
	u := c.authCookieURL()
	if u == nil {
		return ""
	}
	for _, ck := range c.hc.Jar.Cookies(u) {
		if ck.Name == trustedDeviceCookie {
			return ck.Value
		}
	}
	return ""
}

// Logout revokes the throwaway login session once its token is minted, so repeat logins do not pile up sessions.
func (c *authSessionClient) Logout(ctx context.Context) error {
	return c.do(ctx, http.MethodPost, "/api/v1/auth/logout", nil, nil)
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

	start := time.Now()
	resp, err := c.hc.Do(req) //nolint:gosec // same target as above
	if err != nil {
		apiclient.TraceRequest(method, c.baseURL+path, "", time.Since(start), err)
		return fmt.Errorf("request %s %s: %w", method, c.baseURL+path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	apiclient.TraceRequest(method, c.baseURL+path, resp.Status, time.Since(start), nil)

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
	Username          string     `json:"username"`
	ApprovalRequired  bool       `json:"approval_required,omitempty"`
	ApprovalID        string     `json:"approval_id,omitempty"`
	ApprovalMatch     int        `json:"approval_match,omitempty"`
	ApprovalExpiresAt *time.Time `json:"approval_expires_at,omitempty"`
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
	ID         string      `json:"id"`
	Name       string      `json:"name"`
	Abilities  []string    `json:"abilities"`
	Access     string      `json:"access,omitempty"`
	CreatedAt  time.Time   `json:"created_at"`
	LastUsedAt *time.Time  `json:"last_used_at,omitempty"`
	ExpiresAt  *time.Time  `json:"expires_at,omitempty"`
	RevokedAt  *time.Time  `json:"revoked_at,omitempty"`
	RevokesAt  *time.Time  `json:"revokes_at,omitempty"`
	Agent      *tokenAgent `json:"agent,omitempty"`
	Status     string      `json:"status,omitempty"`
}

// tokenAgent mirrors internal/api's agentIdentity.
type tokenAgent struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// createTokenRequest mirrors internal/api's createTokenRequest.
type createTokenRequest struct {
	Name          string      `json:"name"`
	Abilities     []string    `json:"abilities,omitempty"`
	AllowChanges  bool        `json:"allow_changes,omitempty"`
	ExpiresInDays int         `json:"expires_in_days,omitempty"`
	NoExpiry      bool        `json:"no_expiry,omitempty"`
	Agent         *tokenAgent `json:"agent,omitempty"`
	TeamID        string      `json:"team_id,omitempty"`
	ProjectID     string      `json:"project_id,omitempty"`
	TeamRole      string      `json:"team_role,omitempty"`
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

// ListTokensQuery calls GET /api/v1/auth/tokens with filter, sort and page parameters already encoded.
func (c *authSessionClient) ListTokensQuery(ctx context.Context, rawQuery string) (json.RawMessage, error) {
	var out json.RawMessage
	err := c.do(ctx, http.MethodGet, "/api/v1/auth/tokens?"+rawQuery, nil, &out)
	return out, err
}

// BulkRevokeTokens calls POST /api/v1/tokens/bulk-revoke, session only.
func (c *authSessionClient) BulkRevokeTokens(ctx context.Context, req bulkRevokeRequest) (bulkRevokeResponse, error) {
	var out bulkRevokeResponse
	err := c.do(ctx, http.MethodPost, "/api/v1/tokens/bulk-revoke", req, &out)
	return out, err
}

// unusedTokensResponse mirrors internal/api's unusedTokensResponse.
type unusedTokensResponse struct {
	AlertDays int             `json:"alert_days"`
	Items     []tokenResource `json:"items"`
}

// ListUnusedTokens calls GET /api/v1/tokens/unused: idle tokens, never a secret.
func (c *authSessionClient) ListUnusedTokens(ctx context.Context) (unusedTokensResponse, error) {
	var out unusedTokensResponse
	err := c.do(ctx, http.MethodGet, "/api/v1/tokens/unused", nil, &out)
	return out, err
}

// RevokeToken calls DELETE /api/v1/auth/tokens/{id}.
func (c *authSessionClient) RevokeToken(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/auth/tokens/"+pathEscape(id), nil, nil)
}

// RotateToken calls POST /api/v1/auth/tokens/{id}/rotate.
func (c *authSessionClient) RotateToken(ctx context.Context, id string) (apiclient.RotatedToken, error) {
	var out apiclient.RotatedToken
	err := c.do(ctx, http.MethodPost, "/api/v1/auth/tokens/"+pathEscape(id)+"/rotate", nil, &out)
	return out, err
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

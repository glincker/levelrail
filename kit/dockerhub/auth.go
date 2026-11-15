package dockerhub

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultAuthURL     = "https://auth.docker.io"
	defaultRegistryURL = "https://registry-1.docker.io"
	registryService    = "registry.docker.io"
	// previewRepo is the repository Docker documents for reading pull
	// limit headers without spending quota.
	previewRepo = "ratelimitpreview/test"
	// registryTokenDefaultTTL is the registry token spec default when
	// expires_in is omitted.
	registryTokenDefaultTTL = 60 * time.Second
	// hubTokenLifetime is the documented life of a Hub API bearer.
	hubTokenLifetime = 10 * time.Minute
	tokenSkew        = 10 * time.Second

	// KindPersonal names a personal access token.
	KindPersonal = "personal"
	// KindOrganization names an organization access token.
	KindOrganization = "organization"
)

// Credentials is a Docker Hub username and access token. Its formatting
// methods never print the token, so a stray log line cannot leak it.
type Credentials struct {
	Username string
	Token    string
}

// Present reports whether both fields are set.
func (c Credentials) Present() bool { return c.Username != "" && c.Token != "" }

// Kind infers the token type from its documented prefix: organization
// access tokens start with dckr_oat_, everything else is treated as personal.
func (c Credentials) Kind() string {
	if strings.HasPrefix(c.Token, "dckr_oat_") {
		return KindOrganization
	}
	return KindPersonal
}

// String redacts the token.
func (c Credentials) String() string { return "dockerhub.Credentials{redacted}" }

// GoString redacts the token for %#v.
func (c Credentials) GoString() string { return c.String() }

// LogValue redacts the token for slog.
func (c Credentials) LogValue() slog.Value { return slog.StringValue("redacted") }

func (c Credentials) cacheKey() string {
	sum := sha256.Sum256([]byte(c.Username + "\x00" + c.Token))
	return hex.EncodeToString(sum[:8])
}

// Access is one granted entry of a registry token.
type Access struct {
	Type    string   `json:"type"`
	Name    string   `json:"name"`
	Actions []string `json:"actions"`
}

type registryToken struct {
	Token     string
	ExpiresAt time.Time
	Access    []Access
}

func (c *Client) authURL() string {
	if c.AuthURL != "" {
		return c.AuthURL
	}
	return defaultAuthURL
}

func (c *Client) registryURL() string {
	if c.RegistryURL != "" {
		return c.RegistryURL
	}
	return defaultRegistryURL
}

// registryToken runs the auth.docker.io token flow for one scope, Basic
// auth with the account name and token when cred is present.
func (c *Client) registryToken(ctx context.Context, cred Credentials, scope string) (registryToken, error) {
	key := "reg|" + cred.cacheKey() + "|" + scope
	if v, ok := c.cacheLoad(key); ok {
		return v.(registryToken), nil
	}
	q := url.Values{"service": {registryService}, "scope": {scope}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.authURL()+"/token?"+q.Encode(), nil)
	if err != nil {
		return registryToken{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if cred.Present() {
		req.SetBasicAuth(cred.Username, cred.Token)
	}
	var out struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if _, err := c.send(req, &out); err != nil {
		return registryToken{}, fmt.Errorf("dockerhub: registry token: %w", err)
	}
	tok := out.Token
	if tok == "" {
		tok = out.AccessToken
	}
	if tok == "" {
		return registryToken{}, fmt.Errorf("dockerhub: registry token: empty token in response")
	}
	ttl := registryTokenDefaultTTL
	if out.ExpiresIn > 0 {
		ttl = time.Duration(out.ExpiresIn) * time.Second
	}
	res := registryToken{Token: tok, ExpiresAt: time.Now().Add(ttl), Access: jwtAccess(tok)}
	if ttl > tokenSkew {
		c.cacheStore(key, res, ttl-tokenSkew)
	}
	return res, nil
}

// jwtAccess reads the unverified access claim of a registry token, for
// display only. Any parse problem yields nil.
func jwtAccess(tok string) []Access {
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		return nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		return nil
	}
	var claims struct {
		Access []Access `json:"access"`
	}
	if json.Unmarshal(raw, &claims) != nil {
		return nil
	}
	return claims.Access
}

// hubBearer exchanges the account name and token for a Hub API bearer.
// A 404 or 405 from the newer endpoint falls back to the legacy login,
// which accepts a personal access token as the password.
func (c *Client) hubBearer(ctx context.Context, cred Credentials) (string, error) {
	key := "hub|" + cred.cacheKey()
	if v, ok := c.cacheLoad(key); ok {
		return v.(string), nil
	}
	var out struct {
		AccessToken string `json:"access_token"`
		Token       string `json:"token"`
	}
	err := c.doJSON(ctx, http.MethodPost, c.baseURL()+"/v2/auth/token", "",
		map[string]string{"identifier": cred.Username, "secret": cred.Token}, &out)
	if errors.Is(err, ErrNotFound) {
		out.AccessToken = ""
		err = c.doJSON(ctx, http.MethodPost, c.baseURL()+"/v2/users/login", "",
			map[string]string{"username": cred.Username, "password": cred.Token}, &out)
	}
	if err != nil {
		return "", fmt.Errorf("dockerhub: hub token: %w", err)
	}
	tok := out.AccessToken
	if tok == "" {
		tok = out.Token
	}
	if tok == "" {
		return "", fmt.Errorf("dockerhub: hub token: empty token in response")
	}
	c.cacheStore(key, tok, hubTokenLifetime-tokenSkew)
	return tok, nil
}

type cacheEntry struct {
	value   any
	expires time.Time
	stored  time.Time
}

func (c *Client) cacheLoad(key string) (any, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.cache[key]
	if !ok || time.Now().After(e.expires) {
		delete(c.cache, key)
		return nil, false
	}
	return e.value, true
}

func (c *Client) cacheStore(key string, v any, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cache == nil {
		c.cache = map[string]cacheEntry{}
	}
	now := time.Now()
	c.cache[key] = cacheEntry{value: v, expires: now.Add(ttl), stored: now}
}

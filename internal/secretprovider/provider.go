// Package secretprovider reads app secrets live from Doppler, Infisical or 1Password Connect,
// read only, as a sibling of internal/vault. A provider value is never
// persisted: it is read at container create time, kept only in a short
// in-memory cache, and registered with the output masker.
package secretprovider

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/GLINCKER/levelrail/internal/secretmask"
)

// Provider types and their reference schemes.
const (
	TypeDoppler     = "doppler"
	TypeInfisical   = "infisical"
	TypeOnePassword = "onepassword"
	SchemeDoppler   = "doppler://"
	SchemeInfisical = "infisical://"
	SchemeOnePass   = "op://"
)

// Errors a caller may match to explain a failed read.
var (
	ErrUnauthorized    = errors.New("provider rejected the token")
	ErrNotFound        = errors.New("secret not found in provider")
	ErrBlockedAddress  = errors.New("provider address is private or link-local and is not an admin set self-hosted URL")
	ErrRedirectBlocked = errors.New("provider redirected to another host or scheme")
)

var defaultBaseURLs = map[string]string{
	TypeDoppler:   "https://api.doppler.com",
	TypeInfisical: "https://app.infisical.com",
}

var namePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,39}$`)

// Config is one provider connection. The token is stored separately and never appears here.
type Config struct {
	Name        string    `json:"name"`
	Type        string    `json:"type"`
	Project     string    `json:"project"`
	Environment string    `json:"environment"`
	Path        string    `json:"path,omitempty"`
	BaseURL     string    `json:"base_url,omitempty"`
	UpdatedAt   time.Time `json:"updated_at"`
	GraceUntil  time.Time `json:"grace_until,omitempty"`
}

// Validate checks the fields a read needs and the base URL rules.
func (c Config) Validate() error {
	if !namePattern.MatchString(c.Name) {
		return fmt.Errorf("name must be 1 to 40 lowercase letters, digits or underscores, starting with a letter")
	}
	if c.Type != TypeDoppler && c.Type != TypeInfisical && c.Type != TypeOnePassword {
		return fmt.Errorf("type must be %q, %q or %q", TypeDoppler, TypeInfisical, TypeOnePassword)
	}
	if c.Type == TypeOnePassword && c.BaseURL == "" {
		return fmt.Errorf("base_url is required for onepassword (the URL of your Connect server)")
	}
	if c.Type != TypeOnePassword && (c.Project == "" || c.Environment == "") {
		return fmt.Errorf("project and environment are required (Doppler project and config, Infisical workspace ID and environment slug)")
	}
	if c.BaseURL != "" {
		if c.Type == TypeDoppler {
			return fmt.Errorf("base_url is only supported for Infisical and onepassword")
		}
		u, err := url.Parse(c.BaseURL)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil {
			return fmt.Errorf("base_url must be an http or https URL with a host and no credentials")
		}
	}
	return nil
}

// Ref is a parsed provider reference held as an app secret value.
type Ref struct {
	Type     string
	Provider string
	Name     string
	Path     string
	// Vault and Item are set for op:// references only. Name holds the field.
	Vault string
	Item  string
}

// String renders the reference without any value.
func (r Ref) String() string {
	if r.Type == TypeOnePassword {
		if r.Vault == "" {
			return fmt.Sprintf("%s%s/%s/%s", SchemeOnePass, r.Provider, r.Item, r.Name)
		}
		return fmt.Sprintf("%s%s/%s/%s/%s", SchemeOnePass, r.Provider, r.Vault, r.Item, r.Name)
	}
	return fmt.Sprintf("%s://%s/%s", r.Type, r.Provider, strings.TrimPrefix(strings.TrimSuffix(r.Path, "/")+"/"+r.Name, "/"))
}

// ParseRef reports whether v is a provider reference and parses it.
// doppler://<provider>/<NAME>, infisical://<provider>/[dir/...]/<NAME>,
// op://<provider>/[<vault>/]<item>/<field> (vault defaults to the provider's project).
func ParseRef(v string) (Ref, bool, error) {
	var ref Ref
	var rest string
	switch {
	case strings.HasPrefix(v, SchemeDoppler):
		ref.Type, rest = TypeDoppler, strings.TrimPrefix(v, SchemeDoppler)
	case strings.HasPrefix(v, SchemeInfisical):
		ref.Type, rest = TypeInfisical, strings.TrimPrefix(v, SchemeInfisical)
	case strings.HasPrefix(v, SchemeOnePass):
		return parseOnePassword(strings.TrimPrefix(v, SchemeOnePass))
	default:
		return Ref{}, false, nil
	}
	parts := strings.Split(rest, "/")
	if len(parts) < 2 || parts[0] == "" || parts[len(parts)-1] == "" {
		return Ref{}, true, fmt.Errorf("reference must look like %s<provider>/<SECRET_NAME>", ref.Type+"://")
	}
	if ref.Type == TypeDoppler && len(parts) != 2 {
		return Ref{}, true, fmt.Errorf("doppler reference takes no path: doppler://<provider>/<SECRET_NAME>")
	}
	ref.Provider, ref.Name = parts[0], parts[len(parts)-1]
	if len(parts) > 2 {
		ref.Path = "/" + strings.Join(parts[1:len(parts)-1], "/")
	}
	return ref, true, nil
}

func parseOnePassword(rest string) (Ref, bool, error) {
	parts := strings.Split(rest, "/")
	if len(parts) < 3 || len(parts) > 4 {
		return Ref{}, true, fmt.Errorf("reference must look like op://<provider>/[<vault>/]<item>/<field>")
	}
	for i, p := range parts {
		dec, err := url.PathUnescape(p)
		if err != nil || dec == "" || strings.ContainsAny(dec, "\"\\\x00") {
			return Ref{}, true, fmt.Errorf("op reference parts must be non empty and must not contain quotes or backslashes")
		}
		parts[i] = dec
	}
	ref := Ref{Type: TypeOnePassword, Provider: parts[0], Item: parts[len(parts)-2], Name: parts[len(parts)-1]}
	if len(parts) == 4 {
		ref.Vault = parts[1]
	}
	return ref, true, nil
}

// Tunables bound one read; they come from runtime settings.
type Tunables struct {
	Timeout          time.Duration
	CacheTTL         time.Duration
	MaxResponseBytes int64
}

// maxIdentityLifeSeconds bounds a provider supplied expiresIn so it cannot overflow or pin a token.
const maxIdentityLifeSeconds int64 = 24 * 60 * 60

type cacheEntry struct {
	value   string
	expires time.Time
}

// Resolver performs provider reads with an SSRF-guarded HTTP client.
type Resolver struct {
	mu    sync.Mutex
	cache map[string]cacheEntry
	now   func() time.Time
	// idents holds one short-lived Infisical login per provider name; loginMu serializes relogins.
	loginMu sync.Mutex
	idents  map[string]identEntry
	// defaultBase overrides the public API hosts, for tests only.
	defaultBase map[string]string
}

// shared backs Default: app controllers are rebuilt each pass, so the cache must outlive them.
var shared = NewResolver()

// Default returns the process-wide Resolver whose cache every caller shares.
func Default() *Resolver { return shared }

// NewResolver returns a Resolver with an empty cache.
func NewResolver() *Resolver {
	return &Resolver{cache: map[string]cacheEntry{}, idents: map[string]identEntry{}, now: time.Now}
}

func (r *Resolver) endpoint(cfg Config) (*url.URL, bool, error) {
	raw, selfHosted := cfg.BaseURL, cfg.BaseURL != ""
	if !selfHosted {
		raw = defaultBaseURLs[cfg.Type]
		if v, ok := r.defaultBase[cfg.Type]; ok {
			raw = v
		}
	}
	u, err := url.Parse(strings.TrimSuffix(raw, "/"))
	if err != nil || u.Host == "" {
		return nil, false, fmt.Errorf("%s: invalid base URL", cfg.Type)
	}
	return u, selfHosted, nil
}

// blockedIP rejects metadata and link-local always, private and loopback unless self-hosted.
func blockedIP(ip net.IP, selfHosted bool) bool {
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() {
		return true
	}
	return !selfHosted && (ip.IsPrivate() || ip.IsLoopback())
}

func httpClient(base *url.URL, selfHosted bool, timeout time.Duration) *http.Client {
	dialer := &net.Dialer{Timeout: timeout, Control: func(_, address string, _ syscall.RawConn) error {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return err
		}
		if ip := net.ParseIP(host); ip == nil || blockedIP(ip, selfHosted) {
			return ErrBlockedAddress
		}
		return nil
	}}
	// Proxy is off so the dial check sees the real provider address.
	transport := &http.Transport{Proxy: nil, DialContext: dialer.DialContext, TLSHandshakeTimeout: timeout, ResponseHeaderTimeout: timeout}
	return &http.Client{Transport: transport, Timeout: timeout, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if req.URL.Host != base.Host || req.URL.Scheme != base.Scheme || len(via) > 3 {
			return ErrRedirectBlocked
		}
		return nil
	}}
}

func (r *Resolver) get(ctx context.Context, cfg Config, token, path string, query url.Values, t Tunables, out any) error {
	if cfg.Type == TypeInfisical && strings.HasPrefix(token, IdentityPrefix) {
		access, err := r.identityToken(ctx, cfg, token, t)
		if err != nil {
			return err
		}
		err = r.call(ctx, cfg, http.MethodGet, access, path, query, nil, t, out)
		if !errors.Is(err, ErrUnauthorized) {
			return err
		}
		// A revoked or used-up token must not stay cached; log in once more, never loop.
		r.dropIdentity(cfg.Name, access)
		access, lerr := r.identityToken(ctx, cfg, token, t)
		if lerr != nil {
			return lerr
		}
		return r.call(ctx, cfg, http.MethodGet, access, path, query, nil, t, out)
	}
	return r.call(ctx, cfg, http.MethodGet, token, path, query, nil, t, out)
}

// dropIdentity evicts the cached login only if it still holds the rejected token, so a
// concurrent fresh login is kept.
func (r *Resolver) dropIdentity(name, access string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e, ok := r.idents[name]; ok && e.access == access {
		delete(r.idents, name)
	}
}

func (r *Resolver) call(ctx context.Context, cfg Config, method, token, path string, query url.Values, payload []byte, t Tunables, out any) error {
	base, selfHosted, err := r.endpoint(cfg)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, t.Timeout)
	defer cancel()
	u := *base
	u.Path = strings.TrimSuffix(base.Path, "/") + path
	u.RawQuery = query.Encode()
	var body io.Reader
	if payload != nil {
		body = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return fmt.Errorf("%s: build request: %w", cfg.Type, err)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := httpClient(base, selfHosted, t.Timeout).Do(req)
	if err != nil {
		switch {
		case errors.Is(err, ErrRedirectBlocked), errors.Is(err, ErrBlockedAddress):
			return fmt.Errorf("%s: %w", cfg.Type, unwrapKnown(err))
		case errors.Is(err, context.DeadlineExceeded) || isTimeout(err):
			return fmt.Errorf("%s: provider did not answer within %s", cfg.Type, t.Timeout)
		}
		return fmt.Errorf("%s: provider unreachable at %s", cfg.Type, base.Host)
	}
	defer func() { _ = resp.Body.Close() }()
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return fmt.Errorf("%s: %w (HTTP %d)", cfg.Type, ErrUnauthorized, resp.StatusCode)
	case resp.StatusCode == http.StatusNotFound:
		return fmt.Errorf("%s: %w (HTTP 404)", cfg.Type, ErrNotFound)
	case resp.StatusCode != http.StatusOK:
		return fmt.Errorf("%s: provider answered HTTP %d", cfg.Type, resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, t.MaxResponseBytes+1))
	if err != nil {
		return fmt.Errorf("%s: read response: %w", cfg.Type, err)
	}
	if int64(len(raw)) > t.MaxResponseBytes {
		return fmt.Errorf("%s: response larger than %d bytes", cfg.Type, t.MaxResponseBytes)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("%s: unexpected response shape", cfg.Type)
	}
	return nil
}

// IdentityPrefix marks a stored Infisical credential as machine identity universal auth:
// "infisical-identity:<clientId>:<clientSecret>". Anything else is used as an access token.
const IdentityPrefix = "infisical-identity:"

type identEntry struct {
	cred    string
	access  string
	expires time.Time
}

// ParseIdentity splits a stored identity credential; ok is false when it is not one.
func ParseIdentity(token string) (id, secret string, ok bool) {
	rest, found := strings.CutPrefix(token, IdentityPrefix)
	if !found {
		return "", "", false
	}
	id, secret, found = strings.Cut(rest, ":")
	if !found || id == "" || secret == "" {
		return "", "", false
	}
	return id, secret, true
}

// identityToken returns a cached access token for the provider, logging in lazily when it is
// missing, expired or minted from different credentials. No goroutine, one entry per provider.
func (r *Resolver) identityToken(ctx context.Context, cfg Config, cred string, t Tunables) (string, error) {
	cached := func() (string, bool) {
		r.mu.Lock()
		defer r.mu.Unlock()
		e, ok := r.idents[cfg.Name]
		if ok && e.cred == cred && r.now().Before(e.expires) {
			return e.access, true
		}
		return "", false
	}
	if a, ok := cached(); ok {
		return a, nil
	}
	r.loginMu.Lock()
	defer r.loginMu.Unlock()
	if a, ok := cached(); ok {
		return a, nil
	}
	id, secret, ok := ParseIdentity(cred)
	if !ok {
		return "", fmt.Errorf("infisical: machine identity credential must be %s<clientId>:<clientSecret>", IdentityPrefix)
	}
	payload, err := json.Marshal(map[string]string{"clientId": id, "clientSecret": secret})
	if err != nil {
		return "", fmt.Errorf("infisical: encode login")
	}
	var out struct {
		AccessToken string `json:"accessToken"`
		ExpiresIn   int64  `json:"expiresIn"`
	}
	if err := r.call(ctx, cfg, http.MethodPost, "", "/api/v1/auth/universal-auth/login", nil, payload, t, &out); err != nil {
		return "", fmt.Errorf("machine identity login: %w", err)
	}
	if out.AccessToken == "" || out.ExpiresIn <= 0 {
		return "", fmt.Errorf("infisical: machine identity login returned no usable token")
	}
	secretmask.Add(out.AccessToken)
	// Refresh at 90 percent of the lifetime so a read never starts on a token about to lapse.
	secs := min(out.ExpiresIn, maxIdentityLifeSeconds)
	life := time.Duration(secs) * time.Second * 9 / 10
	r.mu.Lock()
	r.idents[cfg.Name] = identEntry{cred: cred, access: out.AccessToken, expires: r.now().Add(life)}
	r.mu.Unlock()
	return out.AccessToken, nil
}

func unwrapKnown(err error) error {
	if errors.Is(err, ErrRedirectBlocked) {
		return ErrRedirectBlocked
	}
	return ErrBlockedAddress
}

func isTimeout(err error) bool {
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

func cacheKey(cfg Config, token string, ref Ref) string {
	h := sha256.Sum256([]byte(strings.Join([]string{cfg.Type, cfg.BaseURL, cfg.Project, cfg.Environment, cfg.Path, ref.Path, ref.Vault, ref.Item, ref.Name, token}, "\x00")))
	return hex.EncodeToString(h[:])
}

// Fetch reads one secret value, serving it from the cache while fresh.
func (r *Resolver) Fetch(ctx context.Context, cfg Config, token string, ref Ref, t Tunables) (string, error) {
	key := cacheKey(cfg, token, ref)
	now := r.now()
	if t.CacheTTL > 0 {
		r.mu.Lock()
		e, ok := r.cache[key]
		r.mu.Unlock()
		if ok && now.Before(e.expires) {
			return e.value, nil
		}
	}
	value, err := r.fetch(ctx, cfg, token, ref, t)
	if err != nil {
		return "", err
	}
	if value == "" {
		return "", fmt.Errorf("%s: secret %q is empty", cfg.Type, ref.Name)
	}
	secretmask.Add(value)
	if t.CacheTTL > 0 {
		r.mu.Lock()
		for k, e := range r.cache {
			if now.After(e.expires) {
				delete(r.cache, k)
			}
		}
		r.cache[key] = cacheEntry{value: value, expires: now.Add(t.CacheTTL)}
		r.mu.Unlock()
	}
	return value, nil
}

// opIDPattern matches a Connect vault or item ID, so names and IDs both work.
var opIDPattern = regexp.MustCompile(`^[a-z0-9]{26}$`)

// opLookup resolves a vault or item name to its Connect ID with an exact filter; an ID passes through.
func (r *Resolver) opLookup(ctx context.Context, cfg Config, token, path, key, name string, t Tunables) (string, error) {
	if opIDPattern.MatchString(name) {
		return name, nil
	}
	if strings.ContainsAny(name, "\"\\") {
		return "", fmt.Errorf("onepassword: vault and item names must not contain quotes or backslashes")
	}
	var list []struct {
		ID string `json:"id"`
	}
	q := url.Values{"filter": {key + ` eq "` + name + `"`}}
	if err := r.get(ctx, cfg, token, path, q, t, &list); err != nil {
		return "", err
	}
	if len(list) != 1 || list[0].ID == "" {
		return "", fmt.Errorf("onepassword: %w", ErrNotFound)
	}
	return url.PathEscape(list[0].ID), nil
}

func (r *Resolver) fetchOnePassword(ctx context.Context, cfg Config, token string, ref Ref, t Tunables) (string, error) {
	vault := ref.Vault
	if vault == "" {
		vault = cfg.Project
	} else if cfg.Project != "" && vault != cfg.Project {
		// The provider's vault is a limit set by root, not a default a secret writer can override.
		return "", fmt.Errorf("onepassword: vault %q is outside the provider's vault", vault)
	}
	if vault == "" {
		return "", fmt.Errorf("onepassword: no vault in the reference and no default vault on the provider")
	}
	vid, err := r.opLookup(ctx, cfg, token, "/v1/vaults", "name", vault, t)
	if err != nil {
		return "", err
	}
	iid, err := r.opLookup(ctx, cfg, token, "/v1/vaults/"+vid+"/items", "title", ref.Item, t)
	if err != nil {
		return "", err
	}
	var item struct {
		Fields []struct {
			ID    string  `json:"id"`
			Label string  `json:"label"`
			Value *string `json:"value"`
		} `json:"fields"`
	}
	if err := r.get(ctx, cfg, token, "/v1/vaults/"+vid+"/items/"+iid, nil, t, &item); err != nil {
		return "", err
	}
	for _, f := range item.Fields {
		if (f.Label == ref.Name || f.ID == ref.Name) && f.Value != nil {
			return *f.Value, nil
		}
	}
	return "", fmt.Errorf("onepassword: %w", ErrNotFound)
}

func (r *Resolver) fetch(ctx context.Context, cfg Config, token string, ref Ref, t Tunables) (string, error) {
	if cfg.Type == TypeOnePassword {
		return r.fetchOnePassword(ctx, cfg, token, ref, t)
	}
	if cfg.Type == TypeDoppler {
		var out struct {
			Value struct {
				Raw      *string `json:"raw"`
				Computed *string `json:"computed"`
			} `json:"value"`
		}
		q := url.Values{"project": {cfg.Project}, "config": {cfg.Environment}, "name": {ref.Name}}
		if err := r.get(ctx, cfg, token, "/v3/configs/config/secret", q, t, &out); err != nil {
			return "", err
		}
		if out.Value.Computed != nil {
			return *out.Value.Computed, nil
		}
		if out.Value.Raw != nil {
			return *out.Value.Raw, nil
		}
		return "", fmt.Errorf("doppler: %w", ErrNotFound)
	}
	dir, err := infisicalPath(cfg.Path, ref.Path)
	if err != nil {
		return "", err
	}
	var out struct {
		Secret struct {
			Value *string `json:"secretValue"`
		} `json:"secret"`
	}
	q := url.Values{"workspaceId": {cfg.Project}, "environment": {cfg.Environment}, "secretPath": {dir}}
	if err := r.get(ctx, cfg, token, "/api/v3/secrets/raw/"+url.PathEscape(ref.Name), q, t, &out); err != nil {
		return "", err
	}
	if out.Secret.Value == nil {
		return "", fmt.Errorf("infisical: %w", ErrNotFound)
	}
	return *out.Secret.Value, nil
}

// infisicalPath treats the provider's path as a limit set by root: a reference may only narrow it.
func infisicalPath(limit, refPath string) (string, error) {
	base := path.Clean("/" + limit)
	if refPath == "" {
		return base, nil
	}
	p := path.Clean("/" + refPath)
	if base != "/" && p != base && !strings.HasPrefix(p, base+"/") {
		return "", fmt.Errorf("infisical: path %q is outside the provider's path %q", p, base)
	}
	return p, nil
}

// Test checks the token can list secret names in the configured scope, without reading values into the result.
func (r *Resolver) Test(ctx context.Context, cfg Config, token string, t Tunables) error {
	if cfg.Type == TypeOnePassword {
		return r.get(ctx, cfg, token, "/v1/vaults", nil, t, nil)
	}
	if cfg.Type == TypeDoppler {
		q := url.Values{"project": {cfg.Project}, "config": {cfg.Environment}}
		return r.get(ctx, cfg, token, "/v3/configs/config/secrets/names", q, t, nil)
	}
	q := url.Values{"workspaceId": {cfg.Project}, "environment": {cfg.Environment}, "secretPath": {path.Clean("/" + cfg.Path)}}
	return r.get(ctx, cfg, token, "/api/v3/secrets/raw", q, t, nil)
}

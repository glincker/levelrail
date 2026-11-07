// Package secretprovider reads app secrets live from Doppler or Infisical,
// read only, as a sibling of internal/vault. A provider value is never
// persisted: it is read at container create time, kept only in a short
// in-memory cache, and registered with the output masker.
package secretprovider

import (
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
	SchemeDoppler   = "doppler://"
	SchemeInfisical = "infisical://"
)

// Errors a caller may match to explain a failed read.
var (
	ErrUnauthorized    = errors.New("provider rejected the token")
	ErrNotFound        = errors.New("secret not found in provider")
	ErrBlockedAddress  = errors.New("provider address is private or link-local and is not an admin set self-hosted URL")
	ErrRedirectBlocked = errors.New("provider redirected to another host")
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
	if c.Type != TypeDoppler && c.Type != TypeInfisical {
		return fmt.Errorf("type must be %q or %q", TypeDoppler, TypeInfisical)
	}
	if c.Project == "" || c.Environment == "" {
		return fmt.Errorf("project and environment are required (Doppler project and config, Infisical workspace ID and environment slug)")
	}
	if c.BaseURL != "" {
		if c.Type != TypeInfisical {
			return fmt.Errorf("base_url is only supported for self-hosted Infisical")
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
}

// String renders the reference without any value.
func (r Ref) String() string {
	return fmt.Sprintf("%s://%s/%s", r.Type, r.Provider, strings.TrimPrefix(strings.TrimSuffix(r.Path, "/")+"/"+r.Name, "/"))
}

// ParseRef reports whether v is a provider reference and parses it.
// doppler://<provider>/<NAME>, infisical://<provider>/[dir/...]/<NAME>.
func ParseRef(v string) (Ref, bool, error) {
	var ref Ref
	var rest string
	switch {
	case strings.HasPrefix(v, SchemeDoppler):
		ref.Type, rest = TypeDoppler, strings.TrimPrefix(v, SchemeDoppler)
	case strings.HasPrefix(v, SchemeInfisical):
		ref.Type, rest = TypeInfisical, strings.TrimPrefix(v, SchemeInfisical)
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

// Tunables bound one read; they come from runtime settings.
type Tunables struct {
	Timeout          time.Duration
	CacheTTL         time.Duration
	MaxResponseBytes int64
}

type cacheEntry struct {
	value   string
	expires time.Time
}

// Resolver performs provider reads with an SSRF-guarded HTTP client.
type Resolver struct {
	mu    sync.Mutex
	cache map[string]cacheEntry
	now   func() time.Time
	// defaultBase overrides the public API hosts, for tests only.
	defaultBase map[string]string
}

// shared backs Default: app controllers are rebuilt each pass, so the cache must outlive them.
var shared = NewResolver()

// Default returns the process-wide Resolver whose cache every caller shares.
func Default() *Resolver { return shared }

// NewResolver returns a Resolver with an empty cache.
func NewResolver() *Resolver {
	return &Resolver{cache: map[string]cacheEntry{}, now: time.Now}
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
		if req.URL.Host != base.Host || len(via) > 3 {
			return ErrRedirectBlocked
		}
		return nil
	}}
}

func (r *Resolver) get(ctx context.Context, cfg Config, token, path string, query url.Values, t Tunables, out any) error {
	base, selfHosted, err := r.endpoint(cfg)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, t.Timeout)
	defer cancel()
	u := *base
	u.Path = strings.TrimSuffix(base.Path, "/") + path
	u.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return fmt.Errorf("%s: build request: %w", cfg.Type, err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
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
	body, err := io.ReadAll(io.LimitReader(resp.Body, t.MaxResponseBytes+1))
	if err != nil {
		return fmt.Errorf("%s: read response: %w", cfg.Type, err)
	}
	if int64(len(body)) > t.MaxResponseBytes {
		return fmt.Errorf("%s: response larger than %d bytes", cfg.Type, t.MaxResponseBytes)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("%s: unexpected response shape", cfg.Type)
	}
	return nil
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
	h := sha256.Sum256([]byte(strings.Join([]string{cfg.Type, cfg.BaseURL, cfg.Project, cfg.Environment, cfg.Path, ref.Path, ref.Name, token}, "\x00")))
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

func (r *Resolver) fetch(ctx context.Context, cfg Config, token string, ref Ref, t Tunables) (string, error) {
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
	path := ref.Path
	if path == "" {
		path = cfg.Path
	}
	if path == "" {
		path = "/"
	}
	var out struct {
		Secret struct {
			Value *string `json:"secretValue"`
		} `json:"secret"`
	}
	q := url.Values{"workspaceId": {cfg.Project}, "environment": {cfg.Environment}, "secretPath": {path}}
	if err := r.get(ctx, cfg, token, "/api/v3/secrets/raw/"+url.PathEscape(ref.Name), q, t, &out); err != nil {
		return "", err
	}
	if out.Secret.Value == nil {
		return "", fmt.Errorf("infisical: %w", ErrNotFound)
	}
	return *out.Secret.Value, nil
}

// Test checks the token can list secret names in the configured scope, without reading values into the result.
func (r *Resolver) Test(ctx context.Context, cfg Config, token string, t Tunables) error {
	if cfg.Type == TypeDoppler {
		q := url.Values{"project": {cfg.Project}, "config": {cfg.Environment}}
		return r.get(ctx, cfg, token, "/v3/configs/config/secrets/names", q, t, nil)
	}
	path := cfg.Path
	if path == "" {
		path = "/"
	}
	q := url.Values{"workspaceId": {cfg.Project}, "environment": {cfg.Environment}, "secretPath": {path}}
	return r.get(ctx, cfg, token, "/api/v3/secrets/raw", q, t, nil)
}

package ingress

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// EnvAuthGateTimeout overrides the auth gate subrequest timeout for domains
// that do not set their own. Go duration ("3s") or whole seconds ("3").
const EnvAuthGateTimeout = "APP_AUTH_GATE_TIMEOUT"

const (
	defaultAuthGateTimeout = 5 * time.Second
	// MaxAuthGateCopyHeaders caps copy_headers per domain.
	MaxAuthGateCopyHeaders = 16
	// MaxAuthGateTimeoutSeconds caps a domain's own timeout override.
	MaxAuthGateTimeoutSeconds = 120
	maxAuthGateCheckPathLen   = 1024
	// DefaultAuthGateCheckPath is the check path when none is configured.
	DefaultAuthGateCheckPath = "/"
)

// Copying these would either break the proxied request or strip the very
// credentials the auth service needs, since copies are deleted up front.
var reservedAuthGateHeaders = map[string]bool{
	"Host": true, "Content-Length": true, "Transfer-Encoding": true, "Connection": true,
	"Upgrade": true, "Te": true, "Trailer": true, "Keep-Alive": true, "Proxy-Connection": true,
	"Cookie": true, "Authorization": true, "Proxy-Authorization": true,
	"X-Forwarded-For": true, "X-Forwarded-Host": true, "X-Forwarded-Proto": true,
	"X-Forwarded-Method": true, "X-Forwarded-Uri": true,
}

// AuthGate is one domain's forward-auth check, run before the request
// reaches the backend.
type AuthGate struct {
	URL         string
	CheckPath   string
	CopyHeaders []string
	Timeout     time.Duration
}

// VarsHandler is Caddy's no-op "vars" handler; handle_response skips a
// response handler whose routes are empty, so the 2xx branch carries one.
type VarsHandler struct {
	Handler string `json:"handler"`
}

// AuthGateTimeoutFromEnv returns the default subrequest timeout.
func AuthGateTimeoutFromEnv(getenv func(string) string) (time.Duration, error) {
	raw := strings.TrimSpace(getenv(EnvAuthGateTimeout))
	if raw == "" {
		return defaultAuthGateTimeout, nil
	}
	if n, err := strconv.Atoi(raw); err == nil {
		if n <= 0 {
			return defaultAuthGateTimeout, fmt.Errorf("ingress: %s=%q must be positive", EnvAuthGateTimeout, raw)
		}
		return time.Duration(n) * time.Second, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return defaultAuthGateTimeout, fmt.Errorf("ingress: %s=%q is not a positive duration", EnvAuthGateTimeout, raw)
	}
	return d, nil
}

// ValidateAuthGateURL checks an auth service URL: http or https, a host,
// no userinfo, query or fragment. Private and internal hosts are allowed.
func ValidateAuthGateURL(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.New("auth_url is required")
	}
	if strings.ContainsAny(raw, "{} \t\r\n") {
		return nil, errors.New("auth_url must not contain spaces or braces")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("auth_url is not a valid URL: %w", err)
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return nil, errors.New("auth_url must use http or https")
	}
	u.Scheme = scheme
	if u.User != nil {
		return nil, errors.New("auth_url must not contain a username or password")
	}
	if u.Opaque != "" || u.Hostname() == "" {
		return nil, errors.New("auth_url must include a host")
	}
	if u.RawQuery != "" || u.Fragment != "" || strings.Contains(raw, "#") {
		return nil, errors.New("auth_url must not contain a query or fragment; put it in check_path")
	}
	if p := u.Port(); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 || n > 65535 {
			return nil, errors.New("auth_url has an invalid port")
		}
	}
	return u, nil
}

// NormalizeAuthGateCheckPath defaults an empty path to "/" and rejects
// anything that is not an absolute path with an optional query.
func NormalizeAuthGateCheckPath(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return DefaultAuthGateCheckPath, nil
	}
	if !strings.HasPrefix(p, "/") || strings.HasPrefix(p, "//") {
		return "", errors.New("check_path must start with a single /")
	}
	if len(p) > maxAuthGateCheckPathLen {
		return "", fmt.Errorf("check_path must be at most %d characters", maxAuthGateCheckPathLen)
	}
	for _, r := range p {
		if r <= ' ' || r == 0x7f || r == '{' || r == '}' || r == '#' || r == '\\' {
			return "", errors.New("check_path contains a character that is not allowed")
		}
	}
	return p, nil
}

// NormalizeAuthGateCopyHeaders canonicalizes and dedupes header names,
// rejecting non-tokens, reserved names and lists over the cap.
func NormalizeAuthGateCopyHeaders(names []string) ([]string, error) {
	seen := map[string]bool{}
	out := make([]string, 0, len(names))
	for _, raw := range names {
		name := strings.TrimSpace(raw)
		if name == "" {
			return nil, errors.New("copy_headers must not contain empty names")
		}
		if !isHeaderToken(name) {
			return nil, fmt.Errorf("copy_headers: %q is not a valid header name", name)
		}
		canon := http.CanonicalHeaderKey(name)
		if reservedAuthGateHeaders[canon] {
			return nil, fmt.Errorf("copy_headers: %q cannot be copied", canon)
		}
		if seen[canon] {
			continue
		}
		seen[canon] = true
		out = append(out, canon)
	}
	if len(out) > MaxAuthGateCopyHeaders {
		return nil, fmt.Errorf("copy_headers allows at most %d names", MaxAuthGateCopyHeaders)
	}
	return out, nil
}

func isHeaderToken(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case strings.IndexByte("!#$%&'*+-.^_`|~", c) >= 0:
		default:
			return false
		}
	}
	return s != ""
}

// Validate reports whether g compiles into a gate.
func (g AuthGate) Validate() error {
	_, err := g.handlers()
	return err
}

// handlers compiles the gate: strip client copies of every copy header,
// then a reverse_proxy subrequest whose 2xx branch copies the configured
// headers and falls through, while any other status is returned as is.
func (g AuthGate) handlers() ([]any, error) {
	u, err := ValidateAuthGateURL(g.URL)
	if err != nil {
		return nil, err
	}
	checkPath, err := NormalizeAuthGateCheckPath(g.CheckPath)
	if err != nil {
		return nil, err
	}
	copyHeaders, err := NormalizeAuthGateCopyHeaders(g.CopyHeaders)
	if err != nil {
		return nil, err
	}
	timeout := g.Timeout
	if timeout <= 0 {
		timeout = defaultAuthGateTimeout
	}
	port := u.Port()
	if port == "" {
		port = "80"
		if u.Scheme == "https" {
			port = "443"
		}
	}
	transport := &HTTPTransport{Protocol: "http", DialTimeout: timeout.String(), ResponseHeaderTimeout: timeout.String()}
	if u.Scheme == "https" {
		transport.TLS = &UpstreamTLSConfig{ServerName: u.Hostname()}
	}

	uri := strings.TrimRight(u.EscapedPath(), "/") + checkPath
	if !strings.Contains(checkPath, "?") {
		// A bare "?" clears the query; without it Caddy keeps the client's.
		uri += "?"
	}

	good := []Route{{Handle: []any{VarsHandler{Handler: "vars"}}}}
	for _, name := range copyHeaders {
		ph := "{http.reverse_proxy.header." + name + "}"
		good = append(good, Route{
			Match:  []Matcher{{Not: []Matcher{{Vars: map[string][]string{ph: {""}}}}}},
			Handle: []any{HeadersHandler{Handler: "headers", Request: &HeaderOps{Set: map[string][]string{name: {ph}}}}},
		})
	}

	// GET drops the request body so the backend still receives it.
	proxy := ReverseProxyHandler{
		Handler:   "reverse_proxy",
		Upstreams: []Upstream{{Dial: net.JoinHostPort(u.Hostname(), port)}},
		Transport: transport,
		Rewrite:   &ProxyRewrite{Method: http.MethodGet, URI: uri},
		Headers: &ProxyHeaders{Request: &HeaderOps{Set: map[string][]string{
			"Host":               {u.Host},
			"X-Forwarded-Method": {"{http.request.method}"},
			"X-Forwarded-Uri":    {"{http.request.uri}"},
		}}},
		HandleResponse: []ResponseHandler{{Match: &ResponseMatcher{StatusCode: []int{2}}, Routes: good}},
	}

	var out []any
	if len(copyHeaders) > 0 {
		out = append(out, HeadersHandler{Handler: "headers", Request: &HeaderOps{Delete: copyHeaders}})
	}
	return append(out, proxy), nil
}

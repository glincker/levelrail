package ingress

import (
	"fmt"
	"html"
	"net/netip"
	"strconv"
	"strings"
	"time"
)

// Environment variables that tune Hardening. Durations use Go syntax ("10s"),
// "0" disables a timeout or limit where noted.
const (
	EnvReadHeaderTimeout = "APP_INGRESS_READ_HEADER_TIMEOUT"
	EnvReadTimeout       = "APP_INGRESS_READ_TIMEOUT"
	EnvWriteTimeout      = "APP_INGRESS_WRITE_TIMEOUT"
	EnvIdleTimeout       = "APP_INGRESS_IDLE_TIMEOUT"
	EnvMaxHeaderBytes    = "APP_INGRESS_MAX_HEADER_BYTES"
	EnvMaxBodyBytes      = "APP_INGRESS_MAX_BODY_BYTES"
	EnvProtocols         = "APP_INGRESS_PROTOCOLS"
	EnvTrustedProxies    = "APP_INGRESS_TRUSTED_PROXIES"
	EnvClientIPHeaders   = "APP_INGRESS_CLIENT_IP_HEADERS"
	EnvRetryWindow       = "APP_INGRESS_RETRY_WINDOW"
	EnvRetryInterval     = "APP_INGRESS_RETRY_INTERVAL"
	EnvDialTimeout       = "APP_INGRESS_DIAL_TIMEOUT"
	EnvFailDuration      = "APP_INGRESS_PASSIVE_FAIL_DURATION"
	EnvRetryAfter        = "APP_INGRESS_UNAVAILABLE_RETRY_AFTER"
	EnvGracePeriod       = "APP_INGRESS_GRACE_PERIOD"
)

// Hardening is the edge policy applied to every proxied route and to the shared
// HTTP server: slowloris and size limits, protocols, trusted proxies, and the
// failover behaviour that replaces a raw 502 with a friendly 503.
type Hardening struct {
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	MaxHeaderBytes    int
	MaxBodyBytes      int64
	Protocols         []string
	TrustedProxies    []string
	ClientIPHeaders   []string
	RetryWindow       time.Duration
	RetryInterval     time.Duration
	DialTimeout       time.Duration
	FailDuration      time.Duration
	RetryAfterSeconds int
	// GracePeriod lets in-flight requests finish when the config is replaced or
	// the process stops. Zero closes them at once.
	GracePeriod time.Duration
}

// DefaultHardening returns defaults that are safe for general web apps: no
// whole-request read or write deadline (uploads and SSE must keep working), but
// bounded headers, idle connections and dial time.
func DefaultHardening() Hardening {
	return Hardening{
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    128 << 10,
		MaxBodyBytes:      1 << 30,
		Protocols:         []string{"h1", "h2", "h3"},
		RetryWindow:       2 * time.Second,
		RetryInterval:     250 * time.Millisecond,
		DialTimeout:       2 * time.Second,
		FailDuration:      5 * time.Second,
		RetryAfterSeconds: 5,
	}
}

// HardeningFromEnv reads every APP_INGRESS_* tuning variable over the defaults.
func HardeningFromEnv(getenv func(string) string) (Hardening, error) {
	h := DefaultHardening()
	durations := []struct {
		env string
		dst *time.Duration
	}{
		{EnvReadHeaderTimeout, &h.ReadHeaderTimeout},
		{EnvReadTimeout, &h.ReadTimeout},
		{EnvWriteTimeout, &h.WriteTimeout},
		{EnvIdleTimeout, &h.IdleTimeout},
		{EnvRetryWindow, &h.RetryWindow},
		{EnvRetryInterval, &h.RetryInterval},
		{EnvDialTimeout, &h.DialTimeout},
		{EnvFailDuration, &h.FailDuration},
		{EnvGracePeriod, &h.GracePeriod},
	}
	for _, d := range durations {
		raw := strings.TrimSpace(getenv(d.env))
		if raw == "" {
			continue
		}
		v, err := time.ParseDuration(raw)
		if err != nil || v < 0 {
			return h, fmt.Errorf("ingress: %s=%q is not a non-negative duration", d.env, raw)
		}
		*d.dst = v
	}
	if raw := strings.TrimSpace(getenv(EnvMaxHeaderBytes)); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v < 0 {
			return h, fmt.Errorf("ingress: %s=%q is not a non-negative integer", EnvMaxHeaderBytes, raw)
		}
		h.MaxHeaderBytes = v
	}
	if raw := strings.TrimSpace(getenv(EnvMaxBodyBytes)); raw != "" {
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || v < 0 {
			return h, fmt.Errorf("ingress: %s=%q is not a non-negative integer", EnvMaxBodyBytes, raw)
		}
		h.MaxBodyBytes = v
	}
	if raw := strings.TrimSpace(getenv(EnvRetryAfter)); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v < 0 {
			return h, fmt.Errorf("ingress: %s=%q is not a non-negative integer", EnvRetryAfter, raw)
		}
		h.RetryAfterSeconds = v
	}
	if raw := getenv(EnvProtocols); strings.TrimSpace(raw) != "" {
		protos, err := parseProtocols(raw)
		if err != nil {
			return h, err
		}
		h.Protocols = protos
	}
	if raw := getenv(EnvTrustedProxies); strings.TrimSpace(raw) != "" {
		ranges, err := parseTrustedProxies(raw)
		if err != nil {
			return h, err
		}
		h.TrustedProxies = ranges
	}
	h.ClientIPHeaders = splitList(getenv(EnvClientIPHeaders))
	return h, nil
}

func splitList(raw string) []string {
	var out []string
	for _, part := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ' ' || r == ';' }) {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func parseProtocols(raw string) ([]string, error) {
	var out []string
	for _, p := range splitList(raw) {
		switch p = strings.ToLower(p); p {
		case "h1", "h2", "h2c", "h3":
			out = append(out, p)
		default:
			return nil, fmt.Errorf("ingress: %s: unknown protocol %q (want h1, h2, h2c or h3)", EnvProtocols, p)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("ingress: %s lists no protocols", EnvProtocols)
	}
	return out, nil
}

// privateRanges is expanded here instead of relying on Caddy's own shorthand so
// the effective list is visible in the applied config.
var privateRanges = []string{
	"192.168.0.0/16", "172.16.0.0/12", "10.0.0.0/8", "127.0.0.1/8", "fd00::/8", "::1",
}

func parseTrustedProxies(raw string) ([]string, error) {
	var out []string
	for _, entry := range splitList(raw) {
		if strings.EqualFold(entry, "private_ranges") {
			out = append(out, privateRanges...)
			continue
		}
		if _, err := netip.ParsePrefix(entry); err != nil {
			if _, addrErr := netip.ParseAddr(entry); addrErr != nil {
				return nil, fmt.Errorf("ingress: %s: %q is not an IP or CIDR", EnvTrustedProxies, entry)
			}
		}
		out = append(out, entry)
	}
	return out, nil
}

// TrustedProxiesConfig is Caddy's static trusted_proxies source.
type TrustedProxiesConfig struct {
	Source string   `json:"source"`
	Ranges []string `json:"ranges"`
}

// RequestBodyHandler is Caddy's request_body handler: it caps how much of a
// request body the proxy will read.
type RequestBodyHandler struct {
	Handler string `json:"handler"`
	MaxSize int64  `json:"max_size"`
}

// HeaderOps is Caddy's headers.HeaderOps.
type HeaderOps struct {
	Set map[string][]string `json:"set,omitempty"`
}

// ProxyHeaders is reverse_proxy's headers block.
type ProxyHeaders struct {
	Request *HeaderOps `json:"request,omitempty"`
}

func (h Hardening) durationOrNil(d time.Duration) string {
	if d <= 0 {
		return ""
	}
	return d.String()
}

// applyServer copies the server-level limits and client IP policy onto s.
func (h Hardening) applyServer(s *Server) {
	s.ReadHeaderTimeout = h.durationOrNil(h.ReadHeaderTimeout)
	s.ReadTimeout = h.durationOrNil(h.ReadTimeout)
	s.WriteTimeout = h.durationOrNil(h.WriteTimeout)
	s.IdleTimeout = h.durationOrNil(h.IdleTimeout)
	s.MaxHeaderBytes = h.MaxHeaderBytes
	s.Protocols = h.Protocols
	if len(h.TrustedProxies) > 0 {
		s.TrustedProxies = &TrustedProxiesConfig{Source: "static", Ranges: h.TrustedProxies}
		s.ClientIPHeaders = h.ClientIPHeaders
		s.TrustedProxiesStrict = 1
	}
	s.Errors = &ErrorsConfig{Routes: h.unavailableRoutes()}
}

// applyProxy gives a reverse_proxy handler the failover defaults: a short retry
// window over dial failures, a bounded dial time, passive ejection for pools
// with more than one upstream, and the real client address for the app.
func (h Hardening) applyProxy(rp *ReverseProxyHandler) {
	if h.RetryWindow > 0 {
		if rp.LoadBalancing == nil {
			rp.LoadBalancing = &LoadBalancing{}
		}
		if rp.LoadBalancing.TryDuration == "" {
			rp.LoadBalancing.TryDuration = h.RetryWindow.String()
			if rp.LoadBalancing.TryInterval == "" && h.RetryInterval > 0 {
				rp.LoadBalancing.TryInterval = h.RetryInterval.String()
			}
		}
	}
	if len(rp.Upstreams) > 1 && rp.HealthChecks == nil && h.FailDuration > 0 {
		rp.HealthChecks = &HealthChecks{Passive: &PassiveHealthChecks{FailDuration: h.FailDuration.String(), MaxFails: 1}}
	}
	if h.DialTimeout > 0 {
		if rp.Transport == nil {
			rp.Transport = &HTTPTransport{Protocol: "http"}
		}
		if rp.Transport.DialTimeout == "" {
			rp.Transport.DialTimeout = h.DialTimeout.String()
		}
	}
	if rp.Headers == nil {
		rp.Headers = &ProxyHeaders{}
	}
	if rp.Headers.Request == nil {
		rp.Headers.Request = &HeaderOps{Set: map[string][]string{}}
	}
	if _, ok := rp.Headers.Request.Set["X-Real-IP"]; !ok {
		rp.Headers.Request.Set["X-Real-IP"] = []string{"{http.vars.client_ip}"}
	}
}

// bodyLimit returns the request_body handler to prepend, or nil when unlimited.
func (h Hardening) bodyLimit() any {
	if h.MaxBodyBytes <= 0 {
		return nil
	}
	return RequestBodyHandler{Handler: "request_body", MaxSize: h.MaxBodyBytes}
}

const unavailablePageTemplate = `<!DOCTYPE html>
<html lang="en">
<head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta http-equiv="refresh" content="%d"><title>%s</title></head>
<body style="font-family:sans-serif;text-align:center;padding:4rem 1rem">
<h1>%s</h1>
<p>%s</p>
</body>
</html>
`

func (h Hardening) retryAfter() int {
	if h.RetryAfterSeconds <= 0 {
		return 5
	}
	return h.RetryAfterSeconds
}

func (h Hardening) unavailableHeaders() map[string][]string {
	return map[string][]string{
		"Content-Type":  {"text/html; charset=utf-8"},
		"Retry-After":   {strconv.Itoa(h.retryAfter())},
		"Cache-Control": {"no-store"},
	}
}

// UnavailableResponse is the friendly 503 served when no backend can answer
// right now: it carries Retry-After, is never cached, and refreshes itself.
func (h Hardening) UnavailableResponse(title, message string) StaticResponseHandler {
	return StaticResponseHandler{
		Handler:    "static_response",
		StatusCode: 503,
		Body:       fmt.Sprintf(unavailablePageTemplate, h.retryAfter(), html.EscapeString(title), html.EscapeString(title), html.EscapeString(message)),
		Headers:    h.unavailableHeaders(),
	}
}

func (h Hardening) unavailableRoutes() []Route {
	page := h.UnavailableResponse("Temporarily unavailable", "This site is restarting or being updated. This page will retry on its own.")
	gateway := page
	gateway.StatusCode = 504
	return []Route{
		{
			Match:  []Matcher{{Expression: &ExpressionMatcher{Expr: "{http.error.status_code} in [502, 503]"}}},
			Handle: []any{page},
		},
		{
			Match:  []Matcher{{Expression: &ExpressionMatcher{Expr: "{http.error.status_code} == 504"}}},
			Handle: []any{gateway},
		},
	}
}

// HoldRoute is a domain whose backend is momentarily absent (mid-deploy or
// restarting): it answers the friendly 503 and keeps the host in the TLS
// automation set so the handshake still succeeds.
type HoldRoute struct {
	Hosts []string
}

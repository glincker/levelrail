package api

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	gateTokenParam     = "_gate_token"
	gateCookieSession  = "gate_session"
	gateCookieState    = "gate_state"
	gateStateMaxAge    = 10 * time.Minute
	maxGateURILen      = 8192
	maxGateRedirectLen = 4096
	maxGateHostLen     = 253
)

var errGateBadRedirect = errors.New("the return address is not a gated app")

// gateRequest is what the ingress tells the check endpoint about the
// original request. Only trusted after gateRequestAuthentic.
type gateRequest struct {
	host      string
	authority string
	https     bool
	uri       string
	token     string
	ip        string
	xhr       bool
}

// gateRequestAuthentic proves the request came from this process's ingress:
// the secret is only ever set on the gate subrequest, never sent to clients.
func (rt *Router) gateRequestAuthentic(r *http.Request) bool {
	if rt.gateSecret == "" || rt.gate == nil {
		return false
	}
	got := r.Header.Get(ingress.GateHeaderSecret)
	return subtle.ConstantTimeCompare([]byte(got), []byte(rt.gateSecret)) == 1
}

func validGateHostname(h string) bool {
	if h == "" || len(h) > maxGateHostLen {
		return false
	}
	for i := 0; i < len(h); i++ {
		c := h[i]
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '.' && c != '-' {
			return false
		}
	}
	return true
}

func validGatePort(p string) bool {
	if p == "" {
		return true
	}
	n, err := strconv.Atoi(p)
	return err == nil && n >= 1 && n <= 65535
}

// splitGateAuthority validates a host[:port] and returns the hostname.
func splitGateAuthority(authority string) (string, bool) {
	host, port := authority, ""
	if i := strings.LastIndexByte(authority, ':'); i >= 0 {
		host, port = authority[:i], authority[i+1:]
		if port == "" {
			return "", false
		}
	}
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	return host, validGateHostname(host) && validGatePort(port)
}

// stripGateToken removes every token param from a raw query, keeping the
// rest byte for byte, and returns the first token value seen.
func stripGateToken(rawQuery string) (rest, token string) {
	if rawQuery == "" {
		return "", ""
	}
	var kept []string
	for _, part := range strings.Split(rawQuery, "&") {
		key, val, _ := strings.Cut(part, "=")
		if k, err := url.QueryUnescape(key); err == nil && k == gateTokenParam {
			if token == "" {
				token, _ = url.QueryUnescape(val)
			}
			continue
		}
		kept = append(kept, part)
	}
	return strings.Join(kept, "&"), token
}

// cleanGatePath forces a single leading slash so a Location built from it
// can never be read as a scheme-relative URL to another host.
func cleanGatePath(p string) string {
	return "/" + strings.TrimLeft(p, "/")
}

func parseGateRequest(r *http.Request) (gateRequest, error) {
	var req gateRequest
	req.authority = strings.TrimSpace(r.Header.Get(ingress.GateHeaderHost))
	host, ok := splitGateAuthority(req.authority)
	if !ok {
		return req, errors.New("invalid gate host")
	}
	req.host = host
	switch strings.ToLower(r.Header.Get(ingress.GateHeaderProto)) {
	case "https":
		req.https = true
	case "http":
	default:
		return req, errors.New("invalid gate proto")
	}
	raw := r.Header.Get("X-Forwarded-Uri")
	if raw == "" || len(raw) > maxGateURILen || !strings.HasPrefix(raw, "/") {
		return req, errors.New("invalid gate uri")
	}
	u, err := url.ParseRequestURI(raw)
	if err != nil {
		return req, fmt.Errorf("invalid gate uri: %w", err)
	}
	rest, token := stripGateToken(u.RawQuery)
	req.uri, req.token = cleanGatePath(u.EscapedPath()), token
	if rest != "" {
		req.uri += "?" + rest
	}
	if ip := net.ParseIP(r.Header.Get(ingress.GateHeaderClientIP)); ip != nil {
		req.ip = ip.String()
	}
	req.xhr = strings.Contains(r.Header.Get("Accept"), "application/json") || r.Header.Get("X-Requested-With") != ""
	return req, nil
}

func gateCookieName(base string, secure bool) string {
	if secure {
		return "__Host-" + base
	}
	return base
}

// setGateCookie writes a host-only cookie: no Domain attribute, so a gate
// session never leaks to sibling subdomains.
func setGateCookie(w http.ResponseWriter, base, value string, secure bool, maxAge time.Duration) {
	c := &http.Cookie{ //nolint:gosec // Secure follows the app's scheme; plain http only with APP_GATE_ALLOW_HTTP
		Name: gateCookieName(base, secure), Value: value, Path: "/",
		HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode,
	}
	if value == "" {
		c.MaxAge = -1
	} else {
		c.MaxAge = int(maxAge.Seconds())
	}
	http.SetCookie(w, c)
}

// validateGateRedirect accepts only an absolute URL on a host with an
// enabled built-in gate and returns it rebuilt from its parts, without a
// fragment, userinfo or token.
func (rt *Router) validateGateRedirect(ctx context.Context, raw string) (string, string, error) {
	if raw == "" || len(raw) > maxGateRedirectLen {
		return "", "", errGateBadRedirect
	}
	for _, c := range raw {
		if c <= ' ' || c == 0x7f || c == '\\' {
			return "", "", errGateBadRedirect
		}
	}
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.Opaque != "" || u.Host == "" {
		return "", "", errGateBadRedirect
	}
	if u.Scheme != "https" && (u.Scheme != "http" || !rt.effectiveGateSettings().AllowHTTP) {
		return "", "", errGateBadRedirect
	}
	host, ok := splitGateAuthority(u.Host)
	if !ok {
		return "", "", errGateBadRedirect
	}
	g, found, err := rt.gate.GetDomainAuthGate(ctx, host)
	if err != nil {
		return "", "", fmt.Errorf("api: gate redirect lookup: %w", err)
	}
	if !found || !g.Enabled || g.Provider != store.GateProviderBuiltin {
		return "", "", errGateBadRedirect
	}
	authority := host
	if p := u.Port(); p != "" {
		authority = net.JoinHostPort(host, p)
	}
	target := u.Scheme + "://" + authority + cleanGatePath(u.EscapedPath())
	if rest, _ := stripGateToken(u.RawQuery); rest != "" {
		target += "?" + rest
	}
	return target, host, nil
}

var gatePageTemplate = template.Must(template.New("gate").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="robots" content="noindex"><title>{{.Title}} | {{.Brand}}</title>
<style>body{font-family:system-ui,sans-serif;max-width:32rem;margin:15vh auto;padding:0 1rem;color:#1f2328;background:#fff}
@media (prefers-color-scheme:dark){body{color:#e6edf3;background:#0d1117}a{color:#58a6ff}}</style></head>
<body><h1>{{.Title}}</h1><p>{{.Message}}</p>{{if .Link}}<p><a href="{{.Link}}">{{.LinkLabel}}</a></p>{{end}}
<p><small>{{.Brand}}</small></p></body></html>`))

type gatePage struct {
	Brand, Title, Message, Link, LinkLabel string
}

func (rt *Router) writeGatePage(w http.ResponseWriter, status int, p gatePage) {
	p.Brand = rt.brand.Name
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := gatePageTemplate.Execute(w, p); err != nil {
		rt.logger.Warn("api: render gate page failed", slog.String("error", err.Error()))
	}
}

func (rt *Router) recordGateEvent(ctx context.Context, e store.GateEvent) {
	if err := rt.gate.RecordAuthGateEvent(ctx, e); err != nil {
		rt.logger.Warn("api: record gate event failed", slog.String("domain", e.Domain), slog.String("error", err.Error()))
	}
}

// gateEventCounter keeps hot check-path events off the database: the
// first of a kind per minute is written at once, repeats are counted here
// and flushed by the sweeper. Anonymous floods cost one write per minute.
type gateEventCounter struct {
	mu      sync.Mutex
	pending map[gateEventKey]*store.GateEvent
}

type gateEventKey struct{ domain, user, decision, reason, ip, bucket string }

func (rt *Router) recordGateCoalesced(ctx context.Context, e store.GateEvent) {
	e.At = time.Now()
	key := gateEventKey{e.Domain, e.UserID, e.Decision, e.Reason, e.IP, store.GateEventBucket(e.At)}
	c := &rt.gateEvents
	c.mu.Lock()
	if c.pending == nil {
		c.pending = map[gateEventKey]*store.GateEvent{}
	}
	if p, ok := c.pending[key]; ok {
		p.Count++
		p.At = e.At
		c.mu.Unlock()
		return
	}
	pending := e
	pending.Count = 0
	c.pending[key] = &pending
	c.mu.Unlock()
	e.Count = 1
	rt.recordGateEvent(ctx, e)
}

// flushGateEvents writes the repeats counted since the last flush.
func (rt *Router) flushGateEvents(ctx context.Context) {
	c := &rt.gateEvents
	c.mu.Lock()
	pending := c.pending
	c.pending = nil
	c.mu.Unlock()
	for _, e := range pending {
		if e.Count > 0 {
			rt.recordGateEvent(ctx, *e)
		}
	}
}

package ingress

import (
	"net/http"
	"net/netip"
	"os"
	"strconv"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"

	"github.com/GLINCKER/levelrail/internal/abuse"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

func init() {
	caddy.RegisterModule(abuseModule{})
}

const headerForwardedFor = "X-Forwarded-For"

// AbusePolicy is one app's ingress abuse defense, applied to each of the
// app's proxied routes.
type AbusePolicy struct {
	App    string
	Policy abuse.Policy
}

// AbuseHandler is the wire shape of the levelrail_abuse handler.
type AbuseHandler struct {
	Handler string       `json:"handler"`
	App     string       `json:"app"`
	Policy  abuse.Policy `json:"policy"`
}

func newAbuseHandler(a AbusePolicy) AbuseHandler {
	return AbuseHandler{Handler: "levelrail_abuse", App: a.App, Policy: a.Policy}
}

// wrap puts the abuse handler first, ahead of every other control, so a
// denied client costs the least. A nil or inactive policy adds nothing.
func (a *AbusePolicy) wrap(handle []any) []any {
	if a == nil || !a.Policy.Active() {
		return handle
	}
	return append([]any{newAbuseHandler(*a)}, handle...)
}

// abuseModule (http.handlers.levelrail_abuse) applies one app's abuse policy.
// Any failure to evaluate it, including a panic, serves the request and
// counts a fail-open: the defense must never take an app down.
type abuseModule struct {
	App    string       `json:"app"`
	Policy abuse.Policy `json:"policy"`

	state   *abuse.State
	trusted []netip.Prefix
	decide  func(netip.Addr, string) abuse.Decision
}

func (abuseModule) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.levelrail_abuse",
		New: func() caddy.Module { return new(abuseModule) },
	}
}

// Provision compiles the policy. A policy that does not compile is not an
// error: the route keeps serving and every request is counted as fail-open.
func (m *abuseModule) Provision(caddy.Context) error {
	limits := abuse.LimitsFromEnv(os.Getenv)
	m.state = abuse.DefaultRegistry().For(m.App, limits)
	m.trusted, _ = abuse.ParseTrusted(os.Getenv(EnvTrustedProxies))
	if eng, err := abuse.NewEngine(m.Policy, m.state, limits); err == nil {
		m.decide = eng.Decide
	}
	return nil
}

func (m *abuseModule) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	d := m.judge(r)
	if d.Action == abuse.ActionBlock {
		writeAbuseReject(w, d)
		return nil
	}
	return next.ServeHTTP(w, r)
}

// judge never lets a failure escape: a panic or a missing engine allows the
// request and is counted.
func (m *abuseModule) judge(r *http.Request) (d abuse.Decision) {
	defer func() {
		if rec := recover(); rec != nil {
			m.state.Counters.NoteFailOpen()
			d = abuse.Decision{}
		}
	}()
	if m.decide == nil {
		m.state.Counters.NoteFailOpen()
		return d
	}
	return m.decide(m.clientAddr(r), r.UserAgent())
}

// clientAddr uses Caddy's client_ip var, which already honours only the
// configured trusted proxies. Without it the peer address stands, and a
// forwarded header counts only when the peer is a trusted proxy.
func (m *abuseModule) clientAddr(r *http.Request) netip.Addr {
	if s, ok := caddyhttp.GetVar(r.Context(), caddyhttp.ClientIPVarKey).(string); ok && s != "" {
		if a, err := netip.ParseAddr(s); err == nil {
			return a.Unmap()
		}
	}
	return abuse.ResolveClientIP(r.RemoteAddr, r.Header.Values(headerForwardedFor), m.trusted)
}

func writeAbuseReject(w http.ResponseWriter, d abuse.Decision) {
	h := w.Header()
	h.Set("Cache-Control", "no-store")
	h.Set("Content-Type", "text/plain; charset=utf-8")
	if d.Reason == abuse.ReasonRateLimit {
		h.Set("Retry-After", strconv.Itoa(max(d.RetryAfter, 1)))
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte("Too many requests. Slow down and retry later.\n"))
		return
	}
	w.WriteHeader(http.StatusForbidden)
	_, _ = w.Write([]byte("Access denied.\n"))
}

func abuseWindow(s abuse.Snapshot) telemetry.AbuseWindow {
	w := telemetry.AbuseWindow{Allowed: s.Allowed, FailOpen: s.FailOpen}
	for i := range s.WouldBlock {
		w.WouldBlock[i], w.Blocked[i] = s.WouldBlock[i], s.Blocked[i]
	}
	return w
}

var (
	_ caddyhttp.MiddlewareHandler = (*abuseModule)(nil)
	_ caddy.Provisioner           = (*abuseModule)(nil)
)

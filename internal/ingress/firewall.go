package ingress

import (
	"net/http"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"

	"github.com/GLINCKER/levelrail/internal/edgefw"
	"github.com/GLINCKER/levelrail/internal/trafficpolicy"
)

func init() {
	caddy.RegisterModule(firewallModule{})
}

// shieldOutcomeVar carries a per-domain shield rejection (reason + 1) back
// to the firewall handler so each request is classified exactly once.
const shieldOutcomeVar = "levelrail_fw_shield"

const firewallLimiterKey = "__firewall__"

// FirewallHandler is the wire shape of the levelrail_firewall handler, which
// sits in front of every host route and runs the instance firewall.
type FirewallHandler struct {
	Handler string `json:"handler"`
}

// NewFirewallHandler builds the server-wide firewall handler.
func NewFirewallHandler() FirewallHandler { return FirewallHandler{Handler: "levelrail_firewall"} }

// firewallModule (http.handlers.levelrail_firewall) evaluates rules, bans,
// attack mode and bot modes for every request.
type firewallModule struct {
	limits  trafficpolicy.Limits
	limiter *trafficpolicy.IPLimiter
}

func (firewallModule) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.levelrail_firewall",
		New: func() caddy.Module { return new(firewallModule) },
	}
}

// Provision attaches the shared attack-mode limiter, which survives reloads.
func (m *firewallModule) Provision(caddy.Context) error {
	m.limits = trafficpolicy.LimitsFromEnv(os.Getenv)
	m.limiter = shieldLimiterFor(firewallLimiterKey, m.limits.ShieldMaxTrackedIPs)
	return nil
}

func (m *firewallModule) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	now := time.Now()
	client := requestClientAddr(r)
	eng := edgefw.Default()
	if r.Method == http.MethodPost && r.URL.Path == edgefw.ChallengePath {
		serveChallengeSolve(w, r, eng, client, now)
		return nil
	}
	fr := firewallRequest(r, client, eng.NeedsCountry())
	v := eng.Evaluate(&fr, now)
	switch v.Action {
	case edgefw.ActionAllow:
		eng.Record(&fr, edgefw.OutcomeAllowed, edgefw.ReasonNone, now)
		return next.ServeHTTP(w, r)
	case edgefw.ActionDeny:
		eng.Record(&fr, v.Outcome, v.Reason, now)
		writeFirewallDeny(w, v)
		return nil
	case edgefw.ActionChallenge:
		eng.Record(&fr, v.Outcome, v.Reason, now)
		serveChallengePage(w, eng, &fr, now)
		return nil
	}
	if st := eng.CurrentSettings(); st.AttackActive(now) && client.IsValid() {
		eff := trafficpolicy.Shield{UnderAttackUntil: st.AttackUntil}.Effective(now, m.limits)
		release, _, ok := m.limiter.Acquire(client, eff.RatePerMinute, eff.Burst, eff.MaxConcurrent)
		if !ok {
			eng.Record(&fr, edgefw.OutcomeRateLimited, edgefw.ReasonAttack, now)
			writeFirewallDeny(w, edgefw.Verdict{Status: http.StatusTooManyRequests, RetryAfter: eff.RetryAfter})
			return nil
		}
		defer release()
	}
	err := next.ServeHTTP(w, r)
	out, reason := edgefw.OutcomeAllowed, edgefw.ReasonNone
	if v.Logged {
		out = edgefw.OutcomeLogged
	}
	if n, ok := caddyhttp.GetVar(r.Context(), shieldOutcomeVar).(int); ok && n > 0 {
		out, reason = shieldClassification(trafficpolicy.ShieldReason(n - 1))
	}
	eng.Record(&fr, out, reason, now)
	return err
}

func shieldClassification(why trafficpolicy.ShieldReason) (edgefw.Outcome, edgefw.Reason) {
	switch why {
	case trafficpolicy.ShieldRateLimit:
		return edgefw.OutcomeRateLimited, edgefw.ReasonShieldRate
	case trafficpolicy.ShieldConcurrency:
		return edgefw.OutcomeRateLimited, edgefw.ReasonShieldConcurrency
	case trafficpolicy.ShieldBodyTooLarge:
		return edgefw.OutcomeDenied, edgefw.ReasonShieldBody
	}
	return edgefw.OutcomeDenied, edgefw.ReasonShieldBot
}

var (
	_ caddyhttp.MiddlewareHandler = (*firewallModule)(nil)
	_ caddy.Provisioner           = (*firewallModule)(nil)
)

func firewallRequest(r *http.Request, client netip.Addr, wantCountry bool) edgefw.Request {
	host := r.Host
	if i := strings.LastIndexByte(host, ':'); i > 0 && strings.IndexByte(host[i:], ']') < 0 {
		host = host[:i]
	}
	fr := edgefw.Request{
		Method: r.Method, Host: strings.ToLower(host), Path: r.URL.Path, RawQuery: r.URL.RawQuery,
		UserAgent: r.UserAgent(), Header: r.Header, IP: client,
		PassCookie: cookieValue(r.Header.Get("Cookie"), edgefw.PassCookie),
	}
	if wantCountry {
		res := DefaultGeoResolver()
		trusted, _ := caddyhttp.GetVar(r.Context(), caddyhttp.TrustedProxyVarKey).(bool)
		fr.Country, _ = res.Country(client, r.Header.Get(res.HeaderName()), trusted)
	}
	return fr
}

// cookieValue finds name in a Cookie header without allocating.
func cookieValue(header, name string) string {
	for header != "" {
		var part string
		part, header, _ = strings.Cut(header, ";")
		part = strings.TrimLeft(part, " ")
		if k, v, ok := strings.Cut(part, "="); ok && k == name {
			return v
		}
	}
	return ""
}

func writeFirewallDeny(w http.ResponseWriter, v edgefw.Verdict) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if v.Status == http.StatusTooManyRequests {
		w.Header().Set("Retry-After", strconv.Itoa(max(v.RetryAfter, 1)))
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte("Too many requests. Slow down and retry later.\n"))
		return
	}
	w.WriteHeader(http.StatusForbidden)
	_, _ = w.Write([]byte("Access denied.\n"))
}

func serveChallengePage(w http.ResponseWriter, eng *edgefw.Engine, fr *edgefw.Request, now time.Time) {
	page, csp := edgefw.ChallengePage(eng.Title(), eng.Issue(fr, now), eng.Limits().ChallengeBits)
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	h.Set("Content-Security-Policy", csp)
	h.Set("X-Robots-Tag", "noindex")
	h.Set("Referrer-Policy", "no-referrer")
	w.WriteHeader(http.StatusForbidden)
	_, _ = w.Write(page)
}

const challengeFormMax = 2048

func serveChallengeSolve(w http.ResponseWriter, r *http.Request, eng *edgefw.Engine, client netip.Addr, now time.Time) {
	r.Body = http.MaxBytesReader(w, r.Body, challengeFormMax)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	cookie, ok := eng.Solve(client, r.PostFormValue("t"), r.PostFormValue("n"), now)
	w.Header().Set("Cache-Control", "no-store")
	if !ok {
		http.Error(w, "challenge failed", http.StatusBadRequest)
		return
	}
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // Secure follows the request scheme, the proxy may terminate TLS
		Name: edgefw.PassCookie, Value: cookie, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode,
		Secure: r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https", MaxAge: int(eng.Limits().PassTTL.Seconds()),
	})
	w.WriteHeader(http.StatusNoContent)
}

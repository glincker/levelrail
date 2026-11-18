package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	demoBodyLimit        = 4 << 10
	demoSessionPerMinute = 20
	demoLimiterMaxIPs    = 10000
)

// HumanVerifier is the pluggable human check in front of the passwordless
// demo session. Verify returns nil when the visitor passed.
type HumanVerifier interface {
	Verify(ctx context.Context, token, remoteIP string) error
}

// DemoConfig turns the Router into the read-only public demo. It is only ever
// built by the demo entrypoint, never from the ordinary process wiring.
type DemoConfig struct {
	// VisitorUserID is the seeded read-only account sessions are minted for.
	VisitorUserID string
	Username      string
	Password      string
	// Verifier is optional; nil means no human check is enforced.
	Verifier HumanVerifier
	// SessionsPerMinute caps session mints per client IP; zero uses the default.
	SessionsPerMinute int
	ResetsAt          func() time.Time
	InstallCommand    string
	RepoURL           string
	DocsURL           string
	Version           string
}

// WithDemoMode puts the Router in read-only demo mode: default-deny write gate,
// read elevation for the visitor account, noindex headers and the demo routes.
func WithDemoMode(cfg DemoConfig) Option {
	return func(rt *Router) {
		if cfg.SessionsPerMinute <= 0 {
			cfg.SessionsPerMinute = demoSessionPerMinute
		}
		rt.demo = &demoState{cfg: cfg, ctaCounts: map[string]int{}, mints: map[string][]time.Time{}, analytics: demoAnalyticsFromEnv(os.Getenv), gtm: demoGTMFromEnv(os.Getenv), stars: demoStarsFromProcessEnv()}
	}
}

type demoState struct {
	cfg DemoConfig
	// analytics is nil unless the operator opted in with both env vars.
	analytics *demoAnalytics
	// gtm is nil unless APP_DEMO_GTM_ID holds a valid container id.
	gtm *demoGTMInfo
	// stars is nil when the star count fetch is disabled.
	stars *demoStars

	mu        sync.Mutex
	ctaCounts map[string]int
	mints     map[string][]time.Time
	sessions  int
}

// DemoStats is the aggregate, PII-free counter snapshot the demo host flushes.
type DemoStats struct {
	SessionsMinted int            `json:"sessions_minted"`
	CTAClicks      map[string]int `json:"cta_clicks"`
}

// DemoStats returns a copy of the demo counters, or the zero value outside demo mode.
func (rt *Router) DemoStats() DemoStats {
	if rt.demo == nil {
		return DemoStats{}
	}
	rt.demo.mu.Lock()
	defer rt.demo.mu.Unlock()
	out := DemoStats{SessionsMinted: rt.demo.sessions, CTAClicks: make(map[string]int, len(rt.demo.ctaCounts))}
	for k, v := range rt.demo.ctaCounts {
		out.CTAClicks[k] = v
	}
	return out
}

type demoInfoResponse struct {
	Enabled        bool               `json:"enabled"`
	ResetsAt       *time.Time         `json:"resetsAt,omitempty"`
	InstallCommand string             `json:"installCommand,omitempty"`
	RepoURL        string             `json:"repoUrl,omitempty"`
	DocsURL        string             `json:"docsUrl,omitempty"`
	Version        string             `json:"version,omitempty"`
	Username       string             `json:"username,omitempty"`
	Password       string             `json:"password,omitempty"`
	HumanCheck     bool               `json:"humanCheck"`
	Analytics      *demoAnalyticsInfo `json:"analytics,omitempty"`
	GTM            *demoGTMInfo       `json:"gtm,omitempty"`
	Stars          *int               `json:"stars,omitempty"`
}

type demoAnalyticsInfo struct {
	Provider string `json:"provider"`
	Host     string `json:"host"`
	Token    string `json:"token"`
}

// demoAnalytics is the opt-in, hosted-demo-only analytics target.
type demoAnalytics struct {
	info demoAnalyticsInfo
	// allowed is the space-separated origin list added to script-src and connect-src.
	allowed string
}

// demoAnalyticsFromEnv returns nil unless APP_DEMO_ANALYTICS_HOST (an https
// origin) and APP_DEMO_ANALYTICS_TOKEN are both set, so a real instance never sends anything.
func demoAnalyticsFromEnv(getenv func(string) string) *demoAnalytics {
	raw := strings.TrimSpace(getenv("APP_DEMO_ANALYTICS_HOST"))
	token := strings.TrimSpace(getenv("APP_DEMO_ANALYTICS_TOKEN"))
	if raw == "" || token == "" || strings.ContainsAny(token, " \t\r\n\"'<>;") {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" {
		return nil
	}
	origin := "https://" + u.Host
	allowed := origin
	// PostHog cloud serves its script from a sibling assets host.
	if h := u.Hostname(); strings.HasSuffix(h, ".i.posthog.com") && !strings.Contains(h, "-assets.") {
		allowed += " https://" + strings.Replace(h, ".i.posthog.com", "-assets.i.posthog.com", 1)
	}
	return &demoAnalytics{info: demoAnalyticsInfo{Provider: "posthog", Host: origin, Token: token}, allowed: allowed}
}

type demoGTMInfo struct {
	ID string `json:"id"`
}

var demoGTMIDPattern = regexp.MustCompile(`^GTM-[A-Z0-9]{4,12}$`)

// demoGTMFromEnv returns nil unless APP_DEMO_GTM_ID is a well-formed container id;
// a malformed value is ignored so it can never reach a script URL or the CSP.
func demoGTMFromEnv(getenv func(string) string) *demoGTMInfo {
	id := strings.TrimSpace(getenv("APP_DEMO_GTM_ID"))
	if id == "" {
		return nil
	}
	if !demoGTMIDPattern.MatchString(id) {
		slog.Warn("ignoring APP_DEMO_GTM_ID: not a valid GTM container id")
		return nil
	}
	return &demoGTMInfo{ID: id}
}

// demoGTMCSP adds the Google Tag Manager and GA4 origins, never unsafe-inline or unsafe-eval.
func demoGTMCSP(csp string) string {
	const gtm = "https://www.googletagmanager.com"
	const beacons = " https://*.google-analytics.com https://*.analytics.google.com https://*.googletagmanager.com"
	return widenCSP(csp, gtm, gtm+beacons, gtm+beacons)
}

// demoAnalyticsCSP widens the CSP to the analytics hosts in demo mode only.
func (rt *Router) demoAnalyticsCSP(next http.Handler) http.Handler {
	if rt.demo == nil {
		return next
	}
	csp := rt.DemoCSP(contentSecurityPolicy)
	if csp == contentSecurityPolicy {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Runs inside securityHeadersMiddleware, so this overrides its default.
		w.Header().Set("Content-Security-Policy", csp)
		next.ServeHTTP(w, r)
	})
}

// DemoCSP returns base widened with the demo analytics origins, or base
// unchanged outside demo mode. The embedded dashboard handler sits outside the
// API middleware chain and sets its own policy, so the demo entrypoint feeds
// it this value too.
func (rt *Router) DemoCSP(base string) string {
	if rt.demo == nil {
		return base
	}
	csp := base
	if a := rt.demo.analytics; a != nil {
		csp = widenCSP(csp, a.allowed, a.allowed, "")
	}
	if rt.demo.gtm != nil {
		csp = demoGTMCSP(csp)
	}
	return csp
}

// widenCSP appends origins to script-src, connect-src and img-src when each
// directive is present in the base policy.
func widenCSP(csp, script, connect, img string) string {
	add := func(csp, directive, origins string) string {
		if origins == "" {
			return csp
		}
		return strings.Replace(csp, directive+" 'self'", directive+" 'self' "+origins, 1)
	}
	csp = add(csp, "script-src", script)
	csp = add(csp, "connect-src", connect)
	return add(csp, "img-src", img)
}

// handleDemoInfo serves GET /api/v1/demo: public, and {"enabled":false} on a real instance.
func (rt *Router) handleDemoInfo(w http.ResponseWriter, _ *http.Request) {
	if rt.demo == nil {
		writeJSON(w, http.StatusOK, demoInfoResponse{})
		return
	}
	c := rt.demo.cfg
	resp := demoInfoResponse{
		Enabled: true, InstallCommand: c.InstallCommand, RepoURL: c.RepoURL, DocsURL: c.DocsURL,
		Version: c.Version, Username: c.Username, Password: c.Password, HumanCheck: c.Verifier != nil,
	}
	if a := rt.demo.analytics; a != nil {
		info := a.info
		resp.Analytics = &info
	}
	if g := rt.demo.gtm; g != nil {
		info := *g
		resp.GTM = &info
	}
	if n, ok := rt.demo.stars.Get(); ok {
		resp.Stars = &n
	}
	if c.ResetsAt != nil {
		t := c.ResetsAt()
		resp.ResetsAt = &t
	}
	writeJSON(w, http.StatusOK, resp)
}

type demoSessionRequest struct {
	HumanToken string `json:"humanToken"`
}

// handleDemoSession serves POST /api/v1/demo/session: a passwordless sign-in
// as the read-only visitor, behind the human check and a per-IP limit.
func (rt *Router) handleDemoSession(w http.ResponseWriter, r *http.Request) {
	if rt.demo == nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if rt.refuseInsecureLogin(w, r) {
		return
	}
	ip := demoClientIP(r)
	if !rt.demo.allowMint(ip, time.Now()) {
		writeRateLimited(w, time.Minute)
		return
	}
	var req demoSessionRequest
	if r.ContentLength != 0 {
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, demoBodyLimit)).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}
	if v := rt.demo.cfg.Verifier; v != nil {
		if err := v.Verify(r.Context(), req.HumanToken, ip); err != nil {
			writeDemoError(w, http.StatusForbidden, "demo_human_check_failed", "Human verification failed. Reload and try again.")
			return
		}
	}
	user, err := rt.auth.GetUserByID(r.Context(), rt.demo.cfg.VisitorUserID)
	if err != nil || user == nil {
		rt.internalError(w, "api: demo session: visitor account unavailable", errors.New("visitor user not found"))
		return
	}
	if err := rt.establishSession(w, r, *user); err != nil {
		rt.internalError(w, "api: demo session: establish failed", err)
		return
	}
	rt.demo.mu.Lock()
	rt.demo.sessions++
	rt.demo.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

var demoCTAKinds = map[string]bool{"install": true, "github": true, "docs": true, "banner_dismiss": true}

// handleDemoCTA serves POST /api/v1/demo/cta: an enum-only click beacon, counted in memory.
func (rt *Router) handleDemoCTA(w http.ResponseWriter, r *http.Request) {
	if rt.demo == nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	var req struct {
		Kind string `json:"kind"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256)).Decode(&req); err != nil || !demoCTAKinds[req.Kind] {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	rt.demo.mu.Lock()
	rt.demo.ctaCounts[req.Kind]++
	rt.demo.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (d *demoState) allowMint(ip string, now time.Time) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.mints) > demoLimiterMaxIPs {
		d.mints = map[string][]time.Time{}
	}
	cutoff := now.Add(-time.Minute)
	recent := d.mints[ip][:0]
	for _, t := range d.mints[ip] {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}
	if len(recent) >= d.cfg.SessionsPerMinute {
		d.mints[ip] = recent
		return false
	}
	d.mints[ip] = append(recent, now)
	return true
}

// demoClientIP trusts CF-Connecting-IP only from a loopback peer (the tunnel).
func demoClientIP(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
			if h := r.Header.Get("CF-Connecting-IP"); h != "" {
				return h
			}
		}
	}
	return clientIP(r)
}

type demoErrorBody struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

func writeDemoError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, demoErrorBody{Error: code, Message: msg})
}

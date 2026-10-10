package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/store"
)

// ACMEStagingDirectoryURL is Let's Encrypt's staging CA: no rate limits that
// matter, certificates browsers do not trust.
const ACMEStagingDirectoryURL = "https://acme-staging-v02.api.letsencrypt.org/directory"

// HTTPS states reported by GET /api/v1/settings/ingress/https.
const (
	httpsStateOff     = "off"
	httpsStatePending = "pending"
	httpsStateIssued  = "issued"
	httpsStateFailed  = "failed"
)

// Failure hint codes; the UI maps each to localized copy.
const (
	httpsHintRateLimited = "rate_limited"
	httpsHintUnreachable = "unreachable"
	httpsHintOtherProxy  = "other_proxy"
	httpsHintDNS         = "dns"
	httpsHintCAA         = "caa"
	httpsHintUnknown     = "unknown"
)

const (
	defaultHTTPSMaxAttemptsPerHour = 5
	defaultHTTPSPendingTimeout     = 2 * time.Minute
)

// httpsStatusResource is GET /api/v1/settings/ingress/https's wire shape.
type httpsStatusResource struct {
	State            string     `json:"state"`
	Domain           string     `json:"domain,omitempty"`
	PublicHost       string     `json:"public_host,omitempty"`
	PublicHostSource string     `json:"public_host_source,omitempty"`
	SuggestedDomain  string     `json:"suggested_domain,omitempty"`
	Staging          bool       `json:"staging"`
	Issuer           string     `json:"issuer,omitempty"`
	NotAfter         *time.Time `json:"not_after,omitempty"`
	Error            string     `json:"error,omitempty"`
	Hint             string     `json:"hint,omitempty"`
	DashboardURL     string     `json:"dashboard_url,omitempty"`
	// Preflight is set on the enable response only: a non-blocking dial of
	// ports 80 and 443 on the public IP, so a closed firewall shows up now
	// instead of as a failed issuance minutes later.
	Preflight []httpsPreflightPort `json:"preflight,omitempty"`
}

// httpsPreflightPort is one port's self-dial result; false is only a hint,
// since hairpin NAT can fail this dial while the port is open to the internet.
type httpsPreflightPort struct {
	Port      int  `json:"port"`
	Reachable bool `json:"reachable"`
}

const httpsPreflightTimeout = 3 * time.Second

func (rt *Router) httpsPreflight(ctx context.Context, host string) []httpsPreflightPort {
	if net.ParseIP(host) == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, httpsPreflightTimeout)
	defer cancel()
	ports := []int{80, 443}
	out := make([]httpsPreflightPort, len(ports))
	var wg sync.WaitGroup
	for i, port := range ports {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out[i] = httpsPreflightPort{Port: port}
			conn, err := rt.doctorDialContextOrDefault()(ctx, "tcp", net.JoinHostPort(host, strconv.Itoa(port)))
			if err == nil {
				_ = conn.Close()
				out[i].Reachable = true
			}
		}()
	}
	wg.Wait()
	return out
}

type enableHTTPSRequest struct {
	Email   string `json:"email"`
	Staging bool   `json:"staging"`
}

// WithHTTPSEnableLimits sets how many production enable attempts are allowed
// per hour (a guard against burning Let's Encrypt's failed-validation limit)
// and how long a certificate may stay pending before it is reported failed.
// Zero values keep the defaults.
func WithHTTPSEnableLimits(maxPerHour int, pendingTimeout time.Duration) Option {
	return func(rt *Router) {
		rt.httpsMaxAttemptsPerHour = maxPerHour
		rt.httpsPendingTimeout = pendingTimeout
	}
}

// WithACMEDefaultDirectory sets the CA directory enable-https uses when the
// caller does not ask for staging. Empty means Let's Encrypt production.
func WithACMEDefaultDirectory(url string) Option {
	return func(rt *Router) { rt.acmeDefaultDirectory = url }
}

func (rt *Router) httpsMaxAttempts() int {
	if rt.httpsMaxAttemptsPerHour > 0 {
		return rt.httpsMaxAttemptsPerHour
	}
	return defaultHTTPSMaxAttemptsPerHour
}

func (rt *Router) httpsPendingAfter() time.Duration {
	if rt.httpsPendingTimeout > 0 {
		return rt.httpsPendingTimeout
	}
	return defaultHTTPSPendingTimeout
}

// allowHTTPSAttempt records an attempt and reports false once maxPerHour
// attempts already happened in the last hour.
func (rt *Router) allowHTTPSAttempt(now time.Time, production bool) bool {
	rt.httpsMu.Lock()
	defer rt.httpsMu.Unlock()
	cutoff := now.Add(-time.Hour)
	kept := rt.httpsAttempts[:0]
	for _, t := range rt.httpsAttempts {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	rt.httpsAttempts = kept
	if production && len(kept) >= rt.httpsMaxAttempts() {
		return false
	}
	if production {
		rt.httpsAttempts = append(rt.httpsAttempts, now)
	}
	rt.httpsStartedAt = now
	return true
}

// classifyACMEError maps a CA error message to a hint code.
func classifyACMEError(msg string) string {
	l := strings.ToLower(msg)
	switch {
	case strings.Contains(l, "ratelimited") || strings.Contains(l, "rate limit") || strings.Contains(l, "too many"):
		return httpsHintRateLimited
	case strings.Contains(l, "caa"):
		return httpsHintCAA
	case strings.Contains(l, "nxdomain") || strings.Contains(l, "no valid a records") || strings.Contains(l, "dns problem"):
		return httpsHintDNS
	case strings.Contains(l, "invalid response from http://") || strings.Contains(l, "acme:error:tls") || strings.Contains(l, "unrecognized name"):
		return httpsHintOtherProxy
	case strings.Contains(l, "timeout") || strings.Contains(l, "connection refused") || strings.Contains(l, "no route") || strings.Contains(l, "firewall") || strings.Contains(l, "unreachable"):
		return httpsHintUnreachable
	}
	return httpsHintUnknown
}

func isStagingDirectory(dir string) bool { return strings.Contains(strings.ToLower(dir), "staging") }

func isInternalIssuer(issuer string) bool {
	return strings.Contains(strings.ToLower(issuer), "caddy local authority")
}

// computeHTTPSStatus derives the state from settings, stored certificates and
// the last ACME failure Caddy reported. Pure so it can be table tested.
func computeHTTPSStatus(s store.IngressSettings, certs []alerting.CertInfo, failure *ingress.ACMEFailure, startedAt, now time.Time, pendingAfter time.Duration) httpsStatusResource {
	out := httpsStatusResource{State: httpsStateOff, Domain: s.PrimaryDomain, Staging: isStagingDirectory(s.ACMEDirectoryURL)}
	if s.PrimaryDomain == "" || !s.EffectiveACMEEnabled() {
		return out
	}
	for _, c := range certs {
		if !strings.EqualFold(c.Domain, s.PrimaryDomain) || isInternalIssuer(c.Issuer) {
			continue
		}
		// A leftover staging certificate must not read as success after
		// switching to the production CA.
		if !out.Staging && strings.Contains(strings.ToUpper(c.Issuer), "(STAGING)") {
			continue
		}
		na := c.NotAfter
		out.State, out.Issuer, out.NotAfter = httpsStateIssued, c.Issuer, &na
		return out
	}
	out.State = httpsStatePending
	if failure != nil {
		out.State, out.Error, out.Hint = httpsStateFailed, failure.Error, classifyACMEError(failure.Error)
		return out
	}
	if !startedAt.IsZero() && now.Sub(startedAt) > pendingAfter {
		out.State, out.Hint = httpsStateFailed, httpsHintUnreachable
		out.Error = "no certificate was issued and the certificate authority reported no error; port 80 is most likely not reachable from the internet"
	}
	return out
}

func (rt *Router) buildHTTPSStatus(r *http.Request) (httpsStatusResource, error) {
	ctx := r.Context()
	s, err := rt.ingressSettings.GetIngressSettings(ctx)
	if err != nil {
		return httpsStatusResource{}, err
	}
	window := rt.certExpiryWarningWindow
	if window <= 0 {
		window = alerting.DefaultCertExpiryWarningWindow
	}
	certs, err := alerting.ListCertificates(ctx, rt.certs, window, time.Now(), rt.logger)
	if err != nil {
		return httpsStatusResource{}, err
	}
	var failure *ingress.ACMEFailure
	if f, ok := ingress.DefaultACMEFailures().Get(strings.ToLower(s.PrimaryDomain)); ok {
		failure = &f
	}
	rt.httpsMu.Lock()
	started := rt.httpsStartedAt
	rt.httpsMu.Unlock()
	out := computeHTTPSStatus(s, certs, failure, started, time.Now(), rt.httpsPendingAfter())
	out.PublicHost, out.PublicHostSource = rt.publicHost, rt.publicHostSource
	if d, ok := ingress.SSLIPHost(rt.publicHost); ok {
		out.SuggestedDomain = d
	}
	if u, uerr := rt.ingressSettings.GetDashboardURL(ctx); uerr == nil {
		out.DashboardURL = u
	}
	return out, nil
}

// handleGetHTTPSStatus handles GET /api/v1/settings/ingress/https.
func (rt *Router) handleGetHTTPSStatus(w http.ResponseWriter, r *http.Request) {
	out, err := rt.buildHTTPSStatus(r)
	if err != nil {
		rt.internalError(w, "api: https status failed", err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// handleEnableHTTPS handles POST /api/v1/settings/ingress/https: points the
// platform primary domain at <dashed-ip>.sslip.io and turns on real ACME for
// it, so a fresh install gets a trusted https dashboard with no DNS setup.
func (rt *Router) handleEnableHTTPS(w http.ResponseWriter, r *http.Request) {
	var req enableHTTPSRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	domain, ok := ingress.SSLIPHost(rt.publicHost)
	if !ok {
		writeError(w, http.StatusConflict, "no public IP address is known for this server; set APP_PUBLIC_HOST to it, or configure your own domain instead")
		return
	}
	email := strings.TrimSpace(req.Email)
	if err := validateIngressSettingsRequest(ingressSettingsResource{ACMEEnabled: true, ACMEEmail: email}); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if owner, err := rt.primaryDomainOwner(r.Context(), domain); err != nil {
		rt.internalError(w, "api: enable https: check domain conflict", err)
		return
	} else if owner != "" {
		writeError(w, http.StatusConflict, (&store.ErrDomainTaken{Domain: domain, Owner: owner}).Error())
		return
	}
	current, err := rt.ingressSettings.GetIngressSettings(r.Context())
	if err != nil {
		rt.internalError(w, "api: enable https: load settings", err)
		return
	}
	if current.TLSTerminatedUpstream {
		writeError(w, http.StatusConflict, "HTTPS is handled by your proxy: turn off tls_terminated_upstream before enabling certificates here")
		return
	}
	directory := rt.acmeDefaultDirectory
	if req.Staging {
		directory = ACMEStagingDirectoryURL
	}
	if !rt.allowHTTPSAttempt(time.Now(), !isStagingDirectory(directory)) {
		writeError(w, http.StatusTooManyRequests, "too many certificate attempts in the last hour; each failed attempt counts against Let's Encrypt's own limits. Fix the cause (see the status hint), try staging, or wait before retrying")
		return
	}
	prev := current
	current.PrimaryDomain = domain
	current.ACMEEnabled = true
	current.ACMEEmail = email
	current.ACMEDirectoryURL = directory
	rt.purgeOnIssuerChange(r.Context(), prev, current)
	if err := rt.ingressSettings.UpdateIngressSettings(r.Context(), current); err != nil {
		rt.internalError(w, "api: enable https: save settings", err)
		return
	}
	ingress.DefaultACMEFailures().Clear(strings.ToLower(domain))
	if rt.reconcileNudger != nil {
		rt.reconcileNudger.Nudge()
	}
	rt.logger.Info("api: https enabled for the dashboard", slog.String("domain", domain), slog.Bool("staging", isStagingDirectory(directory)))
	out, err := rt.buildHTTPSStatus(r)
	if err != nil {
		rt.internalError(w, "api: enable https: status", err)
		return
	}
	out.Preflight = rt.httpsPreflight(r.Context(), rt.publicHost)
	writeJSON(w, http.StatusAccepted, out)
}

// purgeOnIssuerChange drops stored certificates from other issuers when ACME
// is newly enabled or its CA changes, so Caddy actually re-issues from the new
// CA instead of reusing a still-valid staging or internal certificate.
func (rt *Router) purgeOnIssuerChange(ctx context.Context, prev, next store.IngressSettings) {
	if rt.certs == nil || !next.EffectiveACMEEnabled() {
		return
	}
	if prev.EffectiveACMEEnabled() && prev.ACMEDirectoryURL == next.ACMEDirectoryURL {
		return
	}
	n, err := ingress.PurgeCertsFromOtherIssuers(ctx, rt.certs, next.ACMEDirectoryURL)
	if err != nil {
		rt.logger.Warn("api: purging certificates from the previous issuer failed", slog.String("error", err.Error()))
		return
	}
	if n > 0 {
		rt.logger.Info("api: dropped certificates from the previous issuer so they re-issue", slog.Int("keys", n))
	}
}

package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/certflow"
	"github.com/GLINCKER/levelrail/internal/domaindoctor"
	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Progress states of a guided certificate request.
const (
	certFlowOff     = "off"
	certFlowPending = "pending"
	certFlowIssued  = "issued"
	certFlowFailed  = "failed"
)

// acknowledgePorts lets the operator proceed when the port 80 self-check failed
// but they know the port is open (a router that cannot loop back to itself).
const acknowledgePorts = "ports"

// domainCertStatusResource is the progress and result of getting a real certificate.
type domainCertStatusResource struct {
	Domain  string `json:"domain"`
	State   string `json:"state"`
	Staging bool   `json:"staging"`
	// AppliesToAll is true: public certificates are a platform setting, so
	// turning them on covers every domain on this server.
	AppliesToAll bool          `json:"applies_to_all"`
	Issuer       string        `json:"issuer,omitempty"`
	Kind         certflow.Kind `json:"kind"`
	NotAfter     *time.Time    `json:"not_after,omitempty"`
	DaysLeft     *int          `json:"days_left,omitempty"`
	// RenewalAt is when automatic renewal starts (a third of the lifetime before expiry).
	RenewalAt   *time.Time        `json:"renewal_at,omitempty"`
	LastRenewal *certflow.Renewal `json:"last_renewal,omitempty"`
	Error       string            `json:"error,omitempty"`
	// Explanation is the CA error in plain language.
	Explanation string     `json:"explanation,omitempty"`
	Hint        string     `json:"hint,omitempty"`
	NextSafeAt  *time.Time `json:"next_safe_at,omitempty"`
}

type getCertRequest struct {
	Email       string   `json:"email"`
	Staging     bool     `json:"staging"`
	Acknowledge []string `json:"acknowledge"`
}

type getCertBlocked struct {
	Error     string          `json:"error"`
	Code      string          `json:"code,omitempty"`
	Preflight certflow.Result `json:"preflight"`
}

func (rt *Router) certFlowEmail(r *http.Request, s store.IngressSettings, typed string) (typedOrDefault, def string) {
	def = strings.TrimSpace(s.ACMEEmail)
	if def == "" {
		def = rt.defaultACMEEmail(r)
	}
	typed = strings.TrimSpace(typed)
	if typed == "" {
		typed = def
	}
	return typed, def
}

// certPreflight runs the doctor with public certificates assumed on, then
// judges the result together with the email and the rate-limit ledger.
func (rt *Router) certPreflight(r *http.Request, app, domain, email string, staging bool) (certflow.Result, int, string) {
	s, err := rt.ingressSettings.GetIngressSettings(r.Context())
	if err != nil {
		rt.logger.Error("api: cert preflight: load settings", slog.String("error", err.Error()), slog.String("domain", domain))
		return certflow.Result{}, http.StatusInternalServerError, "internal error"
	}
	rep, status, msg := rt.runDomainDoctorWith(r.Context(), app, domain, nil, publicCertDoctorFacts(s))
	if status != 0 {
		return certflow.Result{}, status, msg
	}
	return rt.certPreflightFromReport(r, domain, email, staging, s, rep)
}

// publicCertDoctorFacts runs the doctor as if public certificates were on, so the CAA and port checks really run.
func publicCertDoctorFacts(s store.IngressSettings) func(*domaindoctor.Facts) {
	return func(f *domaindoctor.Facts) { f.ACMEEnabled, f.TLSTerminatedUpstream = true, s.TLSTerminatedUpstream }
}

// certPreflightFromReport judges an already-run doctor report.
func (rt *Router) certPreflightFromReport(r *http.Request, domain, email string, staging bool, s store.IngressSettings, rep domaindoctor.Report) (certflow.Result, int, string) {
	typed, def := rt.certFlowEmail(r, s, email)
	rows, rerr := rt.certTruthRows(r.Context(), time.Now())
	if rerr != nil {
		rt.logger.Error("api: cert preflight: list certificates", slog.String("error", rerr.Error()), slog.String("domain", domain))
		return certflow.Result{}, http.StatusInternalServerError, "internal error"
	}
	in := certflow.Input{
		Existing: existingCerts(rows),
		Domain:   domain, Email: typed, DefaultEmail: def, Staging: staging, TLSUpstream: s.TLSTerminatedUpstream,
		Report: rep, History: rt.certLedger.Snapshot(domain), Limits: certflow.LimitsFromEnv(), Now: time.Now(),
	}
	if f, ok := ingress.DefaultACMEFailures().Get(domain); ok {
		in.LastCAError, in.LastCAErrorTime = f.Error, f.At
	}
	return certflow.Evaluate(in), 0, ""
}

// handleCertPreflight handles GET /api/v1/domains/{domain}/cert/preflight.
func (rt *Router) handleCertPreflight(w http.ResponseWriter, r *http.Request) {
	app, domain := rt.diagnoseOwner(w, r)
	if app == "" {
		return
	}
	res, status, msg := rt.certPreflight(r, app, domain, r.URL.Query().Get("email"), r.URL.Query().Get("staging") == "true")
	if status != 0 {
		writeError(w, status, msg)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// handleGetCert handles POST /api/v1/domains/{domain}/cert: runs the
// preflight, and when nothing blocks, turns on public certificates and asks
// the ingress for one. The progress is read from GET .../cert/status.
func (rt *Router) handleGetCert(w http.ResponseWriter, r *http.Request) {
	app, domain := rt.diagnoseOwner(w, r)
	if app == "" {
		return
	}
	var req getCertRequest
	if r.Body != nil && r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}
	res, status, msg := rt.certPreflight(r, app, domain, req.Email, req.Staging)
	if status != 0 {
		writeError(w, status, msg)
		return
	}
	if blocker := firstBlocker(res, req.Acknowledge); blocker != nil {
		writeJSON(w, http.StatusConflict, getCertBlocked{Error: blocker.Detail, Preflight: res})
		return
	}
	if err := rt.applyPublicCert(r.Context(), domain, res.Email, req.Staging); err != nil {
		var rep *certflow.ReplaceError
		if errors.As(err, &rep) {
			writeJSON(w, http.StatusConflict, getCertBlocked{Error: rep.Error(), Code: certflow.CodeReplaces, Preflight: res})
			return
		}
		rt.internalError(w, "api: get certificate: apply", err)
		return
	}
	rt.logger.Info("api: public certificate requested", slog.String("domain", domain), slog.Bool("staging", req.Staging))
	out, err := rt.domainCertStatus(r.Context(), domain)
	if err != nil {
		rt.internalError(w, "api: get certificate: status", err)
		return
	}
	writeJSON(w, http.StatusAccepted, out)
}

// firstBlocker is the first failed item the caller did not acknowledge; only
// the port self-check can be acknowledged.
func firstBlocker(res certflow.Result, ack []string) *certflow.Item {
	for i, it := range res.Items {
		if it.State != certflow.StateFail {
			continue
		}
		if it.Code == certflow.CodePorts && slices.Contains(ack, acknowledgePorts) {
			continue
		}
		return &res.Items[i]
	}
	return nil
}

// applyPublicCert switches public ACME on with the chosen CA and clears the
// domain's stored internal or wrong-CA certificate so ingress re-issues it.
func (rt *Router) applyPublicCert(ctx context.Context, domain, email string, staging bool) error {
	cur, err := rt.ingressSettings.GetIngressSettings(ctx)
	if err != nil {
		return err
	}
	rows, err := rt.certTruthRows(ctx, time.Now())
	if err != nil {
		return err
	}
	if staging {
		if c := certflow.StagingConflict(domain, existingCerts(rows)); c != nil {
			return c
		}
	}
	next := cur
	next.ACMEEnabled, next.ACMEEmail = true, email
	next.ACMEDirectoryURL = rt.acmeDefaultDirectory
	if staging {
		next.ACMEDirectoryURL = ACMEStagingDirectoryURL
	}
	rt.purgeOnIssuerChange(ctx, cur, next)
	if err := rt.ingressSettings.UpdateIngressSettings(ctx, next); err != nil {
		return err
	}
	for _, c := range rows {
		if strings.EqualFold(c.Info.Domain, domain) && wrongIssuerFor(c.Kind, staging) {
			if _, derr := alerting.DeleteCertificateForDomain(ctx, rt.certs, rt.certs, domain); derr != nil {
				return derr
			}
		}
	}
	ingress.DefaultACMEFailures().Clear(domain)
	rt.certLedger.MarkStarted(domain, time.Now())
	rt.nudgeReconciler()
	return nil
}

// existingCerts maps stored certificates to what the staging guard needs.
func existingCerts(rows []certTruthRow) []certflow.Existing {
	out := make([]certflow.Existing, 0, len(rows))
	for _, c := range rows {
		out = append(out, certflow.Existing{Domain: c.Info.Domain, Kind: c.Kind, Expired: c.Info.Status == "expired", Stalled: c.Renewal == "stalled"})
	}
	return out
}

// wrongIssuerFor is true only for certificates a request may replace: the
// internal one, or a staging one when asking for production. A working
// production certificate is never replaced by a staging request.
func wrongIssuerFor(k certflow.Kind, staging bool) bool {
	return k == certflow.KindInternal || (!staging && k == certflow.KindStaging)
}

// handleCertStatus handles GET /api/v1/domains/{domain}/cert/status.
func (rt *Router) handleCertStatus(w http.ResponseWriter, r *http.Request) {
	app, domain := rt.diagnoseOwner(w, r)
	if app == "" {
		return
	}
	out, err := rt.domainCertStatus(r.Context(), domain)
	if err != nil {
		rt.internalError(w, "api: certificate status", err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// domainCertStatus derives the progress state from settings, the stored
// certificate and the CA's last error, reusing the dashboard HTTPS logic.
func (rt *Router) domainCertStatus(ctx context.Context, domain string) (domainCertStatusResource, error) {
	s, err := rt.ingressSettings.GetIngressSettings(ctx)
	if err != nil {
		return domainCertStatusResource{}, err
	}
	now := time.Now()
	rows, err := rt.certTruthRows(ctx, now)
	if err != nil {
		return domainCertStatusResource{}, err
	}
	infos := make([]alerting.CertInfo, 0, len(rows))
	var mine *certTruthRow
	for i := range rows {
		infos = append(infos, rows[i].Info)
		if certInfoCovers(rows[i].Info, domain) && (mine == nil || rows[i].Info.NotAfter.After(mine.Info.NotAfter)) {
			mine = &rows[i]
		}
	}
	scoped := s
	scoped.PrimaryDomain = domain
	var failure *ingress.ACMEFailure
	if f, ok := ingress.DefaultACMEFailures().Get(domain); ok {
		failure = &f
	}
	started := rt.certLedger.StartedAt(domain)
	hs := computeHTTPSStatus(scoped, coveringInfos(infos, domain), failure, started, now, rt.httpsPendingAfter())
	out := domainCertStatusResource{
		Domain: domain, State: hs.State, Staging: hs.Staging, AppliesToAll: true,
		Issuer: hs.Issuer, Error: hs.Error, Hint: hs.Hint, Kind: certflow.KindNone, NotAfter: hs.NotAfter,
	}
	if mine != nil {
		b := rt.certBadgeFor(*mine)
		out.Kind, out.LastRenewal = b.Kind, b.LastRenewal
		if hs.State == certFlowIssued || hs.State == certFlowOff {
			days := b.DaysLeft
			out.DaysLeft = &days
			if out.Issuer == "" {
				out.Issuer = mine.Info.Issuer
			}
			if out.NotAfter == nil {
				na := mine.Info.NotAfter
				out.NotAfter = &na
			}
			renew := renewalStart(mine.Info.NotBefore, mine.Info.NotAfter)
			if mine.Kind.AutoRenewed() && !renew.IsZero() {
				out.RenewalAt = &renew
			}
		}
	}
	if out.Error != "" {
		out.Explanation = domaindoctor.ExplainACME(out.Error)
		if f := failure; f != nil && !s.TLSTerminatedUpstream {
			rt.certLedger.RecordFailure(domain, f.At, now)
		}
	}
	snap := rt.certLedger.Snapshot(domain)
	var errText string
	var errAt time.Time
	if failure != nil {
		errText, errAt = failure.Error, failure.At
	}
	if v := snap.Check(certflow.LimitsFromEnv(), now, errText, errAt); v.Blocked && !out.Staging {
		next := v.NextSafeAt
		out.NextSafeAt = &next
	}
	return out, nil
}

func certInfoCovers(info alerting.CertInfo, domain string) bool {
	return strings.EqualFold(info.Domain, domain) || slices.ContainsFunc(info.SANs, func(n string) bool { return strings.EqualFold(n, domain) })
}

func coveringInfos(infos []alerting.CertInfo, domain string) []alerting.CertInfo {
	var out []alerting.CertInfo
	for _, in := range infos {
		if certInfoCovers(in, domain) {
			c := in
			c.Domain = domain
			out = append(out, c)
		}
	}
	return out
}

// renewalStart is when automatic renewal begins: a third of the lifetime before expiry.
func renewalStart(notBefore, notAfter time.Time) time.Time {
	if notBefore.IsZero() || !notAfter.After(notBefore) {
		return time.Time{}
	}
	return notAfter.Add(-notAfter.Sub(notBefore) / 3)
}

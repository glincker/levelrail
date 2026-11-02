package api

import (
	"context"
	"errors"
	"net"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/certflow"
	"github.com/GLINCKER/levelrail/internal/domaincutover"
	"github.com/GLINCKER/levelrail/internal/domaindoctor"
)

// Env knobs of the cutover checklist.
const (
	envCutoverLowTTL      = "APP_CUTOVER_LOW_TTL_SECONDS"
	envCutoverDefaultETA  = "APP_CUTOVER_DEFAULT_ETA_SECONDS"
	envCutoverCertETA     = "APP_CUTOVER_CERT_ETA_SECONDS"
	envCutoverMinInterval = "APP_CUTOVER_MIN_INTERVAL"
	envCutoverNSLimit     = "APP_CUTOVER_NS_LIMIT"

	defaultCutoverMinInterval = 3 * time.Second
	defaultCutoverNSLimit     = 4
	cutoverCacheMax           = 256
	cutoverStepBudget         = 6 * time.Second
)

// cutoverCache keeps each domain's last answer for a few seconds so a polling
// dashboard cannot turn the probes into a load generator.
var cutoverCache = struct {
	sync.Mutex
	m map[string]cutoverCached
}{m: map[string]cutoverCached{}}

type cutoverCached struct {
	at  time.Time
	res domaincutover.Result
}

func cutoverOptions() domaincutover.Options {
	return domaincutover.Options{
		LowTTL:     envIntDefault(envCutoverLowTTL, 300),
		DefaultETA: envIntDefault(envCutoverDefaultETA, 1800),
		CertETA:    envIntDefault(envCutoverCertETA, 120),
	}
}

// handleDomainCutover handles GET /api/v1/domains/{domain}/cutover: the
// test-before-you-switch checklist. It dials only this server's own ingress
// (never an address from the request) until DNS already points here.
func (rt *Router) handleDomainCutover(w http.ResponseWriter, r *http.Request) {
	app, domain := rt.diagnoseOwner(w, r)
	if app == "" {
		return
	}
	interval := envDurationOr(envCutoverMinInterval, defaultCutoverMinInterval)
	cutoverCache.Lock()
	hit, ok := cutoverCache.m[domain]
	cutoverCache.Unlock()
	if ok && time.Since(hit.at) < interval {
		writeJSON(w, http.StatusOK, hit.res)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), cutoverStepBudget*3)
	defer cancel()
	res := domaincutover.Evaluate(rt.cutoverFacts(ctx, r, domain), cutoverOptions())
	cutoverCache.Lock()
	if len(cutoverCache.m) >= cutoverCacheMax {
		clear(cutoverCache.m)
	}
	cutoverCache.m[domain] = cutoverCached{at: time.Now(), res: res}
	cutoverCache.Unlock()
	writeJSON(w, http.StatusOK, res)
}

func (rt *Router) cutoverFacts(ctx context.Context, r *http.Request, domain string) domaincutover.Facts {
	f := domaincutover.Facts{Domain: domain}
	s, serr := rt.ingressSettings.GetIngressSettings(ctx)
	f.UpstreamTLS = serr == nil && s.TLSTerminatedUpstream

	rt.cutoverAppProbe(ctx, &f)
	if st, err := rt.domainCertStatus(ctx, domain); err == nil {
		f.CertState, f.CertIssuer, f.CertError, f.CertExplanation = st.State, st.Issuer, st.Error, st.Explanation
	}
	f.ProviderConnected = rt.dns01ProviderCheck != nil && rt.dns01ProviderCheck(ctx)
	f.DNS01Host, _, _ = rt.domainACME.DomainDNS01(ctx, domain)

	expectedHost, inferred := advertisedHost(r, rt.advertisedPublicHost(ctx))
	check := rt.runDomainCheckOpts(ctx, domain, expectedHost, inferred, false)
	f.DNSStatus, f.ExpectedHost, f.ExpectedIPv4, f.ExpectedIPv6 = check.Status, expectedHost, check.ExpectedIPv4, check.ExpectedIPv6
	f.Resolved = resolvedAddresses(check)
	f.Proxied = len(f.Resolved) > 0 && !slices.ContainsFunc(f.Resolved, func(ip string) bool { return !domaindoctor.IsCloudflareIP(ip) })
	f.TTLSeconds = rt.cutoverTTL(ctx, domain)

	want := append(append([]string{}, check.ExpectedIPv4...), check.ExpectedIPv6...)
	if len(want) > 0 {
		f.AuthKnown, f.AuthHit, f.AuthTotal = rt.cutoverAuthoritative(ctx, domain, want)
	}
	// The real name is dialed only once it points here or at Cloudflare, so a
	// request can never steer the server at an arbitrary host.
	if (check.Status == domainCheckStatusConnected || f.Proxied) && !f.UpstreamTLS {
		rt.cutoverLiveProbe(ctx, &f, s.PublicLinkPort())
	}
	return f
}

// resolvedAddresses prefers this server's own resolver and falls back to the
// first public resolver that answered.
func resolvedAddresses(c domainCheckResponse) []string {
	if len(c.ResolvedHosts) > 0 {
		return c.ResolvedHosts
	}
	for _, rr := range c.Resolvers {
		if len(rr.Addresses) > 0 {
			return rr.Addresses
		}
	}
	return nil
}

func (rt *Router) cutoverProbes() (func(context.Context, string, string) (tlsProbeResult, error), func(context.Context, string, string) (int, error)) {
	tlsP, httpP := rt.domainAuto.tlsProbe, rt.domainAuto.httpProbe
	if tlsP == nil {
		tlsP = defaultTLSProbe
	}
	if httpP == nil {
		httpP = defaultHTTPProbe
	}
	return tlsP, httpP
}

// cutoverAppProbe asks this server's own ingress for the domain, DNS not involved.
func (rt *Router) cutoverAppProbe(ctx context.Context, f *domaincutover.Facts) {
	if f.UpstreamTLS {
		f.App.Err = "a proxy in front owns TLS, so this server cannot be asked directly. Check the route in the proxy integration card"
		return
	}
	tlsP, httpP := rt.cutoverProbes()
	dial := rt.probeDialAddr(f.Domain, false, 0)
	pctx, cancel := context.WithTimeout(ctx, cutoverStepBudget)
	defer cancel()
	f.App.Done = true
	if pr, err := tlsP(pctx, dial, f.Domain); err == nil {
		f.App.CertTrusted, f.App.CertIssuer = pr.Trusted, pr.Issuer
	}
	code, err := httpP(pctx, "https://"+f.Domain+"/", dial)
	if err != nil {
		f.App.Err = "this server did not answer for " + f.Domain + ": " + err.Error()
		return
	}
	f.App.Status = code
	if code >= http.StatusInternalServerError {
		f.App.Err = "this server answered with status " + strconv.Itoa(code) + " for " + f.Domain
	}
}

// cutoverLiveProbe is the end to end check over the real name.
func (rt *Router) cutoverLiveProbe(ctx context.Context, f *domaincutover.Facts, port int) {
	tlsP, httpP := rt.cutoverProbes()
	if port == 0 {
		port = defaultDoctorHTTPSPort
	}
	dial := net.JoinHostPort(f.Domain, strconv.Itoa(port))
	pctx, cancel := context.WithTimeout(ctx, cutoverStepBudget)
	defer cancel()
	f.Live.Done = true
	if pr, err := tlsP(pctx, dial, f.Domain); err == nil {
		f.Live.CertTrusted, f.Live.CertIssuer = pr.Trusted, pr.Issuer
	}
	code, err := httpP(pctx, "https://"+net.JoinHostPort(f.Domain, strconv.Itoa(port))+"/", "")
	if err != nil {
		f.Live.Err = "https://" + f.Domain + " did not answer: " + err.Error()
		return
	}
	f.Live.Status = code
	if code >= http.StatusInternalServerError {
		f.Live.Err = "https://" + f.Domain + " answered with status " + strconv.Itoa(code)
	}
}

// cutoverTTL reads the live record's TTL from the connected DNS provider, 0 when unknown.
func (rt *Router) cutoverTTL(ctx context.Context, domain string) int {
	tctx, cancel := context.WithTimeout(ctx, cutoverStepBudget)
	defer cancel()
	t, err := rt.resolveDNSTarget(tctx, domain)
	if err != nil || t == nil {
		return 0
	}
	recs, err := t.Manager.GetRecords(tctx, t.Zone)
	if err != nil {
		return 0
	}
	zone := strings.TrimSuffix(t.Zone, ".")
	rel := "@"
	if !strings.EqualFold(domain, zone) {
		rel = strings.TrimSuffix(strings.ToLower(domain), "."+strings.ToLower(zone))
	}
	ttl := 0
	for _, rec := range recs {
		rr := rec.RR()
		if rr.Type != "A" && rr.Type != "AAAA" && rr.Type != "CNAME" {
			continue
		}
		if n := strings.ToLower(strings.TrimSuffix(rr.Name, ".")); n != rel && n != strings.ToLower(domain) {
			continue
		}
		if s := int(rr.TTL.Seconds()); s > 0 && (ttl == 0 || s > ttl) {
			ttl = s
		}
	}
	return ttl
}

// cutoverAuthoritative asks the zone's own name servers directly through the
// shared inspector (public-address rule included), skipping every cache.
func (rt *Router) cutoverAuthoritative(ctx context.Context, domain string, want []string) (known bool, hit, total int) {
	if rt.domainAuto.authoritative != nil {
		return rt.domainAuto.authoritative(ctx, domain, want)
	}
	actx, cancel := context.WithTimeout(ctx, cutoverStepBudget)
	defer cancel()
	limit := envIntDefault(envCutoverNSLimit, defaultCutoverNSLimit)
	for _, src := range rt.dnsInspectorOrDefault().Inspect(actx, domain).Authoritative {
		if total >= limit {
			break
		}
		if src.Error == "no answer" || src.Error == "no usable address" {
			continue
		}
		total++
		if hostsOverlap(src.Addresses, want) {
			hit++
		}
	}
	return total > 0, hit, total
}

type cutoverCertBlocked struct {
	Error       string   `json:"error"`
	Code        string   `json:"code"`
	Runbook     string   `json:"runbook_href"`
	Alternative []string `json:"alternatives,omitempty"`
}

// DomainACMEChallengeStore keeps the per-host DNS-01 flag.
type DomainACMEChallengeStore interface {
	DomainDNS01(ctx context.Context, domain string) (bool, time.Time, error)
	SetDomainDNS01(ctx context.Context, domain string, on bool) error
}

// handleCutoverCertificate handles POST /api/v1/domains/{domain}/cutover/certificate:
// requests this host's certificate through the connected DNS provider (DNS-01)
// so it exists before DNS moves. Only this host changes challenge; the
// all-hosts setting is never touched. Progress is read from the checklist.
func (rt *Router) handleCutoverCertificate(w http.ResponseWriter, r *http.Request) {
	app, domain := rt.diagnoseOwner(w, r)
	if app == "" || !rt.runtimeSettingsOr501(w) {
		return
	}
	ctx := r.Context()
	if rt.dns01ProviderCheck == nil || !rt.dns01ProviderCheck(ctx) {
		writeJSON(w, http.StatusConflict, cutoverCertBlocked{
			Error: "no DNS provider is connected, so the certificate cannot be issued before DNS points here", Code: "no_dns_provider", Runbook: domaincutover.RunbookHref,
			Alternative: []string{
				"connect Cloudflare, Route53 or Google Cloud DNS and run this again",
				"or lower the record's TTL now, switch DNS, and the certificate is issued over HTTP-01 once DNS points here",
			},
		})
		return
	}
	if z := rt.zoneLookup(ctx, domain); !z.Found {
		writeJSON(w, http.StatusConflict, cutoverCertBlocked{Error: "the connected DNS provider does not manage the zone for " + domain, Code: "zone_not_managed", Runbook: domaincutover.RunbookHref})
		return
	}
	s, err := rt.ingressSettings.GetIngressSettings(ctx)
	if err != nil {
		rt.internalError(w, "api: cutover certificate: settings", err)
		return
	}
	if s.TLSTerminatedUpstream {
		writeJSON(w, http.StatusConflict, cutoverCertBlocked{Error: "a proxy in front of this server owns the certificate", Code: "upstream_proxy", Runbook: domaincutover.RunbookHref})
		return
	}
	if st, err := rt.domainCertStatus(ctx, domain); err == nil && st.State == certFlowIssued {
		writeJSON(w, http.StatusOK, st)
		return
	}
	now := time.Now()
	if v := rt.certLedger.Snapshot(domain).Check(certflow.LimitsFromEnv(), now, "", time.Time{}); v.Blocked {
		writeJSON(w, http.StatusConflict, cutoverCertBlocked{Error: "the certificate authority's rate limit would be hit; try again after " + v.NextSafeAt.Format(time.RFC3339), Code: "rate_limit", Runbook: domaincutover.RunbookHref})
		return
	}
	email, _ := rt.certFlowEmail(r, s, "")
	if email == "" {
		writeError(w, http.StatusBadRequest, "set an ACME contact email first (ingress settings), then run this again")
		return
	}
	if err := rt.domainACME.SetDomainDNS01(ctx, domain, true); err != nil {
		rt.internalError(w, "api: cutover certificate: store challenge", err)
		return
	}
	if err := rt.applyPublicCert(ctx, domain, email, false); err != nil {
		var rep *certflow.ReplaceError
		if errors.As(err, &rep) {
			writeJSON(w, http.StatusConflict, cutoverCertBlocked{Error: rep.Error(), Code: certflow.CodeReplaces, Runbook: domaincutover.RunbookHref})
			return
		}
		rt.internalError(w, "api: cutover certificate: apply", err)
		return
	}
	rt.cutoverCertDone(ctx, w, domain, http.StatusAccepted)
}

// handleCutoverCertificateRevert handles DELETE /api/v1/domains/{domain}/cutover/certificate:
// the host goes back to the default challenge. The issued certificate stays.
func (rt *Router) handleCutoverCertificateRevert(w http.ResponseWriter, r *http.Request) {
	app, domain := rt.diagnoseOwner(w, r)
	if app == "" {
		return
	}
	ctx := r.Context()
	if err := rt.domainACME.SetDomainDNS01(ctx, domain, false); err != nil {
		rt.internalError(w, "api: cutover certificate: revert", err)
		return
	}
	rt.nudgeReconciler()
	rt.cutoverCertDone(ctx, w, domain, http.StatusOK)
}

type cutoverCertResponse struct {
	domainCertStatusResource
	Challenge string `json:"challenge,omitempty"`
}

func (rt *Router) cutoverCertDone(ctx context.Context, w http.ResponseWriter, domain string, code int) {
	cutoverCache.Lock()
	delete(cutoverCache.m, domain)
	cutoverCache.Unlock()
	out, err := rt.domainCertStatus(ctx, domain)
	if err != nil {
		rt.internalError(w, "api: cutover certificate: status", err)
		return
	}
	res := cutoverCertResponse{domainCertStatusResource: out}
	if on, _, err := rt.domainACME.DomainDNS01(ctx, domain); err == nil && on {
		res.Challenge = "dns01"
	}
	writeJSON(w, code, res)
}

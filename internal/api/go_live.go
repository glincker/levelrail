package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/dnsrecords"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Go-live step ids, in the order they run.
const (
	stepDNS         = "dns"
	stepCounterpart = "counterpart"
	stepRedirect    = "redirect"
	stepProxyRoute  = "proxy_route"
	stepForceHTTPS  = "force_https"
	stepPropagation = "propagation"
	stepCertificate = "certificate"
	stepHTTP        = "http"
)

// Step states.
const (
	stepDone     = "done"
	stepPending  = "pending"
	stepSkipped  = "skipped"
	stepFailed   = "failed"
	stepManual   = "manual"
	stepConflict = "conflict"
)

// Overall go-live states.
const (
	goLiveLive    = "live"
	goLivePending = "pending"
	goLiveFailed  = "failed"
	goLivePlanned = "planned"
	goLiveApplied = "applied"
)

const (
	goLiveStatusBudget   = 25 * time.Second
	notAvailableDetail   = "not available on this server"
	statusRedirectCode   = http.StatusMovedPermanently
	redirectTargetScheme = "https://"
)

type goLiveStep struct {
	ID         string             `json:"id"`
	State      string             `json:"state"`
	Detail     string             `json:"detail,omitempty"`
	Provider   string             `json:"provider,omitempty"`
	Record     *dnsRecordResource `json:"record,omitempty"`
	Resolvers  []resolverResult   `json:"resolvers,omitempty"`
	Issuer     string             `json:"issuer,omitempty"`
	NotAfter   *time.Time         `json:"not_after,omitempty"`
	URL        string             `json:"url,omitempty"`
	HTTPStatus int                `json:"http_status,omitempty"`
}

type goLiveResult struct {
	RunID     string                 `json:"run_id,omitempty"`
	App       string                 `json:"app"`
	Domain    string                 `json:"domain"`
	State     string                 `json:"state"`
	URL       string                 `json:"url,omitempty"`
	Steps     []goLiveStep           `json:"steps"`
	DNS       []domainDNSResult      `json:"dns,omitempty"`
	Policy    domainAutomationPolicy `json:"policy"`
	Undoable  bool                   `json:"undoable,omitempty"`
	CheckedAt string                 `json:"checked_at"`
	undo      []undoAction
}

type goLiveRequest struct {
	Automation *domainAutomationOverride `json:"automation,omitempty"`
	DNS        string                    `json:"dns,omitempty"`
	Replace    bool                      `json:"replace,omitempty"`
	// skipVerify leaves propagation, certificate and http unprobed, so a
	// request that only adds a domain never waits on the network.
	skipVerify bool
}

type goLiveMode int

const (
	goLiveModePlan   goLiveMode = iota // no writes, no probes
	goLiveModeApply                    // writes, then probes if probeAfter
	goLiveModeStatus                   // no writes, probes
)

// goLive computes (and in apply mode performs) the whole add-a-domain flow
// for one domain.
func (rt *Router) goLive(ctx context.Context, r *http.Request, app, domain string, policy domainAutomationPolicy, req goLiveRequest, mode goLiveMode) goLiveResult {
	res := goLiveResult{App: app, Domain: domain, Policy: policy, CheckedAt: time.Now().UTC().Format(time.RFC3339)}
	dnsMode := dnsModeAuto
	switch {
	case req.DNS == dnsModeOff || !policy.AutoDNS:
		dnsMode = dnsModeOff
	case mode == goLiveModePlan || req.DNS == dnsModePreview:
		dnsMode = dnsModePreview
	}
	opts := dnsOpts{Mode: dnsMode, Replace: req.Replace, CanWrite: mode == goLiveModeApply && rt.callerIsRoot(r)}

	settings, _ := rt.ingressSettings.GetIngressSettings(ctx)

	res.Steps = append(res.Steps, rt.dnsStep(ctx, r, &res, app, domain, opts, mode))
	canonical := domain
	if policy.AttachWWWCounterpart || policy.WWWPolicy != wwwPolicyOff {
		var steps []goLiveStep
		steps, canonical = rt.counterpartSteps(ctx, r, &res, app, domain, policy, opts, mode)
		res.Steps = append(res.Steps, steps...)
	}

	view := (*proxyIntegrationView)(nil)
	if policy.AutoProxyRoute {
		view = rt.proxyIntegration(ctx, r)
	}
	if view != nil {
		res.Steps = append(res.Steps, proxyRouteStep(view, canonical, mode))
	}
	if policy.ForceHTTPS {
		res.Steps = append(res.Steps, goLiveStep{ID: stepForceHTTPS, State: stepSkipped, Detail: notAvailableDetail})
	}

	verify := policy.VerifyAfter && mode != goLiveModePlan && !req.skipVerify
	res.Steps = append(res.Steps, rt.verifySteps(ctx, canonical, settings, view, verify, mode)...)
	res.URL = rt.publicHTTPSURL(ctx, canonical)
	res.State = overallState(res.Steps, mode, policy.VerifyAfter)
	res.Undoable = len(res.undo) > 0
	return res
}

func overallState(steps []goLiveStep, mode goLiveMode, verify bool) string {
	if mode == goLiveModePlan {
		return goLivePlanned
	}
	live := verify
	for _, s := range steps {
		if s.State == stepFailed || s.State == stepConflict {
			return goLiveFailed
		}
		if (s.ID == stepPropagation || s.ID == stepCertificate || s.ID == stepHTTP) && s.State != stepDone {
			live = false
		}
	}
	switch {
	case live:
		return goLiveLive
	case !verify:
		return goLiveApplied
	}
	return goLivePending
}

func (rt *Router) dnsStep(ctx context.Context, r *http.Request, res *goLiveResult, app, domain string, opts dnsOpts, mode goLiveMode) goLiveStep {
	if mode == goLiveModeStatus {
		return rt.dnsStatusStep(ctx, domain)
	}
	d, undo := rt.applyDomainDNS(ctx, r, app, domain, opts)
	res.DNS = append(res.DNS, d)
	res.undo = append(res.undo, undo...)
	step := goLiveStep{ID: stepDNS, Provider: d.Provider, Record: d.Record, Detail: d.Message}
	switch d.DNS {
	case dnsrecords.OutcomeCreated, dnsrecords.OutcomeUpdated, dnsrecords.OutcomeUnchanged:
		step.State = stepDone
		if step.Detail == "" {
			step.Detail = "DNS record " + d.DNS
		}
	case dnsResultPreview:
		step.State = stepPending
	case dnsrecords.OutcomeConflict:
		step.State = stepConflict
	case dnsResultError:
		step.State = stepFailed
	default:
		step.State = stepManual
		if opts.Mode == dnsModeOff {
			step.State = stepSkipped
		}
	}
	return step
}

// dnsStatusStep reports DNS state without writing: tracked by Levelrail,
// already resolving, or still to do.
func (rt *Router) dnsStatusStep(ctx context.Context, domain string) goLiveStep {
	step := goLiveStep{ID: stepDNS, State: stepManual, Detail: "no DNS record from Levelrail is tracked for this domain"}
	if ms := rt.managedDNS(); ms != nil {
		if rows, err := ms.ListManagedDNSRecords(ctx, domain); err == nil && len(rows) > 0 {
			row := rows[0]
			step.State, step.Provider = stepDone, row.Provider
			step.Record = &dnsRecordResource{Name: row.Name, Type: row.RecordType, Value: row.Value}
			step.Detail = "DNS record created at " + row.Provider
		}
	}
	return step
}

func (rt *Router) counterpartSteps(ctx context.Context, r *http.Request, res *goLiveResult, app, domain string, policy domainAutomationPolicy, opts dnsOpts, mode goLiveMode) ([]goLiveStep, string) {
	zoneRel, zoneKnown := "", false
	if t, err := rt.resolveDNSTarget(ctx, domain); err == nil && t != nil {
		zoneRel, zoneKnown = dnsrecords.RelativeName(domain, t.Zone), true
	}
	partner, isWWW, ok := counterpartHost(domain, zoneRel, zoneKnown)
	if !ok {
		return []goLiveStep{{ID: stepCounterpart, State: stepSkipped, Detail: "this host is neither an apex domain nor a www host"}}, domain
	}
	canonical, source, redirect := canonicalFor(domain, partner, isWWW, policy.WWWPolicy)
	step := goLiveStep{ID: stepCounterpart, State: stepPending, Detail: partner}
	steps := []goLiveStep{step}
	if mode == goLiveModePlan {
		steps[0].Detail = "would attach " + partner + " and create its DNS record"
		if redirect {
			steps = append(steps, goLiveStep{ID: stepRedirect, State: stepPending, Detail: fmt.Sprintf("would redirect %s to %s", source, canonical)})
		}
		return steps, canonical
	}
	if mode == goLiveModeStatus {
		attached, _ := rt.appOwnsDomain(ctx, app, partner)
		steps[0].State = stepPending
		if attached {
			steps[0].State, steps[0].Detail = stepDone, partner+" is attached"
		}
		if redirect {
			steps = append(steps, rt.redirectStatusStep(ctx, source, canonical))
		}
		return steps, canonical
	}

	steps[0] = rt.attachCounterpart(ctx, res, app, partner)
	if steps[0].State == stepDone {
		d, undo := rt.applyDomainDNS(ctx, r, app, partner, opts)
		res.DNS = append(res.DNS, d)
		res.undo = append(res.undo, undo...)
		steps[0].Detail = partner + ": " + firstNonEmpty(d.Message, "DNS "+d.DNS)
	}
	if redirect && steps[0].State == stepDone {
		steps = append(steps, rt.applyRedirect(ctx, res, source, canonical))
	}
	return steps, canonical
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func (rt *Router) attachCounterpart(ctx context.Context, res *goLiveResult, app, partner string) goLiveStep {
	step := goLiveStep{ID: stepCounterpart}
	editor, ok := rt.apps.(serviceDomainsEditor)
	if !ok {
		step.State, step.Detail = stepSkipped, notAvailableDetail
		return step
	}
	_, changed, err := editor.EditServiceDomains(ctx, app, func(cur []string) ([]string, error) {
		return applyDomainEdit(cur, []string{partner}, nil)
	})
	var taken *store.ErrDomainTaken
	switch {
	case errors.As(err, &taken):
		step.State, step.Detail = stepSkipped, partner+" is already used by another app"
	case err != nil:
		rt.logger.Error("api: go-live: attach counterpart failed", slog.String("error", err.Error()), slog.String("app", app))
		step.State, step.Detail = stepFailed, "could not attach "+partner
	default:
		step.State, step.Detail = stepDone, partner+" attached"
		if changed {
			res.undo = append(res.undo, undoAction{Kind: undoKindAppDomain, App: app, Domain: partner})
			rt.nudgeReconciler()
		}
	}
	return step
}

func (rt *Router) redirectStatusStep(ctx context.Context, source, canonical string) goLiveStep {
	row, found, err := rt.domainRedirect.GetDomainRedirect(ctx, source)
	step := goLiveStep{ID: stepRedirect, State: stepPending, Detail: fmt.Sprintf("%s to %s not set yet", source, canonical)}
	if err == nil && found && strings.Contains(row.TargetURL, canonical) {
		step.State, step.Detail = stepDone, fmt.Sprintf("%s redirects to %s", source, canonical)
	}
	return step
}

func (rt *Router) applyRedirect(ctx context.Context, res *goLiveResult, source, canonical string) goLiveStep {
	target := redirectTargetScheme + canonical
	step := goLiveStep{ID: stepRedirect}
	row, found, err := rt.domainRedirect.GetDomainRedirect(ctx, source)
	switch {
	case err != nil:
		step.State, step.Detail = stepFailed, "could not read the existing redirect"
	case found && strings.TrimSuffix(row.TargetURL, "/") == target:
		step.State, step.Detail = stepDone, fmt.Sprintf("%s already redirects to %s", source, canonical)
	case found:
		step.State, step.Detail = stepSkipped, source+" already has a different redirect; left unchanged"
	default:
		if err := rt.domainRedirect.SetDomainRedirect(ctx, source, target, statusRedirectCode); err != nil {
			rt.logger.Error("api: go-live: set redirect failed", slog.String("error", err.Error()), slog.String("domain", source))
			step.State, step.Detail = stepFailed, "could not set the redirect"
			break
		}
		step.State, step.Detail = stepDone, fmt.Sprintf("%s redirects to %s", source, canonical)
		res.undo = append(res.undo, undoAction{Kind: undoKindRedirect, Domain: source})
		rt.nudgeReconciler()
	}
	return step
}

func proxyRouteStep(view *proxyIntegrationView, domain string, mode goLiveMode) goLiveStep {
	step := goLiveStep{ID: stepProxyRoute, State: stepPending, Detail: "waiting for the proxy route"}
	if mode == goLiveModePlan {
		step.Detail = "the managed proxy gets a route for this domain"
		return step
	}
	row, ok := view.Domains[strings.ToLower(domain)]
	if !ok {
		return step
	}
	switch strings.ToLower(row.State) {
	case "ready", "active", "ok", "live", "synced", "configured", "applied":
		step.State, step.Detail = stepDone, "proxy route is "+row.State
	case "error", "failed", "rejected":
		step.State, step.Detail = stepFailed, "proxy route is "+row.State
	default:
		step.Detail = "proxy route is " + firstNonEmpty(row.State, "pending")
	}
	return step
}

// verifySteps are propagation, certificate and http; with verify false they
// are reported pending without any network probe.
func (rt *Router) verifySteps(ctx context.Context, domain string, s store.IngressSettings, view *proxyIntegrationView, verify bool, mode goLiveMode) []goLiveStep {
	prop := goLiveStep{ID: stepPropagation, State: stepPending}
	cert := goLiveStep{ID: stepCertificate, State: stepPending}
	httpStep := goLiveStep{ID: stepHTTP, State: stepPending}
	if mode == goLiveModePlan || !verify {
		if mode != goLiveModePlan {
			prop.Detail = "not checked yet"
		} else {
			prop.Detail, cert.Detail, httpStep.Detail = "waits for DNS to resolve to this server", "issued automatically once DNS resolves", "checks that the site answers"
		}
		return []goLiveStep{prop, cert, httpStep}
	}
	ctx, cancel := context.WithTimeout(ctx, goLiveStatusBudget)
	defer cancel()

	check := rt.runDomainCheckOpts(ctx, domain, rt.publicHost, false, true)
	prop.Resolvers = check.Resolvers
	proxied := s.DNSProxied && rt.activeDNSProvider(ctx) == dnsProviderCloudflare
	switch {
	case check.Status == domainCheckStatusConnected:
		prop.State, prop.Detail = stepDone, "resolves to this server"
	case proxied && check.Resolved:
		prop.State, prop.Detail = stepDone, "resolves through the Cloudflare proxy"
	case check.Status == domainCheckStatusPropagating:
		prop.Detail = "waiting for DNS to propagate; public resolvers already see it"
	case check.Status == domainCheckStatusResolvesElsewhere:
		prop.Detail = "the domain resolves to a different address: " + strings.Join(check.ResolvedHosts, ", ")
	case check.Status == domainCheckStatusUnconfigured:
		prop.State, prop.Detail = stepManual, "this server's public address is unknown; set APP_PUBLIC_HOST"
	default:
		prop.Detail = "waiting for the record to appear in DNS"
	}

	dialAddr := rt.probeDialAddr(domain, s.TLSTerminatedUpstream, s.PublicLinkPort())
	if row, ok := proxyRow(view, domain); ok && row.Certificate != "" {
		cert.State, cert.Detail = stepDone, "certificate: "+row.Certificate
	} else {
		probe := rt.domainAuto.tlsProbe
		if probe == nil {
			probe = defaultTLSProbe
		}
		pr, err := probe(ctx, dialAddr, domain)
		switch {
		case err != nil:
			cert.Detail = "no certificate served yet"
		case isInternalIssuer(pr.Issuer):
			cert.Issuer, cert.Detail = pr.Issuer, "the server presents its internal (self-signed) certificate; a public certificate is issued once DNS resolves and ACME is on"
		case !pr.Trusted:
			cert.Issuer, cert.Detail = pr.Issuer, "a certificate is served but is not trusted yet"
		default:
			na := pr.NotAfter
			cert.State, cert.Issuer, cert.NotAfter = stepDone, pr.Issuer, &na
			cert.Detail = fmt.Sprintf("issued by %s, valid until %s", pr.Issuer, na.Format("2006-01-02"))
		}
	}

	httpProbe := rt.domainAuto.httpProbe
	if httpProbe == nil {
		httpProbe = defaultHTTPProbe
	}
	url := "https://" + domain + "/"
	code, err := httpProbe(ctx, url, dialAddr)
	switch {
	case err != nil:
		httpStep.Detail = "the site did not answer yet"
	case code >= http.StatusInternalServerError:
		httpStep.HTTPStatus, httpStep.Detail = code, fmt.Sprintf("the site answered with HTTP %d", code)
		httpStep.State = stepFailed
	default:
		httpStep.State, httpStep.HTTPStatus = stepDone, code
		httpStep.URL = rt.publicHTTPSURL(ctx, domain)
		httpStep.Detail = fmt.Sprintf("GET / returned %d", code)
	}
	return []goLiveStep{prop, cert, httpStep}
}

func proxyRow(v *proxyIntegrationView, domain string) (proxyDomainState, bool) {
	if v == nil {
		return proxyDomainState{}, false
	}
	row, ok := v.Domains[strings.ToLower(domain)]
	return row, ok
}

// recordGoLiveRun persists an applied run so it can be shown and undone.
func (rt *Router) recordGoLiveRun(ctx context.Context, res *goLiveResult) {
	st, ok := rt.apps.(interface {
		SaveDomainAutomationRun(ctx context.Context, r store.DomainAutomationRun) error
	})
	if !ok {
		return
	}
	id, err := store.NewDomainAutomationRunID()
	if err != nil {
		return
	}
	steps, _ := json.Marshal(res.Steps)
	undo, _ := json.Marshal(res.undo)
	run := store.DomainAutomationRun{ID: id, AppName: res.App, Domain: res.Domain, Result: res.State, Steps: string(steps), Undo: string(undo), CreatedAt: time.Now()}
	if err := st.SaveDomainAutomationRun(ctx, run); err != nil {
		rt.logger.Warn("api: save domain automation run failed", slog.String("error", err.Error()), slog.String("domain", res.Domain))
		return
	}
	res.RunID = id
}

// stepIDs lists a result's step ids, for tests and summaries.
func stepIDs(steps []goLiveStep) []string {
	out := make([]string, 0, len(steps))
	for _, s := range steps {
		out = append(out, s.ID)
	}
	return slices.Clone(out)
}

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

	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/trafficpolicy"
)

// Effective force HTTPS owners shown on the Redirects tab.
const (
	httpsHandledHere  = "here"
	httpsHandledProxy = "proxy"
	httpsNotServed    = "none"
)

type redirectEffective struct {
	HandledBy             string `json:"handled_by"`
	Explanation           string `json:"explanation"`
	HTTPSPort             int    `json:"https_port"`
	TLSTerminatedUpstream bool   `json:"tls_terminated_upstream"`
	IngressHTTPRedirect   bool   `json:"ingress_http_redirect"`
}

type redirectDomainRow struct {
	Domain      string                      `json:"domain"`
	Redirect    *trafficpolicy.HostRedirect `json:"redirect,omitempty"`
	Maintenance bool                        `json:"maintenance"`
	Wildcard    bool                        `json:"wildcard"`
}

type canonicalState struct {
	Counterpart         string `json:"counterpart,omitempty"`
	CounterpartAttached bool   `json:"counterpart_attached"`
	AttachedElsewhere   string `json:"counterpart_app,omitempty"`
	Preset              string `json:"preset"`
	DNSHint             string `json:"dns_hint,omitempty"`
	Unavailable         string `json:"unavailable,omitempty"`
}

type domainRedirectsResource struct {
	Domain     string                  `json:"domain"`
	Configured bool                    `json:"configured"`
	Settings   trafficpolicy.Redirects `json:"settings"`
	Effective  redirectEffective       `json:"effective"`
	AppDomains []redirectDomainRow     `json:"app_domains"`
	Canonical  canonicalState          `json:"canonical"`
	Samples    []policyPreviewHops     `json:"samples"`
	Warnings   []string                `json:"warnings,omitempty"`
}

// hostRedirects returns every domain_redirect row keyed by host.
func (rt *Router) hostRedirects(ctx context.Context) (map[string]trafficpolicy.HostRedirect, error) {
	rows, err := rt.domainRedirect.ListDomainRedirects(ctx)
	if err != nil {
		return nil, fmt.Errorf("list domain redirects: %w", err)
	}
	out := make(map[string]trafficpolicy.HostRedirect, len(rows))
	for _, row := range rows {
		out[strings.ToLower(row.Domain)] = trafficpolicy.HostRedirect{TargetURL: row.TargetURL, StatusCode: row.StatusCode}
	}
	return out, nil
}

func (rt *Router) redirectSettings(ctx context.Context, domain string) (trafficpolicy.Redirects, bool, error) {
	row, found, err := rt.domainPolicy.store.GetDomainTrafficPolicy(ctx, domain, trafficpolicy.KindRedirects)
	if err != nil || !found {
		return trafficpolicy.Redirects{}, false, err
	}
	var p trafficpolicy.Policy
	if err := p.Decode(trafficpolicy.KindRedirects, row.Spec); err != nil {
		return trafficpolicy.Redirects{}, false, err
	}
	return *p.Redirects, true, nil
}

// hopEnv builds the redirect simulation for app's domains, overlaying a
// draft of domain's settings and a draft canonical preset.
func (rt *Router) hopEnv(ctx context.Context, app, domain string, draft *trafficpolicy.Redirects, preset string) (trafficpolicy.HopEnv, error) {
	settings, err := rt.ingressSettings.GetIngressSettings(ctx)
	if err != nil {
		return trafficpolicy.HopEnv{}, fmt.Errorf("get ingress settings: %w", err)
	}
	env := trafficpolicy.HopEnv{
		Hosts:      map[string]bool{},
		Redirects:  map[string]trafficpolicy.Redirects{},
		AutoHTTPS:  ingress.HTTPRedirectActive() && !settings.TLSTerminatedUpstream,
		Terminated: settings.TLSTerminatedUpstream,
		HTTPSPort:  settings.PublicLinkPort(),
	}
	services, err := rt.apps.ListDesiredServices(ctx)
	if err != nil {
		return env, fmt.Errorf("list apps: %w", err)
	}
	for _, svc := range services {
		for _, d := range svc.Domains {
			env.Hosts[strings.ToLower(d)] = true
			if svc.Name != app {
				continue
			}
			if s, ok, err := rt.redirectSettings(ctx, d); err == nil && ok {
				env.Redirects[d] = s
			}
		}
	}
	if draft != nil {
		env.Redirects[domain] = *draft
	}
	if env.HostRedirects, err = rt.hostRedirects(ctx); err != nil {
		return env, err
	}
	maint, err := rt.domainMaintenance.ListDomainMaintenance(ctx)
	if err != nil {
		return env, fmt.Errorf("list maintenance: %w", err)
	}
	env.Maintenance = map[string]bool{}
	for _, d := range maint {
		env.Maintenance[d] = true
	}
	if preset != "" {
		if from, to, err := trafficpolicy.CanonicalPlan(domain, preset); err == nil {
			twin, _ := trafficpolicy.Counterpart(domain)
			env.Hosts[twin] = true
			for _, h := range []string{domain, twin} {
				if r, ok := env.HostRedirects[h]; ok && (trafficpolicy.TargetHost(r.TargetURL) == domain || trafficpolicy.TargetHost(r.TargetURL) == twin) {
					delete(env.HostRedirects, h)
				}
			}
			if from != "" {
				env.HostRedirects[from] = trafficpolicy.HostRedirect{TargetURL: "https://" + to, StatusCode: 301}
			}
		}
	}
	return env, nil
}

func effectiveHTTPS(env trafficpolicy.HopEnv, forceHTTPS bool) redirectEffective {
	e := redirectEffective{HTTPSPort: env.HTTPSPort, TLSTerminatedUpstream: env.Terminated, IngressHTTPRedirect: env.AutoHTTPS}
	switch {
	case env.Terminated && forceHTTPS:
		e.HandledBy, e.Explanation = httpsHandledHere, "Your proxy terminates TLS. This instance redirects requests your proxy marks as http (X-Forwarded-Proto, honoured only from APP_INGRESS_TRUSTED_PROXIES) to the public HTTPS port."
	case env.Terminated:
		e.HandledBy, e.Explanation = httpsHandledProxy, "Your proxy terminates TLS, so redirecting HTTP to HTTPS is up to it. Turn on force HTTPS to have this instance do it instead."
	case env.AutoHTTPS:
		e.HandledBy, e.Explanation = httpsHandledHere, "The ingress listens on HTTP and redirects every request to HTTPS with 308, for every domain."
	default:
		e.HandledBy, e.Explanation = httpsNotServed, "The ingress does not listen for plain HTTP (APP_INGRESS_HTTP_REDIRECT is off), so there is nothing to redirect."
	}
	return e
}

// sampleURLs are the three requests the Redirects tab walks through.
func sampleURLs(domain string) []string {
	apex := strings.TrimPrefix(domain, "www.")
	return []string{"http://www." + apex + "/a?x=1", "http://" + apex + "/a", "https://www." + apex + "/a"}
}

func (rt *Router) buildRedirectsResource(ctx context.Context, app, domain string) (domainRedirectsResource, error) {
	out := domainRedirectsResource{Domain: domain}
	s, found, err := rt.redirectSettings(ctx, domain)
	if err != nil {
		return out, err
	}
	out.Settings, out.Configured = s, found
	env, err := rt.hopEnv(ctx, app, domain, nil, "")
	if err != nil {
		return out, err
	}
	out.Effective = effectiveHTTPS(env, s.ForceHTTPS)
	svc, err := rt.apps.GetDesiredService(ctx, app)
	if err != nil {
		return out, fmt.Errorf("get app: %w", err)
	}
	for _, d := range svc.Domains {
		row := redirectDomainRow{Domain: d, Maintenance: env.Maintenance[d], Wildcard: ingress.IsWildcardDomain(d)}
		if r, ok := env.HostRedirects[d]; ok {
			r := r
			row.Redirect = &r
		}
		out.AppDomains = append(out.AppDomains, row)
	}
	out.Canonical = rt.canonicalState(ctx, svc, domain, env)
	for _, u := range sampleURLs(domain) {
		hops, err := trafficpolicy.SimulateHops(env, u)
		ph := policyPreviewHops{URL: u, Hops: hops}
		if err != nil {
			ph.Error = err.Error()
		}
		out.Samples = append(out.Samples, ph)
	}
	if env.Maintenance[domain] {
		out.Warnings = append(out.Warnings, "this domain is in maintenance mode, which answers 503 before any redirect")
	}
	return out, nil
}

func (rt *Router) canonicalState(ctx context.Context, svc *store.DesiredService, domain string, env trafficpolicy.HopEnv) canonicalState {
	twin, err := trafficpolicy.Counterpart(domain)
	if err != nil {
		return canonicalState{Preset: trafficpolicy.PresetServeBoth, Unavailable: err.Error()}
	}
	st := canonicalState{Counterpart: twin, Preset: trafficpolicy.PresetServeBoth}
	st.CounterpartAttached = slices.Contains(svc.Domains, twin)
	if !st.CounterpartAttached {
		if owner := rt.domainOwner(ctx, twin); owner != "" {
			st.AttachedElsewhere = owner
		}
		st.DNSHint = dnsHintFor(domain, twin)
	}
	from, _, _ := trafficpolicy.CanonicalPlan(domain, trafficpolicy.PresetWWWToApex)
	if r, ok := env.HostRedirects[from]; ok && trafficpolicy.TargetHost(r.TargetURL) != from {
		st.Preset = trafficpolicy.PresetWWWToApex
	}
	from, _, _ = trafficpolicy.CanonicalPlan(domain, trafficpolicy.PresetApexToWWW)
	if r, ok := env.HostRedirects[from]; ok && trafficpolicy.TargetHost(r.TargetURL) != from {
		st.Preset = trafficpolicy.PresetApexToWWW
	}
	return st
}

func dnsHintFor(domain, twin string) string {
	if strings.HasPrefix(twin, "www.") {
		return fmt.Sprintf("Add a CNAME record for %s pointing to %s.", twin, domain)
	}
	return fmt.Sprintf("Point %s (A and AAAA records) at the same addresses as %s; a CNAME is not allowed at the zone apex.", twin, domain)
}

func (rt *Router) domainOwner(ctx context.Context, domain string) string {
	services, err := rt.apps.ListDesiredServices(ctx)
	if err != nil {
		return ""
	}
	for _, svc := range services {
		if slices.Contains(svc.Domains, domain) {
			return svc.Name
		}
	}
	return ""
}

func (rt *Router) handleGetDomainRedirects(w http.ResponseWriter, r *http.Request) {
	domain, ok := rt.requireOwnedDomain(w, r)
	if !ok {
		return
	}
	res, err := rt.buildRedirectsResource(r.Context(), r.PathValue("name"), domain)
	if err != nil {
		rt.internalError(w, "api: domain redirects failed", err, slog.String("domain", domain))
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (rt *Router) writeRedirectsResource(w http.ResponseWriter, r *http.Request, domain string) {
	rt.nudgeReconciler()
	res, err := rt.buildRedirectsResource(r.Context(), r.PathValue("name"), domain)
	if err != nil {
		rt.internalError(w, "api: domain redirects failed", err, slog.String("domain", domain))
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (rt *Router) handleSetDomainRedirectSettings(w http.ResponseWriter, r *http.Request) {
	domain, ok := rt.requireOwnedDomain(w, r)
	if !ok {
		return
	}
	p, canonical, err := decodePolicy(r, w, trafficpolicy.KindRedirects)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := p.Redirects.Validate(); err != nil {
		writePolicyError(w, err)
		return
	}
	if err := rt.domainPolicy.store.SetDomainTrafficPolicy(r.Context(), domain, trafficpolicy.KindRedirects, canonical); err != nil {
		rt.internalError(w, "api: set redirect settings failed", err, slog.String("domain", domain))
		return
	}
	rt.writeRedirectsResource(w, r, domain)
}

func (rt *Router) handleDeleteDomainRedirectSettings(w http.ResponseWriter, r *http.Request) {
	domain, ok := rt.requireOwnedDomain(w, r)
	if !ok {
		return
	}
	if err := rt.domainPolicy.store.DeleteDomainTrafficPolicy(r.Context(), domain, trafficpolicy.KindRedirects); err != nil {
		rt.internalError(w, "api: delete redirect settings failed", err, slog.String("domain", domain))
		return
	}
	rt.writeRedirectsResource(w, r, domain)
}

type canonicalRequest struct {
	Preset     string `json:"preset"`
	StatusCode int    `json:"status_code,omitempty"`
	// Attach adds the counterpart host to the app when missing (default true).
	Attach *bool `json:"attach,omitempty"`
	// Replace overwrites an existing redirect on the non canonical host.
	Replace bool `json:"replace,omitempty"`
}

var errCanonicalConflict = errors.New("conflict")

// handleSetDomainCanonical handles POST .../redirects/canonical: www to apex,
// apex to www, or serve both. Attaching the counterpart host changes the
// app's domain list, so that part also needs AbilityWrite like PATCH
// /apps/{name}/domains.
func (rt *Router) handleSetDomainCanonical(w http.ResponseWriter, r *http.Request) {
	domain, ok := rt.requireOwnedDomain(w, r)
	if !ok {
		return
	}
	var req canonicalRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	status := req.StatusCode
	if status == 0 {
		status = store.DomainRedirectPermanent
	}
	if !trafficpolicy.AliasStatuses[status] {
		writeError(w, http.StatusBadRequest, "status_code must be 301, 302, 307 or 308")
		return
	}
	from, to, err := trafficpolicy.CanonicalPlan(domain, req.Preset)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	twin, _ := trafficpolicy.Counterpart(domain)
	ctx := r.Context()
	rows, err := rt.hostRedirects(ctx)
	if err != nil {
		rt.internalError(w, "api: canonical list redirects failed", err)
		return
	}
	app := r.PathValue("name")
	if req.Preset == trafficpolicy.PresetServeBoth {
		for _, h := range []string{domain, twin} {
			if row, ok := rows[h]; ok && (trafficpolicy.TargetHost(row.TargetURL) == domain || trafficpolicy.TargetHost(row.TargetURL) == twin) {
				if owns, _ := rt.appOwnsDomain(ctx, app, h); owns {
					if err := rt.domainRedirect.DeleteDomainRedirect(ctx, h); err != nil {
						rt.internalError(w, "api: canonical clear redirect failed", err, slog.String("domain", h))
						return
					}
				}
			}
		}
		rt.writeRedirectsResource(w, r, domain)
		return
	}
	if err := rt.ensureCounterpart(w, r, app, twin, req.Attach == nil || *req.Attach); err != nil {
		return
	}
	if existing, ok := rows[from]; ok && trafficpolicy.TargetHost(existing.TargetURL) != to && !req.Replace {
		writeError(w, http.StatusConflict, fmt.Sprintf("%s already redirects to %s; pass replace to overwrite it", from, existing.TargetURL))
		return
	}
	toRow, toRedirects := rows[to]
	if toRedirects && !req.Replace {
		writeError(w, http.StatusConflict, fmt.Sprintf("%s redirects to %s, so it cannot be the canonical host; pass replace to clear that redirect", to, toRow.TargetURL))
		return
	}
	delete(rows, to)
	if err := trafficpolicy.CheckRedirectLoop(rows, from, to); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if toRedirects {
		if err := rt.domainRedirect.DeleteDomainRedirect(ctx, to); err != nil {
			rt.internalError(w, "api: canonical clear target redirect failed", err, slog.String("domain", to))
			return
		}
	}
	if err := rt.domainRedirect.SetDomainRedirect(ctx, from, "https://"+to, status); err != nil {
		rt.internalError(w, "api: canonical set redirect failed", err, slog.String("domain", from))
		return
	}
	rt.writeRedirectsResource(w, r, domain)
}

// ensureCounterpart attaches twin to app when missing, writing the error
// response itself and returning errCanonicalConflict when it cannot.
func (rt *Router) ensureCounterpart(w http.ResponseWriter, r *http.Request, app, twin string, attach bool) error {
	ctx := r.Context()
	if owns, err := rt.appOwnsDomain(ctx, app, twin); err != nil {
		rt.internalError(w, "api: canonical owner check failed", err)
		return err
	} else if owns {
		return nil
	}
	if owner := rt.domainOwner(ctx, twin); owner != "" {
		writeError(w, http.StatusConflict, fmt.Sprintf("%s belongs to app %s; remove it there first", twin, owner))
		return errCanonicalConflict
	}
	if ingress.IsWildcardDomain(twin) {
		writeError(w, http.StatusBadRequest, "wildcard hosts cannot take part in a www setup")
		return errCanonicalConflict
	}
	if !attach {
		writeError(w, http.StatusConflict, fmt.Sprintf("%s is not attached to app %s; attach it or allow attach", twin, app))
		return errCanonicalConflict
	}
	if !rt.callerHasAbility(r, AbilityWrite) {
		writeError(w, http.StatusForbidden, fmt.Sprintf("adding %s to the app needs the write ability; add the domain first, then apply the preset", twin))
		return errCanonicalConflict
	}
	editor, ok := rt.apps.(serviceDomainsEditor)
	if !ok {
		writeError(w, http.StatusNotImplemented, "editing domains is not supported by this store")
		return errCanonicalConflict
	}
	var before []string
	next, changed, err := editor.EditServiceDomains(ctx, app, func(current []string) ([]string, error) {
		before = slices.Clone(current)
		return applyDomainEdit(current, []string{twin}, nil)
	})
	var taken *store.ErrDomainTaken
	if errors.As(err, &taken) {
		writeError(w, http.StatusConflict, taken.Error())
		return err
	}
	if err != nil {
		rt.internalError(w, "api: canonical attach counterpart failed", err, slog.String("domain", twin))
		return err
	}
	if changed {
		if ev, ok := domainChangeEvent(app, before, next); ok {
			rt.recordAppEvent(r, ev)
		}
	}
	return nil
}

type aliasesRequest struct {
	Aliases    []string `json:"aliases"`
	StatusCode int      `json:"status_code,omitempty"`
}

// handleSetDomainAliases handles PUT .../redirects/aliases: the path domain
// becomes the primary, every listed app domain redirects to it, and app
// domains no longer listed stop redirecting to it.
func (rt *Router) handleSetDomainAliases(w http.ResponseWriter, r *http.Request) {
	primary, ok := rt.requireOwnedDomain(w, r)
	if !ok {
		return
	}
	var req aliasesRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	status := req.StatusCode
	if status == 0 {
		status = store.DomainRedirectPermanent
	}
	if !trafficpolicy.AliasStatuses[status] {
		writeError(w, http.StatusBadRequest, "status_code must be 301, 302, 307 or 308")
		return
	}
	ctx := r.Context()
	app := r.PathValue("name")
	svc, err := rt.apps.GetDesiredService(ctx, app)
	if err != nil {
		rt.internalError(w, "api: aliases get app failed", err)
		return
	}
	rows, err := rt.hostRedirects(ctx)
	if err != nil {
		rt.internalError(w, "api: aliases list redirects failed", err)
		return
	}
	if _, primaryRedirects := rows[primary]; primaryRedirects {
		writeError(w, http.StatusConflict, fmt.Sprintf("%s redirects elsewhere itself; clear its redirect before making it the primary", primary))
		return
	}
	want := map[string]bool{}
	for _, a := range req.Aliases {
		a = strings.ToLower(strings.TrimSpace(a))
		switch {
		case a == primary:
			writeError(w, http.StatusBadRequest, "the primary domain cannot be its own alias")
			return
		case !slices.Contains(svc.Domains, a):
			writeError(w, http.StatusBadRequest, fmt.Sprintf("%s is not one of app %s's domains", a, app))
			return
		case ingress.IsWildcardDomain(a):
			writeError(w, http.StatusBadRequest, fmt.Sprintf("%s is a wildcard host and cannot redirect as a whole", a))
			return
		}
		want[a] = true
	}
	for a := range want {
		delete(rows, a)
	}
	for a := range want {
		if err := trafficpolicy.CheckRedirectLoop(rows, a, primary); err != nil {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
	}
	current, _ := rt.hostRedirects(ctx)
	for _, d := range svc.Domains {
		row, redirected := current[d]
		switch {
		case want[d]:
			if err := rt.domainRedirect.SetDomainRedirect(ctx, d, "https://"+primary, status); err != nil {
				rt.internalError(w, "api: set alias redirect failed", err, slog.String("domain", d))
				return
			}
		case redirected && trafficpolicy.TargetHost(row.TargetURL) == primary:
			if err := rt.domainRedirect.DeleteDomainRedirect(ctx, d); err != nil {
				rt.internalError(w, "api: clear alias redirect failed", err, slog.String("domain", d))
				return
			}
		}
	}
	rt.writeRedirectsResource(w, r, primary)
}

package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/mail"
	"strings"

	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/store"
)

// IngressSettingsStore is the store surface the ingress settings
// handlers need. *store.DB satisfies this structurally, the same
// consumer-defined interface convention every other Store sub-interface
// in this package follows (see CertStore, StaticSiteStore).
type IngressSettingsStore interface {
	GetIngressSettings(ctx context.Context) (store.IngressSettings, error)
	UpdateIngressSettings(ctx context.Context, s store.IngressSettings) error
	GetDashboardURL(ctx context.Context) (string, error)
	SetDashboardURL(ctx context.Context, u string) error
}

// ingressSettingsResource is the wire shape for GET and PUT
// /api/v1/settings/ingress, mirroring store.IngressSettings field for
// field. ACMEEmail is deliberately still included on the GET response
// (unlike, say, a backup target's credentials): it isn't a secret, an
// ACME account contact address is no more sensitive than any other
// operator-entered configuration value, and the settings form needs to
// redisplay it on load the same way DomainEditor redisplays an app's
// existing domains. See handleUpdateIngressSettings's own doc comment
// for the one thing this package does treat carefully around it: never
// logging it above debug.
type ingressSettingsResource struct {
	PrimaryDomain    string `json:"primary_domain,omitempty"`
	ACMEEnabled      bool   `json:"acme_enabled"`
	ACMEEmail        string `json:"acme_email,omitempty"`
	ACMEDirectoryURL string `json:"acme_directory_url,omitempty"`
	HSTSEnabled      bool   `json:"hsts_enabled"`
	// FallbackDomainsEnabled is the automatic <app>.<dashed-ip>.sslip.io
	// hostname toggle (see ingressSettingsUpdate for PUT semantics).
	FallbackDomainsEnabled bool `json:"fallback_domains_enabled"`
	// PublicHost and PublicHostSource are read-only: the address apps are
	// reachable at and how it was found (env, detected, disabled, none).
	PublicHost       string `json:"public_host,omitempty"`
	PublicHostSource string `json:"public_host_source,omitempty"`
	// SuggestedACMEEmail is the first admin's address, offered as the form
	// default only; it is never sent to the CA until the operator saves it.
	SuggestedACMEEmail string `json:"suggested_acme_email,omitempty"`
	// ACMEBlocked is set when real certificates are on but cannot succeed
	// here: certificate authorities validate on ports 80 and 443, and this
	// ingress listens elsewhere with no DNS-01 provider to fall back on.
	ACMEBlocked string `json:"acme_blocked,omitempty"`
	// IngressHTTPPort and IngressHTTPSPort are the ports this ingress listens on.
	IngressHTTPPort  int `json:"ingress_http_port,omitempty"`
	IngressHTTPSPort int `json:"ingress_https_port,omitempty"`
}

// acmeBlockedNonStandardPorts is the ACMEBlocked value for an ingress that
// does not listen on 80 and 443.
const acmeBlockedNonStandardPorts = "non_standard_ports"

func (rt *Router) toIngressSettingsResource(s store.IngressSettings) ingressSettingsResource {
	res := toIngressSettingsResource(s)
	res.PublicHost = rt.publicHost
	res.PublicHostSource = rt.publicHostSource
	httpPort, httpsPort := rt.doctorHTTPPort, rt.doctorHTTPSPort
	if httpPort == 0 {
		httpPort = defaultDoctorHTTPPort
	}
	if httpsPort == 0 {
		httpsPort = defaultDoctorHTTPSPort
	}
	if s.ACMEEnabled && (httpPort != defaultDoctorHTTPPort || httpsPort != defaultDoctorHTTPSPort) &&
		rt.activeDNSProvider(context.Background()) == dnsProviderNone {
		res.ACMEBlocked = acmeBlockedNonStandardPorts
		res.IngressHTTPPort, res.IngressHTTPSPort = httpPort, httpsPort
	}
	return res
}

func toIngressSettingsResource(s store.IngressSettings) ingressSettingsResource {
	return ingressSettingsResource{
		PrimaryDomain:    s.PrimaryDomain,
		ACMEEnabled:      s.ACMEEnabled,
		ACMEEmail:        s.ACMEEmail,
		ACMEDirectoryURL: s.ACMEDirectoryURL,
		HSTSEnabled:      s.HSTSEnabled,

		FallbackDomainsEnabled: !s.FallbackDomainsDisabled,
	}
}

// ingressSettingsUpdate is the PUT body: the resource plus a pointer shadow
// of the toggle, so a client that omits it leaves the stored value alone.
type ingressSettingsUpdate struct {
	ingressSettingsResource
	FallbackDomainsEnabled *bool `json:"fallback_domains_enabled"`
}

// handleGetIngressSettings handles GET /api/v1/settings/ingress: the
// single platform-wide row (migrations/0024_ingress_settings.sql),
// always present, never a 404. A control plane that has never had this
// page visited returns the seeded default (ACME disabled, no primary
// domain), not an error, the same "absence is a real, reportable state"
// shape handleListCertificates already establishes for an empty list.
func (rt *Router) handleGetIngressSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := rt.ingressSettings.GetIngressSettings(r.Context())
	if err != nil {
		rt.logger.Error("api: get ingress settings failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	res := rt.toIngressSettingsResource(settings)
	if settings.ACMEEmail == "" {
		res.SuggestedACMEEmail = rt.firstAdminAddress(r.Context())
	}
	writeJSON(w, http.StatusOK, res)
}

// firstAdminAddress returns the first user's email when it is a real address, else "".
func (rt *Router) firstAdminAddress(ctx context.Context) string {
	users, err := rt.auth.ListUsers(ctx)
	if err != nil {
		return ""
	}
	for _, u := range users {
		if !u.IsFirstUser {
			continue
		}
		if a, err := mail.ParseAddress(u.Email); err == nil && a.Address == u.Email {
			return u.Email
		}
	}
	return ""
}

// errACMEEmailRequired and errACMEEmailInvalid are validateIngressSettingsRequest's
// two rejection reasons, named so handleUpdateIngressSettings's test
// coverage can assert on which one fired rather than string-matching a
// message.
var (
	errACMEEmailRequired = errors.New("acme_email is required when acme_enabled is true")
	errACMEEmailInvalid  = errors.New("acme_email is not a valid email address")
)

// validateIngressSettingsRequest enforces the one real business rule
// this endpoint owns: a syntactically valid, non-empty ACME contact
// address whenever ACMEEnabled is true, per Let's Encrypt's own ACME
// account model (an unreachable account is exactly the kind of
// "renewal fails silently at 3am" risk this project treats as its
// central one to catch before it bites a real user). Uses the standard
// library's net/mail parser rather than a hand-rolled regex or a new
// dependency, matching the project rule against pulling in a package
// for something the standard library already does correctly.
//
// ParseAddress alone accepts display-name forms like "Ops <a@b.com>",
// which certmagic's account registration concatenates verbatim into
// "mailto:"+email, producing an invalid URI and failing ACME account
// setup silently. Requiring the parsed address to equal the input
// rejects those.
//
// PrimaryDomain and ACMEDirectoryURL are deliberately unvalidated here:
// an empty PrimaryDomain is a real, valid "no dashboard route" state
// (see internal/reconcile/ingress's own WithDashboardDial doc comment),
// and ACMEDirectoryURL empty is the equally valid "use Caddy's own
// default" state (internal/ingress.ACMEIssuer.CA's own doc comment); a
// malformed directory URL is Caddy's own problem to surface, at apply
// time, the same way a malformed domain would surface as a routing
// failure rather than a settings-save failure.
//
// PrimaryDomain's domain-conflict check lives in handleUpdateIngressSettings
// instead, which has the store access this pure function doesn't.
func validateIngressSettingsRequest(req ingressSettingsResource) error {
	if !req.ACMEEnabled {
		return nil
	}
	email := strings.TrimSpace(req.ACMEEmail)
	if email == "" {
		return errACMEEmailRequired
	}
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email {
		return errACMEEmailInvalid
	}
	return nil
}

// handleUpdateIngressSettings handles PUT /api/v1/settings/ingress: the
// single most blast-radius-heavy settings change this control plane
// exposes short of node management itself, since ACMEEnabled changes
// what every currently-routed host's certificate automation does on the
// very next ingress reconcile pass (internal/reconcile/ingress reads
// this row fresh every call). Gated at AbilityRoot in router.go, the
// same tier handleDrainNode/handleSetAppNode/POST-system-prune already
// use for "real infrastructure, not an ordinary per-app write" changes;
// see router.go's own route registration comment for the precedent this
// follows.
//
// Neither this handler nor validateIngressSettingsRequest ever logs
// req.ACMEEmail: only the generic "internal error" string and the error
// classification (invalid/missing) are logged or returned, matching this
// codebase's existing discipline around operator-entered values that
// aren't secrets but still don't belong in a log line (see
// DomainEditor's frontend-side equivalent: a domain is shown in the UI
// but never appears in a console.log).
func (rt *Router) handleUpdateIngressSettings(w http.ResponseWriter, r *http.Request) {
	var upd ingressSettingsUpdate
	if err := json.NewDecoder(r.Body).Decode(&upd); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req := upd.ingressSettingsResource
	if err := validateIngressSettingsRequest(req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	primaryDomain := strings.TrimSpace(req.PrimaryDomain)
	if primaryDomain != "" {
		owner, err := rt.primaryDomainOwner(r.Context(), primaryDomain)
		if err != nil {
			rt.internalError(w, "api: update ingress settings: check domain conflict", err)
			return
		}
		if owner != "" {
			taken := &store.ErrDomainTaken{Domain: primaryDomain, Owner: owner}
			writeError(w, http.StatusConflict, taken.Error())
			return
		}
	}

	settings := store.IngressSettings{
		PrimaryDomain:    primaryDomain,
		ACMEEnabled:      req.ACMEEnabled,
		ACMEEmail:        strings.TrimSpace(req.ACMEEmail),
		ACMEDirectoryURL: strings.TrimSpace(req.ACMEDirectoryURL),
		HSTSEnabled:      req.HSTSEnabled,
	}
	prev, prevErr := rt.ingressSettings.GetIngressSettings(r.Context())
	if upd.FallbackDomainsEnabled != nil {
		settings.FallbackDomainsDisabled = !*upd.FallbackDomainsEnabled
	} else if prevErr == nil {
		settings.FallbackDomainsDisabled = prev.FallbackDomainsDisabled
	}
	if prevErr == nil {
		rt.purgeOnIssuerChange(r.Context(), prev, settings)
	}
	if err := rt.ingressSettings.UpdateIngressSettings(r.Context(), settings); err != nil {
		rt.logger.Error("api: update ingress settings failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	rt.hstsDBEnabled.Store(settings.HSTSEnabled)
	rt.nudgeReconciler()
	writeJSON(w, http.StatusOK, rt.toIngressSettingsResource(settings))
}

// primaryDomainOwner reports the service name already claiming domain
// in service_domains, or "" if no app owns it. Case-insensitive: domains
// reach service_domains however the creating app.yaml or dashboard form
// spelled them (DesiredService domains aren't lowercased at the store
// layer, unlike app_domains.go's own normalizeDomainList path), so an
// exact-match comparison here would miss a same-domain, different-case
// collision that still produces two Caddy routes for the same Host
// header once lowercased by the browser and by Caddy's own matcher.
func (rt *Router) primaryDomainOwner(ctx context.Context, domain string) (string, error) {
	domains, err := rt.domains.ListServiceDomains(ctx)
	if err != nil {
		return "", err
	}
	want := strings.ToLower(domain)
	for _, d := range domains {
		if strings.ToLower(d.Domain) == want {
			return d.ServiceName, nil
		}
	}
	return "", nil
}

// hstsEnabledFromDB reports ingress_settings.hsts_enabled, Router.hstsDBEnabled's
// own cached value. The first call in the process's lifetime loads it from
// the database (hstsDBLoaded); handleUpdateIngressSettings keeps it fresh
// after that, so this never blocks a request on a database read past the
// very first one.
func (rt *Router) hstsEnabledFromDB(ctx context.Context) bool {
	rt.hstsDBLoaded.Do(func() {
		settings, err := rt.ingressSettings.GetIngressSettings(ctx)
		if err != nil {
			rt.logger.Error("api: load initial hsts setting failed", slog.String("error", err.Error()))
			return
		}
		rt.hstsDBEnabled.Store(settings.HSTSEnabled)
	})
	return rt.hstsDBEnabled.Load()
}

// ingressDomainCheckResponse is GET /api/v1/settings/ingress/check's wire
// shape: domainCheckResponse's own fields, reused verbatim so the
// frontend's existing per-app rendering logic (DomainDnsCheck.tsx)
// applies unchanged, plus Configured for the "no primary domain set yet"
// state that has no domain to check at all.
type ingressDomainCheckResponse struct {
	Configured bool `json:"configured"`
	domainCheckResponse
}

// handleCheckIngressDomain handles GET /api/v1/settings/ingress/check:
// runDomainCheck (domain_check.go) against the platform's own
// PrimaryDomain instead of a per-app one, the same DNS-check visibility
// DomainEditor already gives an operator per app, extended to the
// dashboard's own domain. AbilityRoot, matching this file's other
// /api/v1/settings/ingress handlers, since the check result reveals
// whether ACMEEnabled (also AbilityRoot) can actually succeed.
func (rt *Router) handleCheckIngressDomain(w http.ResponseWriter, r *http.Request) {
	settings, err := rt.ingressSettings.GetIngressSettings(r.Context())
	if err != nil {
		rt.logger.Error("api: check ingress domain: get settings failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	domain := strings.ToLower(strings.TrimSpace(settings.PrimaryDomain))
	if domain == "" {
		writeJSON(w, http.StatusOK, ingressDomainCheckResponse{Configured: false})
		return
	}

	expectedHost, inferred := advertisedHost(r, rt.publicHost)
	resp := rt.runDomainCheck(r.Context(), domain, expectedHost, inferred)
	rt.enrichDomainCheck(r.Context(), &resp)
	writeJSON(w, http.StatusOK, ingressDomainCheckResponse{Configured: true, domainCheckResponse: resp})
}

// DomainStore is the store surface GET /api/v1/domains needs: every
// domain currently claimed by a service, for the centralized cross-app
// domains list (web/src/routes/domains). Reuses internal/store's
// existing service_domains table (kept in sync by SaveDesiredService on
// every write, see that method's own doc comment); this is a plain read
// of already-tracked state, not a new tracking mechanism.
type DomainStore interface {
	ListServiceDomains(ctx context.Context) ([]store.ServiceDomain, error)
}

// domainResource is one row of GET /api/v1/domains. The four status
// flags are read-only visibility into per-domain settings that are
// otherwise only configurable from the owning app's own Domains tab
// (DomainEditor.tsx): this page stays deliberately read-only, see
// DomainRow.tsx's own doc comment for why.
type domainResource struct {
	Domain             string `json:"domain"`
	ServiceName        string `json:"service_name"`
	WAFEnabled         bool   `json:"waf_enabled"`
	HasRedirect        bool   `json:"has_redirect"`
	MaintenanceEnabled bool   `json:"maintenance_enabled"`
	HasBasicAuth       bool   `json:"has_basic_auth"`
	// Automatic marks a generated <app>.<dashed-ip>.sslip.io hostname: it
	// is routed but not stored on the app, so no per-domain feature applies.
	Automatic bool `json:"automatic,omitempty"`
	// ACMEFailure is the CA's last error for this hostname, so a domain
	// stuck without a certificate shows why on the list.
	ACMEFailure *acmeFailureResource `json:"acme_failure,omitempty"`
}

// handleListDomains handles GET /api/v1/domains: every service_domains
// row, for the centralized domains page (web/src/routes/domains) to
// merge client-side with GET /api/v1/certificates by domain string.
// AbilityRead, same as GET /api/v1/apps: no new ability tier, this is a
// plain read of data every app-domain edit through DomainEditor already
// exposes per-app, just aggregated across every app in one call.
//
// The four status flags are populated from the same List* bulk reads
// the ingress reconciler already uses (internal/reconcile/ingress),
// each one queried once here rather than per domain, so this stays a
// fixed number of queries regardless of how many domains exist.
func (rt *Router) handleListDomains(w http.ResponseWriter, r *http.Request) {
	domains, err := rt.domains.ListServiceDomains(r.Context())
	if err != nil {
		rt.logger.Error("api: list domains failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	canSee, err := rt.appVisibilityFilter(r)
	if err != nil {
		rt.internalError(w, "api: list domains: visibility", err)
		return
	}

	wafEnabled, ok := fetchDomainStatusSet(w, rt, r, "waf", rt.domainWAF.ListDomainWAF,
		func(d store.DomainWAF) string {
			if d.WAFEnabled {
				return d.Domain
			}
			return ""
		})
	if !ok {
		return
	}
	hasRedirect, ok := fetchDomainStatusSet(w, rt, r, "redirects", rt.domainRedirect.ListDomainRedirects,
		func(d store.DomainRedirect) string { return d.Domain })
	if !ok {
		return
	}
	inMaintenance, ok := fetchDomainStatusSet(w, rt, r, "maintenance", rt.domainMaintenance.ListDomainMaintenance,
		func(domain string) string { return domain })
	if !ok {
		return
	}
	hasBasicAuth, ok := fetchDomainStatusSet(w, rt, r, "basic auth", rt.domainBasicAuth.ListDomainBasicAuth,
		func(d store.DomainBasicAuth) string { return d.Domain })
	if !ok {
		return
	}

	out := make([]domainResource, 0, len(domains))
	for _, d := range domains {
		if !canSee(d.ServiceName) {
			continue
		}
		out = append(out, domainResource{
			Domain:             d.Domain,
			ServiceName:        d.ServiceName,
			WAFEnabled:         wafEnabled[d.Domain],
			HasRedirect:        hasRedirect[d.Domain],
			MaintenanceEnabled: inMaintenance[d.Domain],
			HasBasicAuth:       hasBasicAuth[d.Domain],
			ACMEFailure:        acmeFailureFor(d.Domain),
		})
	}
	out = append(out, rt.automaticDomains(r, canSee)...)
	writeJSON(w, http.StatusOK, out)
}

// automaticDomains lists the generated sslip.io hostnames every app without
// a domain is routed under. Best effort: a store error just omits them.
func (rt *Router) automaticDomains(r *http.Request, canSee func(string) bool) []domainResource {
	settings, err := rt.ingressSettings.GetIngressSettings(r.Context())
	if err != nil || settings.FallbackDomainsDisabled {
		return nil
	}
	if _, ok := ingress.SSLIPHost(rt.publicHost); !ok {
		return nil
	}
	svcs, err := rt.apps.ListDesiredServices(r.Context())
	if err != nil {
		rt.logger.Warn("api: list domains: automatic hostnames skipped", slog.String("error", err.Error()))
		return nil
	}
	var out []domainResource
	for _, svc := range svcs {
		if len(svc.Domains) > 0 || !canSee(svc.Name) {
			continue
		}
		if d, ok := ingress.FallbackDomain(rt.publicHost, svc.Name); ok {
			out = append(out, domainResource{Domain: d, ServiceName: svc.Name, Automatic: true})
		}
	}
	return out
}

// fetchDomainStatusSet fetches a per-domain status list and reduces it
// to a set of domain names, so handleListDomains's four status flags
// share one fetch-error-handle-reduce path instead of four near-identical
// copies. domainOf returns "" for a row that shouldn't count (e.g. a WAF
// row present but not actually enabled), which the empty-key check below
// skips. ok is false only after an error response has already been
// written, so every caller's own handling is just `if !ok { return }`.
func fetchDomainStatusSet[T any](
	w http.ResponseWriter,
	rt *Router,
	r *http.Request,
	label string,
	fetch func(context.Context) ([]T, error),
	domainOf func(T) string,
) (set map[string]bool, ok bool) {
	items, err := fetch(r.Context())
	if err != nil {
		rt.internalError(w, "api: list domains: "+label, err)
		return nil, false
	}
	set = make(map[string]bool, len(items))
	for _, item := range items {
		if domain := domainOf(item); domain != "" {
			set[domain] = true
		}
	}
	return set, true
}

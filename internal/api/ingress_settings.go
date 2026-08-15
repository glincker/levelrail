package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/mail"
	"strings"

	"github.com/GLINCKER/levelrail/internal/store"
)

// IngressSettingsStore is the store surface the ingress settings
// handlers need. *store.DB satisfies this structurally, the same
// consumer-defined interface convention every other Store sub-interface
// in this package follows (see CertStore, StaticSiteStore).
type IngressSettingsStore interface {
	GetIngressSettings(ctx context.Context) (store.IngressSettings, error)
	UpdateIngressSettings(ctx context.Context, s store.IngressSettings) error
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
}

func toIngressSettingsResource(s store.IngressSettings) ingressSettingsResource {
	return ingressSettingsResource{
		PrimaryDomain:    s.PrimaryDomain,
		ACMEEnabled:      s.ACMEEnabled,
		ACMEEmail:        s.ACMEEmail,
		ACMEDirectoryURL: s.ACMEDirectoryURL,
	}
}

// handleGetIngressSettings handles GET /api/v1/settings/ingress: the
// single platform-wide row (migrations/0023_ingress_settings.sql),
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
	writeJSON(w, http.StatusOK, toIngressSettingsResource(settings))
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
// PrimaryDomain and ACMEDirectoryURL are deliberately unvalidated here:
// an empty PrimaryDomain is a real, valid "no dashboard route" state
// (see internal/reconcile/ingress's own WithDashboardDial doc comment),
// and ACMEDirectoryURL empty is the equally valid "use Caddy's own
// default" state (internal/ingress.ACMEIssuer.CA's own doc comment); a
// malformed directory URL is Caddy's own problem to surface, at apply
// time, the same way a malformed domain would surface as a routing
// failure rather than a settings-save failure.
func validateIngressSettingsRequest(req ingressSettingsResource) error {
	if !req.ACMEEnabled {
		return nil
	}
	email := strings.TrimSpace(req.ACMEEmail)
	if email == "" {
		return errACMEEmailRequired
	}
	if _, err := mail.ParseAddress(email); err != nil {
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
	var req ingressSettingsResource
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := validateIngressSettingsRequest(req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	settings := store.IngressSettings{
		PrimaryDomain:    strings.TrimSpace(req.PrimaryDomain),
		ACMEEnabled:      req.ACMEEnabled,
		ACMEEmail:        strings.TrimSpace(req.ACMEEmail),
		ACMEDirectoryURL: strings.TrimSpace(req.ACMEDirectoryURL),
	}
	if err := rt.ingressSettings.UpdateIngressSettings(r.Context(), settings); err != nil {
		rt.logger.Error("api: update ingress settings failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, toIngressSettingsResource(settings))
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

// domainResource is one row of GET /api/v1/domains.
type domainResource struct {
	Domain      string `json:"domain"`
	ServiceName string `json:"service_name"`
}

// handleListDomains handles GET /api/v1/domains: every service_domains
// row, for the centralized domains page (web/src/routes/domains) to
// merge client-side with GET /api/v1/certificates by domain string.
// AbilityRead, same as GET /api/v1/apps: no new ability tier, this is a
// plain read of data every app-domain edit through DomainEditor already
// exposes per-app, just aggregated across every app in one call.
func (rt *Router) handleListDomains(w http.ResponseWriter, r *http.Request) {
	domains, err := rt.domains.ListServiceDomains(r.Context())
	if err != nil {
		rt.logger.Error("api: list domains failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	out := make([]domainResource, 0, len(domains))
	for _, d := range domains {
		out = append(out, domainResource{Domain: d.Domain, ServiceName: d.ServiceName})
	}
	writeJSON(w, http.StatusOK, out)
}

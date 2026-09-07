package api

import (
	"context"
	"log/slog"
	"net/http"
)

// DomainMaintenanceStore is the store surface GET/PUT/DELETE
// .../domains/{domain}/maintenance need: whether a domain currently has
// maintenance mode enabled, always set, same "core Store interface"
// shape as DomainBasicAuthStore above.
type DomainMaintenanceStore interface {
	GetDomainMaintenance(ctx context.Context, domain string) (bool, error)
	SetDomainMaintenance(ctx context.Context, domain string) error
	DeleteDomainMaintenance(ctx context.Context, domain string) error
}

// domainMaintenanceResource is the wire shape for GET/PUT/DELETE
// .../domains/{domain}/maintenance.
type domainMaintenanceResource struct {
	Domain  string `json:"domain"`
	Enabled bool   `json:"enabled"`
}

// handleGetDomainMaintenance handles GET
// /api/v1/apps/{name}/domains/{domain}/maintenance: current
// maintenance-mode state for domain. AbilityRead, the same
// passive-visibility tier GET .../domains/{domain}/auth already uses.
func (rt *Router) handleGetDomainMaintenance(w http.ResponseWriter, r *http.Request) {
	domain, ok := rt.requireOwnedDomain(w, r)
	if !ok {
		return
	}

	enabled, err := rt.domainMaintenance.GetDomainMaintenance(r.Context(), domain)
	if err != nil {
		rt.logger.Error("api: get domain maintenance failed", slog.String("error", err.Error()), slog.String("domain", domain))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, domainMaintenanceResource{Domain: domain, Enabled: enabled})
}

// handleSetDomainMaintenance handles PUT
// /api/v1/apps/{name}/domains/{domain}/maintenance: enables maintenance
// mode on domain, enforced by Caddy on the next ingress reconcile pass.
// AbilityDeploy, matching POST .../stop and .../start: this changes an
// app's runtime routing behavior, the same "app lifecycle" tier those
// two actions already use, not AbilityRoot's "real infrastructure,
// security-sensitive" tier .../auth reserves for a credential-bearing
// change.
func (rt *Router) handleSetDomainMaintenance(w http.ResponseWriter, r *http.Request) {
	domain, ok := rt.requireOwnedDomain(w, r)
	if !ok {
		return
	}

	if err := rt.domainMaintenance.SetDomainMaintenance(r.Context(), domain); err != nil {
		rt.logger.Error("api: set domain maintenance failed", slog.String("error", err.Error()), slog.String("domain", domain))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, domainMaintenanceResource{Domain: domain, Enabled: true})
}

// handleClearDomainMaintenance handles DELETE
// /api/v1/apps/{name}/domains/{domain}/maintenance: disables
// maintenance mode on domain, taking effect on the next ingress
// reconcile pass. AbilityDeploy. Idempotent: clearing a domain with
// none configured is not an error.
func (rt *Router) handleClearDomainMaintenance(w http.ResponseWriter, r *http.Request) {
	domain, ok := rt.requireOwnedDomain(w, r)
	if !ok {
		return
	}

	if err := rt.domainMaintenance.DeleteDomainMaintenance(r.Context(), domain); err != nil {
		rt.logger.Error("api: clear domain maintenance failed", slog.String("error", err.Error()), slog.String("domain", domain))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, domainMaintenanceResource{Domain: domain, Enabled: false})
}

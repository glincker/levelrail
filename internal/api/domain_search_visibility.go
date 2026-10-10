package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
)

// DomainSearchVisibilityStore is the store surface for
// .../domains/{domain}/search-visibility.
type DomainSearchVisibilityStore interface {
	IsDomainHidden(ctx context.Context, domain string) (bool, error)
	SetDomainHidden(ctx context.Context, domain string, hidden bool) error
}

type domainSearchVisibilityResource struct {
	Domain string `json:"domain"`
	Hidden bool   `json:"hidden"`
}

type setDomainSearchVisibilityRequest struct {
	Hidden *bool `json:"hidden"`
}

// handleGetDomainSearchVisibility handles GET
// /api/v1/apps/{name}/domains/{domain}/search-visibility.
func (rt *Router) handleGetDomainSearchVisibility(w http.ResponseWriter, r *http.Request) {
	domain, ok := rt.requireOwnedDomain(w, r)
	if !ok {
		return
	}
	hidden, err := rt.domainSearchVisibility.IsDomainHidden(r.Context(), domain)
	if err != nil {
		rt.logger.Error("api: read domain search visibility failed", slog.String("error", err.Error()), slog.String("domain", domain))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, domainSearchVisibilityResource{Domain: domain, Hidden: hidden})
}

// handleSetDomainSearchVisibility handles PUT
// /api/v1/apps/{name}/domains/{domain}/search-visibility with {"hidden": bool}.
// Takes effect on the next ingress reconcile pass. AbilityDeploy, like the
// other per-domain routing settings.
func (rt *Router) handleSetDomainSearchVisibility(w http.ResponseWriter, r *http.Request) {
	domain, ok := rt.requireOwnedDomain(w, r)
	if !ok {
		return
	}
	var req setDomainSearchVisibilityRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Hidden == nil {
		writeError(w, http.StatusBadRequest, "hidden (true or false) is required")
		return
	}
	if err := rt.domainSearchVisibility.SetDomainHidden(r.Context(), domain, *req.Hidden); err != nil {
		rt.logger.Error("api: set domain search visibility failed", slog.String("error", err.Error()), slog.String("domain", domain))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, domainSearchVisibilityResource{Domain: domain, Hidden: *req.Hidden})
}

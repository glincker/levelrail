package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/GLINCKER/levelrail/internal/store"
)

// DomainRedirectStore is the store surface GET/PUT/DELETE
// .../domains/{domain}/redirect need: an opt-in redirect to an arbitrary
// target URL for a domain, always set, same "core Store interface"
// shape as DomainMaintenanceStore above; like that store, there is no
// secrets dependency at all, since neither the target URL nor the
// status code is credential material.
type DomainRedirectStore interface {
	GetDomainRedirect(ctx context.Context, domain string) (store.DomainRedirect, bool, error)
	SetDomainRedirect(ctx context.Context, domain, targetURL string, statusCode int) error
	DeleteDomainRedirect(ctx context.Context, domain string) error
}

// domainRedirectResource is the wire shape for GET/PUT/DELETE
// .../domains/{domain}/redirect. Enabled is derived (a row exists)
// rather than its own stored column, the same shape
// domainMaintenanceResource's Enabled field already establishes.
type domainRedirectResource struct {
	Domain     string `json:"domain"`
	Enabled    bool   `json:"enabled"`
	TargetURL  string `json:"target_url,omitempty"`
	StatusCode int    `json:"status_code"`
}

func toDomainRedirectResource(domain string, row store.DomainRedirect, found bool) domainRedirectResource {
	statusCode := row.StatusCode
	if statusCode == 0 {
		statusCode = store.DomainRedirectPermanent
	}
	return domainRedirectResource{
		Domain:     domain,
		Enabled:    found,
		TargetURL:  row.TargetURL,
		StatusCode: statusCode,
	}
}

// setDomainRedirectRequest is PUT .../redirect's request body. StatusCode
// 0 (the zero value, omitted or explicitly 0) defaults to
// store.DomainRedirectPermanent (301), matching Caddy's own default
// redirect status.
type setDomainRedirectRequest struct {
	TargetURL  string `json:"target_url"`
	StatusCode int    `json:"status_code,omitempty"`
}

// validateRedirectTargetURL requires an absolute http(s) URL with a
// non-empty host: a redirect target must be a real, reachable location a
// browser can be sent to, not a relative path or a bare hostname a
// caller forgot the scheme on.
func validateRedirectTargetURL(raw string) error {
	if raw == "" {
		return errors.New("target_url is required")
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return errors.New("target_url must be an absolute http(s) URL, e.g. https://example.com")
	}
	return nil
}

// handleGetDomainRedirect handles GET
// /api/v1/apps/{name}/domains/{domain}/redirect: current redirect
// configuration for domain. AbilityRead, the same passive-visibility
// tier GET .../domains/{domain}/maintenance already uses.
func (rt *Router) handleGetDomainRedirect(w http.ResponseWriter, r *http.Request) {
	domain, ok := rt.requireOwnedDomain(w, r)
	if !ok {
		return
	}

	row, found, err := rt.domainRedirect.GetDomainRedirect(r.Context(), domain)
	if err != nil {
		rt.logger.Error("api: get domain redirect failed", slog.String("error", err.Error()), slog.String("domain", domain))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, toDomainRedirectResource(domain, row, found))
}

// handleSetDomainRedirect handles PUT
// /api/v1/apps/{name}/domains/{domain}/redirect: configures domain to
// redirect to a target URL, enforced by Caddy on the next ingress
// reconcile pass. AbilityDeploy, matching PUT .../domains/{domain}/
// maintenance: this changes an app's runtime routing behavior, not a
// credential, so it doesn't need AbilityRoot's higher bar.
func (rt *Router) handleSetDomainRedirect(w http.ResponseWriter, r *http.Request) {
	domain, ok := rt.requireOwnedDomain(w, r)
	if !ok {
		return
	}

	var req setDomainRedirectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := validateRedirectTargetURL(req.TargetURL); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	statusCode := req.StatusCode
	if statusCode == 0 {
		statusCode = store.DomainRedirectPermanent
	}
	if statusCode != store.DomainRedirectPermanent && statusCode != store.DomainRedirectTemporary {
		writeError(w, http.StatusBadRequest, "status_code must be 301 or 302")
		return
	}

	if err := rt.domainRedirect.SetDomainRedirect(r.Context(), domain, req.TargetURL, statusCode); err != nil {
		rt.logger.Error("api: set domain redirect failed", slog.String("error", err.Error()), slog.String("domain", domain))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	row, found, err := rt.domainRedirect.GetDomainRedirect(r.Context(), domain)
	if err != nil {
		rt.logger.Error("api: get domain redirect failed", slog.String("error", err.Error()), slog.String("domain", domain))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, toDomainRedirectResource(domain, row, found))
}

// handleClearDomainRedirect handles DELETE
// /api/v1/apps/{name}/domains/{domain}/redirect: removes domain's
// redirect, taking effect on the next ingress reconcile pass.
// AbilityDeploy. Idempotent: clearing a domain with none configured is
// not an error.
func (rt *Router) handleClearDomainRedirect(w http.ResponseWriter, r *http.Request) {
	domain, ok := rt.requireOwnedDomain(w, r)
	if !ok {
		return
	}

	if err := rt.domainRedirect.DeleteDomainRedirect(r.Context(), domain); err != nil {
		rt.logger.Error("api: clear domain redirect failed", slog.String("error", err.Error()), slog.String("domain", domain))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, toDomainRedirectResource(domain, store.DomainRedirect{}, false))
}

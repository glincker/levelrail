package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/store"
)

// DomainWAFStore is the store surface GET/PUT/DELETE
// .../domains/{domain}/waf need: opt-in WAF and rate-limit settings for
// a domain, always set, same "core Store interface" shape as
// DomainMaintenanceStore above; like that store, there is no secrets
// dependency at all, since neither setting involves credential material.
type DomainWAFStore interface {
	GetDomainWAF(ctx context.Context, domain string) (store.DomainWAF, bool, error)
	SetDomainWAF(ctx context.Context, domain string, wafEnabled bool, mode string, rateLimitRPS, rateLimitBurst int) error
	DeleteDomainWAF(ctx context.Context, domain string) error
}

// domainWAFResource is the wire shape for GET/PUT/DELETE
// .../domains/{domain}/waf. RateLimitEnabled is derived (RateLimitRPS >
// 0) rather than its own stored column: it exists only so the frontend
// doesn't need to know "0 means off" is this field's convention.
type domainWAFResource struct {
	Domain           string `json:"domain"`
	WAFEnabled       bool   `json:"waf_enabled"`
	WAFMode          string `json:"waf_mode"`
	RateLimitEnabled bool   `json:"rate_limit_enabled"`
	RateLimitRPS     int    `json:"rate_limit_rps"`
	RateLimitBurst   int    `json:"rate_limit_burst"`
}

func toDomainWAFResource(domain string, row store.DomainWAF) domainWAFResource {
	mode := row.WAFMode
	if mode == "" {
		mode = store.DomainWAFModeDetect
	}
	return domainWAFResource{
		Domain:           domain,
		WAFEnabled:       row.WAFEnabled,
		WAFMode:          mode,
		RateLimitEnabled: row.RateLimitRPS > 0,
		RateLimitRPS:     row.RateLimitRPS,
		RateLimitBurst:   row.RateLimitBurst,
	}
}

// setDomainWAFRequest is PUT .../waf's request body. Mode empty defaults
// to store.DomainWAFModeDetect, the same safer-default reasoning
// docs/domains-and-ingress.md documents for a first-time enable.
// RateLimitRPS 0 (the zero value) means rate limiting is off, matching
// store.DomainWAF's own convention; there is no separate boolean to go
// out of sync with it.
type setDomainWAFRequest struct {
	WAFEnabled     bool   `json:"waf_enabled"`
	WAFMode        string `json:"waf_mode,omitempty"`
	RateLimitRPS   int    `json:"rate_limit_rps"`
	RateLimitBurst int    `json:"rate_limit_burst"`
}

// handleGetDomainWAF handles GET /api/v1/apps/{name}/domains/{domain}/waf:
// current WAF/rate-limit state for domain. AbilityRead, the same
// passive-visibility tier GET .../domains/{domain}/maintenance already
// uses.
func (rt *Router) handleGetDomainWAF(w http.ResponseWriter, r *http.Request) {
	domain, ok := rt.requireOwnedDomain(w, r)
	if !ok {
		return
	}

	row, _, err := rt.domainWAF.GetDomainWAF(r.Context(), domain)
	if err != nil {
		rt.logger.Error("api: get domain waf failed", slog.String("error", err.Error()), slog.String("domain", domain))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, toDomainWAFResource(domain, row))
}

// handleSetDomainWAF handles PUT /api/v1/apps/{name}/domains/{domain}/waf:
// sets WAF and/or rate-limit configuration for domain, enforced by Caddy
// on the next ingress reconcile pass. AbilityDeploy, matching PUT
// .../domains/{domain}/maintenance: this changes an app's runtime
// routing behavior, not a credential, so it doesn't need AbilityRoot's
// higher bar.
func (rt *Router) handleSetDomainWAF(w http.ResponseWriter, r *http.Request) {
	domain, ok := rt.requireOwnedDomain(w, r)
	if !ok {
		return
	}

	var req setDomainWAFRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.WAFMode == "" {
		req.WAFMode = store.DomainWAFModeDetect
	}
	if req.WAFMode != store.DomainWAFModeDetect && req.WAFMode != store.DomainWAFModeBlock {
		writeError(w, http.StatusBadRequest, "waf_mode must be \"detect\" or \"block\"")
		return
	}
	if req.RateLimitRPS < 0 || req.RateLimitBurst < 0 {
		writeError(w, http.StatusBadRequest, "rate_limit_rps and rate_limit_burst must not be negative")
		return
	}

	if err := rt.domainWAF.SetDomainWAF(r.Context(), domain, req.WAFEnabled, req.WAFMode, req.RateLimitRPS, req.RateLimitBurst); err != nil {
		rt.logger.Error("api: set domain waf failed", slog.String("error", err.Error()), slog.String("domain", domain))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	row, _, err := rt.domainWAF.GetDomainWAF(r.Context(), domain)
	if err != nil {
		rt.logger.Error("api: get domain waf failed", slog.String("error", err.Error()), slog.String("domain", domain))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, toDomainWAFResource(domain, row))
}

// handleClearDomainWAF handles DELETE
// /api/v1/apps/{name}/domains/{domain}/waf: resets domain to no WAF and
// no rate limiting, taking effect on the next ingress reconcile pass.
// AbilityDeploy. Idempotent: clearing a domain with none configured is
// not an error.
func (rt *Router) handleClearDomainWAF(w http.ResponseWriter, r *http.Request) {
	domain, ok := rt.requireOwnedDomain(w, r)
	if !ok {
		return
	}

	if err := rt.domainWAF.DeleteDomainWAF(r.Context(), domain); err != nil {
		rt.logger.Error("api: clear domain waf failed", slog.String("error", err.Error()), slog.String("domain", domain))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, toDomainWAFResource(domain, store.DomainWAF{}))
}

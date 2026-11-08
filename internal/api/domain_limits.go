package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

// DomainLimitsStore is the store surface .../domains/{domain}/limits needs.
type DomainLimitsStore interface {
	GetDomainLimits(ctx context.Context, domain string) (store.DomainLimits, bool, error)
	SetDomainLimits(ctx context.Context, l store.DomainLimits) error
	DeleteDomainLimits(ctx context.Context, domain string) error
}

// domainLimitsResource is the wire shape for GET/PUT/DELETE .../limits. A 0
// value means no per-domain limit. The Max fields are the current ceilings a
// save is checked against.
type domainLimitsResource struct {
	Domain                string     `json:"domain"`
	Configured            bool       `json:"configured"`
	MaxBodyBytes          int64      `json:"max_body_bytes"`
	ReadTimeoutSeconds    int        `json:"read_timeout_seconds"`
	WriteTimeoutSeconds   int        `json:"write_timeout_seconds"`
	MinBodyBytes          int64      `json:"min_body_bytes"`
	CeilingBodyBytes      int64      `json:"ceiling_body_bytes"`
	CeilingTimeoutSeconds int        `json:"ceiling_timeout_seconds"`
	UpdatedAt             *time.Time `json:"updated_at,omitempty"`
}

type setDomainLimitsRequest struct {
	MaxBodyBytes        int64 `json:"max_body_bytes"`
	ReadTimeoutSeconds  int   `json:"read_timeout_seconds"`
	WriteTimeoutSeconds int   `json:"write_timeout_seconds"`
}

// minDomainBodyBytes keeps a typo from turning a domain into a wall of 413s.
const minDomainBodyBytes = int64(1) << 20

func (rt *Router) toDomainLimitsResource(ctx context.Context, domain string, l store.DomainLimits, found bool) domainLimitsResource {
	res := domainLimitsResource{
		Domain:                domain,
		MinBodyBytes:          minDomainBodyBytes,
		CeilingBodyBytes:      int64(rt.runtimeSettings.Limit(ctx, runtimesettings.KeyDomainLimitsMaxBodyBytes)),
		CeilingTimeoutSeconds: rt.runtimeSettings.Limit(ctx, runtimesettings.KeyDomainLimitsMaxTimeoutSeconds),
	}
	if !found {
		return res
	}
	res.Configured = true
	res.MaxBodyBytes = l.MaxBodyBytes
	res.ReadTimeoutSeconds = l.ReadTimeoutSeconds
	res.WriteTimeoutSeconds = l.WriteTimeoutSeconds
	if !l.UpdatedAt.IsZero() {
		t := l.UpdatedAt
		res.UpdatedAt = &t
	}
	return res
}

func validateDomainLimits(domain string, req setDomainLimitsRequest, bodyCeiling int64, timeoutCeiling int) (store.DomainLimits, error) {
	if req.MaxBodyBytes != 0 && (req.MaxBodyBytes < minDomainBodyBytes || req.MaxBodyBytes > bodyCeiling) {
		return store.DomainLimits{}, fmt.Errorf("max_body_bytes must be 0 (no domain limit) or between %d and %d", minDomainBodyBytes, bodyCeiling)
	}
	for name, v := range map[string]int{"read_timeout_seconds": req.ReadTimeoutSeconds, "write_timeout_seconds": req.WriteTimeoutSeconds} {
		if v < 0 || v > timeoutCeiling {
			return store.DomainLimits{}, fmt.Errorf("%s must be 0 (no domain limit) or between 1 and %d", name, timeoutCeiling)
		}
	}
	return store.DomainLimits{
		Domain: domain, MaxBodyBytes: req.MaxBodyBytes,
		ReadTimeoutSeconds: req.ReadTimeoutSeconds, WriteTimeoutSeconds: req.WriteTimeoutSeconds,
	}, nil
}

// handleGetDomainLimits handles GET /api/v1/apps/{name}/domains/{domain}/limits.
func (rt *Router) handleGetDomainLimits(w http.ResponseWriter, r *http.Request) {
	domain, ok := rt.requireOwnedDomain(w, r)
	if !ok {
		return
	}
	l, found, err := rt.domainLimits.GetDomainLimits(r.Context(), domain)
	if err != nil {
		rt.internalError(w, "api: get domain limits failed", err, slog.String("domain", domain))
		return
	}
	writeJSON(w, http.StatusOK, rt.toDomainLimitsResource(r.Context(), domain, l, found))
}

// handleSetDomainLimits handles PUT .../limits; the ingress applies it on its
// next reconcile pass. A request with every field 0 clears the limits.
func (rt *Router) handleSetDomainLimits(w http.ResponseWriter, r *http.Request) {
	domain, ok := rt.requireOwnedDomain(w, r)
	if !ok {
		return
	}
	var req setDomainLimitsRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	ctx := r.Context()
	row, err := validateDomainLimits(domain, req,
		int64(rt.runtimeSettings.Limit(ctx, runtimesettings.KeyDomainLimitsMaxBodyBytes)),
		rt.runtimeSettings.Limit(ctx, runtimesettings.KeyDomainLimitsMaxTimeoutSeconds))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if row.MaxBodyBytes == 0 && row.ReadTimeoutSeconds == 0 && row.WriteTimeoutSeconds == 0 {
		rt.clearDomainLimits(w, r, domain)
		return
	}
	if err := rt.domainLimits.SetDomainLimits(ctx, row); err != nil {
		rt.internalError(w, "api: set domain limits failed", err, slog.String("domain", domain))
		return
	}
	saved, found, err := rt.domainLimits.GetDomainLimits(ctx, domain)
	if err != nil {
		rt.internalError(w, "api: read domain limits failed", err, slog.String("domain", domain))
		return
	}
	writeJSON(w, http.StatusOK, rt.toDomainLimitsResource(ctx, domain, saved, found))
}

// handleClearDomainLimits handles DELETE .../limits. Idempotent.
func (rt *Router) handleClearDomainLimits(w http.ResponseWriter, r *http.Request) {
	domain, ok := rt.requireOwnedDomain(w, r)
	if !ok {
		return
	}
	rt.clearDomainLimits(w, r, domain)
}

func (rt *Router) clearDomainLimits(w http.ResponseWriter, r *http.Request, domain string) {
	if err := rt.domainLimits.DeleteDomainLimits(r.Context(), domain); err != nil {
		rt.internalError(w, "api: clear domain limits failed", err, slog.String("domain", domain))
		return
	}
	writeJSON(w, http.StatusOK, rt.toDomainLimitsResource(r.Context(), domain, store.DomainLimits{}, false))
}

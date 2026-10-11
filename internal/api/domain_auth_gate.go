package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"os"
	"time"

	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/store"
)

// DomainAuthGateStore is the store surface .../domains/{domain}/auth-gate needs.
type DomainAuthGateStore interface {
	GetDomainAuthGate(ctx context.Context, domain string) (store.DomainAuthGate, bool, error)
	SetDomainAuthGate(ctx context.Context, g store.DomainAuthGate) error
	DeleteDomainAuthGate(ctx context.Context, domain string) error
	ListDomainAuthGates(ctx context.Context) ([]store.DomainAuthGate, error)
}

// domainAuthGateResource is the wire shape for GET/PUT/DELETE .../auth-gate.
// TimeoutSeconds 0 means DefaultTimeoutSeconds, read from APP_AUTH_GATE_TIMEOUT.
type domainAuthGateResource struct {
	Domain                string     `json:"domain"`
	Configured            bool       `json:"configured"`
	Enabled               bool       `json:"enabled"`
	AuthURL               string     `json:"auth_url"`
	CheckPath             string     `json:"check_path"`
	CopyHeaders           []string   `json:"copy_headers"`
	TimeoutSeconds        int        `json:"timeout_seconds"`
	DefaultTimeoutSeconds int        `json:"default_timeout_seconds"`
	UpdatedAt             *time.Time `json:"updated_at,omitempty"`
}

type setDomainAuthGateRequest struct {
	Enabled        bool     `json:"enabled"`
	AuthURL        string   `json:"auth_url"`
	CheckPath      string   `json:"check_path"`
	CopyHeaders    []string `json:"copy_headers"`
	TimeoutSeconds int      `json:"timeout_seconds"`
}

func defaultAuthGateTimeoutSeconds() int {
	d, _ := ingress.AuthGateTimeoutFromEnv(os.Getenv)
	return int(math.Ceil(d.Seconds()))
}

func toDomainAuthGateResource(domain string, g store.DomainAuthGate, found bool) domainAuthGateResource {
	res := domainAuthGateResource{
		Domain:                domain,
		CheckPath:             ingress.DefaultAuthGateCheckPath,
		CopyHeaders:           []string{},
		DefaultTimeoutSeconds: defaultAuthGateTimeoutSeconds(),
	}
	if !found {
		return res
	}
	res.Configured = true
	res.Enabled = g.Enabled
	res.AuthURL = g.AuthURL
	res.CheckPath = g.CheckPath
	if g.CopyHeaders != nil {
		res.CopyHeaders = g.CopyHeaders
	}
	res.TimeoutSeconds = g.TimeoutSeconds
	if !g.UpdatedAt.IsZero() {
		t := g.UpdatedAt
		res.UpdatedAt = &t
	}
	return res
}

// validateDomainAuthGate normalizes req into a store row, or returns the
// operator-facing reason it was rejected.
func validateDomainAuthGate(domain string, req setDomainAuthGateRequest) (store.DomainAuthGate, error) {
	u, err := ingress.ValidateAuthGateURL(req.AuthURL)
	if err != nil {
		return store.DomainAuthGate{}, err
	}
	checkPath, err := ingress.NormalizeAuthGateCheckPath(req.CheckPath)
	if err != nil {
		return store.DomainAuthGate{}, err
	}
	headers, err := ingress.NormalizeAuthGateCopyHeaders(req.CopyHeaders)
	if err != nil {
		return store.DomainAuthGate{}, err
	}
	if req.TimeoutSeconds < 0 || req.TimeoutSeconds > ingress.MaxAuthGateTimeoutSeconds {
		return store.DomainAuthGate{}, fmt.Errorf("timeout_seconds must be between 0 and %d", ingress.MaxAuthGateTimeoutSeconds)
	}
	return store.DomainAuthGate{
		Domain: domain, Enabled: req.Enabled, AuthURL: u.String(), CheckPath: checkPath,
		CopyHeaders: headers, TimeoutSeconds: req.TimeoutSeconds,
	}, nil
}

// handleGetDomainAuthGate handles GET /api/v1/apps/{name}/domains/{domain}/auth-gate.
func (rt *Router) handleGetDomainAuthGate(w http.ResponseWriter, r *http.Request) {
	domain, ok := rt.requireOwnedDomain(w, r)
	if !ok {
		return
	}
	g, found, err := rt.domainAuthGate.GetDomainAuthGate(r.Context(), domain)
	if err != nil {
		rt.logger.Error("api: get domain auth gate failed", slog.String("error", err.Error()), slog.String("domain", domain))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, toDomainAuthGateResource(domain, g, found))
}

// handleSetDomainAuthGate handles PUT .../auth-gate, enforced by the ingress
// on its next reconcile pass. AbilityRoot, like basic auth.
func (rt *Router) handleSetDomainAuthGate(w http.ResponseWriter, r *http.Request) {
	domain, ok := rt.requireOwnedDomain(w, r)
	if !ok {
		return
	}
	var req setDomainAuthGateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	row, err := validateDomainAuthGate(domain, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := rt.domainAuthGate.SetDomainAuthGate(r.Context(), row); err != nil {
		rt.logger.Error("api: set domain auth gate failed", slog.String("error", err.Error()), slog.String("domain", domain))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	g, found, err := rt.domainAuthGate.GetDomainAuthGate(r.Context(), domain)
	if err != nil {
		rt.logger.Error("api: get domain auth gate failed", slog.String("error", err.Error()), slog.String("domain", domain))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, toDomainAuthGateResource(domain, g, found))
}

// handleClearDomainAuthGate handles DELETE .../auth-gate. Idempotent.
func (rt *Router) handleClearDomainAuthGate(w http.ResponseWriter, r *http.Request) {
	domain, ok := rt.requireOwnedDomain(w, r)
	if !ok {
		return
	}
	if err := rt.domainAuthGate.DeleteDomainAuthGate(r.Context(), domain); err != nil {
		rt.logger.Error("api: clear domain auth gate failed", slog.String("error", err.Error()), slog.String("domain", domain))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, toDomainAuthGateResource(domain, store.DomainAuthGate{}, false))
}

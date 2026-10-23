package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"os"
	"strings"
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
	// Provider is "external" (AuthURL) or "builtin" (this installation's sign-in).
	Provider         string             `json:"provider"`
	Allow            []gateRuleResource `json:"allow"`
	LiveSessions     int                `json:"live_sessions"`
	BuiltinAvailable bool               `json:"builtin_available"`
}

type setDomainAuthGateRequest struct {
	Enabled        bool            `json:"enabled"`
	AuthURL        string          `json:"auth_url"`
	CheckPath      string          `json:"check_path"`
	CopyHeaders    []string        `json:"copy_headers"`
	TimeoutSeconds int             `json:"timeout_seconds"`
	Provider       string          `json:"provider"`
	Allow          []gateRuleInput `json:"allow"`
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
		Provider:              store.GateProviderExternal,
		Allow:                 []gateRuleResource{},
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
	if g.Provider != "" {
		res.Provider = g.Provider
	}
	if !g.UpdatedAt.IsZero() {
		t := g.UpdatedAt
		res.UpdatedAt = &t
	}
	return res
}

// normalizeGateProvider maps the wire value to a stored provider. The
// brand's short name is accepted as an alias for the built-in provider.
func (rt *Router) normalizeGateProvider(p string) (string, error) {
	p = strings.TrimSpace(p)
	switch {
	case p == "" || strings.EqualFold(p, store.GateProviderExternal):
		return store.GateProviderExternal, nil
	case strings.EqualFold(p, store.GateProviderBuiltin), rt.brand != nil && rt.brand.ShortName != "" && strings.EqualFold(p, rt.brand.ShortName):
		return store.GateProviderBuiltin, nil
	}
	return "", fmt.Errorf("provider must be %q or %q", store.GateProviderExternal, store.GateProviderBuiltin)
}

// validateBuiltinAuthGate normalizes a built-in provider request. The
// identity headers are always added at the ingress, so they use up three
// of the copy header slots.
func validateBuiltinAuthGate(domain string, req setDomainAuthGateRequest) (store.DomainAuthGate, error) {
	headers, err := ingress.NormalizeAuthGateCopyHeaders(req.CopyHeaders)
	if err != nil {
		return store.DomainAuthGate{}, err
	}
	if len(headers) > ingress.MaxAuthGateCopyHeaders-3 {
		return store.DomainAuthGate{}, fmt.Errorf("copy_headers allows at most %d extra names with the built-in provider", ingress.MaxAuthGateCopyHeaders-3)
	}
	if req.TimeoutSeconds < 0 || req.TimeoutSeconds > ingress.MaxAuthGateTimeoutSeconds {
		return store.DomainAuthGate{}, fmt.Errorf("timeout_seconds must be between 0 and %d", ingress.MaxAuthGateTimeoutSeconds)
	}
	return store.DomainAuthGate{
		Domain: domain, Enabled: req.Enabled, Provider: store.GateProviderBuiltin, CheckPath: ingress.DefaultAuthGateCheckPath,
		CopyHeaders: headers, TimeoutSeconds: req.TimeoutSeconds,
	}, nil
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
		Domain: domain, Enabled: req.Enabled, Provider: store.GateProviderExternal, AuthURL: u.String(), CheckPath: checkPath,
		CopyHeaders: headers, TimeoutSeconds: req.TimeoutSeconds,
	}, nil
}

// handleGetDomainAuthGate handles GET /api/v1/apps/{name}/domains/{domain}/auth-gate.
func (rt *Router) handleGetDomainAuthGate(w http.ResponseWriter, r *http.Request) {
	domain, ok := rt.requireOwnedDomain(w, r)
	if !ok {
		return
	}
	rt.writeDomainAuthGate(w, r, domain)
}

// writeDomainAuthGate answers with domain's current gate, rules included.
func (rt *Router) writeDomainAuthGate(w http.ResponseWriter, r *http.Request, domain string) {
	ctx := r.Context()
	g, found, err := rt.domainAuthGate.GetDomainAuthGate(ctx, domain)
	if err != nil {
		rt.internalError(w, "api: get domain auth gate failed", err, slog.String("domain", domain))
		return
	}
	res := toDomainAuthGateResource(domain, g, found)
	if rt.gate != nil {
		_, baseErr := rt.controlPlaneBaseURL(ctx)
		res.BuiltinAvailable = baseErr == nil && rt.gateSecret != ""
		if found && g.Provider == store.GateProviderBuiltin {
			rules, err := rt.gate.ListGateRules(ctx, domain)
			if err != nil {
				rt.internalError(w, "api: list gate rules failed", err, slog.String("domain", domain))
				return
			}
			res.Allow = rt.gateRuleResources(ctx, rules)
			if res.LiveSessions, err = rt.gate.CountLiveGateSessions(ctx, domain); err != nil {
				rt.internalError(w, "api: count gate sessions failed", err, slog.String("domain", domain))
				return
			}
		}
	}
	writeJSON(w, http.StatusOK, res)
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
	provider, err := rt.normalizeGateProvider(req.Provider)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if provider == store.GateProviderExternal {
		if len(req.Allow) > 0 {
			writeError(w, http.StatusBadRequest, "allow applies only to the built-in provider")
			return
		}
		row, err := validateDomainAuthGate(domain, req)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if !rt.saveExternalAuthGate(w, r, row) {
			return
		}
		rt.writeDomainAuthGate(w, r, domain)
		return
	}
	if !rt.saveBuiltinAuthGate(w, r, domain, req) {
		return
	}
	rt.writeDomainAuthGate(w, r, domain)
}

func (rt *Router) saveExternalAuthGate(w http.ResponseWriter, r *http.Request, row store.DomainAuthGate) bool {
	ctx := r.Context()
	if err := rt.domainAuthGate.SetDomainAuthGate(ctx, row); err != nil {
		rt.internalError(w, "api: set domain auth gate failed", err, slog.String("domain", row.Domain))
		return false
	}
	if rt.gate != nil {
		if err := rt.gate.ReplaceGateRules(ctx, row.Domain, nil); err != nil {
			rt.internalError(w, "api: clear gate rules failed", err, slog.String("domain", row.Domain))
			return false
		}
		if _, err := rt.gate.RevokeGateSessions(ctx, row.Domain); err != nil {
			rt.internalError(w, "api: revoke gate sessions failed", err, slog.String("domain", row.Domain))
			return false
		}
	}
	return true
}

// saveBuiltinAuthGate narrows before it widens: on an existing gate the
// rules change first, on a new one the gate row exists (with no rules,
// so nobody passes) before any rule is written.
func (rt *Router) saveBuiltinAuthGate(w http.ResponseWriter, r *http.Request, domain string, req setDomainAuthGateRequest) bool {
	ctx := r.Context()
	if rt.gate == nil || rt.gateSecret == "" {
		writeError(w, http.StatusNotImplemented, "the built-in sign-in provider is not available on this server")
		return false
	}
	if _, err := rt.controlPlaneBaseURL(ctx); err != nil {
		writeError(w, http.StatusBadRequest, "set the platform primary domain first: the built-in sign-in provider sends visitors to the dashboard to sign in")
		return false
	}
	row, err := validateBuiltinAuthGate(domain, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return false
	}
	rules, err := rt.resolveGateRules(ctx, req.Allow)
	if err != nil {
		var ruleErr *gateRuleError
		if errors.As(err, &ruleErr) {
			writeError(w, http.StatusBadRequest, err.Error())
		} else {
			rt.internalError(w, "api: resolve gate rules failed", err, slog.String("domain", domain))
		}
		return false
	}
	existing, found, err := rt.domainAuthGate.GetDomainAuthGate(ctx, domain)
	if err != nil {
		rt.internalError(w, "api: get domain auth gate failed", err, slog.String("domain", domain))
		return false
	}
	steps := []func() error{
		func() error { return rt.domainAuthGate.SetDomainAuthGate(ctx, row) },
		func() error { return rt.gate.ReplaceGateRules(ctx, domain, rules) },
	}
	if found && existing.Provider == store.GateProviderBuiltin {
		steps[0], steps[1] = steps[1], steps[0]
	}
	for _, step := range steps {
		if err := step(); err != nil {
			rt.internalError(w, "api: set built-in auth gate failed", err, slog.String("domain", domain))
			return false
		}
	}
	return true
}

// handleClearDomainAuthGate handles DELETE .../auth-gate. Idempotent.
func (rt *Router) handleClearDomainAuthGate(w http.ResponseWriter, r *http.Request) {
	domain, ok := rt.requireOwnedDomain(w, r)
	if !ok {
		return
	}
	if err := rt.domainAuthGate.DeleteDomainAuthGate(r.Context(), domain); err != nil {
		rt.internalError(w, "api: clear domain auth gate failed", err, slog.String("domain", domain))
		return
	}
	writeJSON(w, http.StatusOK, toDomainAuthGateResource(domain, store.DomainAuthGate{}, false))
}

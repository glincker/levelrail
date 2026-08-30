package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/GLINCKER/levelrail/internal/store"
)

// DomainBasicAuthStore is the store surface GET/PUT/DELETE
// .../domains/{domain}/auth need: the username claimed for a domain,
// always set, same "core Store interface" shape as DomainStore above.
type DomainBasicAuthStore interface {
	GetDomainBasicAuth(ctx context.Context, domain string) (store.DomainBasicAuth, bool, error)
	SetDomainBasicAuth(ctx context.Context, domain, username string) error
	DeleteDomainBasicAuth(ctx context.Context, domain string) error
}

// DomainBasicAuthSecrets is the surface these handlers need from
// internal/secrets.Manager for a domain's basic-auth password: set it
// (PUT), check whether one exists (GET's has_password, and PUT's
// "already set" check for the omitted-password case), and clear it
// (DELETE). *secrets.Manager satisfies this structurally, the same
// narrow shape CloudflareTunnelSecrets already establishes.
type DomainBasicAuthSecrets interface {
	SetValue(ctx context.Context, serviceName, envKey, plaintext string) error
	Exists(ctx context.Context, serviceName, envKey string) (bool, error)
	DeleteAll(ctx context.Context, serviceName string) error
}

// domainBasicAuthResource is the wire shape for GET/PUT/DELETE
// .../domains/{domain}/auth. The password itself never appears in
// either direction: PUT accepts one as a write-only request field,
// responses only ever report HasPassword, mirroring
// cloudflareTunnelResource's own has_token convention.
type domainBasicAuthResource struct {
	Domain      string `json:"domain"`
	Enabled     bool   `json:"enabled"`
	Username    string `json:"username,omitempty"`
	HasPassword bool   `json:"has_password"`
}

func (rt *Router) toDomainBasicAuthResource(ctx context.Context, domain string, auth store.DomainBasicAuth, found bool) domainBasicAuthResource {
	res := domainBasicAuthResource{Domain: domain}
	if !found {
		return res
	}
	res.Username = auth.Username
	if rt.domainBasicAuthSecrets != nil {
		exists, err := rt.domainBasicAuthSecrets.Exists(ctx, store.DomainBasicAuthSecretsKey(domain), store.DomainBasicAuthPasswordEnvKey)
		if err != nil {
			rt.logger.Warn("api: check domain basic auth password failed", slog.String("error", err.Error()), slog.String("domain", domain))
		}
		res.HasPassword = exists
	}
	res.Enabled = res.HasPassword
	return res
}

// appOwnsDomain reports whether appName currently claims domain, per
// service_domains (kept in sync by SaveDesiredService). The URL scopes
// basic auth by app for a predictable, DomainEditor-shaped route, but
// this is only a check against a stale or mistyped domain in the
// request: the protection itself is domain-wide, keyed by
// store.DomainBasicAuth.Domain, not by service.
func (rt *Router) appOwnsDomain(ctx context.Context, appName, domain string) (bool, error) {
	svc, err := rt.apps.GetDesiredService(ctx, appName)
	if err != nil {
		return false, err
	}
	for _, d := range svc.Domains {
		if d == domain {
			return true, nil
		}
	}
	return false, nil
}

// requireOwnedDomain resolves and validates the {name}/{domain} path
// values shared by all three handlers below, writing the appropriate
// error response itself when the app doesn't exist or doesn't own
// domain. ok is false when the caller should return immediately.
func (rt *Router) requireOwnedDomain(w http.ResponseWriter, r *http.Request) (domain string, ok bool) {
	name := r.PathValue("name")
	domain = strings.ToLower(strings.TrimSpace(r.PathValue("domain")))

	owns, err := rt.appOwnsDomain(r.Context(), name, domain)
	if err != nil {
		if errors.Is(err, store.ErrServiceNotFound) {
			writeError(w, http.StatusNotFound, "app not found")
			return "", false
		}
		rt.logger.Error("api: domain basic auth: get app failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return "", false
	}
	if !owns {
		writeError(w, http.StatusNotFound, "domain not found for this app")
		return "", false
	}
	return domain, true
}

// handleGetDomainBasicAuth handles GET
// /api/v1/apps/{name}/domains/{domain}/auth: current basic-auth state
// for domain. AbilityRead, the same passive-visibility tier GET
// /api/v1/settings/cloudflare-tunnel already uses for its own
// has_token-shaped read.
func (rt *Router) handleGetDomainBasicAuth(w http.ResponseWriter, r *http.Request) {
	domain, ok := rt.requireOwnedDomain(w, r)
	if !ok {
		return
	}

	auth, found, err := rt.domainBasicAuth.GetDomainBasicAuth(r.Context(), domain)
	if err != nil {
		rt.logger.Error("api: get domain basic auth failed", slog.String("error", err.Error()), slog.String("domain", domain))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, rt.toDomainBasicAuthResource(r.Context(), domain, auth, found))
}

type setDomainBasicAuthRequest struct {
	Username string `json:"username"`
	// Password is optional on update: empty means "leave the currently
	// stored password unchanged", the same convention
	// updateCloudflareTunnelRequest.Token already establishes.
	Password string `json:"password,omitempty"`
}

// handleSetDomainBasicAuth handles PUT
// /api/v1/apps/{name}/domains/{domain}/auth: enables HTTP Basic Auth on
// domain, enforced by Caddy on the next ingress reconcile pass.
// AbilityRoot. Returns 501 without domainBasicAuthSecrets configured
// (no master key).
func (rt *Router) handleSetDomainBasicAuth(w http.ResponseWriter, r *http.Request) {
	if rt.domainBasicAuthSecrets == nil {
		writeError(w, http.StatusNotImplemented, "domain basic auth is not configured on this control plane (no master key set)")
		return
	}

	domain, ok := rt.requireOwnedDomain(w, r)
	if !ok {
		return
	}

	var req setDomainBasicAuthRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" {
		writeError(w, http.StatusBadRequest, "username is required")
		return
	}

	key := store.DomainBasicAuthSecretsKey(domain)
	hasPassword := req.Password != ""
	if !hasPassword {
		exists, err := rt.domainBasicAuthSecrets.Exists(r.Context(), key, store.DomainBasicAuthPasswordEnvKey)
		if err != nil {
			rt.logger.Error("api: check domain basic auth password failed", slog.String("error", err.Error()), slog.String("domain", domain))
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		hasPassword = exists
	}
	if !hasPassword {
		writeError(w, http.StatusBadRequest, "password is required the first time basic auth is enabled on a domain")
		return
	}

	if req.Password != "" {
		if err := rt.domainBasicAuthSecrets.SetValue(r.Context(), key, store.DomainBasicAuthPasswordEnvKey, req.Password); err != nil {
			rt.logger.Error("api: save domain basic auth password failed", slog.String("error", err.Error()), slog.String("domain", domain))
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
	}

	if err := rt.domainBasicAuth.SetDomainBasicAuth(r.Context(), domain, req.Username); err != nil {
		rt.logger.Error("api: set domain basic auth failed", slog.String("error", err.Error()), slog.String("domain", domain))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	auth, found, err := rt.domainBasicAuth.GetDomainBasicAuth(r.Context(), domain)
	if err != nil {
		rt.logger.Error("api: get domain basic auth failed", slog.String("error", err.Error()), slog.String("domain", domain))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, rt.toDomainBasicAuthResource(r.Context(), domain, auth, found))
}

// handleClearDomainBasicAuth handles DELETE
// /api/v1/apps/{name}/domains/{domain}/auth: removes basic auth
// protection from domain, taking effect on the next ingress reconcile
// pass. AbilityRoot. Idempotent: clearing a domain with none configured
// is not an error.
func (rt *Router) handleClearDomainBasicAuth(w http.ResponseWriter, r *http.Request) {
	if rt.domainBasicAuthSecrets == nil {
		writeError(w, http.StatusNotImplemented, "domain basic auth is not configured on this control plane (no master key set)")
		return
	}

	domain, ok := rt.requireOwnedDomain(w, r)
	if !ok {
		return
	}

	if err := rt.domainBasicAuthSecrets.DeleteAll(r.Context(), store.DomainBasicAuthSecretsKey(domain)); err != nil {
		rt.logger.Error("api: clear domain basic auth password failed", slog.String("error", err.Error()), slog.String("domain", domain))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if err := rt.domainBasicAuth.DeleteDomainBasicAuth(r.Context(), domain); err != nil {
		rt.logger.Error("api: clear domain basic auth failed", slog.String("error", err.Error()), slog.String("domain", domain))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, domainBasicAuthResource{Domain: domain})
}

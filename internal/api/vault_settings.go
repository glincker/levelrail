package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/store"
)

// VaultSecrets is the surface the Vault settings handlers need from
// internal/secrets.Manager: set the credential (PUT), check whether one
// exists (GET's has_credential, and PUT's "was a credential already set"
// check for the omitted-credential case), and clear it (DELETE).
// *secrets.Manager satisfies this structurally, the same "narrow,
// consumer-defined interface" shape CloudflareTunnelSecrets already
// establishes.
type VaultSecrets interface {
	SetValue(ctx context.Context, serviceName, envKey, plaintext string) error
	Exists(ctx context.Context, serviceName, envKey string) (bool, error)
	DeleteAll(ctx context.Context, serviceName string) error
}

// vaultSettingsResource is the wire shape for GET, PUT, and DELETE
// /api/v1/settings/vault. The credential (a Vault token or AppRole
// secret ID, depending on AuthMethod) never appears here in either
// direction: PUT accepts one as a write-only request field (see
// updateVaultSettingsRequest), GET/PUT/DELETE responses only ever
// report HasCredential.
type vaultSettingsResource struct {
	Enabled       bool   `json:"enabled"`
	Address       string `json:"address"`
	AuthMethod    string `json:"auth_method"`
	Namespace     string `json:"namespace,omitempty"`
	RoleID        string `json:"role_id,omitempty"`
	MountPath     string `json:"mount_path"`
	HasCredential bool   `json:"has_credential"`
}

func (rt *Router) toVaultSettingsResource(ctx context.Context, s store.VaultSettings) vaultSettingsResource {
	res := vaultSettingsResource{
		Enabled:    s.Enabled,
		Address:    s.Address,
		AuthMethod: s.AuthMethod,
		Namespace:  s.Namespace,
		RoleID:     s.RoleID,
		MountPath:  s.MountPath,
	}

	if rt.vaultSecrets != nil {
		exists, err := rt.vaultSecrets.Exists(ctx, store.VaultSecretsKey(), store.VaultCredentialEnvKey)
		if err != nil {
			rt.logger.Warn("api: check vault credential failed", slog.String("error", err.Error()))
		}
		res.HasCredential = exists
	}
	return res
}

// handleGetVaultSettings handles GET /api/v1/settings/vault. AbilityRead,
// matching GET /api/v1/settings/cloudflare-tunnel.
func (rt *Router) handleGetVaultSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := rt.vault.GetVaultSettings(r.Context())
	if err != nil {
		rt.logger.Error("api: get vault settings failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, rt.toVaultSettingsResource(r.Context(), settings))
}

type updateVaultSettingsRequest struct {
	Enabled    bool   `json:"enabled"`
	Address    string `json:"address"`
	AuthMethod string `json:"auth_method"`
	Namespace  string `json:"namespace,omitempty"`
	RoleID     string `json:"role_id,omitempty"`
	MountPath  string `json:"mount_path,omitempty"`
	// Credential is optional on update: empty means "leave the currently
	// stored credential unchanged", the same convention
	// updateCloudflareTunnelRequest.Token already establishes.
	Credential string `json:"credential,omitempty"`
}

// handleUpdateVaultSettings handles PUT /api/v1/settings/vault.
// AbilityRoot: instance-level infrastructure config, the same tier PUT
// /api/v1/settings/cloudflare-tunnel uses. Returns 501 without
// vaultSecrets configured (no master key).
func (rt *Router) handleUpdateVaultSettings(w http.ResponseWriter, r *http.Request) {
	if rt.vaultSecrets == nil {
		writeError(w, http.StatusNotImplemented, "vault is not configured on this control plane (no master key set)")
		return
	}

	var req updateVaultSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.AuthMethod == "" {
		req.AuthMethod = store.VaultAuthMethodToken
	}
	if req.AuthMethod != store.VaultAuthMethodToken && req.AuthMethod != store.VaultAuthMethodAppRole {
		writeError(w, http.StatusBadRequest, "auth_method must be \"token\" or \"approle\"")
		return
	}
	if req.Enabled && req.Address == "" {
		writeError(w, http.StatusBadRequest, "address is required to enable vault")
		return
	}
	if req.AuthMethod == store.VaultAuthMethodAppRole && req.RoleID == "" {
		writeError(w, http.StatusBadRequest, "role_id is required for approle auth")
		return
	}

	key := store.VaultSecretsKey()
	hasCredential := req.Credential != ""
	if !hasCredential {
		exists, err := rt.vaultSecrets.Exists(r.Context(), key, store.VaultCredentialEnvKey)
		if err != nil {
			rt.logger.Error("api: check vault credential failed", slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		hasCredential = exists
	}
	if req.Enabled && !hasCredential {
		writeError(w, http.StatusBadRequest, "credential is required the first time vault is enabled")
		return
	}

	if req.Credential != "" {
		if err := rt.vaultSecrets.SetValue(r.Context(), key, store.VaultCredentialEnvKey, req.Credential); err != nil {
			rt.logger.Error("api: save vault credential failed", slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
	}

	settings := store.VaultSettings{
		Enabled:    req.Enabled,
		Address:    req.Address,
		AuthMethod: req.AuthMethod,
		Namespace:  req.Namespace,
		RoleID:     req.RoleID,
		MountPath:  req.MountPath,
	}
	if err := rt.vault.UpdateVaultSettings(r.Context(), settings); err != nil {
		rt.logger.Error("api: update vault settings failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, rt.toVaultSettingsResource(r.Context(), settings))
}

// handleDisconnectVault handles DELETE /api/v1/settings/vault: disables
// vault and clears the stored credential, matching DELETE
// /api/v1/settings/cloudflare-tunnel's own AbilityRoot tier and
// idempotent "disconnect an already-disconnected integration is not an
// error" behavior.
func (rt *Router) handleDisconnectVault(w http.ResponseWriter, r *http.Request) {
	if rt.vaultSecrets == nil {
		writeError(w, http.StatusNotImplemented, "vault is not configured on this control plane (no master key set)")
		return
	}

	if err := rt.vaultSecrets.DeleteAll(r.Context(), store.VaultSecretsKey()); err != nil {
		rt.logger.Error("api: clear vault credential failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	settings, err := rt.vault.GetVaultSettings(r.Context())
	if err != nil {
		rt.logger.Error("api: get vault settings failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	settings.Enabled = false
	if err := rt.vault.UpdateVaultSettings(r.Context(), settings); err != nil {
		rt.logger.Error("api: disconnect vault failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, rt.toVaultSettingsResource(r.Context(), settings))
}

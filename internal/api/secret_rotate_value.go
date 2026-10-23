package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/GLINCKER/levelrail/internal/secretrotation"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	auditActionVaultRotate = "vault.credential.rotate"
	vaultPreviousKey       = store.VaultCredentialEnvKey + secretrotation.PreviousSuffix
)

type rotateSecretValueRequest struct {
	Value string `json:"value"`
}

func rotationGraceHours() int {
	return secretrotation.IntFromEnv(os.Getenv, secretrotation.EnvGraceHours, secretrotation.DefaultGraceHrs)
}

// handleRotateSecretValue handles POST /api/v1/apps/{name}/secrets/{key}/rotate.
// It writes a new value (the request's, or a generated one), keeps the old
// one under <key>_PREVIOUS for the grace window and never restarts the app,
// so the caller can offer a redeploy. No value is ever returned.
func (rt *Router) handleRotateSecretValue(w http.ResponseWriter, r *http.Request) {
	if !rt.rotationReady(w) {
		return
	}
	name, key := r.PathValue("name"), r.PathValue("key")
	if secretrotation.IsPreviousKey(key) {
		writeError(w, http.StatusBadRequest, "a grace-window key cannot be rotated")
		return
	}
	if _, err := rt.apps.GetDesiredService(r.Context(), name); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: rotate secret value: load app failed", err, slog.String("name", name))
		return
	}
	var req rotateSecretValueRequest
	if r.ContentLength != 0 {
		if err := decodeJSONBody(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}
	exists, err := rt.secrets.Exists(r.Context(), name, key)
	if err != nil {
		rt.internalError(w, "api: rotate secret value: check secret failed", err, slog.String("name", name), slog.String("key", key))
		return
	}
	if !exists {
		writeError(w, http.StatusNotFound, "no secret with this key, set its value first")
		return
	}
	state := rt.rotation.Load()
	actor := rt.accessActor(r).name
	if _, err := state.store.GetSecretRotationPolicy(r.Context(), name, key); errors.Is(err, store.ErrSecretRotationPolicyNotFound) {
		p := store.SecretRotationPolicy{
			ServiceName: name, EnvKey: key, Owner: actor,
			IntervalDays: secretrotation.IntFromEnv(os.Getenv, secretrotation.EnvDefaultDays, secretrotation.DefaultDays),
			GraceHours:   rotationGraceHours(),
		}
		if err := state.store.UpsertSecretRotationPolicy(r.Context(), p, time.Now()); err != nil {
			rt.internalError(w, "api: rotate secret value: create policy failed", err, slog.String("name", name), slog.String("key", key))
			return
		}
	} else if err != nil {
		rt.internalError(w, "api: rotate secret value: load policy failed", err, slog.String("name", name), slog.String("key", key))
		return
	}
	if rotErr := state.engine.RotateValue(r.Context(), name, key, req.Value, actor); rotErr != nil {
		p, perr := state.store.GetSecretRotationPolicy(r.Context(), name, key)
		if perr == nil && p.State == store.SecretRotationFailed {
			writeError(w, http.StatusConflict, p.LastError)
			return
		}
		rt.internalError(w, "api: rotate secret value failed", rotErr, slog.String("name", name), slog.String("key", key))
		return
	}
	rt.writeRotationPolicy(r.Context(), w, name, key)
}

// expireVaultPrevious drops the kept previous Vault credential once its grace
// window ended and returns the settings with the expiry cleared.
func (rt *Router) expireVaultPrevious(ctx context.Context, s store.VaultSettings) store.VaultSettings {
	if rt.vaultSecrets == nil || s.PreviousExpiresAt.IsZero() || time.Now().Before(s.PreviousExpiresAt) {
		return s
	}
	if err := rt.vaultSecrets.DeleteValue(ctx, store.VaultSecretsKey(), vaultPreviousKey); err != nil {
		rt.logger.Warn("api: expire previous vault credential failed", slog.String("error", err.Error()))
		return s
	}
	if err := rt.vault.SetVaultCredentialRotation(ctx, s.CredentialRotatedAt, time.Time{}); err != nil {
		rt.logger.Warn("api: record vault credential grace expiry failed", slog.String("error", err.Error()))
		return s
	}
	rt.saveRotationAudit(ctx, secretrotation.ActionGraceExpired, "/api/v1/settings/vault", "system")
	s.PreviousExpiresAt = time.Time{}
	return s
}

type rotateVaultCredentialRequest struct {
	Credential string `json:"credential"`
}

// handleRotateVaultCredential handles POST /api/v1/settings/vault/rotate-credential.
// The new credential is required (Vault issues it, it is not generated here);
// the old one stays under a previous key for the grace window.
func (rt *Router) handleRotateVaultCredential(w http.ResponseWriter, r *http.Request) {
	if rt.vaultSecrets == nil {
		writeError(w, http.StatusNotImplemented, "vault is not configured on this control plane (no master key set)")
		return
	}
	var req rotateVaultCredentialRequest
	if err := decodeJSONBody(r, &req); err != nil || req.Credential == "" {
		writeError(w, http.StatusBadRequest, "credential is required")
		return
	}
	ctx := r.Context()
	key := store.VaultSecretsKey()
	exists, err := rt.vaultSecrets.Exists(ctx, key, store.VaultCredentialEnvKey)
	if err != nil {
		rt.internalError(w, "api: rotate vault credential: check credential failed", err)
		return
	}
	if !exists {
		writeError(w, http.StatusConflict, "no vault credential is stored, connect vault first")
		return
	}
	grace := rotationGraceHours()
	now := time.Now()
	var expires time.Time
	if grace > 0 {
		old, rerr := rt.vaultSecrets.Resolve(ctx, key, store.VaultCredentialEnvKey)
		if rerr != nil {
			rt.internalError(w, "api: rotate vault credential: read current credential failed", rerr)
			return
		}
		if serr := rt.vaultSecrets.SetValue(ctx, key, vaultPreviousKey, old); serr != nil {
			rt.internalError(w, "api: rotate vault credential: keep previous credential failed", serr)
			return
		}
		expires = now.Add(time.Duration(grace) * time.Hour)
	}
	if err := rt.vaultSecrets.SetValue(ctx, key, store.VaultCredentialEnvKey, req.Credential); err != nil {
		rt.internalError(w, "api: rotate vault credential: write credential failed", err)
		return
	}
	if err := rt.vault.SetVaultCredentialRotation(ctx, now, expires); err != nil {
		rt.internalError(w, "api: rotate vault credential: record rotation failed", err)
		return
	}
	rt.saveRotationAudit(ctx, auditActionVaultRotate, "/api/v1/settings/vault", rt.accessActor(r).name)
	rt.logger.Info("api: vault credential rotated", slog.Int("grace_hours", grace))
	settings, err := rt.vault.GetVaultSettings(ctx)
	if err != nil {
		rt.internalError(w, "api: rotate vault credential: reload settings failed", err)
		return
	}
	writeJSON(w, http.StatusOK, rt.toVaultSettingsResource(ctx, settings))
}

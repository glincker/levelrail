package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/footprint"
	"github.com/GLINCKER/levelrail/internal/registrytoken"
	"github.com/GLINCKER/levelrail/internal/store"
)

// RegistryTokenRefresher mints and renews cloud registry logins;
// *registrytoken.Refresher satisfies it.
type RegistryTokenRefresher interface {
	Preview(ctx context.Context, preset string, cfg registrytoken.Config) (registrytoken.Minted, error)
	Refresh(ctx context.Context, id string) (store.RegistryCredential, error)
	NextRefresh(now, expires time.Time) time.Time
	Tick(ctx context.Context) error
	Interval() time.Duration
}

// WithRegistryRefresher enables the ECR, Artifact Registry and ACR presets.
// Without it those requests return 501.
func WithRegistryRefresher(r RegistryTokenRefresher) Option {
	return func(rt *Router) { rt.registryRefresher = r }
}

// RunRegistryRefresher renews preset registry credentials until ctx ends.
func (rt *Router) RunRegistryRefresher(ctx context.Context) error {
	if rt.registryRefresher == nil {
		return nil
	}
	return footprint.Every(ctx, "registry credential refresh", rt.registryRefresher.Interval(), rt.logger, rt.registryRefresher.Tick)
}

func validRegistryPreset(p string) bool {
	switch p {
	case registrytoken.PresetECR, registrytoken.PresetArtifactRegistry, registrytoken.PresetACR:
		return true
	}
	return false
}

type registryPresetRequest struct {
	Preset       string `json:"preset"`
	Region       string `json:"region"`
	RegistryHost string `json:"registry_host"`
}

func (q registryPresetRequest) config() registrytoken.Config {
	return registrytoken.Config{Region: q.Region, RegistryHost: q.RegistryHost}
}

func validateRegistryPresetRequest(q registryPresetRequest) error {
	if !validRegistryPreset(q.Preset) {
		return errors.New("preset must be ecr, artifact_registry or acr")
	}
	if q.Preset == registrytoken.PresetACR && q.RegistryHost == "" {
		return errors.New("registry_host is required for acr, for example myregistry.azurecr.io")
	}
	return nil
}

// registryPresetStatus maps a mint failure to a client-facing status and message.
func registryPresetStatus(err error) (int, string) {
	if errors.Is(err, registrytoken.ErrNoCloudConnection) {
		return http.StatusPreconditionFailed, "connect the matching cloud account first (Settings, Nodes, provider credentials)"
	}
	return http.StatusBadGateway, registrytoken.SafeError(err)
}

type registryPresetTestResponse struct {
	RegistryHost string    `json:"registry_host"`
	Username     string    `json:"username"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// handleTestRegistryPreset handles POST /api/v1/registry-credentials/presets/test:
// mints a login to prove the cloud connection works, stores nothing and
// returns no password.
func (rt *Router) handleTestRegistryPreset(w http.ResponseWriter, r *http.Request) {
	if rt.registryRefresher == nil {
		writeError(w, http.StatusNotImplemented, "registry presets are not configured on this control plane (no master key set)")
		return
	}
	var req registryPresetRequest
	if !decodeReq(w, r, &req) {
		return
	}
	if err := validateRegistryPresetRequest(req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), registryCredentialTestTimeout)
	defer cancel()
	minted, err := rt.registryRefresher.Preview(ctx, req.Preset, req.config())
	if err != nil {
		status, msg := registryPresetStatus(err)
		writeError(w, status, msg)
		return
	}
	writeJSON(w, http.StatusOK, registryPresetTestResponse{RegistryHost: minted.RegistryHost, Username: minted.Username, ExpiresAt: minted.ExpiresAt})
}

// createPresetRegistryCredential is handleCreateRegistryCredential's preset
// branch: mint first, so a credential that cannot log in is never stored.
func (rt *Router) createPresetRegistryCredential(w http.ResponseWriter, r *http.Request, req createRegistryCredentialRequest) {
	if rt.registryRefresher == nil {
		writeError(w, http.StatusNotImplemented, "registry presets are not configured on this control plane (no master key set)")
		return
	}
	preq := registryPresetRequest{Preset: req.Preset, Region: req.Region, RegistryHost: req.RegistryHost}
	if err := validateRegistryPresetRequest(preq); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), registryCredentialTestTimeout)
	defer cancel()
	minted, err := rt.registryRefresher.Preview(ctx, req.Preset, preq.config())
	if err != nil {
		status, msg := registryPresetStatus(err)
		writeError(w, status, msg)
		return
	}
	cfgJSON, err := registrytoken.EncodeConfig(preq.config())
	if err != nil {
		rt.internalError(w, "api: create registry preset: encode config failed", err)
		return
	}
	id, err := randomRegistryCredentialID()
	if err != nil {
		rt.internalError(w, "api: create registry preset: generate id failed", err)
		return
	}
	if err := rt.registryCredentialSecrets.SetValue(r.Context(), store.RegistryCredentialSecretsKey(id), "password", minted.Password); err != nil {
		rt.internalError(w, "api: create registry preset: set password failed", err, slog.String("id", id))
		return
	}
	now := time.Now().UTC()
	next := rt.registryRefresher.NextRefresh(now, minted.ExpiresAt)
	exp := minted.ExpiresAt
	cred := store.RegistryCredential{
		ID: id, Name: req.Name, RegistryHost: minted.RegistryHost, Username: minted.Username,
		CreatedAt: now.Format(time.RFC3339), ExpiresAt: &exp,
		Preset: req.Preset, PresetConfig: cfgJSON, LastRefreshedAt: &now, NextRefreshAt: &next,
	}
	if err := rt.registryCredentials.SaveRegistryCredential(r.Context(), cred); err != nil {
		rt.internalError(w, "api: create registry preset: save failed", err, slog.String("id", id))
		return
	}
	writeJSON(w, http.StatusCreated, rt.toRegistryCredentialResource(cred))
}

// handleRefreshRegistryCredential handles POST
// /api/v1/registry-credentials/{id}/refresh: the Fix action for a failing refresh.
func (rt *Router) handleRefreshRegistryCredential(w http.ResponseWriter, r *http.Request) {
	if rt.registryRefresher == nil {
		writeError(w, http.StatusNotImplemented, "registry presets are not configured on this control plane (no master key set)")
		return
	}
	id := r.PathValue("id")
	if _, err := rt.registryCredentials.GetRegistryCredential(r.Context(), id); errors.Is(err, store.ErrRegistryCredentialNotFound) {
		writeError(w, http.StatusNotFound, "registry credential not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: refresh registry credential: load failed", err, slog.String("id", id))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), registryCredentialTestTimeout)
	defer cancel()
	updated, err := rt.registryRefresher.Refresh(ctx, id)
	if err != nil {
		status, msg := registryPresetStatus(fmt.Errorf("refresh: %w", err))
		writeError(w, status, msg)
		return
	}
	writeJSON(w, http.StatusOK, rt.toRegistryCredentialResource(updated))
}

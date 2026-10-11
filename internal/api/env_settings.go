package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/GLINCKER/levelrail/internal/envredact"
	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

const maxSensitiveOverrides = 500

type envSettingsResponse struct {
	RevealPolicy       string          `json:"reveal_policy"`
	PolicyValid        bool            `json:"policy_valid"`
	TokenReveal        bool            `json:"token_reveal"`
	SecretsWriteOnly   bool            `json:"secrets_write_only"`
	SensitiveOverrides map[string]bool `json:"sensitive_overrides"`
	GrantTTLSeconds    int             `json:"grant_ttl_seconds"`
	HideAfterSeconds   int             `json:"hide_after_seconds"`
	SecretsConfigured  bool            `json:"secrets_configured"`
}

func (rt *Router) envSettingsResponse(s store.AppEnvSettings) envSettingsResponse {
	policy, valid := revealPolicy()
	overrides := s.SensitiveOverrides
	if overrides == nil {
		overrides = map[string]bool{}
	}
	return envSettingsResponse{
		RevealPolicy: policy, PolicyValid: valid, TokenReveal: tokenRevealAllowed(),
		SecretsWriteOnly: s.SecretsWriteOnly, SensitiveOverrides: overrides,
		GrantTTLSeconds: int(stepUpTTL().Seconds()), HideAfterSeconds: int(revealHideAfter().Seconds()),
		SecretsConfigured: rt.secrets != nil,
	}
}

// handleGetEnvSettings handles GET /api/v1/apps/{name}/env/settings.
func (rt *Router) handleGetEnvSettings(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if _, ok := rt.loadRevealApp(w, r, name); !ok {
		return
	}
	s, ok := rt.envSettingsFor(r.Context(), name)
	if !ok {
		writeCodedError(w, http.StatusServiceUnavailable, errCodeRevealSettingsFailed, "could not read this app's protection settings, try again")
		return
	}
	writeJSON(w, http.StatusOK, rt.envSettingsResponse(s))
}

type putEnvSettingsRequest struct {
	SensitiveOverrides map[string]*bool `json:"sensitive_overrides,omitempty"`
	SecretsWriteOnly   *bool            `json:"secrets_write_only,omitempty"`
	StepUp             *stepUpProof     `json:"step_up,omitempty"`
}

// loosenedKeys lists variables that would stop being masked under next.
func loosenedKeys(env map[string]string, prev, next map[string]bool) []string {
	var out []string
	for k, v := range env {
		before, _ := envredact.Value(k, v, prev)
		after, _ := envredact.Value(k, v, next)
		if before == envredact.Placeholder && after != envredact.Placeholder {
			out = append(out, k)
		}
	}
	return out
}

// handlePutEnvSettings handles PUT /api/v1/apps/{name}/env/settings.
// Tightening needs write:sensitive; loosening also needs step-up.
func (rt *Router) handlePutEnvSettings(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if rt.envProtect.settings == nil {
		writeError(w, http.StatusNotImplemented, "env protection settings are not available on this control plane")
		return
	}
	var req putEnvSettingsRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, revealBodyLimit*4)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	svc, ok := rt.loadRevealApp(w, r, name)
	if !ok {
		return
	}
	cur, ok := rt.envSettingsFor(r.Context(), name)
	if !ok {
		writeCodedError(w, http.StatusServiceUnavailable, errCodeRevealSettingsFailed, "could not read this app's protection settings, try again")
		return
	}
	next := store.AppEnvSettings{AppName: name, SecretsWriteOnly: cur.SecretsWriteOnly, SensitiveOverrides: map[string]bool{}}
	for k, v := range cur.SensitiveOverrides {
		next.SensitiveOverrides[k] = v
	}
	for k, v := range req.SensitiveOverrides {
		if strings.TrimSpace(k) == "" {
			writeError(w, http.StatusBadRequest, "override keys must not be empty")
			return
		}
		if v == nil {
			delete(next.SensitiveOverrides, k)
			continue
		}
		next.SensitiveOverrides[k] = *v
	}
	if len(next.SensitiveOverrides) > maxSensitiveOverrides {
		writeError(w, http.StatusBadRequest, "too many sensitivity overrides")
		return
	}
	if req.SecretsWriteOnly != nil {
		next.SecretsWriteOnly = *req.SecretsWriteOnly
	}

	loosened := loosenedKeys(svc.Env, cur.SensitiveOverrides, next.SensitiveOverrides)
	unlocking := cur.SecretsWriteOnly && !next.SecretsWriteOnly
	if len(loosened) > 0 || unlocking {
		opts := revealOptions{ignoreLock: unlocking, ignorePolicy: len(loosened) == 0}
		actor, ok := rt.authorizeReveal(w, r, name, stepUpActionManage, req.StepUp, opts)
		if !ok {
			return
		}
		rt.saveEnvAudit(r.Context(), r, actor, auditActionEnvSettings, "apps/"+name+"/env/settings (loosened "+strings.Join(loosened, ",")+")", http.StatusOK)
	}
	if err := rt.envProtect.settings.SaveAppEnvSettings(r.Context(), next); err != nil {
		rt.internalError(w, "api: save env settings failed", err, slog.String("name", name))
		return
	}
	rt.logger.Info("api: env settings changed", slog.String("name", name), slog.Int("overrides", len(next.SensitiveOverrides)), slog.Bool("write_only", next.SecretsWriteOnly))
	writeJSON(w, http.StatusOK, rt.envSettingsResponse(next))
}

type plainEnvKeyRemover interface {
	RemoveServicePlainEnvKey(ctx context.Context, name, key string) (bool, error)
}

// handleConvertEnvToSecret handles POST
// /api/v1/apps/{name}/env/{key}/convert-to-secret: moves a plain value into
// encrypted storage server side, so the value never travels to the browser.
func (rt *Router) handleConvertEnvToSecret(w http.ResponseWriter, r *http.Request) {
	if rt.secrets == nil {
		writeError(w, http.StatusNotImplemented, "secrets are not configured on this control plane (no master key set)")
		return
	}
	remover, ok := rt.apps.(plainEnvKeyRemover)
	if !ok {
		writeError(w, http.StatusNotImplemented, "converting variables is not available on this control plane")
		return
	}
	name, key := r.PathValue("name"), r.PathValue("key")
	svc, ok := rt.loadRevealApp(w, r, name)
	if !ok {
		return
	}
	value, exists := svc.Env[key]
	if !exists {
		writeError(w, http.StatusNotFound, "no plain variable with this key on this app")
		return
	}
	if value == "" {
		writeError(w, http.StatusBadRequest, "an empty variable cannot become a secret")
		return
	}
	err := rt.secrets.SetValueGuarded(r.Context(), name, key, value, false)
	if errors.Is(err, secrets.ErrSecretLocked) {
		writeError(w, http.StatusConflict, "a locked secret with this key already exists")
		return
	}
	if err != nil {
		rt.internalError(w, "api: convert to secret: store value failed", err, slog.String("name", name), slog.String("key", key))
		return
	}
	if _, err := rt.declareSecretEnv(r.Context(), name, key, true); err != nil {
		rt.internalError(w, "api: convert to secret: declare failed", err, slog.String("name", name), slog.String("key", key))
		return
	}
	if _, err := remover.RemoveServicePlainEnvKey(r.Context(), name, key); err != nil {
		rt.logger.Error("api: convert to secret: remove plain copy failed", slog.String("error", err.Error()), slog.String("name", name), slog.String("key", key))
		writeError(w, http.StatusInternalServerError, "the secret was saved but the plain copy was not removed, retry the request")
		return
	}
	rt.recordAppEvent(r, store.AppEvent{AppName: name, Kind: store.AppEventSecretChange, Keys: []string{key}, Title: "Converted to secret: " + key})
	rt.nudgeReconciler()
	w.WriteHeader(http.StatusNoContent)
}

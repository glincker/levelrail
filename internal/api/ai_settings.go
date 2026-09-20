package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/ai"
	"github.com/GLINCKER/levelrail/internal/store"
)

// AIAssistantSettingsStore is the core internal/store surface GET/PUT/
// DELETE /api/v1/settings/ai-assistant need: always set, the provider/
// model row always exists (migrations/0106's own seeded row), the same
// "core Store interface" shape ingressSettings/emailSettings already
// establish.
type AIAssistantSettingsStore interface {
	GetAIAssistantSettings(ctx context.Context) (store.AIAssistantSettings, error)
	UpdateAIAssistantSettings(ctx context.Context, s store.AIAssistantSettings) error
}

// AIAssistantSecrets is the internal/secrets.Manager surface PUT/DELETE
// need to store and clear the BYOK key. nil is valid: both routes return
// 501 without one configured, the same shape EmailSecretsStore's absence
// produces; GET works regardless.
type AIAssistantSecrets interface {
	SetValue(ctx context.Context, serviceName, envKey, plaintext string) error
	Exists(ctx context.Context, serviceName, envKey string) (bool, error)
	DeleteAll(ctx context.Context, serviceName string) error
}

var errAIProviderInvalid = errors.New("provider must be \"anthropic\"")

type aiAssistantSettingsResource struct {
	Configured bool   `json:"configured"`
	Provider   string `json:"provider"`
	Model      string `json:"model"`
}

type updateAIAssistantSettingsRequest struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	APIKey   string `json:"api_key"`
}

// handleGetAIAssistantSettings handles GET /api/v1/settings/ai-assistant.
// configured reports whether a key has actually been saved, not merely
// whether provider/model fields are set; the key itself never appears in
// the response.
func (rt *Router) handleGetAIAssistantSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := rt.aiSettings.GetAIAssistantSettings(r.Context())
	if err != nil {
		rt.logger.Error("api: get ai assistant settings failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	var configured bool
	if rt.aiSecrets != nil {
		configured, err = rt.aiSecrets.Exists(r.Context(), store.AIAssistantSecretsKey(), ai.SecretsAPIKeyEnvKey)
		if err != nil {
			rt.logger.Error("api: get ai assistant settings: check key failed", slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
	}

	writeJSON(w, http.StatusOK, aiAssistantSettingsResource{
		Configured: configured, Provider: settings.Provider, Model: settings.Model,
	})
}

// handleUpdateAIAssistantSettings handles
// PUT /api/v1/settings/ai-assistant. The key is written to
// internal/secrets before the settings row, matching
// handleUpdateEmailSettings' own ordering: a failed store write leaves an
// orphaned secret, not a settings row that looks configured but isn't.
func (rt *Router) handleUpdateAIAssistantSettings(w http.ResponseWriter, r *http.Request) {
	if rt.aiSecrets == nil {
		writeError(w, http.StatusNotImplemented, "the ai assistant is not configurable on this control plane (no master key set)")
		return
	}

	var req updateAIAssistantSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Provider != store.AIProviderAnthropic {
		writeError(w, http.StatusBadRequest, errAIProviderInvalid.Error())
		return
	}
	if req.Model == "" {
		writeError(w, http.StatusBadRequest, "model is required")
		return
	}
	if req.APIKey == "" {
		writeError(w, http.StatusBadRequest, "api_key is required")
		return
	}

	key := store.AIAssistantSecretsKey()
	if err := rt.aiSecrets.SetValue(r.Context(), key, ai.SecretsAPIKeyEnvKey, req.APIKey); err != nil {
		rt.logger.Error("api: update ai assistant settings: set key failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	settings := store.AIAssistantSettings{Provider: req.Provider, Model: req.Model}
	if err := rt.aiSettings.UpdateAIAssistantSettings(r.Context(), settings); err != nil {
		rt.logger.Error("api: update ai assistant settings failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, aiAssistantSettingsResource{Configured: true, Provider: settings.Provider, Model: settings.Model})
}

// handleDeleteAIAssistantSettings handles
// DELETE /api/v1/settings/ai-assistant: clears the stored key and resets
// provider/model, the same "clear secret, then reset the settings row"
// order handleDisconnectVault already establishes.
func (rt *Router) handleDeleteAIAssistantSettings(w http.ResponseWriter, r *http.Request) {
	if rt.aiSecrets == nil {
		writeError(w, http.StatusNotImplemented, "the ai assistant is not configurable on this control plane (no master key set)")
		return
	}

	if err := rt.aiSecrets.DeleteAll(r.Context(), store.AIAssistantSecretsKey()); err != nil {
		rt.logger.Error("api: delete ai assistant settings: clear key failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if err := rt.aiSettings.UpdateAIAssistantSettings(r.Context(), store.AIAssistantSettings{}); err != nil {
		rt.logger.Error("api: delete ai assistant settings failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, aiAssistantSettingsResource{})
}

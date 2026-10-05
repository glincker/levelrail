package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/kit/upgrade"
)

// UpdateSettingsStore is the store surface the update settings handlers
// need. *store.DB satisfies this structurally, the same consumer-defined
// interface convention IngressSettingsStore already establishes.
type UpdateSettingsStore interface {
	GetUpdateSettings(ctx context.Context) (store.UpdateSettings, error)
	UpdateUpdateSettings(ctx context.Context, s store.UpdateSettings) error
}

// updateSettingsResource is the wire shape for GET and PUT
// /api/v1/updates/settings, mirroring store.UpdateSettings field for field.
type updateSettingsResource struct {
	Channel           string `json:"channel"`
	AutoUpdateEnabled bool   `json:"auto_update_enabled"`
}

func toUpdateSettingsResource(s store.UpdateSettings) updateSettingsResource {
	return updateSettingsResource{Channel: s.Channel, AutoUpdateEnabled: s.AutoUpdateEnabled}
}

// handleGetUpdateSettings handles GET /api/v1/updates/settings: the
// single platform-wide row (migrations/0258_update_settings.sql), always
// present, never a 404. AbilityRoot on both this and its PUT sibling
// below: whether this instance auto-applies unreviewed releases isn't
// exposed to a merely-read-scoped token.
func (rt *Router) handleGetUpdateSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := rt.updateSettings.GetUpdateSettings(r.Context())
	if err != nil {
		rt.logger.Error("api: get update settings failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, toUpdateSettingsResource(settings))
}

// handleUpdateSettings handles PUT /api/v1/updates/settings: sets the
// release channel internal/updatecheck.Scheduler and GET /api/v1/updates
// compare against, and whether that scheduler's periodic check runs at
// all. Gated at AbilityRoot in routes.go.
func (rt *Router) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	var req updateSettingsResource
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !upgrade.ValidChannel(req.Channel) {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("channel must be %q, %q, or %q", upgrade.ChannelStable, upgrade.ChannelBeta, upgrade.ChannelEdge))
		return
	}

	settings := store.UpdateSettings{Channel: req.Channel, AutoUpdateEnabled: req.AutoUpdateEnabled}
	if err := rt.updateSettings.UpdateUpdateSettings(r.Context(), settings); err != nil {
		rt.logger.Error("api: update update settings failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, toUpdateSettingsResource(settings))
}

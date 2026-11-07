package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

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

// updateSettingsResponse adds the channels the dashboard offers; others stay valid for the API but hidden.
type updateSettingsResponse struct {
	updateSettingsResource
	OfferedChannels []string `json:"offered_channels"`
}

// envOfferedChannels seeds which channels are offered for selection; only stable until 1.0.
const envOfferedChannels = "APP_UPDATE_CHANNELS_OFFERED"

func offeredUpdateChannels() []string {
	raw, ok := os.LookupEnv(envOfferedChannels)
	if !ok || strings.TrimSpace(raw) == "" {
		return []string{upgrade.ChannelStable}
	}
	out := []string{}
	for _, c := range strings.Split(raw, ",") {
		if c = strings.TrimSpace(c); upgrade.ValidChannel(c) {
			out = append(out, c)
		}
	}
	if len(out) == 0 {
		return []string{upgrade.ChannelStable}
	}
	return out
}

func (rt *Router) toUpdateSettingsResource(ctx context.Context, s store.UpdateSettings) updateSettingsResponse {
	return updateSettingsResponse{
		updateSettingsResource: updateSettingsResource{Channel: s.Channel, AutoUpdateEnabled: s.AutoUpdateEnabled},
		OfferedChannels:        rt.offeredUpdateChannels(ctx),
	}
}

// handleGetUpdateSettings handles GET /api/v1/updates/settings: the
// single platform-wide row (migrations/0258_update_settings.sql), always
// present, never a 404. AbilityRoot on both this and its PUT sibling
// below: whether this instance auto-applies unreviewed releases isn't
// exposed to a merely-read-scoped token.
func (rt *Router) handleGetUpdateSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := rt.updateSettings.GetUpdateSettings(r.Context())
	if err != nil {
		rt.internalError(w, "api: get update settings failed", err)
		return
	}
	writeJSON(w, http.StatusOK, rt.toUpdateSettingsResource(r.Context(), settings))
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
		rt.internalError(w, "api: update update settings failed", err)
		return
	}
	writeJSON(w, http.StatusOK, rt.toUpdateSettingsResource(r.Context(), settings))
}

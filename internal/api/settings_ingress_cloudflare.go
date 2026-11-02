package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/cloudflareips"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

// cloudflarePresetResource is the wire shape of /api/v1/settings/ingress/cloudflare; source is stored, env or default.
type cloudflarePresetResource struct {
	Preset          string     `json:"preset"`
	Source          string     `json:"source"`
	Ranges          int        `json:"ranges"`
	RangesSource    string     `json:"ranges_source"`
	LastRefresh     *time.Time `json:"last_refresh,omitempty"`
	LastError       string     `json:"last_error,omitempty"`
	RefreshInterval string     `json:"refresh_interval"`
}

type cloudflarePresetRequest struct {
	Preset           string `json:"preset"`
	ConfirmWeakening bool   `json:"confirm_weakening"`
}

func (rt *Router) cloudflarePresetState(ctx context.Context) cloudflarePresetResource {
	st := cloudflareips.Default().Status()
	out := cloudflarePresetResource{
		Preset:          rt.runtimeSettings.TrustedProxyPreset(ctx),
		Source:          rt.runtimeSettings.Source(ctx, runtimesettings.KeyIngressTrustedProxyPreset),
		Ranges:          st.Ranges,
		RangesSource:    st.Source,
		LastError:       st.LastError,
		RefreshInterval: rt.runtimeSettings.Duration(ctx, runtimesettings.KeyCloudflareIPsRefresh).String(),
	}
	if !st.LastRefresh.IsZero() {
		out.LastRefresh = &st.LastRefresh
	}
	return out
}

func (rt *Router) handleGetIngressCloudflare(w http.ResponseWriter, r *http.Request) {
	if !rt.runtimeSettingsOr501(w) {
		return
	}
	writeJSON(w, http.StatusOK, rt.cloudflarePresetState(r.Context()))
}

func (rt *Router) handlePutIngressCloudflare(w http.ResponseWriter, r *http.Request) {
	if !rt.runtimeSettingsOr501(w) {
		return
	}
	var req cloudflarePresetRequest
	if err := decodeJSONBody(r, &req); err != nil || req.Preset == "" {
		writeError(w, http.StatusBadRequest, "body must be {\"preset\": \"none|cloudflare\"}")
		return
	}
	ctx := r.Context()
	raw, _ := json.Marshal(req.Preset)
	before, err := rt.runtimeSettings.Get(ctx, runtimesettings.KeyIngressTrustedProxyPreset)
	if err != nil {
		rt.writeSettingError(w, "api: get trusted proxy preset", err)
		return
	}
	if rt.runtimeSettings.WeakensTo(ctx, runtimesettings.KeyIngressTrustedProxyPreset, raw) && !req.ConfirmWeakening {
		writeJSON(w, http.StatusBadRequest, runtimeSettingError{
			Error: "cloudflare makes the ingress trust CF-Connecting-IP from Cloudflare's ranges, which any Cloudflare customer can reach; send confirm_weakening: true to proceed",
			Code:  errCodeConfirmRequired, Field: "confirm_weakening",
		})
		return
	}
	after, err := rt.runtimeSettings.Set(ctx, runtimesettings.KeyIngressTrustedProxyPreset, raw, rt.eventActor(r))
	if err != nil {
		rt.writeSettingError(w, "api: set trusted proxy preset", err)
		return
	}
	rt.recordSettingChange(r, before, after)
	rt.nudgeReconciler()
	writeJSON(w, http.StatusOK, rt.cloudflarePresetState(ctx))
}

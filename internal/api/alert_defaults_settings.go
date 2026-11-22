package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

// AlertDigestPreview renders the weekly digest for the last seven days without sending it.
type AlertDigestPreview func(ctx context.Context) (subject, text string, err error)

// SetAlertDigestPreview enables the preview route after the router is built, once its data sources exist.
func (rt *Router) SetAlertDigestPreview(p AlertDigestPreview) { rt.alertDigestPreview = p }

// WithAlertDigestPreview enables GET /api/v1/alert-defaults/digest-preview.
func WithAlertDigestPreview(p AlertDigestPreview) Option {
	return func(rt *Router) { rt.alertDigestPreview = p }
}

type alertDefaultsSettings struct {
	Enabled           bool   `json:"enabled"`
	CrashloopRestarts int    `json:"crashloop_restarts"`
	ErrorRatePercent  int    `json:"error_rate_percent"`
	DigestSchedule    string `json:"digest_schedule"`
}

func (rt *Router) alertDefaultsSettingsNow(ctx context.Context) alertDefaultsSettings {
	s := rt.runtimeSettings
	return alertDefaultsSettings{
		Enabled:           s.Bool(ctx, runtimesettings.KeyAlertsDefaultsEnabled),
		CrashloopRestarts: s.Int(ctx, runtimesettings.KeyAlertsDefaultCrashLoop),
		ErrorRatePercent:  s.Int(ctx, runtimesettings.KeyAlertsDefault5xxPercent),
		DigestSchedule:    s.String(ctx, runtimesettings.KeyAlertsDigestSchedule),
	}
}

func (rt *Router) handleGetAlertDefaultsSettings(w http.ResponseWriter, r *http.Request) {
	if rt.runtimeSettings == nil {
		writeError(w, http.StatusNotImplemented, "runtime settings are not available")
		return
	}
	writeJSON(w, http.StatusOK, rt.alertDefaultsSettingsNow(r.Context()))
}

// seeder is satisfied by *alerting.DB; turning the switch on creates any default rule not yet seeded.
type alertDefaultsSeeder interface {
	SeedDefaultsFromEnv(ctx context.Context) error
}

func (rt *Router) handlePutAlertDefaultsSettings(w http.ResponseWriter, r *http.Request) {
	if rt.runtimeSettings == nil {
		writeError(w, http.StatusNotImplemented, "runtime settings are not available")
		return
	}
	type alertDefaultsReq struct {
		Enabled           *bool   `json:"enabled"`
		CrashloopRestarts *int    `json:"crashloop_restarts"`
		ErrorRatePercent  *int    `json:"error_rate_percent"`
		DigestSchedule    *string `json:"digest_schedule"`
	}
	req, ok := decodeJSON[alertDefaultsReq](w, r, 1<<10, decodeMessage("body must be JSON with any of enabled, crashloop_restarts, error_rate_percent, digest_schedule"))
	if !ok {
		return
	}
	ctx, actor := r.Context(), rt.eventActor(r)
	set := func(key string, raw []byte) bool {
		if _, err := rt.runtimeSettings.Set(ctx, key, json.RawMessage(raw), actor); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return false
		}
		return true
	}
	if req.Enabled != nil && !set(runtimesettings.KeyAlertsDefaultsEnabled, []byte(strconv.FormatBool(*req.Enabled))) {
		return
	}
	if req.CrashloopRestarts != nil && !set(runtimesettings.KeyAlertsDefaultCrashLoop, []byte(strconv.Itoa(*req.CrashloopRestarts))) {
		return
	}
	if req.ErrorRatePercent != nil && !set(runtimesettings.KeyAlertsDefault5xxPercent, []byte(strconv.Itoa(*req.ErrorRatePercent))) {
		return
	}
	if req.DigestSchedule != nil {
		raw, _ := json.Marshal(*req.DigestSchedule)
		if !set(runtimesettings.KeyAlertsDigestSchedule, raw) {
			return
		}
	}
	if req.Enabled != nil && *req.Enabled {
		if sd, ok := rt.alertDefaults.(alertDefaultsSeeder); ok {
			if err := sd.SeedDefaultsFromEnv(ctx); err != nil {
				rt.internalError(w, "api: seed default alerts failed", err)
				return
			}
		}
	}
	writeJSON(w, http.StatusOK, rt.alertDefaultsSettingsNow(ctx))
}

func (rt *Router) handleAlertDigestPreview(w http.ResponseWriter, r *http.Request) {
	if rt.alertDigestPreview == nil {
		writeError(w, http.StatusNotImplemented, "the weekly digest is not available on this control plane")
		return
	}
	subject, text, err := rt.alertDigestPreview(r.Context())
	if err != nil {
		rt.internalError(w, "api: weekly digest preview failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"subject": subject, "text": text})
}

package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	// AuditActionSettingsChange is the audit action written for every runtime setting change.
	AuditActionSettingsChange = "settings_change"

	runtimeSettingsPathPrefix = "/api/v1/settings/runtime/"
	errCodeInvalidValue       = "invalid_value"
	errCodeConfirmRequired    = "confirmation_required"
)

// loopbackDistrusted is read on every request, so it is an atomic set once at boot.
var loopbackDistrusted atomic.Bool

func loopbackTrusted() bool { return !loopbackDistrusted.Load() }

func (rt *Router) attachRuntimeSettings(s Store) {
	rs, ok := any(s).(runtimesettings.Store)
	if !ok {
		return
	}
	svc := runtimesettings.New(rs, nil)
	if err := svc.CaptureBoot(context.Background()); err != nil {
		rt.logger.Warn("api: capture runtime settings at boot failed", slog.String("error", err.Error()))
	}
	rt.runtimeSettings = svc
	loopbackDistrusted.Store(!svc.Bool(context.Background(), runtimesettings.KeyTrustLoopbackForwarded))
}

// RuntimeSettings exposes the resolver so boot code can inject it into packages the API does not own.
func (rt *Router) RuntimeSettings() *runtimesettings.Service { return rt.runtimeSettings }

func (rt *Router) webhookReplayEnabled(ctx context.Context) bool {
	return rt.runtimeSettings.Bool(ctx, runtimesettings.KeyWebhookReplayProtection)
}

func (rt *Router) webhookReplayWindow(ctx context.Context) time.Duration {
	return rt.runtimeSettings.Duration(ctx, runtimesettings.KeyWebhookReplayWindow)
}

// sensitiveReadAudited reports whether r is a sensitive read the operator wants audited.
func (rt *Router) sensitiveReadAudited(r *http.Request) bool {
	return sensitiveReadRoute(r) && rt.runtimeSettings.Bool(r.Context(), runtimesettings.KeyAuditSensitiveReads)
}

type runtimeSettingsRestart struct {
	Supported      bool `json:"supported"`
	PendingRestart bool `json:"pending_restart"`
}

type runtimeSettingsList struct {
	Settings []runtimesettings.Setting `json:"settings"`
	Restart  runtimeSettingsRestart    `json:"restart"`
}

type runtimeOnboarding struct {
	Items    []runtimesettings.Setting `json:"items"`
	Reviewed bool                      `json:"reviewed"`
}

type runtimeSettingRequest struct {
	Value            json.RawMessage `json:"value"`
	ConfirmWeakening bool            `json:"confirm_weakening"`
}

type runtimeSettingError struct {
	Error string `json:"error"`
	Code  string `json:"code"`
	Field string `json:"field"`
}

type runtimeSettingChange struct {
	Key string          `json:"key"`
	Old json.RawMessage `json:"old"`
	New json.RawMessage `json:"new"`
}

func (rt *Router) runtimeSettingsOr501(w http.ResponseWriter) bool {
	if rt.runtimeSettings == nil {
		writeError(w, http.StatusNotImplemented, "runtime settings are not available on this control plane")
		return false
	}
	return true
}

func (rt *Router) writeSettingError(w http.ResponseWriter, what string, err error) {
	var verr *runtimesettings.ValidationError
	switch {
	case errors.Is(err, runtimesettings.ErrUnknownKey):
		writeError(w, http.StatusNotFound, "unknown setting")
	case errors.As(err, &verr):
		writeJSON(w, http.StatusBadRequest, runtimeSettingError{Error: verr.Message, Code: errCodeInvalidValue, Field: "value"})
	default:
		rt.internalError(w, what, err)
	}
}

func (rt *Router) handleListRuntimeSettings(w http.ResponseWriter, r *http.Request) {
	if !rt.runtimeSettingsOr501(w) {
		return
	}
	all, err := rt.runtimeSettings.All(r.Context())
	if err != nil {
		rt.internalError(w, "api: list runtime settings", err)
		return
	}
	pending, err := rt.runtimeSettings.AnyPendingRestart(r.Context())
	if err != nil {
		rt.internalError(w, "api: runtime settings pending restart", err)
		return
	}
	writeJSON(w, http.StatusOK, runtimeSettingsList{
		Settings: all,
		Restart:  runtimeSettingsRestart{Supported: rt.restartResource().Supported, PendingRestart: pending},
	})
}

func (rt *Router) handleGetRuntimeSetting(w http.ResponseWriter, r *http.Request) {
	if !rt.runtimeSettingsOr501(w) {
		return
	}
	s, err := rt.runtimeSettings.Get(r.Context(), r.PathValue("key"))
	if err != nil {
		rt.writeSettingError(w, "api: get runtime setting", err)
		return
	}
	writeJSON(w, http.StatusOK, s)
}

func (rt *Router) handleSetRuntimeSetting(w http.ResponseWriter, r *http.Request) {
	if !rt.runtimeSettingsOr501(w) {
		return
	}
	key := r.PathValue("key")
	var req runtimeSettingRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	before, err := rt.runtimeSettings.Get(r.Context(), key)
	if err != nil {
		rt.writeSettingError(w, "api: set runtime setting", err)
		return
	}
	if rt.runtimeSettings.WeakensTo(r.Context(), key, req.Value) && !req.ConfirmWeakening {
		writeJSON(w, http.StatusBadRequest, runtimeSettingError{
			Error: "turning this protection off weakens security; send confirm_weakening: true to proceed",
			Code:  errCodeConfirmRequired, Field: "confirm_weakening",
		})
		return
	}
	after, err := rt.runtimeSettings.Set(r.Context(), key, req.Value, rt.eventActor(r))
	if err != nil {
		rt.writeSettingError(w, "api: set runtime setting", err)
		return
	}
	rt.recordSettingChange(r, before, after)
	writeJSON(w, http.StatusOK, after)
}

func (rt *Router) handleResetRuntimeSetting(w http.ResponseWriter, r *http.Request) {
	if !rt.runtimeSettingsOr501(w) {
		return
	}
	key := r.PathValue("key")
	before, err := rt.runtimeSettings.Get(r.Context(), key)
	if err != nil {
		rt.writeSettingError(w, "api: reset runtime setting", err)
		return
	}
	after, err := rt.runtimeSettings.Reset(r.Context(), key)
	if err != nil {
		rt.writeSettingError(w, "api: reset runtime setting", err)
		return
	}
	rt.recordSettingChange(r, before, after)
	writeJSON(w, http.StatusOK, after)
}

func (rt *Router) handleApplyRuntimeSettings(w http.ResponseWriter, r *http.Request) {
	if !rt.runtimeSettingsOr501(w) {
		return
	}
	pending, err := rt.runtimeSettings.AnyPendingRestart(r.Context())
	if err != nil {
		rt.internalError(w, "api: runtime settings pending restart", err)
		return
	}
	if !pending {
		writeJSON(w, http.StatusOK, rt.restartResource())
		return
	}
	rt.handleSystemRestart(w, r)
}

func (rt *Router) handleGetRuntimeOnboarding(w http.ResponseWriter, r *http.Request) {
	if !rt.runtimeSettingsOr501(w) {
		return
	}
	rt.writeRuntimeOnboarding(w, r)
}

func (rt *Router) writeRuntimeOnboarding(w http.ResponseWriter, r *http.Request) {
	items, err := rt.runtimeSettings.Onboarding(r.Context())
	if err != nil {
		rt.internalError(w, "api: runtime settings onboarding", err)
		return
	}
	reviewed, err := rt.runtimeSettings.Reviewed(r.Context())
	if err != nil {
		rt.internalError(w, "api: runtime settings onboarding", err)
		return
	}
	writeJSON(w, http.StatusOK, runtimeOnboarding{Items: items, Reviewed: reviewed})
}

func (rt *Router) handleAckRuntimeOnboarding(w http.ResponseWriter, r *http.Request) {
	if !rt.runtimeSettingsOr501(w) {
		return
	}
	if err := rt.runtimeSettings.AckReviewed(r.Context(), rt.eventActor(r)); err != nil {
		rt.internalError(w, "api: ack runtime settings review", err)
		return
	}
	rt.writeRuntimeOnboarding(w, r)
}

// recordSettingChange writes the settings_change audit row; best effort like recordAudit.
func (rt *Router) recordSettingChange(r *http.Request, before, after runtimesettings.Setting) {
	id, err := store.NewAuditEntryID()
	if err != nil {
		rt.logger.Warn("api: settings change audit id failed", slog.String("key", after.Key))
		return
	}
	detail, _ := json.Marshal(runtimeSettingChange{Key: after.Key, Old: before.Value, New: after.Value})
	actorType, actorID, actorName, _ := rt.currentActor(r)
	kind := "session"
	if actorType == store.PrincipalTypeToken {
		kind = "token"
	}
	entry := store.AuditEntry{
		ID: id, ActorType: kind, ActorID: actorID, ActorName: actorName, Ability: AbilityRoot,
		Method: r.Method, Path: r.URL.Path, StatusCode: http.StatusOK, RemoteAddr: clientIP(r),
		CreatedAt: store.FormatAuditTime(time.Now()), ClientKind: clientKindFromUserAgent(r.Header.Get("User-Agent")),
		Action: AuditActionSettingsChange, Detail: string(detail),
	}
	if err := rt.auditLog.SaveAuditEntry(r.Context(), entry); err != nil {
		rt.logger.Warn("api: save settings change audit failed", slog.String("key", after.Key), slog.String("error", err.Error()))
		return
	}
	rt.emitAuditEvent(entry)
}

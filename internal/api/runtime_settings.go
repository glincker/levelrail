package api

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/GLINCKER/levelrail/internal/netclient"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/GLINCKER/levelrail/internal/abuse"
	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/iac"
	"github.com/GLINCKER/levelrail/internal/ingress"
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
	runtimesettings.BindRuntimeLimits(svc)
	build.SetNetworkResolver(func(ctx context.Context) (string, error) { return svc.StringChecked(ctx, build.KeyBuildNetwork) })
	wireBuildCacheSettings(svc)
	wireJobHTTPSettings(svc)
	wireJobGuardSettings(svc)
	netclient.SetTimeoutSource(func() time.Duration { return svc.Duration(context.Background(), runtimesettings.KeyNetOutboundTimeout) })
	wireNextSkewSettings(svc)
	wireDiagnosticSettings(svc)
	abuse.SetLiveLimits(svc.AbuseLimits)
	ingress.SetLiveAuthGateTimeout(svc.AuthGateTimeout)
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
	// Stack fields are set only when the change came from a stack apply.
	Stack    string `json:"stack,omitempty"`
	Revision int    `json:"revision,omitempty"`
	TokenID  string `json:"token_id,omitempty"`
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
	case errors.Is(err, runtimesettings.ErrSensitiveKey):
		writeError(w, http.StatusForbidden, "this caller cannot write a sensitive setting")
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
	// Operators may write sensitive settings here; stack-driven writers set RefuseSensitive.
	opts := runtimesettings.WriteOptions{ConfirmWeakening: req.ConfirmWeakening}
	if _, err := rt.runtimeSettings.ValidateForWrite(r.Context(), key, req.Value, opts); err != nil {
		if errors.Is(err, runtimesettings.ErrWeakeningNotConfirmed) {
			writeJSON(w, http.StatusBadRequest, runtimeSettingError{
				Error: "this change weakens a security protection (a protection turned off or shortened, or a limit raised); send confirm_weakening: true to proceed",
				Code:  errCodeConfirmRequired, Field: "confirm_weakening",
			})
			return
		}
		rt.writeSettingError(w, "api: set runtime setting", err)
		return
	}
	after, err := rt.runtimeSettings.Set(r.Context(), key, req.Value, rt.eventActor(r))
	if err != nil {
		rt.writeSettingError(w, "api: set runtime setting", err)
		return
	}
	rt.recordSettingChange(r, before, after)
	rt.nudgeIfRenderedSetting(key)
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
	if rt.runtimeSettings.WeakensOnReset(r.Context(), key) && r.URL.Query().Get("confirm_weakening") != "true" {
		writeJSON(w, http.StatusBadRequest, runtimeSettingError{
			Error: "resetting restores a weaker value from the environment seed; pass --confirm-weakening on the CLI or confirm_weakening=true on the API to proceed",
			Code:  errCodeConfirmRequired, Field: "confirm_weakening",
		})
		return
	}
	after, err := rt.runtimeSettings.Reset(r.Context(), key)
	if err != nil {
		rt.writeSettingError(w, "api: reset runtime setting", err)
		return
	}
	rt.recordSettingChange(r, before, after)
	rt.nudgeIfRenderedSetting(key)
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
	if item, ok := rt.cleanupPreviewReviewItem(r.Context()); ok {
		items = append(items, item)
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

// maskAuditValue keeps a sensitive value out of the audit log even if the caller passed it raw.
func maskAuditValue() json.RawMessage { return json.RawMessage(`"***"`) }

// recordSettingChange writes the settings_change audit row; best effort like recordAudit.
func (rt *Router) recordSettingChange(r *http.Request, before, after runtimesettings.Setting) {
	id, err := store.NewAuditEntryID()
	if err != nil {
		rt.logger.Warn("api: settings change audit id failed", slog.String("key", after.Key))
		return
	}
	change := runtimeSettingChange{Key: after.Key, Old: before.Value, New: after.Value}
	if after.Sensitive {
		change.Old, change.New = maskAuditValue(), maskAuditValue()
	}
	actorType, actorID, actorName, _ := rt.currentActor(r)
	if sc, ok := iac.StackScopeFrom(r.Context()); ok {
		change.Stack, change.Revision = sc.StackName, sc.Revision
		if actorType == store.PrincipalTypeToken {
			change.TokenID = actorID
		}
	}
	detail, _ := json.Marshal(change)
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

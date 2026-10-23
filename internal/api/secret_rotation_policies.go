package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/secretrotation"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	rotationMaxIntervalDays = 3650
	rotationMaxGraceHours   = 24 * 90
	rotationMaxOwnerLen     = 120
)

// SecretRotationStore is the policy persistence the rotation API needs.
// *store.DB satisfies it.
type SecretRotationStore interface {
	secretrotation.Store
	UpsertSecretRotationPolicy(ctx context.Context, p store.SecretRotationPolicy, now time.Time) error
}

type secretRotationState struct {
	store   SecretRotationStore
	engine  *secretrotation.Engine
	dueSoon time.Duration
}

// SetSecretRotation enables the rotation policy API and returns the engine
// the caller runs on a timer.
func (rt *Router) SetSecretRotation(st SecretRotationStore, maxPerPass int, dueSoon time.Duration) *secretrotation.Engine {
	eng := &secretrotation.Engine{
		Store:      st,
		Backend:    rotationBackend{rt: rt},
		Audit:      rotationAuditor{rt: rt},
		Logger:     rt.logger,
		MaxPerPass: maxPerPass,
	}
	if dueSoon <= 0 {
		dueSoon = secretrotation.DefaultDueSoon
	}
	rt.rotation.Store(&secretRotationState{store: st, engine: eng, dueSoon: dueSoon})
	return eng
}

type rotationBackend struct{ rt *Router }

func (b rotationBackend) AppState(ctx context.Context, app string) (bool, bool, error) {
	svc, err := b.rt.apps.GetDesiredService(ctx, app)
	if errors.Is(err, store.ErrServiceNotFound) {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	return true, svc.Suspended, nil
}

func (b rotationBackend) Value(ctx context.Context, app, key string) (string, error) {
	return b.rt.secrets.Resolve(ctx, app, key)
}

func (b rotationBackend) SetValue(ctx context.Context, app, key, value string) error {
	if err := b.rt.secrets.SetValueGuarded(ctx, app, key, value, false); err != nil {
		return err
	}
	_, err := b.rt.declareSecretEnv(ctx, app, key, true)
	return err
}

func (b rotationBackend) DeleteValue(ctx context.Context, app, key string) error {
	deleter, ok := b.rt.apps.(secretValueDeleter)
	if !ok {
		return errors.New("secret deletion is not supported by this store")
	}
	if _, err := b.rt.declareSecretEnv(ctx, app, key, false); err != nil {
		return err
	}
	_, err := deleter.DeleteSecretValue(ctx, app, key)
	return err
}

func (b rotationBackend) KeyState(ctx context.Context, app, key string) (bool, bool, time.Time, error) {
	keys, err := b.rt.secrets.ListKeys(ctx, app)
	if err != nil {
		return false, false, time.Time{}, err
	}
	for _, k := range keys {
		if k.Key == key {
			return true, k.Locked, k.UpdatedAt, nil
		}
	}
	return false, false, time.Time{}, nil
}

func (b rotationBackend) Restart(ctx context.Context, app string) error {
	if err := b.rt.apps.RestartService(ctx, app); err != nil {
		return err
	}
	if s := b.rt.appEvents(); s != nil {
		_ = s.AddAppEvent(ctx, store.AppEvent{AppName: app, Kind: store.AppEventRestart, Title: "Restarted after secret rotation", Actor: appEventActorSystem})
	}
	b.rt.nudgeReconciler()
	return nil
}

type rotationAuditor struct{ rt *Router }

// Record writes one audit row for a rotation step. It never receives a value.
func (a rotationAuditor) Record(ctx context.Context, action, app, key, actor string) {
	a.rt.saveRotationAudit(ctx, action, "/api/v1/apps/"+app+"/secrets/"+key, actor)
}

func (rt *Router) saveRotationAudit(ctx context.Context, action, path, actor string) {
	if rt.auditLog == nil {
		return
	}
	id, err := store.NewAuditEntryID()
	if err != nil {
		rt.logger.Warn("api: secret rotation audit id failed", slog.String("error", err.Error()))
		return
	}
	entry := store.AuditEntry{
		ID: id, ActorType: "system", ActorName: actor, Ability: AbilityWriteSensitive,
		Method: action, Action: action, Path: path,
		StatusCode: http.StatusOK, CreatedAt: store.FormatAuditTime(time.Now()), ClientKind: "system",
	}
	if actor != "system" {
		entry.ActorType = "session"
		entry.ClientKind = "api"
	}
	if err := rt.auditLog.SaveAuditEntry(ctx, entry); err != nil {
		rt.logger.Warn("api: secret rotation audit save failed", slog.String("error", err.Error()), slog.String("path", path))
	}
}

type secretRotationPolicyRequest struct {
	IntervalDays int    `json:"interval_days"`
	GraceHours   int    `json:"grace_hours"`
	Owner        string `json:"owner"`
	AutoRotate   bool   `json:"auto_rotate"`
}

type secretRotationPolicyResource struct {
	App               string `json:"app,omitempty"`
	Key               string `json:"key"`
	IntervalDays      int    `json:"interval_days"`
	GraceHours        int    `json:"grace_hours"`
	Owner             string `json:"owner"`
	AutoRotate        bool   `json:"auto_rotate"`
	LastRotatedAt     string `json:"last_rotated_at,omitempty"`
	NextDueAt         string `json:"next_due_at,omitempty"`
	Status            string `json:"status"`
	State             string `json:"state"`
	LastError         string `json:"last_error,omitempty"`
	PreviousExpiresAt string `json:"previous_expires_at,omitempty"`
}

func rfc3339OrEmpty(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func (rt *Router) rotationResources(ctx context.Context, policies []store.SecretRotationPolicy, withApp bool) []secretRotationPolicyResource {
	now := time.Now()
	updated := map[string]map[string]time.Time{}
	out := make([]secretRotationPolicyResource, 0, len(policies))
	for _, p := range policies {
		times, ok := updated[p.ServiceName]
		if !ok {
			times = map[string]time.Time{}
			if keys, err := rt.secrets.ListKeys(ctx, p.ServiceName); err == nil {
				for _, k := range keys {
					times[k.Key] = k.UpdatedAt
				}
			}
			updated[p.ServiceName] = times
		}
		due := secretrotation.NextDue(secretrotation.Basis(p.LastRotatedAt, times[p.EnvKey]), p.IntervalDays)
		res := secretRotationPolicyResource{
			Key: p.EnvKey, IntervalDays: p.IntervalDays, GraceHours: p.GraceHours, Owner: p.Owner,
			AutoRotate: p.AutoRotate, LastRotatedAt: rfc3339OrEmpty(secretrotation.Basis(p.LastRotatedAt, times[p.EnvKey])),
			NextDueAt: rfc3339OrEmpty(due), Status: secretrotation.StatusAt(now, due, rt.rotation.Load().dueSoon),
			State: p.State, LastError: p.LastError, PreviousExpiresAt: rfc3339OrEmpty(p.PreviousExpiresAt),
		}
		if withApp {
			res.App = p.ServiceName
		}
		out = append(out, res)
	}
	return out
}

func (rt *Router) rotationReady(w http.ResponseWriter) bool {
	if rt.secrets == nil {
		writeError(w, http.StatusNotImplemented, "secrets are not configured on this control plane (no master key set)")
		return false
	}
	if rt.rotation.Load() == nil {
		writeError(w, http.StatusNotImplemented, "secret rotation is not enabled on this control plane")
		return false
	}
	return true
}

// handleListSecretRotation handles GET /api/v1/apps/{name}/secret-rotation.
func (rt *Router) handleListSecretRotation(w http.ResponseWriter, r *http.Request) {
	if !rt.rotationReady(w) {
		return
	}
	name := r.PathValue("name")
	policies, err := rt.rotation.Load().store.ListSecretRotationPolicies(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: list secret rotation policies failed", err, slog.String("name", name))
		return
	}
	writeJSON(w, http.StatusOK, rt.rotationResources(r.Context(), policies, false))
}

// handleListAllSecretRotation handles GET /api/v1/secret-rotation, every
// policy across apps with its status, for the due-rotations overview.
func (rt *Router) handleListAllSecretRotation(w http.ResponseWriter, r *http.Request) {
	if !rt.rotationReady(w) {
		return
	}
	policies, err := rt.rotation.Load().store.ListSecretRotationPolicies(r.Context(), "")
	if err != nil {
		rt.internalError(w, "api: list all secret rotation policies failed", err)
		return
	}
	writeJSON(w, http.StatusOK, rt.rotationResources(r.Context(), policies, true))
}

func validateRotationRequest(req *secretRotationPolicyRequest) string {
	req.Owner = strings.TrimSpace(req.Owner)
	switch {
	case req.IntervalDays < 1 || req.IntervalDays > rotationMaxIntervalDays:
		return "interval_days must be between 1 and 3650"
	case req.GraceHours < 0 || req.GraceHours > rotationMaxGraceHours:
		return "grace_hours must be between 0 and 2160"
	case req.GraceHours > req.IntervalDays*24:
		return "grace_hours must not exceed the rotation interval"
	case len(req.Owner) > rotationMaxOwnerLen:
		return "owner is too long"
	}
	return ""
}

// handleSetSecretRotation handles PUT /api/v1/apps/{name}/secrets/{key}/rotation.
func (rt *Router) handleSetSecretRotation(w http.ResponseWriter, r *http.Request) {
	if !rt.rotationReady(w) {
		return
	}
	name, key := r.PathValue("name"), r.PathValue("key")
	if secretrotation.IsPreviousKey(key) {
		writeError(w, http.StatusBadRequest, "a grace-window key cannot have its own rotation policy")
		return
	}
	if _, err := rt.apps.GetDesiredService(r.Context(), name); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: set secret rotation: load app failed", err, slog.String("name", name))
		return
	}
	var req secretRotationPolicyRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if msg := validateRotationRequest(&req); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	exists, err := rt.secrets.Exists(r.Context(), name, key)
	if err != nil {
		rt.internalError(w, "api: set secret rotation: check secret failed", err, slog.String("name", name), slog.String("key", key))
		return
	}
	if !exists {
		writeError(w, http.StatusNotFound, "no secret with this key, set its value first")
		return
	}
	if req.Owner == "" {
		req.Owner = rt.accessActor(r).name
	}
	p := store.SecretRotationPolicy{ServiceName: name, EnvKey: key, IntervalDays: req.IntervalDays,
		GraceHours: req.GraceHours, Owner: req.Owner, AutoRotate: req.AutoRotate}
	if err := rt.rotation.Load().store.UpsertSecretRotationPolicy(r.Context(), p, time.Now()); err != nil {
		rt.internalError(w, "api: set secret rotation failed", err, slog.String("name", name), slog.String("key", key))
		return
	}
	rt.writeRotationPolicy(r.Context(), w, name, key)
}

func (rt *Router) writeRotationPolicy(ctx context.Context, w http.ResponseWriter, name, key string) {
	p, err := rt.rotation.Load().store.GetSecretRotationPolicy(ctx, name, key)
	if err != nil {
		rt.internalError(w, "api: reload secret rotation policy failed", err, slog.String("name", name), slog.String("key", key))
		return
	}
	writeJSON(w, http.StatusOK, rt.rotationResources(ctx, []store.SecretRotationPolicy{p}, false)[0])
}

// handleDeleteSecretRotation handles DELETE /api/v1/apps/{name}/secrets/{key}/rotation.
func (rt *Router) handleDeleteSecretRotation(w http.ResponseWriter, r *http.Request) {
	if !rt.rotationReady(w) {
		return
	}
	existed, err := rt.rotation.Load().store.DeleteSecretRotationPolicy(r.Context(), r.PathValue("name"), r.PathValue("key"))
	if err != nil {
		rt.internalError(w, "api: delete secret rotation failed", err)
		return
	}
	if !existed {
		writeError(w, http.StatusNotFound, "no rotation policy for this secret")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleRotateSecretNow handles POST /api/v1/apps/{name}/secrets/{key}/rotation/rotate.
// It never returns the generated value. On a half-succeeded rotation it
// answers 502 and the policy stays restart_failed until a retry succeeds.
func (rt *Router) handleRotateSecretNow(w http.ResponseWriter, r *http.Request) {
	if !rt.rotationReady(w) {
		return
	}
	name, key := r.PathValue("name"), r.PathValue("key")
	actor := rt.accessActor(r).name
	err := rt.rotation.Load().engine.Rotate(r.Context(), name, key, actor)
	switch {
	case errors.Is(err, store.ErrSecretRotationPolicyNotFound):
		writeError(w, http.StatusNotFound, "no rotation policy for this secret")
		return
	case errors.Is(err, secretrotation.ErrNotAuto):
		writeError(w, http.StatusConflict, "this secret is not marked auto-rotatable, rotate it by setting a new value")
		return
	}
	p, perr := rt.rotation.Load().store.GetSecretRotationPolicy(r.Context(), name, key)
	if perr != nil {
		rt.internalError(w, "api: rotate secret: reload policy failed", perr, slog.String("name", name), slog.String("key", key))
		return
	}
	switch {
	case p.State == store.SecretRotationFailed:
		writeError(w, http.StatusConflict, p.LastError)
	case p.State == store.SecretRotationRestartFailed:
		writeError(w, http.StatusBadGateway, p.LastError)
	case err != nil:
		rt.internalError(w, "api: rotate secret failed", err, slog.String("name", name), slog.String("key", key))
	default:
		rt.writeRotationPolicy(r.Context(), w, name, key)
	}
}

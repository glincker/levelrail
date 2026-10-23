package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploysafety"
	"github.com/GLINCKER/levelrail/internal/store"
)

// DeployGateStore is the store surface behind the readiness gate timeline,
// per-app gate settings and rollback pins. *store.DB satisfies it.
type DeployGateStore interface {
	ListGateEvents(ctx context.Context, attemptID string) ([]store.DeployGateEvent, error)
	GetDeploySafetySettings(ctx context.Context, serviceName string) (store.DeploySafetySettings, error)
	SaveDeploySafetySettings(ctx context.Context, s store.DeploySafetySettings) error
	ListRollbackPins(ctx context.Context, serviceName string) ([]store.RollbackPin, error)
	GetRollbackPin(ctx context.Context, serviceName, attemptID string) (*store.RollbackPin, error)
	RecordGateEvent(ctx context.Context, e store.DeployGateEvent) error
}

// WithDeployGate enables the gate timeline, deploy safety settings and
// rollback target endpoints, resolving settings over defaults.
func WithDeployGate(s DeployGateStore, defaults deploysafety.Effective) Option {
	return func(rt *Router) {
		rt.deployGate = s
		rt.deployGateDefaults = defaults
	}
}

type gateEventResource struct {
	Phase     string    `json:"phase"`
	Status    string    `json:"status"`
	Reason    string    `json:"reason,omitempty"`
	Message   string    `json:"message"`
	Container string    `json:"container,omitempty"`
	At        time.Time `json:"at"`
}

type deploySafetySettingsResource struct {
	SuccessThreshold int     `json:"success_threshold"`
	StableWindow     string  `json:"stable_window"`
	DrainTimeout     string  `json:"drain_timeout"`
	WatchWindow      string  `json:"watch_window"`
	WatchMaxRestarts int     `json:"watch_max_restarts"`
	WatchErrorRate   float64 `json:"watch_error_rate"`
	WatchMinRequests int     `json:"watch_min_requests"`
	AutoRollback     bool    `json:"auto_rollback"`
	RollbackPins     int     `json:"rollback_pins"`

	WatchMode            string  `json:"watch_mode"`
	WatchLatencyRatio    float64 `json:"watch_latency_ratio"`
	WatchLatencyMinDelta string  `json:"watch_latency_min_delta"`
	WatchSoftTicks       int     `json:"watch_soft_ticks"`
	WatchSoftRollback    bool    `json:"watch_soft_rollback"`
}

func toSafetySettingsResource(e deploysafety.Effective) deploySafetySettingsResource {
	return deploySafetySettingsResource{
		SuccessThreshold: e.SuccessThreshold, StableWindow: e.StableWindow.String(), DrainTimeout: e.DrainTimeout.String(),
		WatchWindow: e.WatchWindow.String(), WatchMaxRestarts: e.WatchMaxRestarts, WatchErrorRate: e.WatchErrorRate,
		WatchMinRequests: e.WatchMinRequests, AutoRollback: e.AutoRollback, RollbackPins: e.RollbackPins,
		WatchMode: e.WatchMode, WatchLatencyRatio: e.WatchLatencyRatio, WatchLatencyMinDelta: e.WatchLatencyMinDelta.String(),
		WatchSoftTicks: e.WatchSoftTicks, WatchSoftRollback: e.WatchSoftRollback,
	}
}

// deploySafetyOverrides is an app's overrides; null inherits the default.
type deploySafetyOverrides struct {
	SuccessThreshold *int     `json:"success_threshold"`
	StableWindow     *string  `json:"stable_window"`
	DrainTimeout     *string  `json:"drain_timeout"`
	WatchWindow      *string  `json:"watch_window"`
	WatchMaxRestarts *int     `json:"watch_max_restarts"`
	WatchErrorRate   *float64 `json:"watch_error_rate"`
	WatchMinRequests *int     `json:"watch_min_requests"`
	AutoRollback     *bool    `json:"auto_rollback"`

	WatchMode         *string  `json:"watch_mode"`
	WatchLatencyRatio *float64 `json:"watch_latency_ratio"`
	WatchSoftRollback *bool    `json:"watch_soft_rollback"`
}

func durationString(d *time.Duration) *string {
	if d == nil {
		return nil
	}
	s := d.String()
	return &s
}

func toOverrides(s store.DeploySafetySettings) deploySafetyOverrides {
	return deploySafetyOverrides{
		SuccessThreshold: s.SuccessThreshold, StableWindow: durationString(s.StableWindow), DrainTimeout: durationString(s.DrainTimeout),
		WatchWindow: durationString(s.WatchWindow), WatchMaxRestarts: s.WatchMaxRestarts, WatchErrorRate: s.WatchErrorRate,
		WatchMinRequests: s.WatchMinRequests, AutoRollback: s.AutoRollback,
		WatchMode: s.WatchMode, WatchLatencyRatio: s.WatchLatencyRatio, WatchSoftRollback: s.WatchSoftRollback,
	}
}

func parseGateDuration(field string, raw *string) (*time.Duration, error) {
	if raw == nil {
		return nil, nil
	}
	d, err := time.ParseDuration(*raw)
	if err != nil {
		return nil, errors.New(field + " must be a duration such as 10s or 5m")
	}
	return &d, nil
}

func (o deploySafetyOverrides) toStore(name string) (store.DeploySafetySettings, error) {
	s := store.DeploySafetySettings{
		ServiceName: name, SuccessThreshold: o.SuccessThreshold, WatchMaxRestarts: o.WatchMaxRestarts,
		WatchErrorRate: o.WatchErrorRate, WatchMinRequests: o.WatchMinRequests, AutoRollback: o.AutoRollback,
		WatchMode: o.WatchMode, WatchLatencyRatio: o.WatchLatencyRatio, WatchSoftRollback: o.WatchSoftRollback,
	}
	var err error
	if s.StableWindow, err = parseGateDuration("stable_window", o.StableWindow); err != nil {
		return s, err
	}
	if s.DrainTimeout, err = parseGateDuration("drain_timeout", o.DrainTimeout); err != nil {
		return s, err
	}
	if s.WatchWindow, err = parseGateDuration("watch_window", o.WatchWindow); err != nil {
		return s, err
	}
	return s, deploysafety.Validate(s)
}

type deploySafetyResource struct {
	Overrides deploySafetyOverrides        `json:"overrides"`
	Defaults  deploySafetySettingsResource `json:"defaults"`
	Effective deploySafetySettingsResource `json:"effective"`
}

type rollbackTargetResource struct {
	DeployID string    `json:"deploy_id"`
	Image    string    `json:"image"`
	ImageID  string    `json:"image_id,omitempty"`
	PinRef   string    `json:"pin_ref"`
	PinnedAt time.Time `json:"pinned_at"`
	Live     bool      `json:"live"`
}

type deployGateResource struct {
	DeployID    string                       `json:"deploy_id"`
	Image       string                       `json:"image"`
	Status      string                       `json:"status"`
	Reason      string                       `json:"reason,omitempty"`
	Error       string                       `json:"error,omitempty"`
	Stage       string                       `json:"stage"`
	Terminal    bool                         `json:"terminal"`
	Failed      bool                         `json:"failed"`
	Superseded  bool                         `json:"superseded"`
	Events      []gateEventResource          `json:"events"`
	Settings    deploySafetySettingsResource `json:"settings"`
	RollbackPin *rollbackTargetResource      `json:"rollback_pin"`
	Watch       watchStatusResource          `json:"watch"`
}

// gateReady writes 501 when the gate store is not wired; ok false means a
// response was written.
func (rt *Router) gateReady(w http.ResponseWriter) bool {
	if rt.deployGate == nil {
		writeError(w, http.StatusNotImplemented, "deploy safety is not enabled on this control plane")
		return false
	}
	return true
}

func (rt *Router) effectiveSafety(ctx context.Context, name string) (store.DeploySafetySettings, deploysafety.Effective, error) {
	overrides, err := rt.deployGate.GetDeploySafetySettings(ctx, name)
	if err != nil {
		return overrides, rt.deployGateDefaults, err
	}
	return overrides, deploysafety.Resolve(rt.deployGateDefaults, overrides), nil
}

// handleGetDeployGate handles GET /api/v1/apps/{name}/deploys/{deployId}/gate:
// the release's readiness gate, cutover, drain and post-deploy watch.
func (rt *Router) handleGetDeployGate(w http.ResponseWriter, r *http.Request) {
	if !rt.gateReady(w) {
		return
	}
	if r.PathValue("deployId") == latestDeployID {
		if !rt.resolveLatestDeployID(w, r) {
			return
		}
	}
	attempt, ok := rt.lookupAppAttempt(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	events, err := rt.deployGate.ListGateEvents(ctx, attempt.ID)
	if err != nil {
		rt.internalError(w, "api: deploy gate: list events failed", err, slog.String("deploy_id", attempt.ID))
		return
	}
	_, effective, err := rt.effectiveSafety(ctx, attempt.ServiceName)
	if err != nil {
		rt.internalError(w, "api: deploy gate: load settings failed", err, slog.String("name", attempt.ServiceName))
		return
	}
	pin, err := rt.deployGate.GetRollbackPin(ctx, attempt.ServiceName, attempt.ID)
	if err != nil {
		rt.internalError(w, "api: deploy gate: load pin failed", err, slog.String("deploy_id", attempt.ID))
		return
	}
	stage := deploysafety.Stage(events)
	failed := attempt.Status == store.DeployAttemptStatusFailed || deploysafety.StageFailed(stage)
	out := deployGateResource{
		DeployID: attempt.ID, Image: attempt.Image, Status: attempt.Status, Reason: attempt.Reason, Error: attempt.Error,
		Stage: stage, Failed: failed, Superseded: rt.attemptSuperseded(ctx, *attempt),
		Events: make([]gateEventResource, 0, len(events)), Settings: toSafetySettingsResource(effective),
	}
	out.Terminal = deploysafety.StageTerminal(stage) || failed || out.Superseded
	for _, e := range events {
		out.Events = append(out.Events, gateEventResource{Phase: e.Phase, Status: e.Status, Reason: e.Reason, Message: e.Message, Container: e.Container, At: e.At})
	}
	out.Watch = toWatchStatusResource(deploysafety.WatchStatusOf(events, effective, time.Now()))
	if pin != nil {
		out.RollbackPin = &rollbackTargetResource{DeployID: pin.DeployAttemptID, Image: pin.Image, ImageID: pin.ImageID, PinRef: pin.PinRef, PinnedAt: pin.CreatedAt}
	}
	writeJSON(w, http.StatusOK, out)
}

// attemptSuperseded reports whether a newer deploy of the same app exists,
// after which nothing more happens to a's timeline.
func (rt *Router) attemptSuperseded(ctx context.Context, a store.DeployAttempt) bool {
	attempts, err := rt.deployAttempts.ListDeployAttempts(ctx, a.ServiceName)
	return err == nil && len(attempts) > 0 && attempts[0].ID != a.ID
}

// resolveLatestDeployID rewrites the deployId path value to the app's newest
// attempt; ok false means a response was written.
func (rt *Router) resolveLatestDeployID(w http.ResponseWriter, r *http.Request) bool {
	attempts, err := rt.deployAttempts.ListDeployAttempts(r.Context(), r.PathValue("name"))
	if err != nil {
		rt.internalError(w, "api: deploy gate: list attempts", err)
		return false
	}
	if len(attempts) == 0 {
		writeError(w, http.StatusNotFound, "deploy attempt not found")
		return false
	}
	r.SetPathValue("deployId", attempts[0].ID)
	return true
}

func (rt *Router) loadAppForGate(w http.ResponseWriter, r *http.Request) (*store.DesiredService, bool) {
	name := r.PathValue("name")
	svc, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return nil, false
	}
	if err != nil {
		rt.internalError(w, "api: deploy safety: load app failed", err, slog.String("name", name))
		return nil, false
	}
	return svc, true
}

func (rt *Router) writeDeploySafety(w http.ResponseWriter, r *http.Request, name string) {
	overrides, effective, err := rt.effectiveSafety(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: deploy safety: load settings failed", err, slog.String("name", name))
		return
	}
	writeJSON(w, http.StatusOK, deploySafetyResource{
		Overrides: toOverrides(overrides), Defaults: toSafetySettingsResource(rt.deployGateDefaults), Effective: toSafetySettingsResource(effective),
	})
}

// handleGetDeploySafety handles GET /api/v1/apps/{name}/deploy-safety.
func (rt *Router) handleGetDeploySafety(w http.ResponseWriter, r *http.Request) {
	if !rt.gateReady(w) {
		return
	}
	svc, ok := rt.loadAppForGate(w, r)
	if !ok {
		return
	}
	rt.writeDeploySafety(w, r, svc.Name)
}

// handlePutDeploySafety handles PUT /api/v1/apps/{name}/deploy-safety:
// replaces the app's overrides; a null or missing field inherits the default.
func (rt *Router) handlePutDeploySafety(w http.ResponseWriter, r *http.Request) {
	if !rt.gateReady(w) {
		return
	}
	svc, ok := rt.loadAppForGate(w, r)
	if !ok {
		return
	}
	var req deploySafetyOverrides
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<14)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	settings, err := req.toStore(svc.Name)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := rt.deployGate.SaveDeploySafetySettings(r.Context(), settings); err != nil {
		rt.internalError(w, "api: deploy safety: save failed", err, slog.String("name", svc.Name))
		return
	}
	rt.writeDeploySafety(w, r, svc.Name)
}

// handleListRollbackTargets handles GET /api/v1/apps/{name}/rollback-targets:
// the pinned releases image cleanup will never remove, newest first.
func (rt *Router) handleListRollbackTargets(w http.ResponseWriter, r *http.Request) {
	if !rt.gateReady(w) {
		return
	}
	svc, ok := rt.loadAppForGate(w, r)
	if !ok {
		return
	}
	pins, err := rt.deployGate.ListRollbackPins(r.Context(), svc.Name)
	if err != nil {
		rt.internalError(w, "api: rollback targets: list failed", err, slog.String("name", svc.Name))
		return
	}
	out := make([]rollbackTargetResource, 0, len(pins))
	for _, p := range pins {
		out = append(out, rollbackTargetResource{
			DeployID: p.DeployAttemptID, Image: p.Image, ImageID: p.ImageID, PinRef: p.PinRef, PinnedAt: p.CreatedAt,
			Live: p.Image == svc.Image || p.PinRef == svc.Image,
		})
	}
	writeJSON(w, http.StatusOK, map[string][]rollbackTargetResource{"targets": out})
}

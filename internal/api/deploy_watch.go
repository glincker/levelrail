package api

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploysafety"
	"github.com/GLINCKER/levelrail/internal/store"
)

type watchStatusResource struct {
	State            string     `json:"state"`
	Mode             string     `json:"mode"`
	StartedAt        *time.Time `json:"started_at,omitempty"`
	EndsAt           *time.Time `json:"ends_at,omitempty"`
	RemainingSeconds int        `json:"remaining_seconds"`
	WindowSeconds    int        `json:"window_seconds"`
	Reason           string     `json:"reason,omitempty"`
	Message          string     `json:"message,omitempty"`
	Notice           string     `json:"notice,omitempty"`
}

func toWatchStatusResource(s deploysafety.WatchStatus) watchStatusResource {
	out := watchStatusResource{
		State: s.State, Mode: s.Mode, RemainingSeconds: s.RemainingSecs, WindowSeconds: s.WindowSecs,
		Reason: s.Reason, Message: s.Message, Notice: s.Notice,
	}
	if !s.StartedAt.IsZero() {
		started, ends := s.StartedAt, s.EndsAt
		out.StartedAt, out.EndsAt = &started, &ends
	}
	return out
}

type appWatchResource struct {
	DeployID string                       `json:"deploy_id,omitempty"`
	Image    string                       `json:"image,omitempty"`
	Watch    watchStatusResource          `json:"watch"`
	Settings deploySafetySettingsResource `json:"settings"`
}

// appWatch reads the newest release's watch; the attempt is nil when the app
// has never deployed. ok false means a response was written.
func (rt *Router) appWatch(w http.ResponseWriter, r *http.Request, name string) (appWatchResource, *store.DeployAttempt, bool) {
	ctx := r.Context()
	_, effective, err := rt.effectiveSafety(ctx, name)
	if err != nil {
		rt.internalError(w, "api: deploy watch: load settings", err)
		return appWatchResource{}, nil, false
	}
	out := appWatchResource{Settings: toSafetySettingsResource(effective)}
	attempts, err := rt.deployAttempts.ListDeployAttempts(ctx, name)
	if err != nil {
		rt.internalError(w, "api: deploy watch: list attempts", err)
		return out, nil, false
	}
	if len(attempts) == 0 {
		out.Watch = toWatchStatusResource(deploysafety.WatchStatusOf(nil, effective, time.Now()))
		return out, nil, true
	}
	attempt := attempts[0]
	events, err := rt.deployGate.ListGateEvents(ctx, attempt.ID)
	if err != nil {
		rt.internalError(w, "api: deploy watch: list events", err)
		return out, nil, false
	}
	out.DeployID, out.Image = attempt.ID, attempt.Image
	out.Watch = toWatchStatusResource(deploysafety.WatchStatusOf(events, effective, time.Now()))
	return out, &attempt, true
}

// handleGetAppWatch handles GET /api/v1/apps/{name}/watch: the newest
// release's post-deploy watch state next to the effective settings.
func (rt *Router) handleGetAppWatch(w http.ResponseWriter, r *http.Request) {
	if !rt.gateReady(w) {
		return
	}
	svc, ok := rt.loadAppForGate(w, r)
	if !ok {
		return
	}
	if out, _, ok := rt.appWatch(w, r, svc.Name); ok {
		writeJSON(w, http.StatusOK, out)
	}
}

// handleStopAppWatch handles POST /api/v1/apps/{name}/watch/stop: ends the
// open watch of the newest release without judging it.
func (rt *Router) handleStopAppWatch(w http.ResponseWriter, r *http.Request) {
	if !rt.gateReady(w) {
		return
	}
	svc, ok := rt.loadAppForGate(w, r)
	if !ok {
		return
	}
	out, attempt, ok := rt.appWatch(w, r, svc.Name)
	if !ok {
		return
	}
	if attempt == nil || out.Watch.State != deploysafety.WatchStateWatching {
		writeError(w, http.StatusConflict, "no post-deploy watch is open for this app")
		return
	}
	err := rt.deployGate.RecordGateEvent(r.Context(), store.DeployGateEvent{
		DeployAttemptID: attempt.ID, ServiceName: svc.Name, Phase: deploysafety.PhaseWatch, Status: deploysafety.StatusStopped,
		Reason: deploysafety.ReasonWatchStopped, Message: "watch stopped by an operator before the window ended", At: time.Now(),
	})
	if err != nil {
		rt.logger.Error("api: deploy watch: stop failed", slog.String("error", err.Error()), slog.String("name", svc.Name), slog.String("deploy_id", attempt.ID))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if out, _, ok := rt.appWatch(w, r, svc.Name); ok {
		writeJSON(w, http.StatusOK, out)
	}
}

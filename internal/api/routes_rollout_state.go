package api

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploysafety"
	"github.com/GLINCKER/levelrail/internal/store"
)

// rolloutStateResource is GET /api/v1/apps/{name}/rollout/state: the newest
// release's gate, traffic flip, hold countdown and any canary in one view.
type rolloutStateResource struct {
	App           string                       `json:"app"`
	Strategy      string                       `json:"strategy"`
	DeployID      string                       `json:"deploy_id,omitempty"`
	Image         string                       `json:"image,omitempty"`
	Stage         string                       `json:"stage"`
	Traffic       string                       `json:"traffic"`
	GateFailures  int                          `json:"gate_failures"`
	AutoRollback  bool                         `json:"auto_rollback"`
	FlippedAt     *time.Time                   `json:"flipped_at,omitempty"`
	RolledBackAt  *time.Time                   `json:"rolled_back_at,omitempty"`
	FailureReason string                       `json:"failure_reason,omitempty"`
	Hold          watchStatusResource          `json:"hold"`
	Settings      deploySafetySettingsResource `json:"settings"`
	Canary        *canaryResource              `json:"canary,omitempty"`
}

// registerRolloutStateRoutes wires the combined rollout state view.
func (rt *Router) registerRolloutStateRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/apps/{name}/rollout/state", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetRolloutState))
}

func (rt *Router) handleGetRolloutState(w http.ResponseWriter, r *http.Request) {
	if !rt.gateReady(w) {
		return
	}
	svc, ok := rt.loadAppForGate(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	_, effective, err := rt.effectiveSafety(ctx, svc.Name)
	if err != nil {
		rt.internalError(w, "api: rollout state: load settings", err, slog.String("name", svc.Name))
		return
	}
	out := rolloutStateResource{App: svc.Name, Strategy: svc.Strategy, Settings: toSafetySettingsResource(effective)}
	attempts, err := rt.deployAttempts.ListDeployAttempts(ctx, svc.Name)
	if err != nil {
		rt.internalError(w, "api: rollout state: list attempts", err, slog.String("name", svc.Name))
		return
	}
	var gate []store.DeployGateEvent
	if len(attempts) > 0 {
		gate, err = rt.deployGate.ListGateEvents(ctx, attempts[0].ID)
		if err != nil {
			rt.internalError(w, "api: rollout state: list events", err, slog.String("deploy_id", attempts[0].ID))
			return
		}
		out.DeployID, out.Image = attempts[0].ID, attempts[0].Image
	}
	out.fill(deploysafety.RolloutStateOf(gate, effective, time.Now()))
	if c, found, err := rt.canaries.GetCanaryRelease(ctx, svc.Name); err == nil && found {
		cr := rt.toCanaryResource(ctx, svc.Name, c, true)
		out.Canary = &cr
	}
	writeJSON(w, http.StatusOK, out)
}

func (o *rolloutStateResource) fill(st deploysafety.RolloutState) {
	o.Stage, o.Traffic, o.GateFailures, o.AutoRollback = st.Stage, st.Traffic, st.GateFailures, st.AutoRollback
	o.FailureReason, o.Hold = st.FailureReason, toWatchStatusResource(st.Hold)
	if !st.FlippedAt.IsZero() {
		t := st.FlippedAt
		o.FlippedAt = &t
	}
	if !st.RolledBackAt.IsZero() {
		t := st.RolledBackAt
		o.RolledBackAt = &t
	}
}

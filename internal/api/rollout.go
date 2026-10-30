package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/rolloutpolicy"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

// rolloutResource is GET /api/v1/apps/{name}/rollout: the saved policy,
// what it resolves to for the current replica count, and live progress.
type rolloutResource struct {
	App      string                  `json:"app"`
	Strategy string                  `json:"strategy"`
	Replicas int                     `json:"replicas"`
	Policy   rolloutpolicy.Policy    `json:"policy"`
	Resolved rolloutpolicy.Resolved  `json:"resolved"`
	Progress *rolloutpolicy.Progress `json:"progress"`
}

// handleGetRollout handles GET /api/v1/apps/{name}/rollout.
func (rt *Router) handleGetRollout(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	svc, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: rollout: load app failed", err, slog.String("name", name))
		return
	}
	pol, err := rt.rolloutPolicy(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: rollout: load policy failed", err, slog.String("name", name))
		return
	}
	replicas := max(svc.Replicas, store.DefaultReplicas)
	res := rolloutResource{
		App: name, Strategy: svc.Strategy, Replicas: replicas, Policy: pol,
		Resolved: rolloutpolicy.Resolve(pol, rolloutpolicy.OSEnv, replicas),
	}
	if p, ok := rolloutpolicy.Lookup(name); ok {
		res.Progress = &p
	}
	writeJSON(w, http.StatusOK, res)
}

func (rt *Router) rolloutPolicy(ctx context.Context, name string) (rolloutpolicy.Policy, error) {
	src, ok := rt.apps.(application.RolloutPolicySource)
	if !ok {
		return rolloutpolicy.Policy{}, nil
	}
	p, _, err := src.GetRolloutPolicy(ctx, name)
	if err != nil {
		return rolloutpolicy.Policy{}, err
	}
	return rolloutpolicy.Policy{MaxSurge: p.MaxSurge, MaxUnavailable: p.MaxUnavailable, MinAvailable: p.MinAvailable}, nil
}

// Voluntary disruptions the min_available budget guards.
const (
	disruptionRestart = "restart"
	disruptionStop    = "stop"
	disruptionDrain   = "node drain"
)

// checkDisruption refuses action on svc when it would take more replicas
// down at once than rollout.min_available allows; ?force=true overrides.
func (rt *Router) checkDisruption(r *http.Request, svc store.DesiredService, action string) error {
	if r.URL.Query().Get("force") == "true" {
		return nil
	}
	pol, err := rt.rolloutPolicy(r.Context(), svc.Name)
	if err != nil {
		return nil //nolint:nilerr // an unreadable budget must not block an operator's own action
	}
	replicas := max(svc.Replicas, store.DefaultReplicas)
	res := rolloutpolicy.Resolve(pol, rolloutpolicy.OSEnv, replicas)
	removing := replicas
	if action == disruptionRestart {
		switch svc.Strategy {
		case spec.StrategyBlueGreen, spec.StrategyCanary, "":
			removing = 0 // the new set is proven ready before the old one retires
		case spec.StrategyRolling:
			removing = res.MaxUnavailable
		}
	}
	return rolloutpolicy.CheckDisruption(svc.Name, action, res.MinAvailable, replicas, removing)
}

// writeDisruption writes 409 for a budget breach and reports whether it did.
func writeDisruption(w http.ResponseWriter, err error) bool {
	var de *rolloutpolicy.DisruptionError
	if !errors.As(err, &de) {
		return false
	}
	writeError(w, http.StatusConflict, de.Error()+" (or retry with force=true to override)")
	return true
}

// refuseDrainBudget writes 409 when draining the node would stop an app
// below its min_available, and reports whether it did.
func (rt *Router) refuseDrainBudget(w http.ResponseWriter, r *http.Request) bool {
	services, err := rt.apps.ListDesiredServicesByNode(r.Context(), r.PathValue("id"))
	if err != nil {
		return false
	}
	for _, svc := range services {
		if svc.Suspended {
			continue
		}
		if writeDisruption(w, rt.checkDisruption(r, svc, disruptionDrain)) {
			return true
		}
	}
	return false
}

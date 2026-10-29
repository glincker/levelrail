package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/rollbackref"
	"github.com/GLINCKER/levelrail/internal/store"
)

// rollbackPin returns target's rollback pin, or nil when none is recorded.
func (rt *Router) rollbackPin(ctx context.Context, target store.DeployAttempt) *store.RollbackPin {
	if rt.deployGate == nil {
		return nil
	}
	pin, err := rt.deployGate.GetRollbackPin(ctx, target.ServiceName, target.ID)
	if err != nil {
		rt.logger.Warn("api: rollback: load pin failed", slog.String("error", err.Error()), slog.String("deploy_id", target.ID))
		return nil
	}
	return pin
}

// rollbackImageRef returns the image reference that redeploys exactly the
// content target recorded, or an HTTP status and message when it cannot.
func (rt *Router) rollbackImageRef(ctx context.Context, target store.DeployAttempt, pin *store.RollbackPin) (ref string, status int, msg string) {
	inspector, _ := rt.imageResolver.(docker.ImageInspector)
	return rollbackref.Resolve(ctx, inspector, rt.logger, target, pin)
}

// handleRollbackToDeploy handles POST /api/v1/apps/{name}/deploys/{deployId}/rollback:
// redeploys the content a past succeeded deploy recorded, pinned by digest
// or rollback pin, with its config restored (?config=false keeps the
// current config), through the same freeze, approval and readiness gates.
func (rt *Router) handleRollbackToDeploy(w http.ResponseWriter, r *http.Request) {
	target, ok := rt.lookupAppAttempt(w, r)
	if !ok {
		return
	}
	var req deployTriggerRequest
	if r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}
	existing, err := rt.apps.GetDesiredService(r.Context(), target.ServiceName)
	if err != nil {
		rt.internalError(w, "api: rollback: load app failed", err, slog.String("name", target.ServiceName))
		return
	}
	if isStaticAttempt(*target) {
		if _, ok := rt.freezeGate(w, r, target.ServiceName, req.freezeOverride); !ok {
			return
		}
		if status, msg := rt.rollbackStatic(r.Context(), *target); status != 0 {
			writeError(w, status, msg)
			return
		}
		res := toAppResource(*existing)
		writeJSON(w, http.StatusAccepted, deployTriggerResult{appResource: &res})
		return
	}
	pin := rt.rollbackPin(r.Context(), *target)
	ref, status, msg := rt.rollbackImageRef(r.Context(), *target, pin)
	if status != 0 {
		writeError(w, status, msg)
		return
	}
	req.Image = ref
	req.releaseID = rt.rollbackReleaseID(r.Context(), *target)
	restored := *existing
	if r.URL.Query().Get("config") != "false" {
		restored = deploy.RestoreSnapshot(*existing, target.Snapshot)
	}
	if deploy.LocalOnlyRef(pin, ref) {
		restored.PullPolicy = ""
		req.Pull = false
	}
	rt.applyDeploy(w, r, restored, req, target.ID)
}

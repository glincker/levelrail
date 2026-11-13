package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/store"
)

// handleRestartApp handles POST /api/v1/apps/{name}/restart: force a
// running container to be recreated with no image change. No request
// body. See store.DB.RestartService's own doc comment for the mechanism
// (a fresh restart_nonce, folded into the reconciler's container-name
// hash, makes this indistinguishable from an image change to the
// existing, already-tested blue-green/recreate cutover logic).
func (rt *Router) handleRestartApp(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	// A stopped app has no container to recreate, so the nonce bump would
	// be accepted and then do nothing.
	if svc, err := rt.apps.GetDesiredService(r.Context(), name); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: restart app failed", err, slog.String("name", name))
		return
	} else if svc.Suspended {
		writeError(w, http.StatusConflict, "app is stopped, so there is nothing to restart; start it first (apps start, or Start in the dashboard)")
		return
	} else if rt.refuseOfflineNode(r.Context(), w, *svc, queueWhileOffline(r)) {
		return
	} else if writeDisruption(w, rt.checkDisruption(r, *svc, disruptionRestart)) {
		return
	}

	rt.resumeCrashBreaker(name)
	if err := rt.restartService(r.Context(), name); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: restart app failed", err, slog.String("name", name))
		return
	}

	rt.recordAppEvent(r, store.AppEvent{AppName: name, Kind: store.AppEventRestart, Title: "Restarted"})
	rt.reloadAndWriteApp(w, r, name, "restart app")
}

// restartService is the one restart path, shared by the manual and the
// scheduled restart, so both roll out through the same cutover.
func (rt *Router) restartService(ctx context.Context, name string) error {
	if err := rt.apps.RestartService(ctx, name); err != nil {
		return err
	}
	// Own attempt row, like a rollback: the rollout outcome lands on it
	// instead of the deploy being restarted.
	if svc, err := rt.apps.GetDesiredService(ctx, name); err == nil {
		if rt.offlineNodeFor(ctx, *svc) != nil {
			rt.recordPendingAttempt(ctx, *svc, svc.Image, store.DeployAttemptSourceRestart)
		} else {
			rt.recordInstantDeployAttempt(ctx, *svc, svc.Image, store.DeployAttemptSourceRestart)
		}
	}
	return nil
}

// handleStopApp handles POST /api/v1/apps/{name}/stop: sets Suspended,
// distinct from handleDeleteApp, which removes desired state entirely.
// No request body. The reconciler (not this handler) is what actually
// stops containers, on its next pass; see
// store.DesiredService.Suspended's own doc comment.
func (rt *Router) handleStopApp(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	if svc, err := rt.apps.GetDesiredService(r.Context(), name); err == nil {
		if rt.refuseOfflineNode(r.Context(), w, *svc, queueWhileOffline(r)) || writeDisruption(w, rt.checkDisruption(r, *svc, disruptionStop)) {
			return
		}
	}
	if err := rt.apps.UpdateServiceSuspended(r.Context(), name, true); errors.Is(err, store.ErrServiceNotFound) {
		rt.writeAppNotFound(r.Context(), w, name)
		return
	} else if err != nil {
		rt.internalError(w, "api: stop app failed", err, slog.String("name", name))
		return
	}
	// A stop on a reachable node needs no row; a queued one does, so history matches intent.
	if svc, err := rt.apps.GetDesiredService(r.Context(), name); err == nil && rt.offlineNodeFor(r.Context(), *svc) != nil {
		rt.recordPendingAttempt(r.Context(), *svc, svc.Image, store.DeployAttemptSourceStop)
	}

	rt.recordAppEvent(r, store.AppEvent{AppName: name, Kind: store.AppEventSuspend, Title: "Stopped"})
	rt.reloadAndWriteApp(w, r, name, "stop app")
}

// handleStartApp handles POST /api/v1/apps/{name}/start: clears
// Suspended, letting the reconciler bring containers back on its next
// pass. No request body.
func (rt *Router) handleStartApp(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	if rt.volumeTransfers.importing(name) {
		writeError(w, http.StatusConflict, "a volume import is running for this app: start it when the import finishes")
		return
	}
	wasStopped := false
	if svc, err := rt.apps.GetDesiredService(r.Context(), name); err == nil {
		if rt.refuseOfflineNode(r.Context(), w, *svc, queueWhileOffline(r)) {
			return
		}
		wasStopped = svc.Suspended
	}
	if err := rt.apps.UpdateServiceSuspended(r.Context(), name, false); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: start app failed", err, slog.String("name", name))
		return
	}

	// Own attempt, so a waiter follows this start and not the last serving deploy.
	if svc, err := rt.apps.GetDesiredService(r.Context(), name); wasStopped && err == nil {
		if rt.offlineNodeFor(r.Context(), *svc) != nil {
			rt.recordPendingAttempt(r.Context(), *svc, svc.Image, store.DeployAttemptSourceStart)
		} else {
			rt.recordInstantDeployAttempt(r.Context(), *svc, svc.Image, store.DeployAttemptSourceStart)
		}
	}

	rt.recordAppEvent(r, store.AppEvent{AppName: name, Kind: store.AppEventResume, Title: "Started"})
	rt.reloadAndWriteApp(w, r, name, "start app")
}

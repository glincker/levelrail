package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/store"
)

// setAppNodeRequest is PUT /api/v1/apps/{name}/node's body.
type setAppNodeRequest struct {
	// NodeID is which node to place the app on; empty string moves it
	// back to this control plane's own local node, the same convention
	// store.DesiredService.NodeID's own doc comment establishes.
	NodeID string `json:"node_id"`
}

// handleSetAppNode handles PUT /api/v1/apps/{name}/node. A non-empty node_id
// is checked against the node registry so a typo fails with 400 instead of
// parking the service on a node that never reconciles it.
func (rt *Router) reloadAndWriteApp(w http.ResponseWriter, r *http.Request, name, logContext string) {
	svc, err := rt.apps.GetDesiredService(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: "+logContext+": reload after update failed", err, slog.String("name", name))
		return
	}
	rt.nudgeReconciler()
	writeJSON(w, http.StatusOK, rt.appOut(r.Context(), *svc))
}

func (rt *Router) handleSetAppNode(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	var req setAppNodeRequest
	if !decodeReq(w, r, &req) {
		return
	}

	// Cordon means "unschedulable for new placements," and this is a new
	// placement even when the service already exists, since it's
	// actively choosing to move it here.
	if err := rt.authorizePlacementTarget(r, req.NodeID); err != nil {
		rt.respondPlacementValidationError(w, err, req.NodeID, "api: set app node: look up node failed")
		return
	}

	existing, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: set app node: load existing failed", err, slog.String("name", name))
		return
	}
	if msg, err := rt.gpuPlacementError(r.Context(), existing, req.NodeID); err != nil {
		rt.internalError(w, "api: set app node: gpu check failed", err, slog.String("name", name))
		return
	} else if msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	if err := rt.applyPlainNodeMove(r.Context(), name, existing.NodeID, req.NodeID); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: set app node failed", err, slog.String("name", name))
		return
	}

	rt.reloadAndWriteApp(w, r, name, "set app node")
}

// applyPlainNodeMove updates name's node_id and tears down containers left
// on oldNodeID if it actually changed: the mechanics behind both
// handleSetAppNode and handleMoveAppWithVolumes' own no-volumes/same-node
// shortcut (apps_move_with_volumes.go), so there is exactly one place that
// performs an ordinary, no-volumes-involved placement change.
func (rt *Router) applyPlainNodeMove(ctx context.Context, name, oldNodeID, newNodeID string) error {
	if err := rt.apps.UpdateServiceNode(ctx, name, newNodeID); err != nil {
		return err
	}
	if oldNodeID != newNodeID {
		rt.teardownServiceContainers(name, oldNodeID)
	}
	return nil
}

// setAppProjectRequest is PUT /api/v1/apps/{name}/project's body, the
// project-kind counterpart to setAppNodeRequest.
type setAppProjectRequest struct {
	// ProjectID is which project to move this app into; empty string
	// moves it back to "no project", the same convention
	// DesiredService.ProjectID's own doc comment establishes.
	ProjectID string `json:"project_id"`
}

// handleSetAppProject handles PUT /api/v1/apps/{name}/project
// (projects.go): the only way an existing app's project assignment
// actually changes, see appResource's own ProjectID field doc comment.
// A non-empty project_id is checked against the real project registry
// first, the same shape handleSetAppNode already establishes for
// node_id, so a typo'd or already-deleted project id fails loudly here
// with a 400 rather than silently being written.
func (rt *Router) handleSetAppProject(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	var req setAppProjectRequest
	if !decodeReq(w, r, &req) {
		return
	}

	if err := rt.validateProjectID(r.Context(), req.ProjectID); err != nil {
		if errors.Is(err, store.ErrProjectNotFound) {
			writeError(w, http.StatusBadRequest, "unknown project_id")
			return
		}
		rt.internalError(w, "api: set app project: look up project failed", err, slog.String("project_id", req.ProjectID))
		return
	}

	quotaRelease, quotaOK := rt.admitAppMoveQuota(w, r, name, req.ProjectID)
	if !quotaOK {
		return
	}
	defer quotaRelease()

	if err := rt.apps.UpdateServiceProject(r.Context(), name, req.ProjectID); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: set app project failed", err, slog.String("name", name))
		return
	}

	rt.reloadAndWriteApp(w, r, name, "set app project")
}

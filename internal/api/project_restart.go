package api

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/store"
)

// projectRestartResponse is POST /api/v1/projects/{id}/restart's response
// body: which apps were restarted and which failed, so a caller can tell
// a full success from a partial one without inspecting individual app
// state afterward.
type projectRestartResponse struct {
	RestartedCount int      `json:"restarted_count"`
	Apps           []string `json:"apps"`
	Failed         []string `json:"failed,omitempty"`
}

// handleRestartProject handles POST /api/v1/projects/{id}/restart: force
// every app filed under this project to have its running container
// recreated with no image change, the project-scoped counterpart of
// handleRestartApp (apps.go). No request body.
//
// One app's restart failing does not stop the rest: each is attempted
// independently and the response reports which succeeded and which
// failed, the same "keep going, report per-item outcome" shape a bulk
// operation over independently-owned resources needs, since an operator
// restarting a whole project's apps would rather see a partial result
// than have one bad app block every other one.
func (rt *Router) handleRestartProject(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ctx := r.Context()

	if _, err := rt.projects.GetProject(ctx, id); errors.Is(err, store.ErrProjectNotFound) {
		writeError(w, http.StatusNotFound, "project not found")
		return
	} else if err != nil {
		rt.logger.Error("api: restart project: get project failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	services, err := rt.apps.ListDesiredServicesByProject(ctx, id)
	if err != nil {
		rt.logger.Error("api: restart project: list services failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	resp := projectRestartResponse{
		Apps:   []string{},
		Failed: []string{},
	}
	for _, svc := range services {
		if err := rt.apps.RestartService(ctx, svc.Name); err != nil {
			rt.logger.Error("api: restart project: restart app failed", slog.String("error", err.Error()), slog.String("project_id", id), slog.String("name", svc.Name))
			resp.Failed = append(resp.Failed, svc.Name)
			continue
		}
		resp.Apps = append(resp.Apps, svc.Name)
	}
	resp.RestartedCount = len(resp.Apps)

	writeJSON(w, http.StatusOK, resp)
}

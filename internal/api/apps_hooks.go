package api

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// hookRunResource is one hook's most recent outcome, GET
// /api/v1/apps/{name}/hook-runs' own wire shape.
type hookRunResource struct {
	HookType string    `json:"hook_type"`
	Command  string    `json:"command"`
	ExitCode int       `json:"exit_code"`
	Success  bool      `json:"success"`
	Output   string    `json:"output"`
	RanAt    time.Time `json:"ran_at"`
}

// appHookRunsResource is GET /api/v1/apps/{name}/hook-runs' response:
// PreDeploy/PostDeploy are each nil until that hook has actually run
// once, distinct from an empty hookRunResource which would look like a
// real (if empty) run.
type appHookRunsResource struct {
	PreDeploy  *hookRunResource `json:"pre_deploy,omitempty"`
	PostDeploy *hookRunResource `json:"post_deploy,omitempty"`
}

func toHookRunResource(run store.HookRun) hookRunResource {
	return hookRunResource{
		HookType: run.HookType,
		Command:  run.Command,
		ExitCode: run.ExitCode,
		Success:  run.Success,
		Output:   run.Output,
		RanAt:    run.RanAt,
	}
}

// handleGetAppHookRuns handles GET /api/v1/apps/{name}/hook-runs: the
// most recent outcome of each of name's pre/post-deploy hooks
// (internal/reconcile/application's own HookRunRecorder). A 404 for an
// unknown app, matching handleGetAppGroup's own existence check; an app
// with no hooks configured (or configured but never yet reconciled)
// returns 200 with both fields absent, not a 404, the same "empty is a
// valid, non-error result" shape store.GetHookRuns itself already
// documents.
func (rt *Router) handleGetAppHookRuns(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	if _, err := rt.apps.GetDesiredService(r.Context(), name); err != nil {
		if errors.Is(err, store.ErrServiceNotFound) {
			writeError(w, http.StatusNotFound, "app not found")
			return
		}
		rt.internalError(w, "api: get app hook runs: get service failed", err, slog.String("name", name))
		return
	}

	runs, err := rt.hookRuns.GetHookRuns(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: get app hook runs: query failed", err, slog.String("name", name))
		return
	}

	var out appHookRunsResource
	for _, run := range runs {
		resource := toHookRunResource(run)
		switch run.HookType {
		case store.HookTypePreDeploy:
			out.PreDeploy = &resource
		case store.HookTypePostDeploy:
			out.PostDeploy = &resource
		}
	}
	writeJSON(w, http.StatusOK, out)
}

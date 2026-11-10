package api

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/secretmask"
	"github.com/GLINCKER/levelrail/internal/store"
)

// handleNextBuildReport handles GET /api/v1/apps/{name}/deploys/{deployId}/build-report:
// the Next.js route table and warnings parsed from that attempt's stored build log.
func (rt *Router) handleNextBuildReport(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !rt.requireReadableApp(w, r, name) {
		return
	}
	deployID := r.PathValue("deployId")

	if _, err := rt.apps.GetDesiredService(r.Context(), name); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: build report: load app failed", err, slog.String("name", name))
		return
	}
	attempt, err := rt.deployAttempts.GetDeployAttempt(r.Context(), deployID)
	if errors.Is(err, store.ErrDeployAttemptNotFound) || (err == nil && attempt.ServiceName != name) {
		writeError(w, http.StatusNotFound, "deploy attempt not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: build report: load attempt failed", err, slog.String("deploy_id", deployID))
		return
	}

	var lines []string
	if attempt.FinishedAt == nil {
		if rt.deployRecorder != nil {
			snap, _, unsubscribe, ok := rt.deployRecorder.Snapshot(deployID)
			if ok {
				unsubscribe()
			}
			for _, ev := range snap {
				lines = append(lines, ev.Line)
			}
		}
	} else {
		if rt.deployLogStore == nil {
			writeError(w, http.StatusNotImplemented, "deploy log storage is not configured on this control plane")
			return
		}
		entries, err := rt.deployLogStore.QueryDeployLog(r.Context(), deployID)
		if err != nil {
			rt.internalError(w, "api: build report: query persisted log failed", err, slog.String("deploy_id", deployID))
			return
		}
		for _, e := range entries {
			lines = append(lines, e.Message)
		}
	}

	rep := build.ParseNextReport(lines)
	for i, warn := range rep.Warnings {
		rep.Warnings[i] = secretmask.Mask(warn)
	}
	writeJSON(w, http.StatusOK, rep)
}

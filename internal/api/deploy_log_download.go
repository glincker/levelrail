package api

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// handleDownloadDeployLog handles
// GET /api/v1/apps/{name}/deploys/{deployId}/logs/download: one deploy
// attempt's full log as a plain-text attachment, the download
// counterpart to handleDeployLogStream's SSE view. Live if the attempt
// hasn't finished yet (whatever rt.deployRecorder has buffered so far),
// persisted otherwise, mirroring handleDeployLogStream's own
// live-vs-finished split.
func (rt *Router) handleDownloadDeployLog(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	deployID := r.PathValue("deployId")

	_, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: download deploy log: load app failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	attempt, err := rt.deployAttempts.GetDeployAttempt(r.Context(), deployID)
	if errors.Is(err, store.ErrDeployAttemptNotFound) {
		writeError(w, http.StatusNotFound, "deploy attempt not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: download deploy log: load attempt failed", slog.String("error", err.Error()), slog.String("deploy_id", deployID))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	// Attempt IDs are globally unique but the route is app-scoped: a
	// mismatch here is treated as not-found, not a cross-app leak.
	if attempt.ServiceName != name {
		writeError(w, http.StatusNotFound, "deploy attempt not found")
		return
	}

	var body strings.Builder
	if attempt.FinishedAt == nil {
		if rt.deployRecorder == nil {
			writeError(w, http.StatusNotImplemented, "deploy log recording is not configured on this control plane")
			return
		}
		lines, _, unsubscribe, ok := rt.deployRecorder.Snapshot(deployID)
		if ok {
			unsubscribe()
		}
		for _, ev := range lines {
			fmt.Fprintf(&body, "%s %s\n", ev.Stream, ev.Line)
		}
	} else {
		if rt.deployLogStore == nil {
			writeError(w, http.StatusNotImplemented, "deploy log storage is not configured on this control plane")
			return
		}
		entries, err := rt.deployLogStore.QueryDeployLog(r.Context(), deployID)
		if err != nil {
			rt.logger.Error("api: download deploy log: query persisted log failed", slog.String("error", err.Error()), slog.String("deploy_id", deployID))
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		for _, e := range entries {
			fmt.Fprintf(&body, "%s %s\n", e.Stream, e.Message)
		}
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", deployLogDownloadFilename(name, deployID)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(body.String()))
}

// deployLogDownloadFilename mirrors logDownloadFilename's own
// sanitizing rule (a slash or space in name can't turn into an
// unexpected path segment or a visually broken Content-Disposition
// value), keyed by deployID instead of a time-range bound since a
// single deploy attempt's log has no "to" to name it by.
func deployLogDownloadFilename(name, deployID string) string {
	safeName := strings.NewReplacer("/", "-", " ", "-").Replace(name)
	return fmt.Sprintf("%s-deploy-%s-%s.txt", safeName, deployID, time.Now().UTC().Format("20060102T150405Z"))
}

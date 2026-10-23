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

// handleDownloadDeployLog serves live lines while the attempt runs, then the persisted log.
func (rt *Router) handleDownloadDeployLog(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	deployID := r.PathValue("deployId")

	_, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: download deploy log: load app failed", err, slog.String("name", name))
		return
	}

	attempt, err := rt.deployAttempts.GetDeployAttempt(r.Context(), deployID)
	if errors.Is(err, store.ErrDeployAttemptNotFound) {
		writeError(w, http.StatusNotFound, "deploy attempt not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: download deploy log: load attempt failed", err, slog.String("deploy_id", deployID))
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
			rt.internalError(w, "api: download deploy log: query persisted log failed", err, slog.String("deploy_id", deployID))
			return
		}
		for _, e := range rt.mergeTraceIntoLog(r.Context(), deployID, entries) {
			fmt.Fprintf(&body, "%s %s\n", e.Stream, e.Message)
		}
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", deployLogDownloadFilename(name, deployID)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(body.String()))
}

// deployLogDownloadFilename sanitizes like logDownloadFilename, keyed by deployID since there is no time range to name it by.
func deployLogDownloadFilename(name, deployID string) string {
	safeName := strings.NewReplacer("/", "-", " ", "-").Replace(name)
	return fmt.Sprintf("%s-deploy-%s-%s.txt", safeName, deployID, time.Now().UTC().Format("20060102T150405Z"))
}

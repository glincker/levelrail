package api

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// Failure context env vars and defaults.
const (
	envFailureContextRestarts = "APP_FAILURE_CONTEXT_RESTARTS"
	envFailureContextWindow   = "APP_FAILURE_CONTEXT_WINDOW"
	envFailureContextLines    = "APP_FAILURE_CONTEXT_LINES"

	defaultFailureContextRestarts = 3
	defaultFailureContextWindow   = 15 * time.Minute
	defaultFailureContextLines    = 200
	maxFailureContextLines        = 1000

	failureStateHealthy      = "healthy"
	failureStateCrashlooping = "crashlooping"
	failureStateDeployFailed = "deploy_failed"
)

type failureContextDeploy struct {
	ID         string     `json:"id"`
	Status     string     `json:"status"`
	Image      string     `json:"image"`
	Commit     string     `json:"commit,omitempty"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	Error      string     `json:"error,omitempty"`
}

type failureContextLine struct {
	Timestamp time.Time `json:"timestamp"`
	Stream    string    `json:"stream"`
	Message   string    `json:"message"`
	Level     string    `json:"level,omitempty"`
}

type failureContextResponse struct {
	State           string                `json:"state"`
	Since           *time.Time            `json:"since,omitempty"`
	RestartsInWin   int                   `json:"restarts_in_window"`
	WindowSeconds   float64               `json:"window_seconds"`
	ContainerID     string                `json:"container_id,omitempty"`
	Deploy          *failureContextDeploy `json:"deploy,omitempty"`
	Lines           []failureContextLine  `json:"lines"`
	TotalLines      int                   `json:"total_lines"`
	LinesSource     string                `json:"lines_source,omitempty"`
	RestartsToTrip  int                   `json:"restarts_threshold"`
	LogsUnavailable bool                  `json:"logs_unavailable,omitempty"`
}

// handleFailureContext handles GET /api/v1/apps/{name}/failure-context: when
// the app is crashlooping or its newest deploy failed, the last lines of the
// failing container (or the deploy's build log) so the cause is on the page
// without opening the log viewer.
func (rt *Router) handleFailureContext(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	name := r.PathValue("name")
	if _, err := rt.apps.GetDesiredService(ctx, name); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: failure context: load app failed", err, slog.String("name", name))
		return
	}

	window := envDuration(envFailureContextWindow, defaultFailureContextWindow)
	threshold := envInt(envFailureContextRestarts, defaultFailureContextRestarts)
	maxLines := min(envInt(envFailureContextLines, defaultFailureContextLines), maxFailureContextLines)
	now := time.Now()
	resp := failureContextResponse{
		State: failureStateHealthy, WindowSeconds: window.Seconds(), RestartsToTrip: threshold,
		Lines: []failureContextLine{},
	}

	var restarts []telemetry.Sample
	if rt.telemetry != nil {
		var err error
		restarts, err = rt.telemetry.QueryMetrics(ctx, resourceIDForApp(name), telemetry.MetricContainerRestartCount, now.Add(-window), now)
		if err != nil && len(restarts) == 0 {
			rt.logger.Warn("api: failure context: restart query failed", slog.String("error", err.Error()), slog.String("name", name))
		}
	}
	resp.RestartsInWin = len(restarts)

	var failed *store.DeployAttempt
	if attempts, err := rt.deployAttempts.ListDeployAttempts(ctx, name); err != nil {
		rt.logger.Warn("api: failure context: list attempts failed", slog.String("error", err.Error()), slog.String("name", name))
	} else if len(attempts) > 0 && attempts[0].Status == store.DeployAttemptStatusFailed {
		a := attempts[0]
		failed = &a
	}

	switch {
	case len(restarts) >= threshold:
		resp.State = failureStateCrashlooping
		since := restarts[0].Timestamp
		resp.Since = &since
	case failed != nil:
		resp.State = failureStateDeployFailed
		since := failed.StartedAt
		if failed.FinishedAt != nil {
			since = *failed.FinishedAt
		}
		resp.Since = &since
	}
	if failed != nil {
		resp.Deploy = &failureContextDeploy{
			ID: failed.ID, Status: failed.Status, Image: failed.Image, Commit: failed.CommitSHA,
			StartedAt: failed.StartedAt, FinishedAt: failed.FinishedAt, Error: failed.Error,
		}
	}
	if resp.State == failureStateHealthy {
		writeJSON(w, http.StatusOK, resp)
		return
	}

	switch {
	case resp.State == failureStateDeployFailed && attemptFailedDuringBuild(failed) && rt.deployLogStore != nil:
		rt.fillBuildLines(r, &resp, failed.ID, maxLines)
	case rt.telemetry != nil:
		rt.fillRuntimeLines(r, &resp, name, now.Add(-window), now, maxLines)
	default:
		resp.LogsUnavailable = true
	}
	writeJSON(w, http.StatusOK, resp)
}

func (rt *Router) fillBuildLines(r *http.Request, resp *failureContextResponse, attemptID string, maxLines int) {
	entries, err := rt.deployLogStore.QueryDeployLog(r.Context(), attemptID)
	if err != nil {
		rt.logger.Warn("api: failure context: build log query failed", slog.String("error", err.Error()), slog.String("deploy_attempt_id", attemptID))
		resp.LogsUnavailable = true
		return
	}
	resp.TotalLines = len(entries)
	if len(entries) > maxLines {
		entries = entries[len(entries)-maxLines:]
	}
	for _, e := range entries {
		resp.Lines = append(resp.Lines, failureContextLine{Timestamp: e.Timestamp, Stream: e.Stream, Message: e.Message})
	}
	resp.LinesSource = "build"
}

func (rt *Router) fillRuntimeLines(r *http.Request, resp *failureContextResponse, name string, from, to time.Time, maxLines int) {
	entries, err := rt.telemetry.QueryLogs(r.Context(), resourceIDForApp(name), from, to, "")
	if err != nil && len(entries) == 0 {
		rt.logger.Warn("api: failure context: log query failed", slog.String("error", err.Error()), slog.String("name", name))
		resp.LogsUnavailable = true
		return
	}
	if len(entries) > 0 {
		if id := entries[len(entries)-1].ContainerID; id != "" {
			resp.ContainerID = id
			entries = filterLogsByContainer(entries, id)
		}
	}
	resp.TotalLines = len(entries)
	if len(entries) > maxLines {
		entries = entries[len(entries)-maxLines:]
	}
	for _, e := range entries {
		resp.Lines = append(resp.Lines, failureContextLine{
			Timestamp: e.Timestamp, Stream: e.Stream, Message: e.Message, Level: classifyLogLevel(e),
		})
	}
	resp.LinesSource = "runtime"
}

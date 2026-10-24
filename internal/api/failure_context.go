package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// Failure context env vars and defaults.
const (
	envFailureContextRestarts = "APP_FAILURE_CONTEXT_RESTARTS"
	envFailureContextWindow   = "APP_FAILURE_CONTEXT_WINDOW"
	envFailureContextLines    = "APP_FAILURE_CONTEXT_LINES"
	envFailureContextQuiet    = "APP_FAILURE_CONTEXT_QUIET"

	defaultFailureContextRestarts = 3
	defaultFailureContextWindow   = 15 * time.Minute
	defaultFailureContextLines    = 200
	defaultFailureContextQuiet    = 2 * time.Minute
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
	// Exit is how the newest release container stopped, when it was captured.
	Exit *deployExitResource `json:"exit,omitempty"`
}

// Returns the last lines of the failing container, or the deploy's build log, so the cause shows without opening the log viewer.
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

	recovered := len(restarts) >= threshold && rt.crashRecovered(ctx, name, restarts[len(restarts)-1].Timestamp, now)
	switch {
	case len(restarts) >= threshold && !recovered:
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

	exit := rt.latestExit(ctx, name)
	if exit != nil {
		resp.Exit = toDeployExitResource(exit)
	}
	switch {
	case exit != nil && len(exit.LogTail) > 0 && (resp.State != failureStateDeployFailed || !attemptFailedDuringBuild(failed)):
		// The capture is the failing container's own output, unlike the stored
		// stream where the previous release's lines interleave.
		fillExitLines(&resp, maxLines)
	case resp.State == failureStateDeployFailed && attemptFailedDuringBuild(failed) && rt.deployLogStore != nil:
		rt.fillBuildLines(r, &resp, failed.ID, maxLines)
	case rt.telemetry != nil:
		rt.fillRuntimeLines(r, &resp, name, now.Add(-window), now, maxLines)
	default:
		resp.LogsUnavailable = true
	}
	if len(resp.Lines) == 0 && resp.Exit != nil && len(resp.Exit.Lines) > 0 {
		fillExitLines(&resp, maxLines)
	}
	if resp.Exit != nil {
		resp.Exit.Lines = []failureContextLine{}
	}
	writeJSON(w, http.StatusOK, resp)
}

// crashRecovered reports an app that is Ready again with no restart for the
// quiet period, so a past crash loop stops showing as current.
func (rt *Router) crashRecovered(ctx context.Context, name string, lastRestart, now time.Time) bool {
	if rt.deploys == nil || now.Sub(lastRestart) < envDuration(envFailureContextQuiet, defaultFailureContextQuiet) {
		return false
	}
	conds, err := rt.deploys.GetConditions(ctx, applicationControllerName(name))
	if err != nil {
		return false
	}
	for _, c := range conds {
		if c.Type == "Ready" {
			return c.Status == reconcile.ConditionTrue
		}
	}
	return false
}

// fillExitLines uses the exit capture when the log store has nothing: the
// collector only streams running containers, so one that exits at start
// leaves no stored lines.
func fillExitLines(resp *failureContextResponse, maxLines int) {
	lines := resp.Exit.Lines
	resp.TotalLines = len(lines)
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	resp.Lines = lines
	resp.LinesSource = "exit"
	resp.LogsUnavailable = false
	resp.ContainerID = resp.Exit.Container
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

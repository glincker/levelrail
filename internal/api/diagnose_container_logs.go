package api

import (
	"context"
	"log/slog"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

// containerLogReader is the optional runtime capability containerLogTail reads through.
type containerLogReader interface {
	Logs(ctx context.Context, containerID string, follow bool, since time.Time) (<-chan docker.LogLine, <-chan error)
}

// containerLogTail reads the app container's recent output straight from
// Docker. The log collector only streams containers that are running at a
// resync, so one that exits at start can leave the log store empty.
func (rt *Router) containerLogTail(ctx context.Context, svc *store.DesiredService) []string {
	lines := rt.readContainerLogs(ctx, svc, time.Now().Add(-diagnosticLogWindow), diagnosticLogLines)
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		out = append(out, l.Message)
	}
	return out
}

// containerLogLines is the last lines the current release's container
// printed, whenever it ran, up to limit lines.
func (rt *Router) containerLogLines(ctx context.Context, svc *store.DesiredService, limit int) []failureContextLine {
	lines := rt.readContainerLogs(ctx, svc, time.Time{}, limit)
	out := make([]failureContextLine, 0, len(lines))
	for _, l := range lines {
		out = append(out, failureContextLine{Timestamp: l.Timestamp, Stream: l.Stream, Message: l.Message})
	}
	return out
}

func (rt *Router) readContainerLogs(ctx context.Context, svc *store.DesiredService, since time.Time, limit int) []docker.LogLine {
	if rt.execRuntime == nil {
		return nil
	}
	runtime, err := rt.execRuntime(svc.NodeID)
	if err != nil {
		rt.logger.Warn("api: resolve node runtime for logs failed", slog.String("error", err.Error()), slog.String("name", svc.Name))
		return nil
	}
	reader, ok := runtime.(containerLogReader)
	if !ok {
		return nil
	}
	lctx, cancel := context.WithTimeout(ctx, envDurationOr(envDiagnoseProbeBudget, defaultProbeBudget))
	defer cancel()

	lines, errs := reader.Logs(lctx, application.ContainerName(svc.Name, application.NameImage(*svc), svc.RestartNonce), false, since)
	var out []docker.LogLine
	for l := range lines {
		out = append(out, l)
		if len(out) > limit {
			out = out[1:]
		}
	}
	if err := <-errs; err != nil {
		rt.logger.Debug("api: read container logs failed", slog.String("error", err.Error()), slog.String("name", svc.Name))
	}
	return out
}

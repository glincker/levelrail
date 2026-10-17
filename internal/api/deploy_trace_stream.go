package api

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploytrace"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// EnvTracePollInterval and EnvTraceTailIdle tune how a stream follows trace
// rows the reconciler writes after the deploy attempt itself has finished.
const (
	EnvTracePollInterval = "APP_DEPLOY_TRACE_POLL_INTERVAL"
	EnvTraceTailIdle     = "APP_DEPLOY_TRACE_TAIL_IDLE"
)

// traceLister is the optional store surface the trace views need.
type traceLister interface {
	ListDeployTraceSteps(ctx context.Context, attemptID string) ([]store.DeployTraceStep, error)
}

func (rt *Router) traceRows(ctx context.Context, attemptID string) []store.DeployTraceStep {
	l, ok := rt.deployAttempts.(traceLister)
	if !ok {
		return nil
	}
	rows, err := l.ListDeployTraceSteps(ctx, attemptID)
	if err != nil {
		rt.logger.Warn("api: list deploy trace steps failed", "deploy_id", attemptID, "error", err.Error())
		return nil
	}
	return rows
}

func durationEnv(name string, def time.Duration) time.Duration {
	if d, err := time.ParseDuration(strings.TrimSpace(os.Getenv(name))); err == nil && d > 0 {
		return d
	}
	return def
}

// stepSignature changes whenever a row's visible state does.
func stepSignature(s store.DeployTraceStep) string {
	return fmt.Sprintf("%s|%s|%s|%d|%d", s.Status, s.Reason, s.Detail, s.Attempts, s.DurationMS)
}

// toSSETraceEvent maps a persisted row onto the step wire shape.
func toSSETraceEvent(s store.DeployTraceStep) sseStepEvent {
	ts := s.StartedAt
	if !s.EndedAt.IsZero() {
		ts = s.EndedAt
	}
	ev := sseStepEvent{
		Step: s.Step, Status: s.Status, Timestamp: ts.UTC().Format(time.RFC3339Nano),
		Key: s.Key, Reason: s.Reason, Detail: s.Detail, Service: s.Service,
		StartedAt: s.StartedAt.UTC().Format(time.RFC3339Nano), DurationMS: s.DurationMS, Attempts: s.Attempts,
	}
	if !s.EndedAt.IsZero() {
		ev.EndedAt = s.EndedAt.UTC().Format(time.RFC3339Nano)
	}
	return ev
}

// followTrace writes rows that changed since sent, then keeps polling until
// ctx ends. Polling stops after the idle window once the attempt finished, so
// a long-open tab does not hold a query per second forever.
func (rt *Router) followTrace(ctx context.Context, attemptID string, finished bool, sent map[string]string, emit func([]store.DeployTraceStep)) {
	interval := durationEnv(EnvTracePollInterval, time.Second)
	idle := durationEnv(EnvTraceTailIdle, 5*time.Minute)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	lastChange := time.Now()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		rows := rt.traceRows(ctx, attemptID)
		var changed []store.DeployTraceStep
		for _, r := range rows {
			if sig := stepSignature(r); sent[r.Key] != sig {
				sent[r.Key] = sig
				changed = append(changed, r)
			}
		}
		if len(changed) > 0 {
			lastChange = time.Now()
			emit(changed)
		}
		if finished && time.Since(lastChange) > idle {
			<-ctx.Done()
			return
		}
	}
}

// serveTraceSteps streams persisted steps: the snapshot, then live changes.
func (rt *Router) serveTraceSteps(ctx context.Context, w http.ResponseWriter, attemptID string, rows []store.DeployTraceStep, finished bool) {
	flusher, ok := startSSE(w)
	if !ok {
		return
	}
	_, _ = fmt.Fprint(w, ": connected\n\n")
	sent := map[string]string{}
	for _, r := range rows {
		sent[r.Key] = stepSignature(r)
		writeSSEStepEvent(w, toSSETraceEvent(r))
	}
	flusher.Flush()
	rt.followTrace(ctx, attemptID, finished, sent, func(changed []store.DeployTraceStep) {
		for _, r := range changed {
			writeSSEStepEvent(w, toSSETraceEvent(r))
		}
		flusher.Flush()
	})
}

// traceLogEntries renders trace rows as deploy log lines, oldest first.
func traceLogEntries(attemptID string, rows []store.DeployTraceStep) []telemetry.DeployLogEntry {
	out := make([]telemetry.DeployLogEntry, 0, len(rows))
	for _, r := range rows {
		ts := r.StartedAt
		if !r.EndedAt.IsZero() {
			ts = r.EndedAt
		}
		stream := "stdout"
		if r.Status == store.TraceStatusFailed {
			stream = "stderr"
		}
		out = append(out, telemetry.DeployLogEntry{AttemptID: attemptID, Stream: stream, Timestamp: ts, Message: deploytrace.FormatLine(r)})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Timestamp.Before(out[j].Timestamp) })
	return out
}

// mergeTraceIntoLog interleaves trace lines with build output by time.
func (rt *Router) mergeTraceIntoLog(ctx context.Context, attemptID string, entries []telemetry.DeployLogEntry) []telemetry.DeployLogEntry {
	rows := rt.traceRows(ctx, attemptID)
	if len(rows) == 0 {
		return entries
	}
	merged := append(append([]telemetry.DeployLogEntry(nil), entries...), traceLogEntries(attemptID, rows)...)
	sort.SliceStable(merged, func(i, j int) bool { return merged[i].Timestamp.Before(merged[j].Timestamp) })
	return merged
}

// followTraceLog appends trace lines to an open log stream as they change.
func (rt *Router) followTraceLog(ctx context.Context, w http.ResponseWriter, flusher http.Flusher, attemptID string, finished bool, sent map[string]string) {
	rt.followTrace(ctx, attemptID, finished, sent, func(changed []store.DeployTraceStep) {
		for _, e := range traceLogEntries(attemptID, changed) {
			writeSSEEvent(w, sseLogEvent{Line: e.Message, Stream: e.Stream})
		}
		flusher.Flush()
	})
}

// writeChangedTraceLines writes the trace lines that changed since sent.
func (rt *Router) writeChangedTraceLines(ctx context.Context, w http.ResponseWriter, flusher http.Flusher, attemptID string, sent map[string]string) {
	rows := rt.traceRows(ctx, attemptID)
	var changed []store.DeployTraceStep
	for _, r := range rows {
		if sig := stepSignature(r); sent[r.Key] != sig {
			sent[r.Key] = sig
			changed = append(changed, r)
		}
	}
	if len(changed) == 0 {
		return
	}
	for _, e := range traceLogEntries(attemptID, changed) {
		writeSSEEvent(w, sseLogEvent{Line: e.Message, Stream: e.Stream})
	}
	flusher.Flush()
}

// persistStep stores a coarse build step (detecting, building, pushing,
// deploying) in the same trace the reconciler writes to.
func (rt *Router) persistStep(id, step, status string) {
	sink, ok := rt.deployAttempts.(deploytrace.Sink)
	if !ok {
		return
	}
	now := time.Now()
	s := store.DeployTraceStep{AttemptID: id, Key: step, Step: step, Status: status, StartedAt: now}
	if status != store.TraceStatusRunning {
		s.EndedAt = now
	}
	if err := sink.SaveDeployTraceStep(context.Background(), s); err != nil {
		rt.logger.Warn("api: persist deploy step failed", "deploy_id", id, "step", step, "error", err.Error())
	}
}

// coarseBuildSteps are written by both the live recorder and the stored
// trace; the live stream gets them from the recorder only.
var coarseBuildSteps = map[string]bool{"detecting": true, "building": true, "pushing": true, "deploying": true, "failed": true}

// writeTraceExtras writes stored trace rows that changed since sent, except
// the build steps the recorder already streams. It reports whether it wrote.
func (rt *Router) writeTraceExtras(ctx context.Context, w http.ResponseWriter, attemptID string, sent map[string]string) bool {
	rows := rt.traceRows(ctx, attemptID)
	wrote := false
	for _, r := range rows {
		sig := stepSignature(r)
		if coarseBuildSteps[r.Step] {
			sent[r.Key] = sig
			continue
		}
		if sent[r.Key] != sig {
			sent[r.Key] = sig
			writeSSEStepEvent(w, toSSETraceEvent(r))
			wrote = true
		}
	}
	return wrote
}

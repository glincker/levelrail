package application

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/GLINCKER/levelrail/internal/deploytrace"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/kit/probe"
)

// ProbeAttemptRecorder persists individual readiness-probe attempts made
// during a deploy's cutover (migrations/0284_probe_attempts.sql), the
// per-attempt detail (status code, latency) a reconcile condition's own
// single Reason/Message summary never captures. *store.DB satisfies
// this structurally.
type ProbeAttemptRecorder interface {
	RecordProbeAttempt(ctx context.Context, a store.ProbeAttempt) error
}

// WithProbeAttemptRecorder enables persisting every individual
// readiness-probe attempt waitReady makes during a deploy's cutover.
// Without one configured (the default, nil), probing behaves exactly as
// before: attempts are just never persisted. See probe.WithOnAttempt's
// own doc comment for why this can never affect WaitReady's retry or
// pass/fail decision, only what gets recorded alongside it.
func WithProbeAttemptRecorder(r ProbeAttemptRecorder) Option {
	return func(ctrl *Controller) { ctrl.probeAttempts = r }
}

// deployProber is prober() plus, when a ProbeAttemptRecorder is
// configured, a side channel (probe.WithOnAttempt) reporting every
// attempt to it. Scoped to waitReady alone, not prober()'s other
// callers: deploy-time readiness probing specifically. RecordProbeAttempt
// resolves deploy_attempt_id the same way RecordRollout already does
// (Reconcile carries no deploy-attempt identity of its own): the newest
// succeeded deploy_attempts row for (serviceName, image). Best-effort,
// the same shape recordHookRun establishes in controller.go.
func (c *Controller) deployProber(ctx context.Context, desired *store.DesiredService) *probe.Prober {
	tr := c.tracer(ctx, desired)
	if c.probeAttempts == nil && tr == nil {
		return c.prober()
	}
	serviceName, image := c.serviceName, desired.Image
	var n atomic.Int64
	return c.prober(probe.WithOnAttempt(func(a probe.Attempt) {
		if c.probeAttempts != nil {
			_ = c.probeAttempts.RecordProbeAttempt(ctx, store.ProbeAttempt{
				ServiceName: serviceName,
				Image:       image,
				Target:      a.Target,
				Success:     a.Success,
				StatusCode:  a.StatusCode,
				ExitCode:    a.ExitCode,
				Error:       a.Error,
				LatencyMS:   a.Latency.Milliseconds(),
				ProbedAt:    a.Time,
			})
		}
		traceProbe(ctx, tr, int(n.Add(1)), a)
	}))
}

// traceProbe writes one readiness probe attempt. A failed attempt is a
// warning, not a failure: the container is usually still starting, and the
// readiness gate step carries the verdict.
func traceProbe(ctx context.Context, tr *deploytrace.Tracer, n int, a probe.Attempt) {
	if tr == nil {
		return
	}
	key := deploytrace.StepReadinessProbe + "." + strconv.Itoa(n)
	if n > maxTracedProbes() {
		key = deploytrace.StepReadinessProbe + ".latest"
	}
	status, reason := store.TraceStatusDone, ""
	detail := fmt.Sprintf("#%d %s", n, a.Target)
	if a.StatusCode != 0 {
		detail += fmt.Sprintf(" status %d", a.StatusCode)
	} else if a.ExitCode != 0 {
		detail += fmt.Sprintf(" exit %d", a.ExitCode)
	}
	detail += fmt.Sprintf(" in %dms", a.Latency.Milliseconds())
	if !a.Success {
		status, reason = store.TraceStatusWarning, "ProbeFailed"
		if a.Error != "" {
			detail += ": " + a.Error
		}
	}
	tr.NoteKey(ctx, deploytrace.StepReadinessProbe, key, status, reason, detail)
}

// EnvTraceMaxProbes caps how many readiness attempts get their own trace row.
const EnvTraceMaxProbes = "APP_DEPLOY_TRACE_MAX_PROBES"

func maxTracedProbes() int {
	if v, err := strconv.Atoi(strings.TrimSpace(os.Getenv(EnvTraceMaxProbes))); err == nil && v >= 0 {
		return v
	}
	return 20
}

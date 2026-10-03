package application

import (
	"context"

	"github.com/GLINCKER/levelrail/internal/probe"
	"github.com/GLINCKER/levelrail/internal/store"
)

// ProbeAttemptRecorder persists individual readiness-probe attempts made
// during a deploy's cutover (migrations/0280_probe_attempts.sql), the
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
	if c.probeAttempts == nil {
		return c.prober()
	}
	serviceName, image := c.serviceName, desired.Image
	return c.prober(probe.WithOnAttempt(func(a probe.Attempt) {
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
	}))
}

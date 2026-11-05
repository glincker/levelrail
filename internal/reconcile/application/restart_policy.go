package application

import (
	"context"
	"log/slog"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// WithRestartPolicy sets where the Docker restart policy for new long-running
// containers comes from. It is read at each create, so a stored change applies
// to the next container without a restart.
func WithRestartPolicy(get func(context.Context) string) Option {
	return func(ctrl *Controller) { ctrl.restartPolicy = get }
}

// restartPolicyFor returns the Docker restart policy for a container created by
// createAndStart. Only the live replica of a normal app gets one: run-to-completion
// services stay "no", and builds, jobs, previews and deploy URL copies are created
// elsewhere with "no". Older releases keep the policy they were created with, but
// the control plane stops them explicitly, which unless-stopped respects.
func (c *Controller) restartPolicyFor(ctx context.Context, desired *store.DesiredService) string {
	if c.restartPolicy == nil || desired.RunsToCompletion() {
		return ""
	}
	switch p := c.restartPolicy(ctx); p {
	case "unless-stopped", "on-failure":
		return p
	}
	return ""
}

// sweepRestartPolicy brings a ready container created before the default to the
// desired policy through the update API, never a recreate. It only upgrades
// (desired non-empty), so setting containers.restart_policy to no turns it off.
// A failure is logged once per container and never fails the pass.
func (c *Controller) sweepRestartPolicy(ctx context.Context, state *docker.ContainerState, desired *store.DesiredService) {
	syncer, ok := c.runtime.(docker.RestartPolicySyncer)
	if !ok || state == nil {
		return
	}
	want := c.restartPolicyFor(ctx, desired)
	if want == "" {
		return
	}
	if _, done := c.policySwept.Load(state.ID); done {
		return
	}
	changed, err := syncer.SyncRestartPolicy(ctx, state.ID, want)
	if err != nil {
		slog.Warn("reconcile: restart policy sweep failed", slog.String("service", c.serviceName), slog.String("container", state.Name), slog.String("error", err.Error()))
		return
	}
	c.policySwept.Store(state.ID, struct{}{})
	if changed {
		slog.Info("reconcile: restart policy updated in place", slog.String("service", c.serviceName), slog.String("container", state.Name), slog.String("policy", want))
	}
}

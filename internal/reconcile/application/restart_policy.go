package application

import (
	"context"

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

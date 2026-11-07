package ingress

import (
	"context"
	"time"

	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

// WithNextGraceIDs enables Next.js skew routing: ids returns the deployment
// ids of a service's superseded releases that are still inside their grace window.
func WithNextGraceIDs(ids func(service string, now time.Time) []string) Option {
	return func(c *Controller) { c.nextGraceIDs = ids }
}

// nextSkewFor resolves the held previous release for a service in grace. The
// held containers are not in the normal pool: only requests that carry an old id reach them.
func (c *Controller) nextSkewFor(ctx context.Context, svc store.DesiredService, currentDial string, now time.Time) *ingress.NextSkewRoute {
	if c.nextGraceIDs == nil {
		return nil
	}
	ids := c.nextGraceIDs(svc.Name, now)
	if len(ids) == 0 {
		return nil
	}
	target := application.ContainerName(svc.Name, application.NameImage(svc), svc.RestartNonce)
	held, ok := c.dialPreviousRelease(ctx, svc.Name, svc.Port, target, "")
	if !ok || held == currentDial {
		return nil
	}
	return &ingress.NextSkewRoute{HeldDial: held, DeploymentIDs: ids}
}

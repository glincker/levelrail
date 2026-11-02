package application

import (
	"context"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploysafety"
)

// WithDeployWindows reads the drain, stable, watch and hold timings on every
// pass, so a runtime settings change applies to the next deploy. Unset keeps
// the values given at construction.
func WithDeployWindows(get func(context.Context) deploysafety.Windows) Option {
	return func(ctrl *Controller) { ctrl.deployWindows = get }
}

// holdBase is the configured previous release hold before the drain and
// watch floors in effectiveHold.
func (c *Controller) holdBase(ctx context.Context) time.Duration {
	if c.deployWindows != nil {
		return c.deployWindows(ctx).Hold
	}
	return c.previousReleaseHold
}

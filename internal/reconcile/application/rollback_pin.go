package application

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/deploysafety"
	"github.com/GLINCKER/levelrail/internal/store"
)

// RollbackPinner keeps a release that just took traffic available as a
// rollback target, so image cleanup can never remove it.
type RollbackPinner interface {
	PinServing(ctx context.Context, desired store.DesiredService, imageID string) (pinRef string, err error)
}

// WithRollbackPinner pins every release at cutover. nil pins nothing.
func WithRollbackPinner(p RollbackPinner) Option {
	return func(ctrl *Controller) { ctrl.pinner = p }
}

func (c *Controller) pinServing(ctx context.Context, desired *store.DesiredService) {
	if c.pinner == nil {
		return
	}
	imageID := c.servingImageID
	if imageID == "" {
		imageID = desired.LocalImageID()
	}
	ref, err := c.pinner.PinServing(ctx, *desired, imageID)
	switch {
	case err != nil:
		c.recordGate(ctx, desired, "", deploysafety.PhasePin, deploysafety.StatusFailed, "", fmt.Sprintf("could not pin this release as a rollback target: %v", err))
	case ref != "":
		c.recordGate(ctx, desired, "", deploysafety.PhasePin, deploysafety.StatusPassed, "", "pinned as rollback target "+ref)
	}
}

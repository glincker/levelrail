package application

import (
	"context"
	"time"

	"github.com/GLINCKER/levelrail/internal/reconcile"
)

// PendingAttemptStore settles deploy attempts queued while a node was offline.
type PendingAttemptStore interface {
	ResolvePendingDeployAttempts(ctx context.Context, serviceName string, now time.Time) (int, error)
}

// pendingResolver wraps an app controller so queued attempts leave `pending`
// once a live pass reaches the app (Ready, or stopped for a queued stop).
type pendingResolver struct {
	reconcile.Controller
	serviceName string
	store       PendingAttemptStore
}

// WithPendingAttemptResolution wraps inner; a nil store returns inner as is.
func WithPendingAttemptResolution(inner reconcile.Controller, serviceName string, st PendingAttemptStore) reconcile.Controller {
	if st == nil {
		return inner
	}
	return &pendingResolver{Controller: inner, serviceName: serviceName, store: st}
}

// Reconcile implements reconcile.Controller.
func (p *pendingResolver) Reconcile(ctx context.Context) (reconcile.Result, error) {
	res, err := p.Controller.Reconcile(ctx)
	if err != nil {
		return res, err
	}
	for _, c := range res.Conditions {
		if c.Type != reconcile.ConditionTypeReady {
			continue
		}
		if c.Status == reconcile.ConditionTrue || c.Reason == "Suspended" {
			// Best effort: the next pass retries a failed settle.
			_, _ = p.store.ResolvePendingDeployAttempts(ctx, p.serviceName, time.Now())
		}
		break
	}
	return res, nil
}

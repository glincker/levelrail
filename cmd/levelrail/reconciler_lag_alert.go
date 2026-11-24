package main

import (
	"context"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/doctor"
)

// reconcilerLagAlertSource feeds kind=reconciler_lagging rules; warn is read on every tick so the setting applies live.
type reconcilerLagAlertSource struct {
	lag  func() map[string]time.Duration
	warn func(context.Context) time.Duration
}

// LaggingReconcilers implements alerting.ReconcilerLagSource.
func (s reconcilerLagAlertSource) LaggingReconcilers(ctx context.Context) ([]alerting.LaggingReconciler, error) {
	c := doctor.ReconcilerLag(s.lag(), s.warn(ctx))
	out := make([]alerting.LaggingReconciler, 0, len(c.Lagging))
	for _, l := range c.Lagging {
		out = append(out, alerting.LaggingReconciler{Reconciler: l.Reconciler, Lag: l.Lag})
	}
	return out, nil
}

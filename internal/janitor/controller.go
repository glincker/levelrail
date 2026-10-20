package janitor

import (
	"context"
	"errors"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/reconcile"
)

// Controller runs scheduled passes from the reconcile loop.
type Controller struct{ engine *Engine }

// NewController wraps e.
func NewController(e *Engine) *Controller { return &Controller{engine: e} }

// Name implements reconcile.Controller.
func (c *Controller) Name() string { return "janitor" }

// Reconcile implements reconcile.Controller.
func (c *Controller) Reconcile(ctx context.Context) (reconcile.Result, error) {
	p, err := LoadPolicy(ctx, c.engine.Store)
	if err != nil {
		return cond(reconcile.ConditionFalse, "PolicyUnreadable", err.Error()), fmt.Errorf("janitor: %w", err)
	}
	if !p.Enabled {
		return cond(reconcile.ConditionTrue, "Disabled", ""), nil
	}
	if !c.engine.Due(p) {
		return cond(reconcile.ConditionTrue, "Converged", "waiting for the next scheduled pass"), nil
	}
	rep, err := c.engine.Run(ctx, Options{Trigger: TriggerScheduled, DryRun: p.DryRun})
	if errors.Is(err, ErrBusy) {
		return cond(reconcile.ConditionTrue, "Converged", "a pass is already running"), nil
	}
	if err != nil {
		return cond(reconcile.ConditionFalse, "RunFailed", err.Error()), fmt.Errorf("janitor: %w", err)
	}
	if rep.Error != "" {
		return cond(reconcile.ConditionTrue, "StageFailed", rep.Error), fmt.Errorf("janitor: %s", rep.Error)
	}
	if rep.DryRun {
		return cond(reconcile.ConditionTrue, "DryRun", "cleanup is in preview mode, nothing was removed"), nil
	}
	return cond(reconcile.ConditionTrue, "Converged", ""), nil
}

func cond(s reconcile.ConditionStatus, reason, msg string) reconcile.Result {
	return reconcile.Result{Conditions: []reconcile.Condition{{Type: reconcile.ConditionTypeReady, Status: s, Reason: reason, Message: msg}}}
}

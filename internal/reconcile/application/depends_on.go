package application

import (
	"context"
	"fmt"
	"slices"

	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

// ConditionReader reads another controller's last stored conditions.
type ConditionReader interface {
	GetConditions(ctx context.Context, controllerName string) ([]reconcile.Condition, error)
}

// WithConditionReader enables depends_on condition: service_healthy. Without
// it a healthy dependency degrades to start order, never to a deadlock.
func WithConditionReader(r ConditionReader) Option {
	return func(ctrl *Controller) { ctrl.conditions = r }
}

// dependencyBlock returns a not-yet-ready result when desired names a
// dependency whose own containers have not started yet (or, for one in
// DependsOnHealthy, whose Ready condition is not True), nil once every
// dependency is satisfied. AppID unset (the brief window between
// DeploySpec's Deploy call and its own follow-up UpdateServiceApp,
// multi.go) fails this gate open, not closed.
func (c *Controller) dependencyBlock(ctx context.Context, desired *store.DesiredService) *reconcile.Result {
	if len(desired.DependsOn) == 0 || desired.AppID == "" {
		return nil
	}
	for _, dep := range desired.DependsOn {
		depName := desired.AppID + "-" + dep
		started, err := c.dependencyStarted(ctx, depName)
		if err != nil {
			res := notReady("DependencyCheckFailed", fmt.Errorf("check dependency %q: %w", dep, err))
			return &res
		}
		if !started {
			res := waitingFor(dep, depName, "WaitingForDependency", "start")
			c.traceDependencyWait(ctx, desired, dep, "WaitingForDependency", res.Conditions[0].Message)
			return res
		}
		if !slices.Contains(desired.DependsOnHealthy, dep) || c.conditions == nil {
			continue
		}
		ready, err := c.dependencyReady(ctx, depName)
		if err != nil {
			res := notReady("DependencyCheckFailed", fmt.Errorf("check dependency %q health: %w", dep, err))
			return &res
		}
		if !ready {
			res := waitingFor(dep, depName, "WaitingForDependencyHealthy", "pass its readiness check")
			c.traceDependencyWait(ctx, desired, dep, "WaitingForDependencyHealthy", res.Conditions[0].Message)
			return res
		}
	}
	return nil
}

func waitingFor(dep, depName, reason, what string) *reconcile.Result {
	return &reconcile.Result{Conditions: []reconcile.Condition{{
		Type: reconcile.ConditionTypeReady, Status: reconcile.ConditionUnknown, Reason: reason,
		Message: fmt.Sprintf("waiting for dependency %q (%s) to %s", dep, depName, what),
	}}}
}

func (c *Controller) dependencyReady(ctx context.Context, depName string) (bool, error) {
	conds, err := c.conditions.GetConditions(ctx, ControllerName(depName))
	if err != nil {
		return false, fmt.Errorf("read conditions for %s: %w", depName, err)
	}
	for _, cond := range conds {
		if cond.Type == reconcile.ConditionTypeReady {
			return cond.Status == reconcile.ConditionTrue, nil
		}
	}
	return false, nil
}

// dependencyStarted reports whether depName has at least one running
// container of its own, ownsContainer's exact-match rule (not a bare
// prefix match) so a dependency name that happens to prefix-match a
// differently-owned service's containers is never mistaken for it.
func (c *Controller) dependencyStarted(ctx context.Context, depName string) (bool, error) {
	states, err := c.listPrefix(ctx, depName+"-")
	if err != nil {
		return false, fmt.Errorf("list containers for %s: %w", depName, err)
	}
	for _, s := range states {
		if s.Running && ownsContainer(depName, s.Name) {
			return true, nil
		}
	}
	return false, nil
}

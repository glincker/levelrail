package application

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

// dependencyBlock returns a not-yet-ready result when desired names a
// dependency whose own containers have not started yet, nil once every
// dependency has started (real Compose's own default depends_on:
// semantic, service_started not service_healthy). AppID unset (the
// brief window between DeploySpec's Deploy call and its own follow-up
// UpdateServiceApp, multi.go) fails this gate open, not closed.
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
			res := reconcile.Result{Conditions: []reconcile.Condition{{
				Type: "Ready", Status: reconcile.ConditionUnknown, Reason: "WaitingForDependency",
				Message: fmt.Sprintf("waiting for dependency %q (%s) to start", dep, depName),
			}}}
			return &res
		}
	}
	return nil
}

// dependencyStarted reports whether depName has at least one running
// container of its own, ownsContainer's exact-match rule (not a bare
// prefix match) so a dependency name that happens to prefix-match a
// differently-owned service's containers is never mistaken for it.
func (c *Controller) dependencyStarted(ctx context.Context, depName string) (bool, error) {
	states, err := c.runtime.ListByPrefix(ctx, depName+"-")
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

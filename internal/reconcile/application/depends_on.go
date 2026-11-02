package application

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

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
	res, dep := c.dependencyGate(ctx, desired)
	if c.dependsOnTimeout == nil {
		return res
	}
	if res == nil || len(res.Conditions) == 0 || res.Conditions[0].Status != reconcile.ConditionUnknown {
		c.clearDependencyWait(desired.Name)
		return res
	}
	limit := c.dependsOnTimeout(ctx)
	since := c.dependencyWaitStart(desired.Name)
	if limit <= 0 || c.clock().Sub(since) < limit {
		return res
	}
	msg := fmt.Sprintf("dependency %q did not become ready within %s (setting compose.depends_on_timeout)", dep, limit)
	c.traceDependencyWait(ctx, desired, dep, reasonDependencyTimeout, msg)
	return &reconcile.Result{Conditions: []reconcile.Condition{{
		Type: reconcile.ConditionTypeReady, Status: reconcile.ConditionFalse, Reason: reasonDependencyTimeout, Message: msg,
	}}}
}

// WithDependsOnTimeout sets where the depends_on wait limit comes from, read on
// every pass so a stored setting change applies live. Unset keeps waiting forever.
func WithDependsOnTimeout(get func(context.Context) time.Duration) Option {
	return func(ctrl *Controller) { ctrl.dependsOnTimeout = get }
}

func (c *Controller) dependencyWaitStart(name string) time.Time {
	c.dependsOnMu.Lock()
	defer c.dependsOnMu.Unlock()
	if c.dependsOnSince == nil {
		c.dependsOnSince = map[string]time.Time{}
	}
	if t, ok := c.dependsOnSince[name]; ok {
		return t
	}
	t := c.clock()
	c.dependsOnSince[name] = t
	return t
}

func (c *Controller) clearDependencyWait(name string) {
	c.dependsOnMu.Lock()
	delete(c.dependsOnSince, name)
	c.dependsOnMu.Unlock()
}

// dependencyGate returns the blocking result plus the name of the dependency it waits on.
func (c *Controller) dependencyGate(ctx context.Context, desired *store.DesiredService) (*reconcile.Result, string) {
	if len(desired.DependsOn) == 0 || desired.AppID == "" {
		return nil, ""
	}
	for _, dep := range desired.DependsOn {
		depName := desired.AppID + "-" + dep
		if slices.Contains(desired.DependsOnCompleted(), dep) {
			if res := c.completionBlock(ctx, desired, dep, depName); res != nil {
				return res, dep
			}
			continue
		}
		started, err := c.dependencyStarted(ctx, depName)
		if err != nil {
			res := notReady("DependencyCheckFailed", fmt.Errorf("check dependency %q: %w", dep, err))
			return &res, dep
		}
		if !started {
			res := waitingFor(dep, depName, "WaitingForDependency", "start")
			c.traceDependencyWait(ctx, desired, dep, "WaitingForDependency", res.Conditions[0].Message)
			return res, dep
		}
		if !slices.Contains(desired.DependsOnHealthy, dep) || c.conditions == nil {
			continue
		}
		ready, err := c.dependencyReady(ctx, depName)
		if err != nil {
			res := notReady("DependencyCheckFailed", fmt.Errorf("check dependency %q health: %w", dep, err))
			return &res, dep
		}
		if !ready {
			res := waitingFor(dep, depName, "WaitingForDependencyHealthy", "pass its readiness check")
			c.traceDependencyWait(ctx, desired, dep, "WaitingForDependencyHealthy", res.Conditions[0].Message)
			return res, dep
		}
	}
	return nil, ""
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
		if !ownsContainer(depName, s.Name) {
			continue
		}
		// A one-shot that already exited 0 did start; requiring Running would block forever.
		if s.Running || c.completedExit(ctx, s.Name) != nil {
			return true, nil
		}
	}
	return false, nil
}

const (
	reasonWaitingForCompletion = "WaitingForDependencyCompleted"
	reasonDependencyFailed     = "DependencyFailed"
	reasonDependencyTimeout    = "DependencyTimeout"
)

// completionBlock gates condition: service_completed_successfully: nil once a
// container of depName exited 0, a failure result when it exited nonzero.
func (c *Controller) completionBlock(ctx context.Context, desired *store.DesiredService, dep, depName string) *reconcile.Result {
	states, err := c.listPrefix(ctx, depName+"-")
	if err != nil {
		res := notReady("DependencyCheckFailed", fmt.Errorf("check dependency %q: %w", dep, err))
		return &res
	}
	current, err := c.currentTargets(ctx, depName)
	if err != nil {
		res := notReady("DependencyCheckFailed", fmt.Errorf("check dependency %q: %w", dep, err))
		return &res
	}
	var failed *readinessCrashError
	for _, s := range states {
		if !ownsContainer(depName, s.Name) || (current != nil && !current[s.Name]) {
			continue
		}
		if c.completedExit(ctx, s.Name) != nil {
			return nil
		}
		// Exit 0 that is not a completion is a created, never started container: still pending.
		if crash := c.exitedCrash(ctx, s.Name); crash != nil && failed == nil && (crash.exit.ExitCode != 0 || crash.exit.OOMKilled) {
			failed = crash
		}
	}
	if failed != nil {
		msg := fmt.Sprintf("dependency %q (%s) did not complete successfully: exit code %d", dep, depName, failed.exit.ExitCode)
		if failed.exit.OOMKilled {
			msg += " (OOM killed)"
		}
		c.traceDependencyWait(ctx, desired, dep, reasonDependencyFailed, msg)
		return &reconcile.Result{Conditions: []reconcile.Condition{{
			Type: reconcile.ConditionTypeReady, Status: reconcile.ConditionFalse, Reason: reasonDependencyFailed, Message: msg,
		}}}
	}
	res := waitingFor(dep, depName, reasonWaitingForCompletion, "run to completion")
	c.traceDependencyWait(ctx, desired, dep, reasonWaitingForCompletion, res.Conditions[0].Message)
	return res
}

// currentTargets names depName's containers for its desired release, so a
// held older release that exited 0 cannot satisfy a newer migration. nil
// (dependency not found) judges every container of depName.
func (c *Controller) currentTargets(ctx context.Context, depName string) (map[string]bool, error) {
	dep, err := c.store.GetDesiredService(ctx, depName)
	if errors.Is(err, store.ErrServiceNotFound) || (err == nil && dep == nil) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load %s: %w", depName, err)
	}
	replicas := dep.Replicas
	if replicas <= 0 {
		replicas = store.DefaultReplicas
	}
	out := make(map[string]bool, replicas)
	for i := range replicas {
		out[replicaContainerName(depName, NameImage(*dep), dep.RestartNonce, i)] = true
	}
	return out, nil
}

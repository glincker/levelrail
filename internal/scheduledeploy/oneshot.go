package scheduledeploy

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/store"
)

// OneShotStore is the store surface the one-shot runner needs.
type OneShotStore interface {
	ListDueScheduledDeploys(ctx context.Context, now, staleBefore time.Time) ([]store.ScheduledDeploy, error)
	ClaimScheduledDeploy(ctx context.Context, id string, at time.Time) error
	FinishScheduledDeploy(ctx context.Context, id, status, reason, approvalID string, at time.Time) error
}

// Outcome is what an Executor reports for a deploy it started.
type Outcome struct {
	// ApprovalID is set when the deploy became a pending approval instead of running.
	ApprovalID string
	Message    string
}

// Executor starts a due scheduled deploy through the normal deploy path.
type Executor interface {
	ExecuteScheduledDeploy(ctx context.Context, d store.ScheduledDeploy) (Outcome, error)
}

// Notifier tells operators a scheduled deploy did not run as planned.
type Notifier interface {
	NotifyScheduledDeploy(ctx context.Context, d store.ScheduledDeploy, status, reason string)
}

// OneShotRunner is the level-triggered scanner for one-shot scheduled deploys.
// Each tick re-reads the due rows, so a restart loses nothing: rows within
// Grace of run_at still run, older ones are marked missed and notified. A row
// is claimed (pending to running) before executing, so it never fires twice;
// a claim left behind by a crash is closed as failed, never retried.
type OneShotRunner struct {
	Store    OneShotStore
	Freeze   deploy.FreezeStore
	Exec     Executor
	Notify   Notifier
	Logger   *slog.Logger
	Now      func() time.Time
	Grace    time.Duration
	StaleRun time.Duration

	mu sync.Mutex
}

func (o *OneShotRunner) now() time.Time {
	if o.Now != nil {
		return o.Now()
	}
	return time.Now()
}

func (o *OneShotRunner) log() *slog.Logger {
	if o.Logger != nil {
		return o.Logger
	}
	return slog.Default()
}

// Tick processes every due scheduled deploy once.
func (o *OneShotRunner) Tick(ctx context.Context) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	now := o.now()
	due, err := o.Store.ListDueScheduledDeploys(ctx, now, now.Add(-o.StaleRun))
	if err != nil {
		return fmt.Errorf("scheduledeploy: list due: %w", err)
	}
	var errs []error
	for _, d := range due {
		if err := o.runOne(ctx, d, now); err != nil {
			errs = append(errs, fmt.Errorf("scheduled deploy %s: %w", d.ID, err))
		}
	}
	return errors.Join(errs...)
}

func (o *OneShotRunner) runOne(ctx context.Context, d store.ScheduledDeploy, now time.Time) error {
	if d.Status == store.ScheduledDeployRunning {
		return o.finish(ctx, d, store.ScheduledDeployFailed, "interrupted by a control plane restart, not retried", "", now)
	}
	if err := o.Store.ClaimScheduledDeploy(ctx, d.ID, now); err != nil {
		if errors.Is(err, store.ErrScheduledDeployNotPending) {
			return nil
		}
		return err
	}
	if o.Grace > 0 && now.Sub(d.RunAt) > o.Grace {
		reason := fmt.Sprintf("missed: run_at passed %s ago, beyond the %s grace window", now.Sub(d.RunAt).Round(time.Second), o.Grace)
		return o.finish(ctx, d, store.ScheduledDeployMissed, reason, "", now)
	}
	if o.Freeze != nil {
		status, ferr := deploy.CheckFreeze(ctx, o.Freeze, d.ServiceName, now)
		if ferr != nil {
			return o.finish(ctx, d, store.ScheduledDeployFailed, "could not check deploy freeze: "+ferr.Error(), "", now)
		}
		if status.Frozen {
			reason := fmt.Sprintf("skipped: deploy freeze active until %s", status.Until.UTC().Format(time.RFC3339))
			if status.Reason != "" {
				reason += " (" + status.Reason + ")"
			}
			return o.finish(ctx, d, store.ScheduledDeploySkippedFreeze, reason, "", now)
		}
	}
	out, err := o.Exec.ExecuteScheduledDeploy(ctx, d)
	switch {
	case err != nil:
		return o.finish(ctx, d, store.ScheduledDeployFailed, err.Error(), "", now)
	case out.ApprovalID != "":
		return o.finish(ctx, d, store.ScheduledDeployPendingApproval, "waiting for deploy approval", out.ApprovalID, now)
	default:
		return o.finish(ctx, d, store.ScheduledDeployFired, out.Message, "", now)
	}
}

func (o *OneShotRunner) finish(ctx context.Context, d store.ScheduledDeploy, status, reason, approvalID string, now time.Time) error {
	if err := o.Store.FinishScheduledDeploy(ctx, d.ID, status, reason, approvalID, now); err != nil {
		return err
	}
	if o.Notify != nil && status != store.ScheduledDeployFired && status != store.ScheduledDeployPendingApproval {
		o.Notify.NotifyScheduledDeploy(ctx, d, status, reason)
	}
	return nil
}

// Run ticks on interval until ctx is done.
func (o *OneShotRunner) Run(ctx context.Context, interval time.Duration) error {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			if err := o.Tick(ctx); err != nil {
				o.log().Warn("scheduledeploy: one-shot tick had errors", slog.String("error", err.Error()))
			}
		}
	}
}

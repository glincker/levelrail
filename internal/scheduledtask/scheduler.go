package scheduledtask

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/cronexpr"
	"github.com/GLINCKER/levelrail/internal/store"
)

// ScheduleStore is the narrow store surface Scheduler needs: every
// currently enabled scheduled task, re-derived fresh on every tick, the
// same shape internal/backup.ScheduleStore's own doc comment establishes
// for its sibling. *store.DB satisfies this structurally.
type ScheduleStore interface {
	ListEnabledScheduledTasks(ctx context.Context) ([]store.ScheduledTask, error)
}

// TaskRunner is the surface Scheduler needs to actually run a task once
// its schedule comes due. *Runner satisfies this structurally; redeclared
// here rather than depended on directly so a test can substitute a fake
// without constructing a real Runner, the same reasoning
// backup.ScheduledBackupRunner's own doc comment gives.
type TaskRunner interface {
	Run(ctx context.Context, task store.ScheduledTask) error
}

// Scheduler periodically checks which scheduled tasks have a due cron
// schedule and runs them through Runner, mirroring
// internal/backup.Scheduler's own shape field for field: same armed-
// before-running nextRun bookkeeping, same missed-schedule gap (a
// schedule that should have fired while the control plane was down
// waits for its next natural occurrence rather than catching up), same
// per-task error collection so one broken task never blocks the rest of
// a tick. See that type's own doc comment for the full reasoning; it
// applies here unchanged, just for scheduled tasks instead of scheduled
// backups.
type Scheduler struct {
	Store  ScheduleStore
	Runner TaskRunner
	Logger *slog.Logger
	// Now returns the current time; nil falls back to time.Now, the same
	// testable-clock convention backup.Scheduler.Now establishes.
	Now func() time.Time

	mu      sync.Mutex
	nextRun map[string]time.Time
}

// NewScheduler builds a Scheduler ready to Tick or Run. logger defaults
// to slog.Default() if nil, matching backup.NewScheduler's own
// convention.
func NewScheduler(store ScheduleStore, runner TaskRunner, logger *slog.Logger) *Scheduler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Scheduler{
		Store:   store,
		Runner:  runner,
		Logger:  logger,
		nextRun: make(map[string]time.Time),
	}
}

func (s *Scheduler) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Scheduler) log() *slog.Logger {
	if s.Logger != nil {
		return s.Logger
	}
	return slog.Default()
}

// Tick evaluates every currently enabled scheduled task once. Errors
// from an individual task (an invalid cron string, Runner.Run itself
// failing) are collected and joined, never stopping evaluation of the
// remaining tasks, the same "one broken resource must not block the
// rest" principle backup.Scheduler.Tick's own doc comment traces back to
// alerting.Engine.Tick.
func (s *Scheduler) Tick(ctx context.Context) error {
	tasks, err := s.Store.ListEnabledScheduledTasks(ctx)
	if err != nil {
		return fmt.Errorf("scheduledtask: scheduler: list enabled tasks: %w", err)
	}

	now := s.now()
	seen := make(map[string]struct{}, len(tasks))
	var errs []error

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, task := range tasks {
		seen[task.ID] = struct{}{}

		sched, perr := cronexpr.Parse(task.Schedule)
		if perr != nil {
			s.log().Warn("scheduledtask: scheduler: invalid schedule, skipping",
				slog.String("task_id", task.ID), slog.String("schedule", task.Schedule), slog.String("error", perr.Error()))
			continue
		}

		next, known := s.nextRun[task.ID]
		if !known {
			// First tick this task has been observed as enabled: arm its
			// next fire time without running immediately, matching
			// backup.Scheduler.Tick's own reasoning for why a freshly
			// armed schedule never fires on sight.
			s.nextRun[task.ID] = sched.Next(now)
			continue
		}
		if now.Before(next) {
			continue
		}

		// Armed before running, not after: if Runner.Run itself panics or
		// this process is killed mid-run, the next tick still moves
		// forward to the schedule's next real occurrence instead of
		// retrying immediately against, most likely, the same failure.
		s.nextRun[task.ID] = sched.Next(now)

		if rerr := s.Runner.Run(ctx, task); rerr != nil {
			errs = append(errs, fmt.Errorf("task %q: %w", task.ID, rerr))
		}
	}

	// Forget any task no longer in the enabled set (disabled or deleted):
	// otherwise nextRun would grow forever and a task re-enabled later
	// under the same ID would incorrectly inherit a stale armed time from
	// before it was disabled.
	for id := range s.nextRun {
		if _, ok := seen[id]; !ok {
			delete(s.nextRun, id)
		}
	}

	return errors.Join(errs...)
}

// Run calls Tick on interval until ctx is done, matching the shape of
// every other periodic loop in this codebase (backup.Scheduler.Run,
// alerting.Engine.Run, telemetry.Collector.Run).
func (s *Scheduler) Run(ctx context.Context, interval time.Duration) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := s.Tick(ctx); err != nil {
				s.log().Warn("scheduledtask: scheduler tick had errors", slog.String("error", err.Error()))
			}
		}
	}
}

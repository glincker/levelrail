package scheduledtask

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/cronexpr"
	"github.com/GLINCKER/levelrail/internal/store"
)

// ScheduleStore is the narrow store surface Scheduler needs: every
// currently enabled scheduled task, re-derived fresh on every tick,
// mirroring internal/backup.ScheduleStore's identical role.
type ScheduleStore interface {
	ListEnabledScheduledTasks(ctx context.Context) ([]store.ScheduledTask, error)
}

// TaskRunner is the surface Scheduler needs to actually run a task once
// its schedule comes due. *Runner satisfies this structurally.
type TaskRunner interface {
	Run(ctx context.Context, runID string, task store.ScheduledTask) error
}

// Scheduler periodically checks which scheduled tasks are due and runs
// them through the same Runner the manual "run now" trigger
// (internal/api) uses, the scheduled-task counterpart of
// internal/backup.Scheduler. See that type's own doc comment for the
// overall shape (Tick's evaluate-act-persist loop, one broken task never
// blocking the rest) and for the identical, deliberate missed-schedule
// gap this also leaves open: a task due while the control plane was down
// does not catch up on restart, it simply waits for its next natural
// occurrence.
type Scheduler struct {
	Store  ScheduleStore
	Runner TaskRunner
	Logger *slog.Logger
	// Now returns the current time; nil in production (falls back to
	// time.Now), overridden in tests for deterministic schedule matching.
	Now func() time.Time

	mu      sync.Mutex
	nextRun map[string]time.Time
}

// NewScheduler builds a Scheduler ready to Tick or Run. logger defaults
// to slog.Default() if nil.
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
// remaining tasks.
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

	for _, t := range tasks {
		seen[t.ID] = struct{}{}

		sched, perr := cronexpr.Parse(t.Schedule)
		if perr != nil {
			// A schedule string that fails to parse here passed no
			// validation at write time (internal/api validates via
			// cronexpr.Parse before saving), so this should only happen if
			// a row was written some other way; logged and skipped rather
			// than treated as a Tick-ending failure.
			s.log().Warn("scheduledtask: scheduler: invalid schedule, skipping",
				slog.String("task_id", t.ID), slog.String("schedule", t.Schedule), slog.String("error", perr.Error()))
			continue
		}

		next, known := s.nextRun[t.ID]
		if !known {
			// First tick this task has been observed as enabled: arm its
			// next fire time without running immediately, whether that's
			// because the task was just created or because the control
			// plane just started up.
			s.nextRun[t.ID] = sched.Next(now)
			continue
		}
		if now.Before(next) {
			continue
		}

		// Armed before running, not after: if Run itself panics or this
		// process is killed mid-run, the next tick still moves forward to
		// the schedule's next real occurrence instead of retrying
		// immediately against, most likely, the same failure.
		s.nextRun[t.ID] = sched.Next(now)

		if rerr := s.runScheduled(ctx, t); rerr != nil {
			errs = append(errs, fmt.Errorf("task %q: %w", t.ID, rerr))
		}
	}

	// Forget any task no longer in the enabled set (disabled, deleted, or
	// its schedule cleared): otherwise nextRun would grow forever and a
	// task re-enabled later would incorrectly inherit a stale armed time
	// from before it was disabled.
	for id := range s.nextRun {
		if _, ok := seen[id]; !ok {
			delete(s.nextRun, id)
		}
	}

	return errors.Join(errs...)
}

func (s *Scheduler) runScheduled(ctx context.Context, t store.ScheduledTask) error {
	runID, err := randomScheduledTaskRunID()
	if err != nil {
		return fmt.Errorf("generate run id: %w", err)
	}

	if err := s.Runner.Run(ctx, runID, t); err != nil {
		s.log().Error("scheduledtask: scheduled run failed",
			slog.String("task_id", t.ID), slog.String("run_id", runID), slog.String("error", err.Error()))
		return err
	}
	return nil
}

// Run calls Tick on interval until ctx is done, the same select-on-
// ticker-or-ctx-Done shape every periodic loop in this codebase
// (alerting.Engine.Run, backup.Scheduler.Run, telemetry.Collector.Run)
// already uses.
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

// randomScheduledTaskRunID mirrors randomBackupHistoryID's exact shape
// (internal/api/backups.go): 9 random bytes, URL-safe base64, a "sctr_"
// prefix so a scheduled task run ID is never visually confusable with a
// backup history ID.
func randomScheduledTaskRunID() (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("scheduledtask: generate run id: %w", err)
	}
	return "sctr_" + base64.RawURLEncoding.EncodeToString(buf), nil
}

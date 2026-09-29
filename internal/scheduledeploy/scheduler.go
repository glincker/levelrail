// Package scheduledeploy checks, on an interval, which apps have a due
// cron schedule (store.AppSchedule, migrations/0250_app_schedules.sql)
// and redeploys the latest commit on their configured branch.
package scheduledeploy

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/cronexpr"
	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/store"
)

// ScheduleStore is the narrow store surface Scheduler needs, re-derived
// fresh every tick, the same shape internal/scheduledtask.ScheduleStore's
// own doc comment establishes for its sibling. *store.DB satisfies this
// structurally.
type ScheduleStore interface {
	ListEnabledAppSchedules(ctx context.Context) ([]store.AppSchedule, error)
	ArmAppScheduleNextRun(ctx context.Context, serviceName string, next time.Time) error
	RecordAppScheduleHistory(ctx context.Context, e store.AppScheduleHistoryEntry) error
}

// Trigger fires a scheduled redeploy of serviceName's latest commit on
// branch, returning a short human-readable result recorded as the history
// entry's reason. *api.Router (TriggerScheduledDeploy) satisfies this;
// redeclared here rather than depended on directly, the same
// "substitutable without a real Router" reasoning
// internal/scheduledtask.TaskRunner's own doc comment gives for its
// sibling.
type Trigger interface {
	TriggerScheduledDeploy(ctx context.Context, serviceName, branch string) (result string, err error)
}

// Scheduler periodically checks which app schedules are due and fires
// them through Trigger.
//
// Catch-up semantics: every enabled schedule persists its own next fire
// time (store.AppSchedule.NextFireAt) rather than keeping it only in an
// in-memory position. A schedule seen for the first time (NextFireAt
// nil) is armed to its next real occurrence and does not fire
// immediately, the same "no fire on sight" rule internal/scheduledtask.
// Scheduler and internal/backup.Scheduler already apply to a freshly
// observed schedule. Once armed, Tick re-arms NextFireAt to the next
// occurrence strictly after now *before* calling Trigger: if the process
// is killed between that arm and Trigger returning, the missed firing is
// not retried on the next tick, and it is never fired twice either.
//
// Because the armed time is persisted rather than kept in memory, a
// schedule that missed one or more occurrences entirely while the
// control plane was down still fires exactly once the next time it is
// checked, for the most recent due occurrence, never once per missed
// occurrence, then resumes its normal cadence from now. This is a
// deliberate difference from its sibling schedulers
// (internal/scheduledtask, internal/backup), whose in-memory position is
// lost on restart and which therefore never catch up a missed occurrence
// at all: a missed scheduled deploy is worth catching up once, a missed
// backup or task run is not, matching how this codebase already treats
// each.
//
// A schedule due during an active deploy freeze window is skipped, not
// forced: it is recorded in history with a reason and re-armed for its
// next natural occurrence, never queued to run once the freeze lifts.
type Scheduler struct {
	Store   ScheduleStore
	Freeze  deploy.FreezeStore
	Trigger Trigger
	Logger  *slog.Logger
	// Now returns the current time; nil falls back to time.Now, the same
	// testable-clock convention every other scheduler in this codebase
	// establishes.
	Now func() time.Time

	mu sync.Mutex
}

// NewScheduler builds a Scheduler ready to Tick or Run. logger defaults
// to slog.Default() if nil.
func NewScheduler(store ScheduleStore, freeze deploy.FreezeStore, trigger Trigger, logger *slog.Logger) *Scheduler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Scheduler{Store: store, Freeze: freeze, Trigger: trigger, Logger: logger}
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

// Tick evaluates every currently enabled app schedule once. One
// schedule's error (an invalid cron string, Trigger itself failing) is
// collected and joined, never stopping evaluation of the rest, the same
// "one broken resource must not block the rest" principle every other
// scheduler in this codebase already follows.
func (s *Scheduler) Tick(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	schedules, err := s.Store.ListEnabledAppSchedules(ctx)
	if err != nil {
		return fmt.Errorf("scheduledeploy: scheduler: list enabled schedules: %w", err)
	}

	now := s.now()
	var errs []error
	for _, sch := range schedules {
		if err := s.tickOne(ctx, sch, now); err != nil {
			errs = append(errs, fmt.Errorf("app %q: %w", sch.ServiceName, err))
		}
	}
	return errors.Join(errs...)
}

func (s *Scheduler) tickOne(ctx context.Context, sch store.AppSchedule, now time.Time) error {
	sched, perr := cronexpr.Parse(sch.Cron)
	if perr != nil {
		s.log().Warn("scheduledeploy: scheduler: invalid schedule, skipping",
			slog.String("app", sch.ServiceName), slog.String("cron", sch.Cron), slog.String("error", perr.Error()))
		return nil
	}
	loc, lerr := time.LoadLocation(sch.Timezone)
	if lerr != nil {
		s.log().Warn("scheduledeploy: scheduler: invalid timezone, using UTC",
			slog.String("app", sch.ServiceName), slog.String("timezone", sch.Timezone), slog.String("error", lerr.Error()))
		loc = time.UTC
	}

	if sch.NextFireAt == nil {
		next := cronexpr.NextInLocation(sched, now, loc)
		if err := s.Store.ArmAppScheduleNextRun(ctx, sch.ServiceName, next); err != nil {
			return fmt.Errorf("arm first run: %w", err)
		}
		return nil
	}
	if now.Before(*sch.NextFireAt) {
		return nil
	}

	due := *sch.NextFireAt
	next := cronexpr.NextInLocation(sched, now, loc)
	if err := s.Store.ArmAppScheduleNextRun(ctx, sch.ServiceName, next); err != nil {
		return fmt.Errorf("re-arm next run: %w", err)
	}

	if s.Freeze != nil {
		status, ferr := deploy.CheckFreeze(ctx, s.Freeze, sch.ServiceName, now)
		if ferr != nil {
			s.log().Warn("scheduledeploy: scheduler: check freeze failed, deploying anyway",
				slog.String("app", sch.ServiceName), slog.String("error", ferr.Error()))
		} else if status.Frozen {
			reason := fmt.Sprintf("skipped: deploy freeze active until %s", status.Until.UTC().Format(time.RFC3339))
			if status.Reason != "" {
				reason += " (" + status.Reason + ")"
			}
			return s.recordHistory(ctx, sch.ServiceName, due, now, store.AppScheduleHistorySkippedFreeze, reason)
		}
	}

	result, terr := s.Trigger.TriggerScheduledDeploy(ctx, sch.ServiceName, sch.Branch)
	status := store.AppScheduleHistoryFired
	reason := result
	if terr != nil {
		status = store.AppScheduleHistoryFailed
		reason = terr.Error()
	}
	if herr := s.recordHistory(ctx, sch.ServiceName, due, now, status, reason); herr != nil {
		return herr
	}
	return terr
}

func (s *Scheduler) recordHistory(ctx context.Context, serviceName string, due, firedAt time.Time, status, reason string) error {
	if err := s.Store.RecordAppScheduleHistory(ctx, store.AppScheduleHistoryEntry{
		ServiceName: serviceName, ScheduledFor: due, FiredAt: firedAt, Status: status, Reason: reason,
	}); err != nil {
		return fmt.Errorf("record history: %w", err)
	}
	return nil
}

// Run calls Tick on interval until ctx is done, matching the shape of
// every other periodic loop in this codebase (backup.Scheduler.Run,
// scheduledtask.Scheduler.Run, alerting.Engine.Run).
func (s *Scheduler) Run(ctx context.Context, interval time.Duration) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := s.Tick(ctx); err != nil {
				s.log().Warn("scheduledeploy: scheduler tick had errors", slog.String("error", err.Error()))
			}
		}
	}
}

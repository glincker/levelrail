package scheduledtask

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/footprint"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/kit/cronexpr"
)

// maxScanOccurrences bounds how many cron occurrences one evaluation walks;
// past it the scan restarts close to now, since only the latest occurrence
// and an approximate missed count matter.
const maxScanOccurrences = 2000

// defaultGrace is how late an occurrence may be when Grace is unset.
const defaultGrace = 2 * time.Minute

// scanRestartWindow is how far back a truncated scan restarts from now.
const scanRestartWindow = 2 * time.Hour

// ScheduleStore is the store surface Scheduler needs. *store.DB satisfies
// it structurally.
type ScheduleStore interface {
	ListEnabledScheduledTasks(ctx context.Context) ([]store.ScheduledTask, error)
	ClaimScheduledTaskRun(ctx context.Context, run store.ScheduledTaskRun) (bool, error)
	InterruptRunningScheduledTaskRuns(ctx context.Context, now time.Time) ([]store.ScheduledTaskRun, error)
}

// TaskRunner runs one claimed occurrence. *Runner satisfies it; a test
// substitutes a fake.
type TaskRunner interface {
	RunTrigger(ctx context.Context, task store.ScheduledTask, trig Trigger) error
}

// Recoverer cleans up what a previous process left behind. *Runner
// satisfies it; optional.
type Recoverer interface {
	Cleanup(ctx context.Context, runs []store.ScheduledTaskRun)
}

// Scheduler is level-triggered: every tick it derives, from the persisted
// high-water mark (store.ScheduledTask.LastScheduledFor) and the current
// time, which cron occurrence is due, claims it atomically in the database
// and only then runs it. Because the claim is persisted before execution,
// a restart can neither run an occurrence twice nor lose track of one: an
// occurrence found overdue is run once (catch_up run_once, inside the
// lookback), or recorded as missed (skip), never silently dropped.
type Scheduler struct {
	Store  ScheduleStore
	Runner TaskRunner
	Logger *slog.Logger
	// Now returns the current time; nil falls back to time.Now.
	Now func() time.Time
	// Grace is how late an occurrence may be and still count as on time;
	// zero means twice the interval passed to Run.
	Grace time.Duration
	// Lookback is the default catch-up window when a job sets none.
	Lookback time.Duration
	// NewID mints run IDs; nil uses NewRunID.
	NewID func() (string, error)

	wg sync.WaitGroup
	mu sync.Mutex
	// locations caches time.LoadLocation results.
	locations map[string]*time.Location
	baseCtx   context.Context
}

// NewScheduler builds a Scheduler. logger defaults to slog.Default().
func NewScheduler(st ScheduleStore, runner TaskRunner, logger *slog.Logger) *Scheduler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Scheduler{Store: st, Runner: runner, Logger: logger, locations: make(map[string]*time.Location)}
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

func (s *Scheduler) newID() (string, error) {
	if s.NewID != nil {
		return s.NewID()
	}
	return NewRunID()
}

func (s *Scheduler) location(name string) (*time.Location, error) {
	if name == "" {
		name = "UTC"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if loc, ok := s.locations[name]; ok {
		return loc, nil
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, fmt.Errorf("timezone %q: %w", name, err)
	}
	if s.locations == nil {
		s.locations = make(map[string]*time.Location)
	}
	s.locations[name] = loc
	return loc, nil
}

// Wait blocks until every run started by Tick has returned. Tests use it;
// production never needs to.
func (s *Scheduler) Wait() { s.wg.Wait() }

// Recover marks runs a previous process left in flight as interrupted and
// removes their containers. They are never re-run: re-running an occurrence
// whose first execution may have done real work is how double runs happen.
func (s *Scheduler) Recover(ctx context.Context, rec Recoverer) error {
	runs, err := s.Store.InterruptRunningScheduledTaskRuns(ctx, s.now())
	if err != nil {
		return fmt.Errorf("scheduledtask: recover interrupted runs: %w", err)
	}
	for _, r := range runs {
		s.log().Warn("scheduledtask: run interrupted by restart",
			slog.String("run_id", r.ID), slog.String("task_id", r.TaskID), slog.String("service", r.ServiceName))
	}
	if rec != nil && len(runs) > 0 {
		rec.Cleanup(ctx, runs)
	}
	return nil
}

// Occurrences returns the cron occurrences in (after, upTo], evaluated on
// loc's wall clock, oldest first. A wall time skipped by a DST jump runs
// once at the shifted time; one repeated by a fall-back runs once.
func Occurrences(sched *cronexpr.Schedule, loc *time.Location, after, upTo time.Time, limit int) (out []time.Time, truncated bool) {
	t := after
	for {
		next := cronexpr.NextInLocation(sched, t, loc)
		if next.IsZero() || next.After(upTo) {
			return out, false
		}
		if !next.After(t) {
			return out, false
		}
		out = append(out, next)
		if len(out) >= limit {
			return out, true
		}
		t = next
	}
}

// due is the decision for one task at one instant.
type due struct {
	latest  time.Time
	count   int
	run     bool
	catchUp bool
}

// decide derives what, if anything, is owed for task at now.
func (s *Scheduler) decide(task store.ScheduledTask, now time.Time) (due, error) {
	sched, err := cronexpr.Parse(task.Schedule)
	if err != nil {
		return due{}, fmt.Errorf("schedule %q: %w", task.Schedule, err)
	}
	loc, err := s.location(task.Timezone)
	if err != nil {
		return due{}, err
	}
	anchor := task.CreatedAt
	if task.ArmedAt != nil {
		anchor = *task.ArmedAt
	}
	if task.LastScheduledFor != nil && task.LastScheduledFor.After(anchor) {
		anchor = *task.LastScheduledFor
	}
	occ, truncated := Occurrences(sched, loc, anchor, now, maxScanOccurrences)
	if truncated {
		occ, _ = Occurrences(sched, loc, now.Add(-scanRestartWindow), now, maxScanOccurrences)
	}
	if len(occ) == 0 {
		return due{}, nil
	}
	d := due{latest: occ[len(occ)-1], count: len(occ)}
	lateness := now.Sub(d.latest)
	lookback := s.Lookback
	if lookback <= 0 {
		lookback = defaultCatchUpLookback
	}
	if task.CatchUpLookbackSeconds > 0 {
		lookback = time.Duration(task.CatchUpLookbackSeconds) * time.Second
	}
	grace := s.Grace
	if grace <= 0 {
		grace = defaultGrace
	}
	switch {
	case lateness <= grace:
		d.run = true
	case task.CatchUp == store.ScheduledTaskCatchUpRunOnce && lateness <= lookback:
		d.run, d.catchUp = true, true
	}
	return d, nil
}

// Tick evaluates every enabled task once. A broken task never blocks the
// rest; errors are joined.
func (s *Scheduler) Tick(ctx context.Context) error {
	tasks, err := s.Store.ListEnabledScheduledTasks(ctx)
	if err != nil {
		return fmt.Errorf("scheduledtask: scheduler: list enabled tasks: %w", err)
	}
	now := s.now()
	var errs []error
	for _, task := range tasks {
		if err := s.evaluate(ctx, task, now); err != nil {
			errs = append(errs, fmt.Errorf("task %q: %w", task.ID, err))
		}
	}
	return errors.Join(errs...)
}

func (s *Scheduler) evaluate(ctx context.Context, task store.ScheduledTask, now time.Time) error {
	d, err := s.decide(task, now)
	if err != nil {
		s.log().Warn("scheduledtask: scheduler: skipping task with invalid schedule",
			slog.String("task_id", task.ID), slog.String("error", err.Error()))
		return nil
	}
	if d.count == 0 {
		return nil
	}
	id, err := s.newID()
	if err != nil {
		return err
	}
	scheduledFor := d.latest
	run := store.ScheduledTaskRun{
		ID: id, TaskID: task.ID, ServiceName: task.ServiceName, Trigger: store.ScheduledTaskTriggerSchedule,
		ScheduledFor: &scheduledFor, StartedAt: now, Mode: modeOf(task),
	}
	if !d.run {
		fin := now
		run.Status, run.FinishedAt = store.ScheduledTaskStatusMissed, &fin
		run.Message = fmt.Sprintf("%d scheduled occurrence(s) were not run; the control plane was not running and catch_up is %q or past its lookback",
			d.count, orDefaultPolicy(task.CatchUp))
		if _, err := s.Store.ClaimScheduledTaskRun(ctx, run); err != nil {
			return err
		}
		s.log().Warn("scheduledtask: occurrence missed",
			slog.String("task_id", task.ID), slog.String("service", task.ServiceName), slog.Time("scheduled_for", scheduledFor))
		return nil
	}

	run.Status = store.ScheduledTaskStatusRunning
	claimed, err := s.Store.ClaimScheduledTaskRun(ctx, run)
	if err != nil {
		return err
	}
	if !claimed {
		return nil
	}
	trig := Trigger{Kind: store.ScheduledTaskTriggerSchedule, RunID: id, ScheduledFor: &scheduledFor, CatchUp: d.catchUp}
	runCtx := ctx
	if s.baseCtx != nil {
		runCtx = s.baseCtx
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		if err := s.Runner.RunTrigger(runCtx, task, trig); err != nil {
			s.log().Warn("scheduledtask: run failed", slog.String("task_id", task.ID),
				slog.String("run_id", id), slog.String("error", err.Error()))
		}
	}()
	return nil
}

func orDefaultPolicy(p string) string {
	if p == "" {
		return store.ScheduledTaskCatchUpSkip
	}
	return p
}

// Run recovers interrupted runs, ticks once immediately (so a restart
// catches up without waiting a full interval), then ticks every interval
// until ctx is done.
func (s *Scheduler) Run(ctx context.Context, interval time.Duration) error {
	if s.Grace <= 0 {
		s.Grace = 2 * interval
	}
	s.baseCtx = ctx
	var rec Recoverer
	if r, ok := s.Runner.(Recoverer); ok {
		rec = r
	}
	if err := s.Recover(ctx, rec); err != nil {
		s.log().Error("scheduledtask: recovery failed", slog.String("error", err.Error()))
	}
	if err := s.Tick(ctx); err != nil {
		s.log().Warn("scheduledtask: scheduler tick had errors", slog.String("error", err.Error()))
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	loop := footprint.Track("scheduledtask", interval)
	defer loop.Active()()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			start := time.Now()
			err := s.Tick(ctx)
			loop.Observe(start, err)
			if err != nil {
				s.log().Warn("scheduledtask: scheduler tick had errors", slog.String("error", err.Error()))
			}
		}
	}
}

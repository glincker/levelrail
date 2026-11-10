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
	"github.com/GLINCKER/levelrail/kit/ticker"
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
	if err := s.promoteQueued(ctx, tasks, now); err != nil {
		errs = append(errs, err)
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
	claimed, err := s.claim(ctx, run)
	if err != nil {
		return err
	}
	if !claimed {
		return nil
	}
	s.launch(ctx, task, Trigger{Kind: store.ScheduledTaskTriggerSchedule, RunID: id, ScheduledFor: &scheduledFor, CatchUp: d.catchUp})
	return nil
}

// GuardedClaimer is the optional store surface that enforces skip and
// queue-one in the claim itself, so separate scheduler processes agree.
type GuardedClaimer interface {
	ClaimGuardedScheduledTaskRun(ctx context.Context, run store.ScheduledTaskRun) (store.ClaimOutcome, error)
	PromoteQueuedScheduledTaskRuns(ctx context.Context, now time.Time, targets []store.QueuedTarget) ([]store.ScheduledTaskRun, error)
}

func (s *Scheduler) claim(ctx context.Context, run store.ScheduledTaskRun) (bool, error) {
	g, ok := s.Store.(GuardedClaimer)
	if !ok {
		return s.Store.ClaimScheduledTaskRun(ctx, run)
	}
	outcome, err := g.ClaimGuardedScheduledTaskRun(ctx, run)
	if outcome == store.ClaimSkipped || outcome == store.ClaimQueued {
		s.log().Info("scheduledtask: occurrence held by overlap policy",
			slog.String("task_id", run.TaskID), slog.String("outcome", string(outcome)))
	}
	return outcome == store.ClaimStarted, err
}

// promoteQueued starts queue-one runs whose previous run has finished. Only
// this tick's enabled queue-one jobs are passed, so every promoted row is launched.
func (s *Scheduler) promoteQueued(ctx context.Context, tasks []store.ScheduledTask, now time.Time) error {
	g, ok := s.Store.(GuardedClaimer)
	if !ok {
		return nil
	}
	byID := make(map[string]store.ScheduledTask, len(tasks))
	var targets []store.QueuedTarget
	for _, t := range tasks {
		if t.ConcurrencyPolicy != store.ScheduledTaskConcurrencyQueueOne {
			continue
		}
		byID[t.ID] = t
		targets = append(targets, store.QueuedTarget{TaskID: t.ID, MaxWait: queuedMaxWait(t)})
	}
	if len(targets) == 0 {
		return nil
	}
	runs, err := g.PromoteQueuedScheduledTaskRuns(ctx, now, targets)
	for _, run := range runs {
		task, found := byID[run.TaskID]
		if !found {
			continue
		}
		s.launch(ctx, task, Trigger{Kind: store.ScheduledTaskTriggerSchedule, RunID: run.ID, ScheduledFor: run.ScheduledFor})
	}
	if err != nil {
		return fmt.Errorf("scheduledtask: promote queued runs: %w", err)
	}
	return nil
}

// queuedMaxWait is how long a queued run may wait: the job's catch-up
// lookback, else the live timeout ceiling, else the scan restart window.
func queuedMaxWait(t store.ScheduledTask) time.Duration {
	if t.CatchUpLookbackSeconds > 0 {
		return time.Duration(t.CatchUpLookbackSeconds) * time.Second
	}
	if hi := guardSetting(KeyTimeoutMaxSeconds); hi > 0 {
		return time.Duration(hi) * time.Second * time.Duration(t.MaxRetries+1)
	}
	return scanRestartWindow
}

func (s *Scheduler) launch(ctx context.Context, task store.ScheduledTask, trig Trigger) {
	id := trig.RunID
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
}

func orDefaultPolicy(p string) string {
	if p == "" {
		return store.ScheduledTaskCatchUpSkip
	}
	return p
}

// untilNextBoundary returns the wait until the next multiple of interval on the
// wall clock, never zero.
func untilNextBoundary(now time.Time, interval time.Duration) time.Duration {
	next := now.Truncate(interval).Add(interval)
	return next.Sub(now)
}

// Run recovers interrupted runs, ticks once immediately (so a restart
// catches up without waiting a full interval), then ticks every interval
// until ctx is done.
func (s *Scheduler) Run(ctx context.Context, interval time.Duration) error {
	interval = ticker.Safe(interval, ticker.DefaultInterval)
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
	// Ticks land on wall-clock multiples of the interval so a minute cron
	// starts at the minute, not at the phase of the process start.
	timer := time.NewTimer(untilNextBoundary(time.Now(), interval))
	defer timer.Stop()
	loop := footprint.Track("scheduledtask", interval)
	defer loop.Active()()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			timer.Reset(untilNextBoundary(time.Now(), interval))
			start := time.Now()
			err := s.Tick(ctx)
			loop.Observe(start, err)
			if err != nil {
				s.log().Warn("scheduledtask: scheduler tick had errors", slog.String("error", err.Error()))
			}
		}
	}
}

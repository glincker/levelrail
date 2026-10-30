package scheduledtask

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/kit/backoff"
)

// One-shot job states.
const (
	OneShotRunning   = "Running"
	OneShotSucceeded = "Succeeded"
	OneShotFailed    = "Failed"
)

// OneShotLimits are the env-driven defaults and ceilings for one-shot jobs.
type OneShotLimits struct {
	BackoffLimit   int
	Timeout        time.Duration
	MaxCompletions int
	MaxParallelism int
	Keep           int
}

// OneShotLimitsFromEnv reads APP_JOB_DEFAULT_BACKOFF_LIMIT,
// APP_JOB_DEFAULT_TIMEOUT, APP_JOB_MAX_COMPLETIONS, APP_JOB_MAX_PARALLELISM
// and APP_JOB_ONESHOT_KEEP. Unparsable or non-positive values fall back.
func OneShotLimitsFromEnv(lookup func(string) (string, bool)) OneShotLimits {
	num := func(key string, def int, allowZero bool) int {
		if raw, ok := lookup(key); ok {
			if n, err := strconv.Atoi(raw); err == nil && (n > 0 || (allowZero && n == 0)) {
				return n
			}
		}
		return def
	}
	timeout := defaultRunTimeout
	if raw, ok := lookup("APP_JOB_DEFAULT_TIMEOUT"); ok {
		if d, err := time.ParseDuration(raw); err == nil && d > 0 {
			timeout = d
		}
	}
	return OneShotLimits{
		BackoffLimit:   num("APP_JOB_DEFAULT_BACKOFF_LIMIT", 3, true),
		Timeout:        timeout,
		MaxCompletions: num("APP_JOB_MAX_COMPLETIONS", 100, false),
		MaxParallelism: num("APP_JOB_MAX_PARALLELISM", 10, false),
		Keep:           num("APP_JOB_ONESHOT_KEEP", 100, false),
	}
}

// OneShotSpec describes one run-to-completion job. BackoffLimit is the
// number of failed attempts tolerated across the whole job before it is
// declared Failed, as in a Kubernetes Job.
type OneShotSpec struct {
	Completions  int
	Parallelism  int
	BackoffLimit int
	BackoffBase  time.Duration
	BackoffMax   time.Duration
}

// Validate normalizes spec against limits.
func (s *OneShotSpec) Validate(l OneShotLimits) error {
	if s.Completions <= 0 {
		s.Completions = 1
	}
	if s.Parallelism <= 0 {
		s.Parallelism = 1
	}
	if s.Parallelism > s.Completions {
		s.Parallelism = s.Completions
	}
	if s.BackoffLimit < 0 {
		return errors.New("backoff limit cannot be negative")
	}
	if l.MaxCompletions > 0 && s.Completions > l.MaxCompletions {
		return fmt.Errorf("completions %d exceeds the limit %d (APP_JOB_MAX_COMPLETIONS)", s.Completions, l.MaxCompletions)
	}
	if l.MaxParallelism > 0 && s.Parallelism > l.MaxParallelism {
		return fmt.Errorf("parallelism %d exceeds the limit %d (APP_JOB_MAX_PARALLELISM)", s.Parallelism, l.MaxParallelism)
	}
	return nil
}

// AttemptOutcome is the result of one attempt of one completion.
type AttemptOutcome struct {
	RunID  string
	Status string
}

// AttemptFunc runs one attempt of completion index (0-based). Each call is
// expected to record itself in the run history.
type AttemptFunc func(ctx context.Context, completion, attempt int) AttemptOutcome

// OneShotSnapshot is a point-in-time view of a one-shot job.
type OneShotSnapshot struct {
	State          string
	Completions    int
	Succeeded      int
	FailedAttempts int
	Active         int
	RunIDs         []string
	Message        string
	StartedAt      time.Time
	FinishedAt     *time.Time
}

// OneShot drives one job. It needs no controller: Run blocks until the job
// settles and Snapshot can be read concurrently.
type OneShot struct {
	// Sleep waits between retries; nil uses a context-aware timer.
	Sleep func(ctx context.Context, d time.Duration) error
	// Now returns the current time; nil uses time.Now.
	Now func() time.Time

	mu        sync.Mutex
	snap      OneShotSnapshot
	failures  int
	exhausted bool
}

func (o *OneShot) now() time.Time {
	if o.Now != nil {
		return o.Now()
	}
	return time.Now()
}

// Snapshot returns a copy of the current state.
func (o *OneShot) Snapshot() OneShotSnapshot {
	o.mu.Lock()
	defer o.mu.Unlock()
	s := o.snap
	s.RunIDs = append([]string(nil), s.RunIDs...)
	return s
}

func (o *OneShot) update(fn func(s *OneShotSnapshot)) {
	o.mu.Lock()
	defer o.mu.Unlock()
	fn(&o.snap)
}

// Init marks the job Running so a snapshot taken before Run starts is valid.
func (o *OneShot) Init(spec OneShotSpec) {
	o.update(func(s *OneShotSnapshot) {
		*s = OneShotSnapshot{State: OneShotRunning, Completions: spec.Completions, StartedAt: o.now()}
	})
}

// Run executes spec with at most spec.Parallelism attempts in flight and
// returns the final snapshot. A failed attempt is retried after an
// exponential delay until spec.BackoffLimit failures have been spent.
func (o *OneShot) Run(ctx context.Context, spec OneShotSpec, attempt AttemptFunc) OneShotSnapshot {
	o.Init(spec)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	work := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < spec.Parallelism; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range work {
				o.runCompletion(ctx, cancel, spec, idx, attempt)
			}
		}()
	}
feed:
	for i := 0; i < spec.Completions; i++ {
		select {
		case work <- i:
		case <-ctx.Done():
			break feed
		}
	}
	close(work)
	wg.Wait()

	o.update(func(s *OneShotSnapshot) {
		end := o.now()
		s.FinishedAt = &end
		s.Active = 0
		switch {
		case s.Succeeded == spec.Completions:
			s.State = OneShotSucceeded
		case o.exhausted:
			s.State, s.Message = OneShotFailed, fmt.Sprintf("backoff limit %d exceeded", spec.BackoffLimit)
		default:
			s.State, s.Message = OneShotFailed, "cancelled before all completions finished"
		}
	})
	return o.Snapshot()
}

func (o *OneShot) runCompletion(ctx context.Context, cancel context.CancelFunc, spec OneShotSpec, idx int, attempt AttemptFunc) {
	for n := 1; ctx.Err() == nil; n++ {
		o.update(func(s *OneShotSnapshot) { s.Active++ })
		out := attempt(ctx, idx, n)
		o.update(func(s *OneShotSnapshot) {
			s.Active--
			if out.RunID != "" {
				s.RunIDs = append(s.RunIDs, out.RunID)
			}
		})
		switch out.Status {
		case store.ScheduledTaskStatusSuccess:
			o.update(func(s *OneShotSnapshot) { s.Succeeded++ })
			return
		case store.ScheduledTaskStatusSkippedConcurrency:
			// Waiting for a per-app slot is not a failure and spends no budget.
			if o.sleep(ctx, spec.BackoffBase) != nil {
				return
			}
			continue
		}
		o.update(func(s *OneShotSnapshot) { s.FailedAttempts++ })
		spent := o.spend(spec.BackoffLimit)
		if spent > spec.BackoffLimit {
			cancel()
			return
		}
		if o.sleep(ctx, backoff.Exponential(spec.BackoffBase, maxDur(spec.BackoffMax, spec.BackoffBase), spent)) != nil {
			return
		}
	}
}

// spend counts one failure against the job-wide budget and returns the
// running total, flagging the job exhausted once it exceeds limit.
func (o *OneShot) spend(limit int) int {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.failures++
	if o.failures > limit {
		o.exhausted = true
	}
	return o.failures
}

func maxDur(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}

func (o *OneShot) sleep(ctx context.Context, d time.Duration) error {
	if o.Sleep != nil {
		return o.Sleep(ctx, d)
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// RunAttempt performs one attempt of task through the same execution path
// as RunTrigger (history row, redaction, per-app slot) and returns its
// final status. A full per-app slot comes back as skipped_concurrency so a
// one-shot waits instead of failing.
func (r *Runner) RunAttempt(ctx context.Context, task store.ScheduledTask) (AttemptOutcome, error) {
	if !r.acquireAppSlot(task.ServiceName) {
		return AttemptOutcome{Status: store.ScheduledTaskStatusSkippedConcurrency}, nil
	}
	defer r.releaseAppSlot(task.ServiceName)

	trig := Trigger{Kind: store.ScheduledTaskTriggerAPI}
	started := r.now()
	runID, err := r.openRun(ctx, task, trig, started)
	if err != nil {
		return AttemptOutcome{}, err
	}
	res := r.execute(ctx, task, runID)
	err = r.closeRun(ctx, task, trig, runID, started, res)
	return AttemptOutcome{RunID: runID, Status: res.status}, err
}

// OneShotRegistry holds the live and recently finished one-shot jobs of
// this process, bounded by keep. State is in memory: the per-attempt runs
// stay in the run history, the job summary does not survive a restart.
type OneShotRegistry struct {
	mu    sync.Mutex
	keep  int
	order []string
	jobs  map[string]*OneShotEntry
}

// OneShotEntry is one registered job.
type OneShotEntry struct {
	ID     string
	App    string
	TaskID string
	Spec   OneShotSpec
	Job    *OneShot
}

// NewOneShotRegistry returns a registry keeping at most keep finished jobs.
func NewOneShotRegistry(keep int) *OneShotRegistry {
	return &OneShotRegistry{keep: keep, jobs: make(map[string]*OneShotEntry)}
}

// Add registers e and evicts the oldest finished jobs beyond keep.
func (g *OneShotRegistry) Add(e *OneShotEntry) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.jobs[e.ID] = e
	g.order = append(g.order, e.ID)
	for len(g.order) > g.keep {
		oldest := g.order[0]
		if g.jobs[oldest].Job.Snapshot().State == OneShotRunning {
			break
		}
		delete(g.jobs, oldest)
		g.order = g.order[1:]
	}
}

// Get returns the job with id.
func (g *OneShotRegistry) Get(id string) (*OneShotEntry, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	e, ok := g.jobs[id]
	return e, ok
}

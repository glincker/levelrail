package pipeline

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

var errRunCancelled = errors.New("run cancelled")

const reasonTimeout = "run timed out"

// Engine drives pipeline runs from persisted state. Every Tick re-derives
// what to do from the database, so a restart resumes in-flight runs: jobs
// recorded as running with no live goroutine are picked up again.
type Engine struct {
	cfg Config

	base       context.Context
	baseCancel context.CancelFunc
	wg         sync.WaitGroup
	nudge      chan struct{}

	mu     sync.Mutex
	active map[string]context.CancelCauseFunc
	ticks  int
}

// New builds an Engine. Call Run to drive it, or Tick from tests.
func New(cfg Config) *Engine {
	cfg.applyDefaults()
	base, cancel := context.WithCancel(context.Background())
	return &Engine{cfg: cfg, base: base, baseCancel: cancel, nudge: make(chan struct{}, 1), active: map[string]context.CancelCauseFunc{}}
}

// Nudge asks the Run loop to tick soon.
func (e *Engine) Nudge() {
	select {
	case e.nudge <- struct{}{}:
	default:
	}
}

// Run ticks every interval (or sooner after a Nudge) until ctx is done, then
// interrupts running jobs. Interrupted jobs stay recorded as running and are
// resumed by the next process.
func (e *Engine) Run(ctx context.Context, interval time.Duration) error {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			e.Close()
			return ctx.Err()
		case <-t.C:
		case <-e.nudge:
		}
		if err := e.Tick(ctx); err != nil {
			e.cfg.Logger.Warn("pipeline: tick had errors", slog.String("error", err.Error()))
		}
	}
}

// Close interrupts running jobs and waits for them to stop.
func (e *Engine) Close() {
	e.baseCancel()
	e.wg.Wait()
}

// Wait blocks until no job goroutines are running (for tests).
func (e *Engine) Wait() { e.wg.Wait() }

// Tick advances every active run once. Errors from one run never stop the rest.
func (e *Engine) Tick(ctx context.Context) error {
	runs, err := e.cfg.Store.ListActivePipelineRuns(ctx)
	if err != nil {
		return fmt.Errorf("pipeline: list active runs: %w", err)
	}
	var errs []error
	for _, r := range runs {
		if err := e.advance(ctx, r); err != nil {
			errs = append(errs, fmt.Errorf("run %s: %w", r.ID, err))
		}
	}
	e.ticks++
	if e.ticks%60 == 0 {
		if _, err := e.cfg.Store.PrunePipelineRuns(ctx, e.cfg.KeepRuns); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (e *Engine) advance(ctx context.Context, run store.PipelineRun) error {
	st := e.cfg.Store
	now := e.cfg.Now()
	def, err := Parse([]byte(run.Definition))
	if err != nil {
		return e.finishRun(ctx, run, store.PipelineStatusFailed, "invalid definition: "+err.Error())
	}

	if run.Status == store.PipelineStatusQueued {
		if run.CancelRequested {
			return e.finishRun(ctx, run, store.PipelineStatusCancelled, "cancelled before start")
		}
		ok, reason, err := e.admit(ctx, run)
		if err != nil {
			return err
		}
		if !ok {
			if run.Reason != reason {
				return st.SetPipelineRunStatus(ctx, run.ID, store.PipelineStatusQueued, reason, nil, nil)
			}
			return nil
		}
		rows, err := e.materialize(run, def)
		if err != nil {
			return e.finishRun(ctx, run, store.PipelineStatusFailed, err.Error())
		}
		if err := st.CreatePipelineJobs(ctx, rows); err != nil {
			return err
		}
		if err := st.SetPipelineRunStatus(ctx, run.ID, store.PipelineStatusRunning, "running", &now, nil); err != nil {
			return err
		}
		run.Status = store.PipelineStatusRunning
		run.StartedAt = &now
	}

	if run.CancelInProgress && run.ConcurrencyGroup != "" {
		e.cancelOlderInGroup(ctx, run)
	}
	if !run.CancelRequested && def.Timeout > 0 && run.StartedAt != nil && now.Sub(*run.StartedAt) > time.Duration(def.Timeout) {
		if err := st.SetPipelineRunStatus(ctx, run.ID, store.PipelineStatusRunning, reasonTimeout, nil, nil); err != nil {
			return err
		}
		if err := st.RequestPipelineRunCancel(ctx, run.ID); err != nil {
			return err
		}
		run.CancelRequested = true
		run.Reason = reasonTimeout
	}

	jobs, err := st.ListPipelineJobs(ctx, run.ID)
	if err != nil {
		return err
	}
	if run.CancelRequested {
		e.cancelJobs(ctx, run, jobs)
	} else if err := e.schedule(ctx, run, def, jobs); err != nil {
		return err
	}

	jobs, err = st.ListPipelineJobs(ctx, run.ID)
	if err != nil {
		return err
	}
	return e.maybeFinish(ctx, run, def, jobs)
}

// admit applies the run's concurrency group: a run starts only when no
// older run in its group is active.
func (e *Engine) admit(ctx context.Context, run store.PipelineRun) (bool, string, error) {
	if run.ConcurrencyGroup == "" {
		return true, "", nil
	}
	group, err := e.cfg.Store.ListPipelineRunsInGroup(ctx, run.ConcurrencyGroup)
	if err != nil {
		return false, "", err
	}
	if len(group) == 0 || group[0].ID == run.ID {
		return true, "", nil
	}
	ahead := group[0]
	if run.CancelInProgress {
		e.cancelRun(ctx, ahead)
		return false, fmt.Sprintf("waiting for cancelled run #%d to finish", ahead.Number), nil
	}
	return false, fmt.Sprintf("queued behind run #%d in concurrency group %q", ahead.Number, run.ConcurrencyGroup), nil
}

func (e *Engine) cancelRun(ctx context.Context, r store.PipelineRun) {
	if r.CancelRequested {
		return
	}
	if err := e.cfg.Store.RequestPipelineRunCancel(ctx, r.ID); err != nil {
		e.cfg.Logger.Warn("pipeline: cancel run failed", slog.String("run_id", r.ID), slog.String("error", err.Error()))
	}
}

func (e *Engine) cancelOlderInGroup(ctx context.Context, run store.PipelineRun) {
	group, err := e.cfg.Store.ListPipelineRunsInGroup(ctx, run.ConcurrencyGroup)
	if err != nil {
		return
	}
	for _, g := range group {
		if g.ID == run.ID {
			return
		}
		e.cancelRun(ctx, g)
	}
}

func (e *Engine) finishRun(ctx context.Context, run store.PipelineRun, status, reason string) error {
	now := e.cfg.Now()
	e.cfg.Logger.Info("pipeline: run finished", slog.String("run_id", run.ID), slog.String("status", status), slog.String("reason", reason))
	return e.cfg.Store.SetPipelineRunStatus(ctx, run.ID, status, reason, nil, &now)
}

func (e *Engine) maybeFinish(ctx context.Context, run store.PipelineRun, def *Definition, jobs []store.PipelineJob) error {
	if len(jobs) == 0 {
		return nil
	}
	for _, j := range jobs {
		if !store.IsPipelineTerminal(j.Status) {
			return nil
		}
	}
	e.cleanupRun(ctx, run, jobs)
	if run.CancelRequested {
		if run.Reason == reasonTimeout {
			return e.finishRun(ctx, run, store.PipelineStatusFailed, "run timed out after "+time.Duration(def.Timeout).String())
		}
		return e.finishRun(ctx, run, store.PipelineStatusCancelled, "cancelled")
	}
	for _, j := range jobs {
		if j.Status == store.PipelineStatusFailed {
			if jd := def.Jobs[baseJobName(j.Key)]; jd != nil && jd.ContinueOnError {
				continue
			}
			return e.finishRun(ctx, run, store.PipelineStatusFailed, fmt.Sprintf("job %s failed: %s", j.Key, j.Reason))
		}
	}
	for _, j := range jobs {
		if j.Status == store.PipelineStatusCancelled {
			return e.finishRun(ctx, run, store.PipelineStatusCancelled, fmt.Sprintf("job %s cancelled: %s", j.Key, j.Reason))
		}
	}
	return e.finishRun(ctx, run, store.PipelineStatusSucceeded, "all jobs succeeded")
}

// cancelJobs stops a cancelled run's work: live goroutines are interrupted
// (they record their own cancelled status), orphaned running jobs and
// unstarted jobs are marked directly.
func (e *Engine) cancelJobs(ctx context.Context, run store.PipelineRun, jobs []store.PipelineJob) {
	now := e.cfg.Now()
	for _, j := range jobs {
		if store.IsPipelineTerminal(j.Status) {
			continue
		}
		e.mu.Lock()
		cancel, live := e.active[j.ID]
		e.mu.Unlock()
		if live {
			cancel(errRunCancelled)
			continue
		}
		if j.Status == store.PipelineStatusRunning || j.Status == store.PipelineStatusWaitingApproval {
			e.removeJobContainers(ctx, run, j)
		}
		if err := e.cfg.Store.SetPipelineJobStatus(ctx, j.ID, store.PipelineStatusCancelled, "run cancelled", "", j.Attempt, nil, &now); err != nil {
			e.cfg.Logger.Warn("pipeline: mark job cancelled failed", slog.String("job_id", j.ID), slog.String("error", err.Error()))
		}
	}
}

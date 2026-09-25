package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/cronexpr"
	"github.com/GLINCKER/levelrail/internal/store"
)

// StartOptions describes one run to start.
type StartOptions struct {
	Trigger string
	Actor   string
	Ref     string
	SHA     string
	Inputs  map[string]string
	// BaseBranch is a pull request's target branch, which trigger filters
	// match against instead of the source branch in Ref.
	BaseBranch string
	// Hold, when set, creates the run held for approval with this reason:
	// no job starts and no secret is read until an approver releases it.
	Hold string
}

// ErrInvalidInput marks a run request the pipeline's declared inputs reject.
var ErrInvalidInput = errors.New("pipeline: invalid input")

// Start queues a run of a saved pipeline. The definition is re-validated and
// snapshotted onto the run so later edits never change it.
func (e *Engine) Start(ctx context.Context, p store.Pipeline, opt StartOptions) (store.PipelineRun, error) {
	def, issues := Validate([]byte(p.YAML))
	if len(issues) > 0 {
		return store.PipelineRun{}, fmt.Errorf("pipeline: %q is invalid: %s", p.Name, issues[0])
	}
	ev := Event{Kind: opt.Trigger, Branch: strings.TrimPrefix(opt.Ref, "refs/heads/"), Tag: strings.TrimPrefix(opt.Ref, "refs/tags/")}
	if opt.Trigger == TriggerPullRequest && opt.BaseBranch != "" {
		ev.Branch = opt.BaseBranch
	}
	if !def.On.Matches(ev) && opt.Trigger != TriggerSchedule {
		return store.PipelineRun{}, fmt.Errorf("pipeline: %q does not accept %s triggers", p.Name, opt.Trigger)
	}
	inputs, err := resolveInputs(def, opt.Inputs)
	if err != nil {
		return store.PipelineRun{}, err
	}
	inputsJSON, _ := json.Marshal(inputs)

	run := store.PipelineRun{
		ID: e.cfg.NewID(), PipelineID: p.ID, AppName: p.AppName, TriggerKind: opt.Trigger, TriggerActor: opt.Actor,
		Ref: opt.Ref, CommitSHA: opt.SHA, InputsJSON: string(inputsJSON), Definition: p.YAML, CreatedAt: e.cfg.Now(),
	}
	if opt.Hold != "" {
		run.HoldState, run.HoldReason, run.Reason = store.HoldPending, opt.Hold, holdReason(opt.Hold)
	}
	if def.Concurrency != nil {
		group, err := Interpolate(def.Concurrency.Group, Scope{Vars: runVars(run, def)})
		if err != nil {
			return store.PipelineRun{}, fmt.Errorf("pipeline: concurrency group: %w", err)
		}
		run.ConcurrencyGroup = p.AppName + "/" + p.Name + "/" + group
		run.CancelInProgress = def.Concurrency.CancelInProgress
	}
	created, err := e.cfg.Store.CreatePipelineRun(ctx, run)
	if err != nil {
		return store.PipelineRun{}, fmt.Errorf("pipeline: start %q: %w", p.Name, err)
	}
	e.Nudge()
	return created, nil
}

func resolveInputs(def *Definition, given map[string]string) (map[string]string, error) {
	out := map[string]string{}
	var decl map[string]Input
	if def.On.Manual != nil {
		decl = def.On.Manual.Inputs
	}
	for k := range given {
		if _, ok := decl[k]; !ok {
			return nil, fmt.Errorf("%w: unknown input %q", ErrInvalidInput, k)
		}
	}
	for name, in := range decl {
		v, ok := given[name]
		if !ok {
			v = in.Default
		}
		if v == "" && in.Required {
			return nil, fmt.Errorf("%w: input %q is required", ErrInvalidInput, name)
		}
		if len(in.Options) > 0 && v != "" && !containsString(in.Options, v) {
			return nil, fmt.Errorf("%w: input %q must be one of %s", ErrInvalidInput, name, strings.Join(in.Options, ", "))
		}
		out[name] = v
	}
	return out, nil
}

func containsString(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

// Cancel flags a run for cancellation and wakes the engine.
func (e *Engine) Cancel(ctx context.Context, runID string) error {
	if err := e.cfg.Store.RequestPipelineRunCancel(ctx, runID); err != nil {
		return err
	}
	e.Nudge()
	return nil
}

// Rerun starts a new run from an earlier one's trigger and inputs, using the
// pipeline's current definition.
func (e *Engine) Rerun(ctx context.Context, runID, actor string) (store.PipelineRun, error) {
	old, err := e.cfg.Store.GetPipelineRun(ctx, runID)
	if err != nil {
		return store.PipelineRun{}, err
	}
	p, err := e.cfg.Store.GetPipeline(ctx, old.PipelineID)
	if err != nil {
		return store.PipelineRun{}, err
	}
	var inputs map[string]string
	_ = json.Unmarshal([]byte(old.InputsJSON), &inputs)
	trig := old.TriggerKind
	if trig == TriggerSchedule || trig == TriggerPush || trig == TriggerPullRequest || trig == TriggerTag {
		trig = TriggerManual
		if def, _ := Validate([]byte(p.YAML)); def != nil && def.On.Manual == nil && def.On.API {
			trig = TriggerAPI
		}
	}
	return e.Start(ctx, p, StartOptions{Trigger: trig, Actor: actor, Ref: old.Ref, SHA: old.CommitSHA, Inputs: inputs})
}

// Scheduler starts pipelines whose `on.schedule` cron expressions come due.
// It mirrors internal/scheduledtask: a schedule arms on first sight and
// never catches up runs missed while the control plane was down.
type Scheduler struct {
	engine *Engine
	mu     sync.Mutex
	next   map[string]time.Time
}

// NewScheduler builds a Scheduler over engine.
func NewScheduler(engine *Engine) *Scheduler {
	return &Scheduler{engine: engine, next: map[string]time.Time{}}
}

// Tick checks every enabled pipeline once.
func (s *Scheduler) Tick(ctx context.Context) error {
	e := s.engine
	pipes, err := e.cfg.Store.ListPipelines(ctx, "")
	if err != nil {
		return fmt.Errorf("pipeline: scheduler list: %w", err)
	}
	now := e.cfg.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	live := map[string]bool{}
	var errs []error
	for _, p := range pipes {
		if !p.Enabled {
			continue
		}
		def, issues := Validate([]byte(p.YAML))
		if len(issues) > 0 {
			continue
		}
		for i, expr := range def.On.Schedule {
			key := fmt.Sprintf("%s#%d#%s", p.ID, i, expr)
			live[key] = true
			sched, err := cronexpr.Parse(expr)
			if err != nil {
				continue
			}
			next, known := s.next[key]
			if !known {
				s.next[key] = sched.Next(now)
				continue
			}
			if now.Before(next) {
				continue
			}
			s.next[key] = sched.Next(now)
			if _, err := e.Start(ctx, p, StartOptions{Trigger: TriggerSchedule, Actor: "schedule"}); err != nil {
				errs = append(errs, fmt.Errorf("pipeline %q: %w", p.Name, err))
			}
		}
	}
	for k := range s.next {
		if !live[k] {
			delete(s.next, k)
		}
	}
	return errors.Join(errs...)
}

// Run ticks every interval until ctx is done.
func (s *Scheduler) Run(ctx context.Context, interval time.Duration) error {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			if err := s.Tick(ctx); err != nil {
				s.engine.cfg.Logger.Warn("pipeline: scheduler tick had errors", slog.String("error", err.Error()))
			}
		}
	}
}

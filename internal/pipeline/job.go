package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// jobRun is the in-memory state of one job attempt's goroutine. Everything
// that must survive a restart lives in the database instead.
type jobRun struct {
	e     *Engine
	run   store.PipelineRun
	def   *Definition
	jd    *Job
	row   store.PipelineJob
	rt    Runtime
	plan  []plannedStep
	steps []store.PipelineStep
	all   []store.PipelineJob
	mask  *masker
	sink  *logSink

	runEnv map[string]string

	mu          sync.Mutex
	outputs     map[string]string
	containerID string
	failed      bool
	failReason  string
}

// outcome of running a job's plan.
type outcome int

const (
	outcomeSucceeded outcome = iota
	outcomeFailed
	outcomeWaiting
	outcomeCancelled
	outcomeInterrupted
)

func (e *Engine) runJob(ctx context.Context, run store.PipelineRun, def *Definition, row store.PipelineJob) {
	st := e.cfg.Store
	jobs, err := st.ListPipelineJobs(ctx, run.ID)
	if err != nil {
		e.cfg.Logger.Warn("pipeline: load job failed", slog.String("job_id", row.ID), slog.String("error", err.Error()))
		return
	}
	var fresh *store.PipelineJob
	for i := range jobs {
		if jobs[i].ID == row.ID {
			fresh = &jobs[i]
		}
	}
	if fresh == nil || store.IsPipelineTerminal(fresh.Status) || fresh.Status == store.PipelineStatusWaitingApproval {
		return
	}
	row = *fresh
	jd := def.Jobs[baseJobName(row.Key)]

	m := &masker{}
	jr := &jobRun{
		e: e, run: run, def: def, jd: jd, row: row, plan: planSteps(jd), steps: row.Steps, all: jobs, mask: m,
		sink:    newLogSink(st, e.cfg.Logger, run.ID, row.Key, m, e.cfg.Now, e.cfg.MaxLogLines),
		outputs: map[string]string{},
	}
	_ = json.Unmarshal([]byte(row.OutputsJSON), &jr.outputs)
	if e.cfg.RunEnv != nil {
		jr.runEnv = e.cfg.RunEnv(ctx, run)
	}
	defer jr.sink.Close()

	rt, err := e.cfg.Runtime(row.NodeID)
	if err != nil {
		jr.finish(ctx, outcomeFailed, "node unavailable: "+err.Error())
		return
	}
	jr.rt = rt
	e.removeJobContainers(context.WithoutCancel(ctx), run, row)

	timeout := time.Duration(jd.Timeout)
	if timeout <= 0 {
		timeout = e.cfg.JobTimeout
	}
	jctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	attempt := row.Attempt
	var res outcome
	for {
		res = jr.runPlan(jctx)
		if res != outcomeFailed || attempt >= jd.Retries || jctx.Err() != nil {
			break
		}
		attempt++
		jr.logJob(fmt.Sprintf("job failed (%s), retrying (attempt %d of %d)", jr.failReason, attempt+1, jd.Retries+1))
		jr.resetForRetry(ctx, attempt)
	}

	cleanupCtx := context.WithoutCancel(ctx)
	switch {
	case res == outcomeWaiting:
		return
	case ctx.Err() != nil && context.Cause(ctx) != errRunCancelled:
		return
	case context.Cause(ctx) == errRunCancelled:
		jr.teardown(cleanupCtx)
		jr.finish(cleanupCtx, outcomeCancelled, "run cancelled")
	case jctx.Err() != nil && errors.Is(jctx.Err(), context.DeadlineExceeded):
		jr.teardown(cleanupCtx)
		jr.finish(cleanupCtx, outcomeFailed, "job timed out after "+timeout.String())
	default:
		jr.teardown(cleanupCtx)
		reason := "all steps succeeded"
		if res == outcomeFailed {
			reason = jr.failReason
		}
		jr.finish(cleanupCtx, res, reason)
	}
}

func (jr *jobRun) finish(ctx context.Context, res outcome, reason string) {
	status := store.PipelineStatusSucceeded
	switch res {
	case outcomeFailed:
		status = store.PipelineStatusFailed
	case outcomeCancelled:
		status = store.PipelineStatusCancelled
	}
	now := jr.e.cfg.Now()
	if err := jr.e.cfg.Store.SetPipelineJobStatus(ctx, jr.row.ID, status, reason, "", jr.row.Attempt, nil, &now); err != nil {
		jr.e.cfg.Logger.Warn("pipeline: record job result failed", slog.String("job_id", jr.row.ID), slog.String("error", err.Error()))
	}
}

func (jr *jobRun) logJob(msg string) { jr.sink.Line(0, "stdout", "[pipeline] "+msg) }

func (jr *jobRun) resetForRetry(ctx context.Context, attempt int) {
	jr.teardown(ctx)
	now := jr.e.cfg.Now()
	jr.mu.Lock()
	jr.failed, jr.failReason = false, ""
	jr.mu.Unlock()
	jr.row.Attempt = attempt
	for i := range jr.steps {
		jr.steps[i].Status = store.PipelineStatusPending
		if err := jr.e.cfg.Store.SetPipelineStepStatus(ctx, jr.row.ID, i, store.PipelineStatusPending, "", nil, 0, nil, nil); err != nil {
			jr.e.cfg.Logger.Warn("pipeline: reset step failed", slog.String("job_id", jr.row.ID), slog.String("error", err.Error()))
		}
	}
	_ = jr.e.cfg.Store.SetPipelineJobStatus(ctx, jr.row.ID, store.PipelineStatusRunning, fmt.Sprintf("retry %d", attempt), "", attempt, &now, nil)
}

// teardown removes the job container and workspace volume.
func (jr *jobRun) teardown(ctx context.Context) {
	jr.e.removeJobContainers(ctx, jr.run, jr.row)
	jr.mu.Lock()
	jr.containerID = ""
	jr.mu.Unlock()
}

func (jr *jobRun) persistOutputs(ctx context.Context) {
	jr.mu.Lock()
	raw, _ := json.Marshal(jr.outputs)
	jr.mu.Unlock()
	if err := jr.e.cfg.Store.SetPipelineJobOutputs(ctx, jr.row.ID, string(raw)); err != nil {
		jr.e.cfg.Logger.Warn("pipeline: persist outputs failed", slog.String("job_id", jr.row.ID), slog.String("error", err.Error()))
	}
}

func (jr *jobRun) setStep(ctx context.Context, idx int, status, reason string, exit *int, attempt int) {
	now := jr.e.cfg.Now()
	var started, finished *time.Time
	if status == store.PipelineStatusRunning {
		started = &now
	} else {
		finished = &now
	}
	jr.steps[idx].Status = status
	if err := jr.e.cfg.Store.SetPipelineStepStatus(ctx, jr.row.ID, idx, status, reason, exit, attempt, started, finished); err != nil {
		jr.e.cfg.Logger.Warn("pipeline: record step failed", slog.String("job_id", jr.row.ID), slog.Int("step", idx), slog.String("error", err.Error()))
	}
}

// runPlan executes the job's steps in order, skipping ones already
// recorded as done so a resumed job continues where it stopped.
func (jr *jobRun) runPlan(ctx context.Context) outcome {
	for k, p := range jr.plan {
		if ctx.Err() != nil {
			return jr.interruptedOutcome(ctx)
		}
		if s := jr.steps[k].Status; s == store.PipelineStatusSucceeded || s == store.PipelineStatusSkipped {
			continue
		}
		var step Step
		if p.Def >= 0 {
			step = jr.jd.Steps[p.Def]
		}
		if p.Kind == KindApproval {
			if jr.gateApproval(ctx, k, step) {
				return outcomeWaiting
			}
			continue
		}

		jr.mu.Lock()
		failed := jr.failed
		jr.mu.Unlock()
		sc := jobScope(jr.run, jr.def, jr.row, jr.all)
		jr.addOutputScope(&sc)
		sc.Success, sc.Failure = !failed, failed
		if p.Kind == KindSetup && failed {
			continue
		}
		run, err := EvalCondition(step.If, sc)
		if err != nil {
			jr.recordFailure(ctx, k, step, "invalid condition: "+err.Error(), nil)
			continue
		}
		if !run {
			reason := "condition not met"
			if step.If == "" {
				reason = "an earlier step failed"
			}
			jr.setStep(ctx, k, store.PipelineStatusSkipped, reason, nil, 0)
			continue
		}
		jr.runStepWithRetries(ctx, k, p, step)
	}
	jr.persistOutputs(context.WithoutCancel(ctx))
	if ctx.Err() != nil {
		return jr.interruptedOutcome(ctx)
	}
	jr.mu.Lock()
	defer jr.mu.Unlock()
	if jr.failed {
		return outcomeFailed
	}
	return outcomeSucceeded
}

func (jr *jobRun) interruptedOutcome(ctx context.Context) outcome {
	if context.Cause(ctx) == errRunCancelled {
		return outcomeCancelled
	}
	return outcomeInterrupted
}

func (jr *jobRun) addOutputScope(sc *Scope) {
	jr.mu.Lock()
	defer jr.mu.Unlock()
	maps.Copy(sc.Vars, prefixed("outputs.", jr.outputs))
	for k, v := range jr.outputs {
		sc.Vars[k] = v
	}
}

func prefixed(prefix string, m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[prefix+k] = v
	}
	return out
}

func (jr *jobRun) recordFailure(ctx context.Context, k int, step Step, reason string, exitPtr *int) {
	jr.setStep(ctx, k, store.PipelineStatusFailed, reason, exitPtr, 0)
	if step.ContinueOnError {
		return
	}
	jr.mu.Lock()
	defer jr.mu.Unlock()
	if !jr.failed {
		jr.failed = true
		jr.failReason = fmt.Sprintf("step %q failed: %s", jr.plan[k].Name, reason)
	}
}

func (jr *jobRun) runStepWithRetries(ctx context.Context, k int, p plannedStep, step Step) {
	timeout := time.Duration(step.Timeout)
	if timeout <= 0 {
		timeout = jr.e.cfg.StepTimeout
	}
	for attempt := 0; ; attempt++ {
		jr.setStep(ctx, k, store.PipelineStatusRunning, "", nil, attempt)
		sctx, cancel := context.WithTimeout(ctx, timeout)
		exit, err := jr.execStep(sctx, k, p, step)
		timedOut := errors.Is(sctx.Err(), context.DeadlineExceeded) && ctx.Err() == nil
		cancel()
		if err == nil {
			jr.setStep(ctx, k, store.PipelineStatusSucceeded, "", exit, attempt)
			jr.persistOutputs(ctx)
			return
		}
		if ctx.Err() != nil {
			bg := context.WithoutCancel(ctx)
			switch {
			case context.Cause(ctx) == errRunCancelled:
				jr.setStep(bg, k, store.PipelineStatusCancelled, "cancelled", nil, attempt)
			case errors.Is(ctx.Err(), context.DeadlineExceeded):
				jr.setStep(bg, k, store.PipelineStatusFailed, "job timed out", nil, attempt)
			}
			return
		}
		msg := err.Error()
		if timedOut {
			msg = "timed out after " + timeout.String()
		}
		jr.sink.Line(k, "stderr", "[pipeline] "+msg)
		if attempt < step.Retries {
			jr.sink.Line(k, "stdout", fmt.Sprintf("[pipeline] retrying step (attempt %d of %d)", attempt+2, step.Retries+1))
			continue
		}
		jr.recordFailure(ctx, k, step, msg, exit)
		return
	}
}

// gateApproval opens the approval gate for step k and parks the job. It
// returns false when the step was already approved.
func (jr *jobRun) gateApproval(ctx context.Context, k int, step Step) bool {
	st := jr.e.cfg.Store
	timeout := jr.e.cfg.ApprovalTimeout
	if v := step.With["timeout"]; v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			timeout = d
		}
	}
	ability := firstNonEmpty(step.With["approvers"], "deploy")
	msg := firstNonEmpty(step.With["message"], "Approval required")
	now := jr.e.cfg.Now()
	exp := now.Add(timeout)
	if err := st.CreatePipelineApproval(ctx, store.PipelineApproval{
		RunID: jr.run.ID, JobID: jr.row.ID, StepIndex: k, Message: msg, RequiredAbility: ability, ExpiresAt: &exp, CreatedAt: now,
	}); err != nil {
		jr.recordFailure(ctx, k, step, err.Error(), nil)
		return false
	}
	jr.setStep(ctx, k, store.PipelineStatusWaitingApproval, msg, nil, 0)
	if err := st.SetPipelineJobStatus(ctx, jr.row.ID, store.PipelineStatusWaitingApproval, "waiting for approval: "+msg, "", jr.row.Attempt, nil, nil); err != nil {
		jr.e.cfg.Logger.Warn("pipeline: park job failed", slog.String("job_id", jr.row.ID), slog.String("error", err.Error()))
	}
	return true
}

func (jr *jobRun) containerSpec(name string) docker.ContainerSpec {
	e := jr.e
	spec := docker.ContainerSpec{
		Name:       name,
		Image:      jr.jd.Image,
		Entrypoint: keepAliveEntrypoint,
		Labels:     map[string]string{e.cfg.NamePrefix + ".pipeline.run": jr.run.ID, e.cfg.NamePrefix + ".pipeline.job": jr.row.Key},
		Volumes: []docker.VolumeMount{
			{Name: e.workspaceVolume(jr.run.ID, jr.row.ID), ContainerPath: "/workspace"},
			{Name: e.artifactsVolume(jr.run.ID), ContainerPath: "/artifacts"},
		},
	}
	for _, c := range jr.jd.Cache {
		spec.Volumes = append(spec.Volumes, docker.VolumeMount{Name: e.cacheVolume(jr.run.AppName, c.Key), ContainerPath: c.Path})
	}
	if r := jr.jd.Resources; r != nil {
		mem, _ := parseMemory(r.Memory)
		spec.Resources = &docker.Resources{MemoryBytes: mem, NanoCPUs: int64(r.CPU * 1e9)}
	}
	return spec
}

var keepAliveEntrypoint = []string{"sh", "-c", "trap 'exit 0' TERM; while :; do sleep 3600 & wait $!; done"}

func (e *Engine) cacheVolume(app, key string) string {
	return fmt.Sprintf("%s-plcache-%s-%s", e.cfg.NamePrefix, sanitizeName(app), sanitizeName(key))
}

func sanitizeName(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '_', c == '.', c == '-':
			out = append(out, c)
		default:
			out = append(out, '_')
		}
	}
	return string(out)
}

// ensureContainer creates and starts the job container once per attempt.
func (jr *jobRun) ensureContainer(ctx context.Context) (string, error) {
	jr.mu.Lock()
	id := jr.containerID
	jr.mu.Unlock()
	if id != "" {
		return id, nil
	}
	e := jr.e
	for _, v := range []string{e.workspaceVolume(jr.run.ID, jr.row.ID), e.artifactsVolume(jr.run.ID)} {
		if err := jr.rt.EnsureVolume(ctx, v); err != nil {
			return "", fmt.Errorf("prepare volume: %w", err)
		}
	}
	for _, c := range jr.jd.Cache {
		if err := jr.rt.EnsureVolume(ctx, e.cacheVolume(jr.run.AppName, c.Key)); err != nil {
			return "", fmt.Errorf("prepare cache volume: %w", err)
		}
	}
	id, err := jr.rt.Create(ctx, jr.containerSpec(e.jobPrefix(jr.run.ID, jr.row.ID)))
	if err != nil {
		return "", fmt.Errorf("create job container: %w", err)
	}
	if err := jr.rt.Start(ctx, id); err != nil {
		_ = jr.rt.Remove(context.WithoutCancel(ctx), id, true)
		return "", fmt.Errorf("start job container: %w", err)
	}
	jr.mu.Lock()
	jr.containerID = id
	jr.mu.Unlock()
	return id, nil
}

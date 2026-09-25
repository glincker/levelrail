package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"

	"github.com/GLINCKER/levelrail/internal/store"
)

// schedule starts every job whose dependencies are done, resumes jobs the
// database says are running but no goroutine owns, and settles approvals.
func (e *Engine) schedule(ctx context.Context, run store.PipelineRun, def *Definition, jobs []store.PipelineJob) error {
	st := e.cfg.Store
	now := e.cfg.Now()
	for i := range jobs {
		j := jobs[i]
		jd := def.Jobs[baseJobName(j.Key)]
		if jd == nil {
			return fmt.Errorf("job %q not in definition", j.Key)
		}
		switch j.Status {
		case store.PipelineStatusPending:
			ready, err := needsDone(j, jobs)
			if err != nil || !ready {
				if err != nil {
					return err
				}
				continue
			}
			sc := jobScope(run, def, j, jobs)
			ok, err := EvalCondition(jd.If, sc)
			if err != nil {
				if serr := st.SetPipelineJobStatus(ctx, j.ID, store.PipelineStatusFailed, "invalid condition: "+err.Error(), "", 0, nil, &now); serr != nil {
					return serr
				}
				continue
			}
			if !ok {
				reason := "condition not met"
				if jd.If == "" {
					reason = "a needed job did not succeed"
				}
				if err := st.SetPipelineJobStatus(ctx, j.ID, store.PipelineStatusSkipped, reason, "", 0, nil, &now); err != nil {
					return err
				}
				continue
			}
			e.launch(ctx, run, def, j, true)
		case store.PipelineStatusRunning:
			e.launch(ctx, run, def, j, false)
		case store.PipelineStatusWaitingApproval:
			if err := e.settleApproval(ctx, run, j); err != nil {
				return err
			}
		}
	}
	return nil
}

func needsDone(j store.PipelineJob, all []store.PipelineJob) (bool, error) {
	var needs []string
	if err := json.Unmarshal([]byte(j.NeedsJSON), &needs); err != nil {
		return false, fmt.Errorf("job %q needs: %w", j.Key, err)
	}
	for _, o := range all {
		if slices.Contains(needs, baseJobName(o.Key)) && !store.IsPipelineTerminal(o.Status) {
			return false, nil
		}
	}
	return true, nil
}

// launch starts a job goroutine unless one is live or the parallel cap is
// reached. fresh marks a job moving from pending to running.
func (e *Engine) launch(ctx context.Context, run store.PipelineRun, def *Definition, j store.PipelineJob, fresh bool) {
	e.mu.Lock()
	if _, live := e.active[j.ID]; live || len(e.active) >= e.cfg.MaxParallelJobs {
		e.mu.Unlock()
		return
	}
	jctx, cancel := context.WithCancelCause(e.base)
	e.active[j.ID] = cancel
	e.wg.Add(1)
	e.mu.Unlock()

	if fresh {
		now := e.cfg.Now()
		if err := e.cfg.Store.SetPipelineJobStatus(ctx, j.ID, store.PipelineStatusRunning, "running", "", j.Attempt, &now, nil); err != nil {
			e.cfg.Logger.Warn("pipeline: mark job running failed", slog.String("job_id", j.ID), slog.String("error", err.Error()))
		}
	}
	go func() {
		defer func() {
			e.mu.Lock()
			delete(e.active, j.ID)
			e.mu.Unlock()
			cancel(nil)
			e.wg.Done()
			e.Nudge()
		}()
		e.runJob(jctx, run, def, j)
	}()
}

// settleApproval turns a decided or expired approval gate into job progress.
func (e *Engine) settleApproval(ctx context.Context, run store.PipelineRun, j store.PipelineJob) error {
	st := e.cfg.Store
	now := e.cfg.Now()
	approvals, err := st.ListPipelineApprovals(ctx, run.ID)
	if err != nil {
		return err
	}
	for _, a := range approvals {
		if a.JobID != j.ID || (a.Decision == "" && (a.ExpiresAt == nil || now.Before(*a.ExpiresAt))) {
			continue
		}
		switch a.Decision {
		case "approved":
			if err := st.SetPipelineStepStatus(ctx, j.ID, a.StepIndex, store.PipelineStatusSucceeded, "approved by "+a.DecidedBy, nil, 0, nil, &now); err != nil {
				return err
			}
			return st.SetPipelineJobStatus(ctx, j.ID, store.PipelineStatusRunning, "approved by "+a.DecidedBy, "", j.Attempt, nil, nil)
		case "rejected":
			reason := "rejected by " + a.DecidedBy
			if a.Comment != "" {
				reason += ": " + a.Comment
			}
			if err := st.SetPipelineStepStatus(ctx, j.ID, a.StepIndex, store.PipelineStatusFailed, reason, nil, 0, nil, &now); err != nil {
				return err
			}
			return st.SetPipelineJobStatus(ctx, j.ID, store.PipelineStatusFailed, reason, "", j.Attempt, nil, &now)
		default:
			if err := st.SetPipelineStepStatus(ctx, j.ID, a.StepIndex, store.PipelineStatusFailed, "approval timed out", nil, 0, nil, &now); err != nil {
				return err
			}
			return st.SetPipelineJobStatus(ctx, j.ID, store.PipelineStatusFailed, "approval timed out", "", j.Attempt, nil, &now)
		}
	}
	return nil
}

func (e *Engine) jobPrefix(runID, jobID string) string {
	return fmt.Sprintf("%s-pl-%s-%s", e.cfg.NamePrefix, shortID(runID), shortID(jobID))
}

func (e *Engine) workspaceVolume(runID, jobID string) string {
	return e.jobPrefix(runID, jobID) + "-ws"
}

func (e *Engine) artifactsVolume(runID string) string {
	return fmt.Sprintf("%s-pl-%s-art", e.cfg.NamePrefix, shortID(runID))
}

// removeJobContainers force-removes any container left from a previous
// attempt or process for this job. Best effort: it runs on paths where the
// job's outcome is already decided.
func (e *Engine) removeJobContainers(ctx context.Context, run store.PipelineRun, j store.PipelineJob) {
	rt, err := e.cfg.Runtime(j.NodeID)
	if err != nil {
		return
	}
	found, err := rt.ListByPrefix(ctx, e.jobPrefix(run.ID, j.ID))
	if err != nil {
		return
	}
	for _, c := range found {
		if err := rt.Remove(ctx, c.ID, true); err != nil {
			e.cfg.Logger.Warn("pipeline: remove container failed", slog.String("container", c.Name), slog.String("error", err.Error()))
		}
	}
	if vr, ok := rt.(VolumeRemover); ok {
		_ = vr.RemoveVolume(ctx, e.workspaceVolume(run.ID, j.ID))
	}
}

// cleanupRun removes the run's artifact volume from every node it used.
func (e *Engine) cleanupRun(ctx context.Context, run store.PipelineRun, jobs []store.PipelineJob) {
	seen := map[string]bool{}
	for _, j := range jobs {
		if seen[j.NodeID] {
			continue
		}
		seen[j.NodeID] = true
		rt, err := e.cfg.Runtime(j.NodeID)
		if err != nil {
			continue
		}
		if vr, ok := rt.(VolumeRemover); ok {
			_ = vr.RemoveVolume(ctx, e.artifactsVolume(run.ID))
		}
	}
}

package pipeline

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/GLINCKER/levelrail/internal/store"
)

func holdReason(reason string) string { return "waiting for approval: " + reason }

// listensTo reports whether t has the trigger for kind configured at all,
// whatever its filters say.
func listensTo(t Triggers, kind string) bool {
	switch kind {
	case TriggerPush:
		return t.Push != nil
	case TriggerPullRequest:
		return t.PullRequest != nil
	case TriggerTag:
		return t.Tag != nil
	}
	return false
}

// explainMismatch says which filter kept a configured trigger from
// matching ev.
func explainMismatch(t Triggers, ev Event) string {
	switch ev.Kind {
	case TriggerPush:
		return fmt.Sprintf("branch %q does not match on.push.branches %v", ev.Branch, []string(t.Push.Branches))
	case TriggerPullRequest:
		return fmt.Sprintf("target branch %q does not match on.pull_request.branches %v", ev.Branch, []string(t.PullRequest.Branches))
	case TriggerTag:
		return fmt.Sprintf("tag %q does not match on.tag.patterns %v", ev.Tag, []string(t.Tag.Patterns))
	}
	return "trigger filter did not match"
}

func (e *Engine) logTrigger(ctx context.Context, app, pipelineName string, ev Event, ref, sha, decision, reason, runID string) {
	err := e.cfg.Store.AddPipelineTriggerLog(ctx, store.PipelineTriggerLog{
		AppName: app, Pipeline: pipelineName, Event: ev.Kind, Ref: ref, SHA: sha,
		Decision: decision, Reason: reason, RunID: runID, CreatedAt: e.cfg.Now().UTC(),
	})
	if err != nil {
		e.cfg.Logger.Warn("pipeline: record trigger decision failed", slog.String("app", app), slog.String("error", err.Error()))
	}
}

// TriggerEvent starts a run of every enabled pipeline of app that accepts
// ev and records why each pipeline did or did not start. A pull request from
// a fork follows the pipeline's forks policy: blocked, held for approval, or
// allowed. Failures to start one pipeline are logged and never block the rest.
func (e *Engine) TriggerEvent(ctx context.Context, app string, ev Event, ref, sha, actor string) []store.PipelineRun {
	pipes, err := e.cfg.Store.ListPipelines(ctx, app)
	if err != nil {
		e.cfg.Logger.Warn("pipeline: list pipelines for event failed", slog.String("app", app), slog.String("error", err.Error()))
		return nil
	}
	var started []store.PipelineRun
	listeners := 0
	for _, p := range pipes {
		if !p.Enabled {
			continue
		}
		def, issues := Validate([]byte(p.YAML))
		if len(issues) > 0 {
			e.logTrigger(ctx, app, p.Name, ev, ref, sha, store.TriggerSkipped, "definition is invalid: "+issues[0].String(), "")
			continue
		}
		if !listensTo(def.On, ev.Kind) {
			continue
		}
		listeners++
		if !def.On.Matches(ev) {
			e.logTrigger(ctx, app, p.Name, ev, ref, sha, store.TriggerSkipped, explainMismatch(def.On, ev), "")
			continue
		}
		opt := StartOptions{Trigger: ev.Kind, Actor: actor, Ref: ref, SHA: sha}
		if ev.Kind == TriggerPullRequest {
			opt.BaseBranch = ev.Branch
			if ev.Fork {
				switch policy := def.On.PullRequest.ForkPolicy(); policy {
				case ForksBlock:
					e.logTrigger(ctx, app, p.Name, ev, ref, sha, store.TriggerSkipped,
						fmt.Sprintf("pull request from a fork (%s) blocked by on.pull_request.forks: block", forkName(ev)), "")
					continue
				case ForksApprove:
					opt.Hold = fmt.Sprintf("pull request from a fork (%s) needs an approver before it runs", forkName(ev))
				}
			}
		}
		run, err := e.Start(ctx, p, opt)
		if err != nil {
			e.cfg.Logger.Warn("pipeline: start from event failed", slog.String("pipeline", p.Name), slog.String("error", err.Error()))
			e.logTrigger(ctx, app, p.Name, ev, ref, sha, store.TriggerFailed, err.Error(), "")
			continue
		}
		if opt.Hold != "" {
			e.logTrigger(ctx, app, p.Name, ev, ref, sha, store.TriggerHeld, opt.Hold, run.ID)
		} else {
			e.logTrigger(ctx, app, p.Name, ev, ref, sha, store.TriggerStarted, fmt.Sprintf("started run #%d", run.Number), run.ID)
		}
		started = append(started, run)
	}
	if listeners == 0 && len(pipes) > 0 {
		e.logTrigger(ctx, app, "", ev, ref, sha, store.TriggerSkipped, fmt.Sprintf("no enabled pipeline of this app listens for %s events", strings.ReplaceAll(ev.Kind, "_", " ")), "")
	}
	return started
}

func forkName(ev Event) string {
	if ev.HeadRepo == "" {
		return "unknown repository"
	}
	return ev.HeadRepo
}

// DecideHold releases or rejects a run held for approval. It reports false
// when the run is not pending a decision.
func (e *Engine) DecideHold(ctx context.Context, runID string, approved bool, by string) (bool, error) {
	ok, err := e.cfg.Store.DecidePipelineRunHold(ctx, runID, approved, by, e.cfg.Now())
	if err != nil {
		return false, fmt.Errorf("pipeline: decide hold: %w", err)
	}
	if ok {
		e.Nudge()
	}
	return ok, nil
}

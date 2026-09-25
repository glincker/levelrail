package api

import (
	"context"
	"log/slog"
	"strings"

	"github.com/GLINCKER/levelrail/internal/pipeline"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/webhook"
)

// PipelineEvents starts runs of every matching pipeline for a git event.
// *pipeline.Engine satisfies it.
type PipelineEvents interface {
	TriggerEvent(ctx context.Context, app string, ev pipeline.Event, ref, sha, actor string) []store.PipelineRun
}

// SetPipelineEvents wires git webhook deliveries to pipeline triggers.
func (rt *Router) SetPipelineEvents(e PipelineEvents) { rt.pipelineEvents = e }

// firePipelinePush starts pipelines for a push or tag ref. It runs
// independently of whether the app's git source deploys on this push.
func (rt *Router) firePipelinePush(ctx context.Context, app, ref, sha string) {
	if rt.pipelineEvents == nil {
		return
	}
	ev := pipeline.Event{Kind: pipeline.TriggerPush, Branch: strings.TrimPrefix(ref, "refs/heads/")}
	if strings.HasPrefix(ref, "refs/tags/") {
		ev = pipeline.Event{Kind: pipeline.TriggerTag, Tag: strings.TrimPrefix(ref, "refs/tags/")}
	}
	if started := rt.pipelineEvents.TriggerEvent(ctx, app, ev, ref, sha, "webhook"); len(started) > 0 {
		rt.logger.Info("api: git webhook started pipeline runs", slog.String("app", app), slog.String("ref", ref), slog.Int("runs", len(started)))
	}
}

// firePipelinePullRequest starts pull_request pipelines when a pull request
// is opened or updated, matched against its target branch.
func (rt *Router) firePipelinePullRequest(ctx context.Context, app string, pr webhook.PullRequestEvent) {
	if rt.pipelineEvents == nil || pr.Action == webhook.PullRequestClosed {
		return
	}
	rt.pipelineEvents.TriggerEvent(ctx, app, pipeline.Event{Kind: pipeline.TriggerPullRequest, Branch: pr.BaseRef}, "refs/heads/"+pr.HeadRef, pr.HeadSHA, "webhook")
}

// SetPipelines wires the pipeline endpoints after construction, for wiring
// code that builds the engine after the router.
func (rt *Router) SetPipelines(st PipelineStore, runner PipelineRunner) {
	rt.pipelineStore = st
	rt.pipelineRunner = runner
}

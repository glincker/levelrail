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
	if rt.pipelineSync != nil && strings.HasPrefix(ref, "refs/heads/") {
		go rt.syncThenTriggerPush(context.WithoutCancel(ctx), app, ref, sha)
		return
	}
	rt.triggerPipelinePush(ctx, app, ref, sha)
}

// syncThenTriggerPush refreshes the app's repository-sourced definitions
// before starting the pushed ref's pipelines, so a run uses the pushed
// commit's pipeline files. It runs off the request goroutine because a
// clone can outlast a git provider's webhook timeout.
func (rt *Router) syncThenTriggerPush(ctx context.Context, app, ref, sha string) {
	ctx, cancel := context.WithTimeout(ctx, pipelineSyncTimeout)
	defer cancel()
	if _, ran, err := rt.pipelineSync.syncer.SyncOnPush(ctx, app, ref); err != nil {
		rt.logger.Warn("api: pipeline sync on push failed", slog.String("app", app), slog.String("ref", ref), slog.String("error", err.Error()))
	} else if ran {
		rt.logger.Info("api: pipeline definitions synced on push", slog.String("app", app), slog.String("ref", ref))
	}
	rt.triggerPipelinePush(ctx, app, ref, sha)
}

func (rt *Router) triggerPipelinePush(ctx context.Context, app, ref, sha string) {
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
	ev := pipeline.Event{Kind: pipeline.TriggerPullRequest, Branch: pr.BaseRef, Fork: pr.IsFork(), HeadRepo: pr.HeadRepoFullName}
	rt.pipelineEvents.TriggerEvent(ctx, app, ev, "refs/heads/"+pr.HeadRef, pr.HeadSHA, "webhook")
}

// SetPipelines wires the pipeline endpoints after construction, for wiring
// code that builds the engine after the router.
func (rt *Router) SetPipelines(st PipelineStore, runner PipelineRunner) {
	rt.pipelineStore = st
	rt.pipelineRunner = runner
}

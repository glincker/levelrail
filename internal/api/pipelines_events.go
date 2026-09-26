package api

import (
	"context"
	"errors"
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

// firePipelinePushEvent starts pipelines for a push or tag ref. It runs
// independently of whether the app's git source deploys on this push.
func (rt *Router) firePipelinePushEvent(ctx context.Context, app string, push webhook.PushEvent) {
	if rt.pipelineEvents == nil {
		return
	}
	if rt.pipelineSync != nil && strings.HasPrefix(push.Ref, "refs/heads/") {
		go rt.syncThenTriggerPush(context.WithoutCancel(ctx), app, push)
		return
	}
	rt.triggerPipelinePush(ctx, app, push)
}

// syncThenTriggerPush refreshes the app's repository-sourced definitions
// before starting the pushed ref's pipelines, reading the files at the pushed
// commit so a run's SHA and definitions agree. It runs off the request goroutine because a
// clone can outlast a git provider's webhook timeout.
func (rt *Router) syncThenTriggerPush(ctx context.Context, app string, push webhook.PushEvent) {
	ref, sha := push.Ref, push.After
	ctx, cancel := context.WithTimeout(ctx, pipelineSyncTimeout)
	defer cancel()
	if _, ran, err := rt.pipelineSync.syncer.SyncOnPush(ctx, app, ref, sha); errors.Is(err, pipeline.ErrSHAUnavailable) {
		rt.logger.Warn("api: pipeline sync skipped, pushed commit unavailable", slog.String("app", app), slog.String("ref", ref), slog.String("sha", sha), slog.String("error", err.Error()))
	} else if err != nil {
		rt.logger.Warn("api: pipeline sync on push failed", slog.String("app", app), slog.String("ref", ref), slog.String("error", err.Error()))
	} else if ran {
		rt.logger.Info("api: pipeline definitions synced on push", slog.String("app", app), slog.String("ref", ref))
	}
	rt.triggerPipelinePush(ctx, app, push)
}

func (rt *Router) triggerPipelinePush(ctx context.Context, app string, push webhook.PushEvent) {
	ref, sha := push.Ref, push.After
	ev := pipeline.Event{
		Kind: pipeline.TriggerPush, Branch: strings.TrimPrefix(ref, "refs/heads/"), Changed: push.Changed,
		ChangedFn: rt.changedFilesFn(app, changeQuery{Base: push.Before, Head: sha}),
	}
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
	ev := pipeline.Event{
		Kind: pipeline.TriggerPullRequest, Branch: pr.BaseRef, Fork: pr.IsFork(), HeadRepo: pr.HeadRepoFullName,
		Action: string(pr.Action), ChangedFn: rt.changedFilesFn(app, changeQuery{PR: pr.Number}),
	}
	rt.pipelineEvents.TriggerEvent(ctx, app, ev, "refs/heads/"+pr.HeadRef, pr.HeadSHA, "webhook")
}

// firePipelineMergeGroup starts merge_group pipelines when GitHub's merge
// queue asks for checks on a queued group's head commit.
func (rt *Router) firePipelineMergeGroup(ctx context.Context, app string, mg webhook.MergeGroupEvent) {
	if rt.pipelineEvents == nil || !mg.ChecksRequested() {
		return
	}
	ev := pipeline.Event{Kind: pipeline.TriggerMergeGroup, Branch: mg.BaseBranch()}
	rt.pipelineEvents.TriggerEvent(ctx, app, ev, mg.HeadRef, mg.HeadSHA, "webhook")
}

// SetPipelines wires the pipeline endpoints after construction, for wiring
// code that builds the engine after the router.
func (rt *Router) SetPipelines(st PipelineStore, runner PipelineRunner) {
	rt.pipelineStore = st
	rt.pipelineRunner = runner
}

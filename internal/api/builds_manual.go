package api

import (
	"context"
	"log/slog"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

// manualBuildRequest turns a validated build trigger into the pipeline request.
func manualBuildRequest(name string, existing store.DesiredService, req triggerBuildRequest, buildType string) deploy.Request {
	imageRepo := req.ImageRepo
	if imageRepo == "" {
		imageRepo = name
	}
	buildCfg := spec.Build{Type: buildType, Path: req.Build.Path, BaseDirectory: req.Build.BaseDirectory, Args: req.Build.Args}
	if buildType == spec.BuildImage {
		buildCfg = spec.Build{Type: buildType, Image: req.Build.Image}
	}
	return deploy.Request{
		ServiceName: name,
		Service:     specServiceFromDesired(existing, buildCfg),
		CommitSHA:   req.Ref,
		ImageRepo:   imageRepo,
	}
}

// manualBuildRun is one manual build ready to execute: its attempt row exists
// and the pipeline request is complete.
type manualBuildRun struct {
	name, id, buildType  string
	repoURL, ref         string
	buildReq             deploy.Request
	progress             func(build.ProgressEvent)
	finish               func(error)
	setCommit            func(context.Context, string)
	allowPrivateRepoAuth bool
}

// beginManualBuild records the attempt row and wires cancellation into the
// pipeline request. The caller holds buildStartMu.
func (rt *Router) beginManualBuild(ctx context.Context, existing store.DesiredService, buildReq deploy.Request, buildType, source, framework, freezeNote string, allowPrivate bool) manualBuildRun {
	id, progress, finish, setCommit := rt.beginBuildDeployAttempt(ctx, buildReq, existing, source, framework)
	buildReq.AttemptID = id
	if id != "" {
		buildReq.Commit = func() error { return rt.cancels.Commit(id) }
	}
	if freezeNote != "" && id != "" && rt.deploySafety != nil {
		if err := rt.deploySafety.SetDeployAttemptReason(ctx, id, freezeNote); err != nil {
			rt.logger.Warn("api: record freeze override failed", slog.String("attempt_id", id), slog.String("error", err.Error()))
		}
	}
	return manualBuildRun{
		name: existing.Name, id: id, buildType: buildType, buildReq: buildReq,
		progress: progress, finish: finish, setCommit: setCommit, allowPrivateRepoAuth: allowPrivate,
	}
}

// runManualBuild fetches, builds and deploys m, then finishes its attempt.
func (rt *Router) runManualBuild(m manualBuildRun) {
	ctx := rt.cancels.Bind(context.Background(), m.id) //nolint:gosec // deliberately not the request context: it ends when the handler returns, which would abort the build
	buildReq, id, name := m.buildReq, m.id, m.name
	if m.buildType != spec.BuildImage {
		rt.emitStep(id, "detecting", "running")
		var token string
		if m.allowPrivateRepoAuth {
			token = rt.tokenForRepo(ctx, m.repoURL)
		}
		sourceDir, commit, cleanup, err := rt.fetch(ctx, m.repoURL, m.ref, token)
		if err != nil {
			rt.logger.Error("api: trigger build: fetch source failed", slog.String("error", err.Error()), slog.String("name", name), slog.String("repo_url", m.repoURL), slog.String("ref", m.ref))
			rt.emitStep(id, "detecting", "failed")
			m.finish(err)
			return
		}
		defer cleanup()
		buildReq.SourceDir = sourceDir
		// Tag by the resolved commit, never the ref: a reused branch name
		// would retag onto newer content and orphan the rollback target.
		if commit != "" {
			buildReq.CommitSHA = commit
			m.setCommit(ctx, commit)
		}
		rt.emitStep(id, "detecting", "done")
	}

	rt.emitStep(id, "building", "running")
	tag, err := rt.builder.Deploy(ctx, buildReq, m.progress)
	_, canceled := rt.cancels.Canceled(id)
	if err != nil {
		rt.emitStep(id, "building", "failed")
	} else {
		rt.emitStep(id, "building", "done")
		// No separate registry push exists for a single-node control plane:
		// the image loads straight into the local engine.
		rt.emitStep(id, "pushing", "done")
		rt.emitStep(id, "deploying", "done")
	}
	m.finish(err)
	if err != nil {
		if canceled {
			rt.logger.Info("api: trigger build canceled", slog.String("name", name), slog.String("attempt_id", id))
			return
		}
		// Logged, not returned: a build failure can carry internal detail
		// (daemon paths, registry hosts) and no HTTP response is left to write.
		rt.logger.Error("api: trigger build failed", slog.String("error", err.Error()), slog.String("name", name), slog.String("repo_url", m.repoURL), slog.String("ref", m.ref))
		return
	}
	rt.logger.Info("api: manual build triggered", slog.String("name", name), slog.String("repo_url", m.repoURL), slog.String("ref", m.ref), slog.String("build_type", m.buildType), slog.String("tag", tag))
	rt.nudgeReconciler()
}

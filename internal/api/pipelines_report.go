package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/glincker/levelrail/internal/pipeline"
	"github.com/glincker/levelrail/internal/store"
)

// PipelineStatusReporter returns the reporter that posts pipeline run state
// to the git forge hosting an app's repository.
func (rt *Router) PipelineStatusReporter() pipeline.StatusReporter {
	return pipelineStatusReporter{rt: rt}
}

type pipelineStatusReporter struct{ rt *Router }

func (p pipelineStatusReporter) ReportRun(ctx context.Context, req pipeline.ReportRequest) (pipeline.ReportReceipt, error) {
	rt := p.rt
	if !gitStatusEnabled() || rt.gitSources == nil {
		return pipeline.ReportReceipt{}, pipeline.ErrNoReportTarget
	}
	gs, err := rt.gitSources.GetGitSource(ctx, req.App)
	if errors.Is(err, store.ErrGitSourceNotFound) {
		return pipeline.ReportReceipt{}, pipeline.ErrNoReportTarget
	}
	if err != nil {
		return pipeline.ReportReceipt{}, fmt.Errorf("load git source: %w", err)
	}
	if !gs.ReportStatus {
		return pipeline.ReportReceipt{}, pipeline.ErrNoReportTarget
	}
	f, err := rt.resolveForge(ctx, gs.RepoURL, gs.ConnectionID)
	if errors.Is(err, errNoForge) {
		return pipeline.ReportReceipt{}, pipeline.ErrNoReportTarget
	}
	if err != nil {
		return pipeline.ReportReceipt{}, err
	}
	receipt := pipeline.ReportReceipt{Provider: f.kind, URL: forgeCommitURL(f.kind, gs.RepoURL, req.SHA)}
	target := rt.dashboardLink(ctx, "/apps/"+req.App+"/pipelines/runs/"+req.RunID)
	postErr := f.postStatus(ctx, req.SHA, req.State, target, req.Description, req.Context)
	rt.recordForgeStatus(ctx, store.ForgeStatusReport{
		AppName: req.App, Provider: f.kind, SHA: req.SHA, Context: req.Context, State: req.State,
		Description: req.Description, TargetURL: target, Source: store.ForgeStatusSourcePipeline,
	}, postErr)
	if postErr != nil {
		return receipt, errors.New(describeForgeError(postErr))
	}
	return receipt, nil
}

// States the preview writers record, matching the pipeline report values.
const (
	previewStatePending = pipeline.ReportPending
	previewStateSuccess = pipeline.ReportSuccess
	previewStateFailure = pipeline.ReportFailure
)

type forgeStatusStore interface {
	RecordForgeStatusReport(ctx context.Context, r store.ForgeStatusReport) error
}

// recordForgeStatus keeps a posted status for the delivery timeline. It is
// best effort: a storage problem never fails the post it annotates.
func (rt *Router) recordForgeStatus(ctx context.Context, r store.ForgeStatusReport, postErr error) {
	s, ok := rt.apps.(forgeStatusStore)
	if !ok {
		return
	}
	if postErr != nil {
		r.Error = describeForgeError(postErr)
	}
	if err := s.RecordForgeStatusReport(ctx, r); err != nil {
		rt.logger.Warn("api: record forge status failed", slog.String("error", err.Error()), slog.String("app_name", r.AppName))
	}
}

func (rt *Router) recordPreviewStatus(ctx context.Context, app, provider, sha, state, targetURL, description string, postErr error) {
	rt.recordForgeStatus(ctx, store.ForgeStatusReport{
		AppName: app, Provider: provider, SHA: sha, Context: previewStatusContext, State: state,
		Description: description, TargetURL: targetURL, Source: store.ForgeStatusSourcePreview,
	}, postErr)
}

// dashboardLink joins the configured dashboard URL and path, or returns ""
// when no dashboard URL is set.
func (rt *Router) dashboardLink(ctx context.Context, path string) string {
	if rt.ingressSettings == nil {
		return ""
	}
	base, err := rt.ingressSettings.GetDashboardURL(ctx)
	if err != nil || base == "" {
		return ""
	}
	return strings.TrimRight(base, "/") + path
}

// forgeCommitURL is the forge's own page for a commit, where the posted
// status shows up.
func forgeCommitURL(kind, repoURL, sha string) string {
	base := strings.TrimSuffix(strings.TrimRight(repoURL, "/"), ".git")
	switch kind {
	case forgeGitLab:
		return base + "/-/commit/" + sha
	case forgeBitbucket:
		return base + "/commits/" + sha
	}
	return base + "/commit/" + sha
}

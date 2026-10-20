package api

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/GLINCKER/levelrail/internal/pipeline"
	"github.com/GLINCKER/levelrail/internal/store"
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
	f, err := rt.resolveForge(ctx, gs.RepoURL)
	if errors.Is(err, errNoForge) {
		return pipeline.ReportReceipt{}, pipeline.ErrNoReportTarget
	}
	if err != nil {
		return pipeline.ReportReceipt{}, err
	}
	receipt := pipeline.ReportReceipt{Provider: f.kind, URL: forgeCommitURL(f.kind, gs.RepoURL, req.SHA)}
	target := rt.dashboardLink(ctx, "/apps/"+req.App+"/pipelines/runs/"+req.RunID)
	if err := f.postStatus(ctx, req.SHA, req.State, target, req.Description, req.Context); err != nil {
		return receipt, errors.New(describeForgeError(err))
	}
	return receipt, nil
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

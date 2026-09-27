package api

import (
	"context"
	"log/slog"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/pipeline"
	"github.com/GLINCKER/levelrail/internal/store"
)

// PipelineRunEnv gives a pull request pipeline run the PREVIEW_PR_NUMBER,
// PREVIEW_BRANCH and PREVIEW_URL of the live preview built from its branch.
// Other runs, and runs with no live preview, get nothing.
func (rt *Router) PipelineRunEnv(ctx context.Context, run store.PipelineRun) map[string]string {
	if run.TriggerKind != pipeline.TriggerPullRequest {
		return nil
	}
	branch, ok := strings.CutPrefix(run.Ref, "refs/heads/")
	if !ok || branch == "" {
		return nil
	}
	previews, err := rt.previewEnvironments.ListPreviewEnvironmentsByApp(ctx, run.AppName)
	if err != nil {
		rt.logger.Warn("api: pipeline preview env lookup failed", slog.String("error", err.Error()), slog.String("app_name", run.AppName))
		return nil
	}
	var match *store.PreviewEnvironment
	for i := range previews {
		p := &previews[i]
		if p.Branch != branch || !p.Occupies() {
			continue
		}
		if match == nil || p.UpdatedAt > match.UpdatedAt {
			match = p
		}
	}
	if match == nil {
		return nil
	}

	domain := match.Domain
	if domain == "" && match.Status == store.PreviewStatusDeploying {
		domain = rt.previewDomain(ctx, run.AppName, match.PRNumber)
	}
	env := map[string]string{
		"PREVIEW_PR_NUMBER": strconv.Itoa(match.PRNumber),
		"PREVIEW_BRANCH":    match.Branch,
	}
	if domain != "" {
		env["PREVIEW_URL"] = "https://" + domain
	}
	return env
}

package api

import (
	"context"
	"log/slog"

	"github.com/GLINCKER/levelrail/internal/diagnose"
	"github.com/GLINCKER/levelrail/internal/repolayout"
	"github.com/GLINCKER/levelrail/internal/spec"
)

// addBuildSourceFacts records the git source's build settings and the
// Dockerfiles its repository really has, so a wrong path can be fixed in place.
// Best effort: any failure leaves the facts empty and the rule stays quiet.
func (rt *Router) addBuildSourceFacts(ctx context.Context, name string, f *diagnose.Facts) {
	if rt.gitSources == nil {
		return
	}
	gs, err := rt.gitSources.GetGitSource(ctx, name)
	if err != nil || gs == nil || len(gs.Services) > 0 || gs.BuildType != spec.BuildDockerfile {
		return
	}
	f.BuildType, f.BuildPath = gs.BuildType, gs.BuildPath
	if f.BuildPath == "" {
		f.BuildPath = resolveGitSourceBuild(gs.BuildType, gs.BaseDirectory, "", "", "").DockerfilePath
	}
	if requireHTTPOrHTTPSScheme(gs.RepoURL) != nil {
		return
	}
	open := rt.repoLayoutSource
	if open == nil {
		open = defaultRepoLayoutSource
	}
	pctx, cancel := context.WithTimeout(ctx, envDurationOr(envDiagnoseProbeBudget, defaultProbeBudget))
	defer cancel()
	src, err := open(pctx, gs.RepoURL, gs.Branch, rt.tokenForRepo(pctx, name, gs.RepoURL, true), rt.repoLayoutHosts(pctx))
	if err != nil {
		rt.logger.Warn("api: diagnose facts: open repo failed", slog.String("error", err.Error()), slog.String("name", name))
		return
	}
	res, err := repolayout.Detect(pctx, src)
	if err != nil {
		rt.logger.Warn("api: diagnose facts: read repo failed", slog.String("error", err.Error()), slog.String("name", name))
		return
	}
	f.DockerfileCandidates = res.Dockerfiles
}

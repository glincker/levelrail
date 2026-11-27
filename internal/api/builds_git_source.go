package api

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/gitsshkey"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

// ensureBuildGitSource connects req's repo as name's git source when the app
// has none, through connectGitSource and the same webhook registration the
// manual register action uses, so a create-with-repo app deploys on push.
// An existing source is never changed; a different repo only yields a note.
// It returns a short human note, empty when nothing needs saying.
func (rt *Router) ensureBuildGitSource(r *http.Request, name string, req triggerBuildRequest, buildType string) string {
	if rt.gitSources == nil || rt.gitSourceSecrets == nil || req.RepoURL == "" || req.Ref == "" || buildType == spec.BuildImage {
		return ""
	}
	ctx := withRequestOrigin(r.Context(), r)
	gs, err := rt.gitSources.GetGitSource(ctx, name)
	if err == nil && gs != nil {
		if sameRepoURL(gs.RepoURL, req.RepoURL) {
			return ""
		}
		return fmt.Sprintf("kept the app's existing git source %s: it differs from the built repository %s", redactURLCredentials(gs.RepoURL), redactURLCredentials(req.RepoURL))
	}
	if !errors.Is(err, store.ErrGitSourceNotFound) {
		rt.logger.Warn("api: trigger build: load git source failed", slog.String("name", name), slog.String("error", err.Error()))
		return ""
	}
	if !rt.callerHasAbility(r, AbilityWriteSensitive) {
		return "the git source was not connected: connecting a repository needs write access to sensitive data"
	}

	btype, err := normalizeGitSourceBuildType(buildType)
	if err != nil {
		return ""
	}
	base, path, err := validateGitSourceBuildPaths(btype, req.Build.BaseDirectory, req.Build.Path)
	if err != nil || (btype == spec.BuildRailpack && path != "") {
		return ""
	}
	if err := validateGitSourceStaticBuild(name, btype, path, "", ""); err != nil {
		return "the git source was not connected: " + err.Error()
	}
	if !gitsshkey.IsSSHURL(req.RepoURL) {
		if err := requireHTTPOrHTTPSScheme(req.RepoURL); err != nil {
			return ""
		}
	}
	spa := req.Build.StaticSPA
	result, err := rt.connectGitSource(ctx, name, connectGitSourceParams{
		RepoURL: req.RepoURL, Branch: req.Ref, BuildType: btype, BuildPath: path, BaseDirectory: base,
		StaticSPA: &spa, TriggerMode: spec.TriggerModePush,
	})
	if err != nil {
		rt.logger.Warn("api: trigger build: connect git source failed", slog.String("name", name), slog.String("error", err.Error()))
		return "the git source could not be connected: " + err.Error()
	}
	if !result.Creating {
		return ""
	}
	saved, err := rt.gitSources.GetGitSource(ctx, name)
	if err != nil {
		return ""
	}
	regErr := rt.registerGitSourceWebhook(ctx, name, *saved)
	rt.recordWebhookRegistration(ctx, name, regErr)
	note := "connected " + redactURLCredentials(req.RepoURL) + " as the app's git source"
	if regErr != nil {
		rt.logger.Warn("api: trigger build: register git source webhook failed", slog.String("name", name), slog.String("error", regErr.Error()))
		return note + "; the push webhook was not registered: " + regErr.Error()
	}
	return note + " and registered its push webhook"
}

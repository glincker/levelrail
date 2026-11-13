package api

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/GLINCKER/levelrail/internal/appimport"
	"github.com/GLINCKER/levelrail/internal/gitprovider/reporef"
	"github.com/GLINCKER/levelrail/internal/platformimport"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

const connectionFieldURL = "url"

// finishStaging puts a freshly created app into the staged state: suspended,
// repository connected for builds, and joined to the databases it uses.
func (a *importApplier) finishStaging(ctx context.Context, p platformimport.AppPlan) ([]string, error) {
	rt := a.rt
	if err := rt.apps.UpdateServiceSuspended(ctx, p.Name, true); err != nil {
		if _, derr := rt.deleteApp(ctx, p.Name); derr != nil {
			rt.logger.Error("api: app import: remove half-staged app failed", slog.String("error", derr.Error()), slog.String("app", p.Name))
		}
		return nil, fmt.Errorf("hold app stopped: %w", err)
	}
	var warns []string
	if p.GitURL != "" {
		warns = append(warns, a.connectRepository(ctx, p)...)
	}
	svc, err := rt.apps.GetDesiredService(ctx, p.Name)
	if err != nil {
		return warns, fmt.Errorf("reload staged app: %w", err)
	}
	existing := existingServiceEnvKeys(svc)
	for _, db := range a.connect[p.Name] {
		envVar := defaultConnectionEnvVar(db, connectionFieldURL, existing)
		existing[envVar] = true
		if err := rt.apps.SetServiceDatabaseEnvVar(ctx, p.Name, envVar, &store.DatabaseEnvRef{Database: db, Field: connectionFieldURL}); err != nil {
			return warns, fmt.Errorf("join database %q: %w", db, err)
		}
		warns = append(warns, fmt.Sprintf("added %s so the app joins the network of %s and can use its credentials", envVar, db))
	}
	return warns, nil
}

func importBuildType(method string) string {
	switch method {
	case appimport.MapDockerfile:
		return spec.BuildDockerfile
	case appimport.MapRailpack:
		return spec.BuildRailpack
	case appimport.MapStatic:
		return spec.BuildStatic
	}
	return ""
}

// connectRepository saves the app's git source so a build can fetch it. It
// returns warnings rather than failing: the app is usable without it. No
// webhook is registered here; pushes deploy once the source is connected.
func (a *importApplier) connectRepository(ctx context.Context, p platformimport.AppPlan) []string {
	rt := a.rt
	buildType := importBuildType(p.BuildMethod)
	switch {
	case rt.gitSources == nil || rt.gitSourceSecrets == nil:
		return []string{"the repository was not connected: git sources are not configured on this control plane"}
	case buildType == "":
		return []string{"the repository was not connected: choose a build method for it"}
	}
	shown, _ := platformimport.DisplayRepoURL(p.GitURL)
	ref, err := reporef.Parse(p.GitURL)
	if err != nil {
		return []string{fmt.Sprintf("the repository was not connected: %q is not a repository URL that can be cloned over https or ssh", shown)}
	}
	repoURL := ref.CloneURL()
	if err := requireHTTPOrHTTPSScheme(repoURL); err != nil {
		return []string{fmt.Sprintf("the repository was not connected: %s is not allowed (%v)", repoURL, err)}
	}
	var warns []string
	if !ref.IsHTTP() {
		warns = append(warns, fmt.Sprintf("the source cloned %s over ssh; it is connected as %s, so a private repository needs a git provider connection that can read it", shown, repoURL))
	}
	branch := p.GitBranch
	if branch == "" {
		branch = "main"
		warns = append(warns, "the source named no branch, so main is tracked; change it in the app's source settings if that is wrong")
	}
	base, buildPath, pathWarn := importBuildPaths(buildType, p.BaseDirectory, p.BuildPath)
	if pathWarn != "" {
		warns = append(warns, pathWarn)
	}
	_, err = rt.connectGitSource(ctx, p.Name, connectGitSourceParams{RepoURL: repoURL, Branch: branch, BuildType: buildType, BuildPath: buildPath, BaseDirectory: base})
	if err != nil {
		rt.logger.Error("api: app import: connect repository failed", slog.String("error", err.Error()), slog.String("app", p.Name))
		return append(warns, "the repository could not be connected, connect it from the app's source settings")
	}
	return warns
}

// importBuildPaths maps the source's base directory and build path onto a git
// source. A Railpack build has no path, so its path becomes the base directory.
func importBuildPaths(buildType, baseDir, buildPath string) (base, path, warn string) {
	base, path = strings.Trim(baseDir, "/"), strings.Trim(buildPath, "/")
	if buildType == spec.BuildRailpack {
		if base == "" {
			base = path
		}
		path = ""
	}
	if base == path {
		base = ""
	}
	nb, np, err := validateGitSourceBuildPaths(buildType, base, path)
	if err == nil {
		return nb, np, ""
	}
	if base != "" {
		if _, p2, err2 := validateGitSourceBuildPaths(buildType, "", path); err2 == nil {
			return "", p2, fmt.Sprintf("the source base directory %s was not applied (%v); set it in the app's build settings", base, err)
		}
	}
	return "", "", fmt.Sprintf("the source build paths were not applied (%v); set them in the app's build settings", err)
}

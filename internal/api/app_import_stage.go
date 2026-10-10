package api

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/GLINCKER/levelrail/internal/appimport"
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
// returns warnings rather than failing: the app is usable without it.
func (a *importApplier) connectRepository(ctx context.Context, p platformimport.AppPlan) []string {
	rt := a.rt
	buildType := importBuildType(p.BuildMethod)
	switch {
	case rt.gitSources == nil || rt.gitSourceSecrets == nil:
		return []string{"the repository was not connected: git sources are not configured on this control plane"}
	case buildType == "":
		return []string{"the repository was not connected: choose a build method for it"}
	}
	if err := requireHTTPOrHTTPSScheme(p.GitURL); err != nil {
		return []string{"the repository was not connected: only http and https repository URLs are supported"}
	}
	buildPath := p.BuildPath
	var warns []string
	if buildType == spec.BuildRailpack && buildPath != "" {
		warns = append(warns, "the source base directory "+buildPath+" is not applied to Railpack builds")
		buildPath = ""
	}
	branch := p.GitBranch
	if branch == "" {
		branch = "main"
	}
	_, err := rt.connectGitSource(ctx, p.Name, connectGitSourceParams{RepoURL: p.GitURL, Branch: branch, BuildType: buildType, BuildPath: buildPath})
	if err != nil {
		rt.logger.Error("api: app import: connect repository failed", slog.String("error", err.Error()), slog.String("app", p.Name))
		return append(warns, "the repository could not be connected, connect it from the app's source settings")
	}
	return warns
}

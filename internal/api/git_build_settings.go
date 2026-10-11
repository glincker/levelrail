package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/GLINCKER/levelrail/internal/gitprovider"
	"github.com/GLINCKER/levelrail/internal/repolayout"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

// repoLayoutSourceFunc opens a file-tree source for build detection. A seam
// so tests can serve a fake repository tree.
type repoLayoutSourceFunc func(ctx context.Context, repoURL, branch, token string, hosts repolayout.Hosts) (repolayout.Source, error)

func defaultRepoLayoutSource(ctx context.Context, repoURL, branch, token string, hosts repolayout.Hosts) (repolayout.Source, error) {
	src, err := repolayout.NewAPISource(gitprovider.NewGuardedClient(), repoURL, branch, token, hosts)
	if errors.Is(err, repolayout.ErrUnsupportedHost) {
		return repolayout.NewCloneSource(ctx, repoURL, branch, token)
	}
	return src, err
}

// gitSourceResolvedBuild states, in repository-root-relative terms, what a
// deploy of the source builds.
type gitSourceResolvedBuild struct {
	ContextDir     string `json:"context_dir"`
	DockerfilePath string `json:"dockerfile_path,omitempty"`
	Summary        string `json:"summary"`
}

// resolveGitSourceBuild never fails: an invalid stored combination is
// reported through Summary so the dashboard can still render it.
func resolveGitSourceBuild(buildType, baseDirectory, buildPath string) gitSourceResolvedBuild {
	base := strings.Trim(baseDirectory, "/")
	ctxDir := "."
	if base != "" {
		ctxDir = base
	}
	where := "the repository root"
	if base != "" {
		where = base
	}
	switch buildType {
	case spec.BuildDockerfile:
		df := buildPath
		if df == "" {
			df = path.Join(base, "Dockerfile")
		}
		return gitSourceResolvedBuild{ContextDir: ctxDir, DockerfilePath: df, Summary: fmt.Sprintf("Builds %s with %s as the build context.", df, where)}
	case spec.BuildStatic:
		return gitSourceResolvedBuild{ContextDir: ctxDir, DockerfilePath: buildPath, Summary: fmt.Sprintf("Serves a static site built from %s.", where)}
	default:
		return gitSourceResolvedBuild{ContextDir: ctxDir, Summary: fmt.Sprintf("Auto-detects the framework in %s.", where)}
	}
}

// validateGitSourceBuildPaths normalizes and validates a base directory and
// Dockerfile path pair. Both are relative to the repository root.
func validateGitSourceBuildPaths(buildType, baseDirectory, buildPath string) (base, p string, err error) {
	base, err = spec.NormalizeRepoPath("base_directory", baseDirectory)
	if err != nil {
		return "", "", err
	}
	p, err = spec.NormalizeRepoPath("build_path", buildPath)
	if err != nil {
		return "", "", err
	}
	if _, err := spec.ResolveGitBuild(buildType, base, p); err != nil {
		return "", "", err
	}
	return base, p, nil
}

type setGitSourceBuildRequest struct {
	BuildType     string `json:"build_type"`
	BuildPath     string `json:"build_path"`
	BaseDirectory string `json:"base_directory"`
}

// handleSetGitSourceBuild handles PUT /api/v1/apps/{name}/git-source/build:
// replaces only the build type, Dockerfile path and base directory.
func (rt *Router) handleSetGitSourceBuild(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	ctx := r.Context()

	var req setGitSourceBuildRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	buildType, err := normalizeGitSourceBuildType(req.BuildType)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	base, p, err := validateGitSourceBuildPaths(buildType, req.BaseDirectory, req.BuildPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	gs, err := rt.gitSources.GetGitSource(ctx, name)
	if errors.Is(err, store.ErrGitSourceNotFound) {
		writeError(w, http.StatusNotFound, "no git source connected for this app")
		return
	}
	if err != nil {
		rt.internalError(w, "api: set git source build: load failed", err, slog.String("name", name))
		return
	}
	if len(gs.Services) > 0 {
		writeError(w, http.StatusConflict, "this app deploys from a services map, edit the build settings in that map instead")
		return
	}
	if err := rt.gitSources.SetGitSourceBuild(ctx, name, buildType, p, base); err != nil {
		rt.internalError(w, "api: set git source build failed", err, slog.String("name", name))
		return
	}
	gs.BuildType, gs.BuildPath, gs.BaseDirectory = buildType, p, base
	hasToken := false
	if rt.gitSourceSecrets != nil {
		hasToken, err = rt.gitSourceSecrets.Exists(ctx, store.GitSourceSecretsKey(name), gitSourceTokenKey)
		if err != nil {
			rt.internalError(w, "api: set git source build: check token failed", err, slog.String("name", name))
			return
		}
	}
	rt.logger.Info("api: git source build settings updated", slog.String("name", name), slog.String("build_type", buildType), slog.String("base_directory", base), slog.String("build_path", p))
	writeJSON(w, http.StatusOK, toGitSourceResource(*gs, hasToken, rt.gitSourceWebhookURL(ctx, name)))
}

type detectGitSourceRequest struct {
	Branch string `json:"branch,omitempty"`
}

type detectGitSourceResponse struct {
	Branch string `json:"branch"`
	repolayout.Result
	// NeedsBuildSettings is true when the source auto-detects the repository
	// root, there is no application at the root, and the repository has
	// Dockerfiles or workspace tooling: a build there would fail.
	NeedsBuildSettings bool `json:"needs_build_settings"`
}

// handleDetectGitSourceBuild handles POST /api/v1/apps/{name}/git-source/detect:
// lists candidate build roots from the provider's tree API, with no clone for
// GitHub, GitLab and Gitea.
func (rt *Router) handleDetectGitSourceBuild(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	ctx := r.Context()

	var req detectGitSourceRequest
	if r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}
	gs, err := rt.gitSources.GetGitSource(ctx, name)
	if errors.Is(err, store.ErrGitSourceNotFound) {
		writeError(w, http.StatusNotFound, "no git source connected for this app")
		return
	}
	if err != nil {
		rt.internalError(w, "api: detect git source build: load failed", err, slog.String("name", name))
		return
	}
	branch := strings.TrimSpace(req.Branch)
	if branch == "" {
		branch = gs.Branch
	}
	if err := requireHTTPOrHTTPSScheme(gs.RepoURL); err != nil {
		writeError(w, http.StatusBadRequest, "repo_url must use http or https")
		return
	}

	token := rt.tokenForRepo(ctx, name, gs.RepoURL, true)
	open := rt.repoLayoutSource
	if open == nil {
		open = defaultRepoLayoutSource
	}
	src, err := open(ctx, gs.RepoURL, branch, token, rt.repoLayoutHosts(ctx))
	if err != nil {
		rt.writeDetectError(w, name, gs.RepoURL, err)
		return
	}
	res, err := repolayout.Detect(ctx, src)
	if err != nil {
		rt.writeDetectError(w, name, gs.RepoURL, err)
		return
	}
	needs := gs.BuildType == spec.BuildRailpack && gs.BaseDirectory == "" && !res.RootHasApp &&
		(len(res.Dockerfiles) > 0 || res.LooksLikeMonorepo)
	writeJSON(w, http.StatusOK, detectGitSourceResponse{Branch: branch, Result: res, NeedsBuildSettings: needs})
}

func (rt *Router) writeDetectError(w http.ResponseWriter, name, repoURL string, err error) {
	var se *repolayout.StatusError
	switch {
	case errors.As(err, &se) && se.Status == http.StatusNotFound:
		writeError(w, http.StatusNotFound, "the repository or branch was not found, or the stored token cannot see it")
	case errors.As(err, &se) && (se.Status == http.StatusUnauthorized || se.Status == http.StatusForbidden):
		writeError(w, http.StatusUnprocessableEntity, "the provider refused access to this repository, check the deploy token or app installation")
	default:
		rt.logger.Error("api: detect git source build failed", slog.String("error", err.Error()), slog.String("name", name), slog.String("repo_url", redactURLCredentials(repoURL)))
		writeError(w, http.StatusBadGateway, "could not read the repository file tree")
	}
}

func (rt *Router) repoLayoutHosts(ctx context.Context) repolayout.Hosts {
	var hosts repolayout.Hosts
	if rt.githubApp == nil {
		return hosts
	}
	conn, err := rt.githubApp.GetGitHubAppConnection(ctx)
	if err != nil {
		return hosts
	}
	if u, perr := url.Parse(conn.InstanceURL); perr == nil {
		hosts.GitHub = u.Host
	}
	return hosts
}

package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Builder is the surface the manual build trigger handler
// (handleTriggerBuild) needs from internal/deploy.Pipeline, the exact
// same narrow-interface shape internal/webhook.Deployer already
// establishes for the identical method, so both call sites can be
// satisfied by the one real *deploy.Pipeline cmd/levelrail/main.go
// builds. *deploy.Pipeline satisfies this structurally.
type Builder interface {
	Deploy(ctx context.Context, req deploy.Request, progress func(build.ProgressEvent)) (string, error)
	// DeploySpec is handleDeploySpec's (apps_multi.go) own narrow need:
	// the multi-service fan-out entry point. Same *deploy.Pipeline
	// satisfies both methods, so this stays one interface rather than a
	// second router field, the same "one concrete type, one interface"
	// shape every other narrow store/builder interface in this codebase
	// already follows.
	DeploySpec(ctx context.Context, req deploy.MultiRequest, progress func(serviceKey string, ev build.ProgressEvent)) ([]deploy.ServiceOutcome, error)
}

// fetchFunc fetches repoURL at ref to a local directory, authenticating
// with token when non-empty (see tokenForRepo), and returns the
// directory and a cleanup func that removes it. ref is a general git
// revision (branch, tag, or commit hash, resolved via gitCheckout's own
// ResolveRevision call), unlike internal/webhook's own always-a-full-SHA
// fetchFunc.
type fetchFunc func(ctx context.Context, repoURL, ref, token string) (dir string, cleanup func(), err error)

// gitCheckout is the real fetchFunc implementation, the manual-build
// counterpart to internal/webhook's own cloneAndCheckout (see that
// function's own doc comment for why a full clone, not shallow). token
// authenticates the same way gitCheckoutWithToken does (git_webhook.go):
// GitHub's "any username, token as password" scheme, empty meaning an
// unauthenticated clone.
func gitCheckout(ctx context.Context, repoURL, ref, token string) (dir string, cleanup func(), err error) {
	dir, err = os.MkdirTemp("", "levelrail-build-*")
	if err != nil {
		return "", nil, fmt.Errorf("api: create temp checkout dir: %w", err)
	}
	cleanup = func() {
		if rmErr := os.RemoveAll(dir); rmErr != nil {
			slog.Default().Error("api: failed to remove temp checkout dir", slog.String("dir", dir), slog.String("error", rmErr.Error()))
		}
	}

	cloneOpts := &git.CloneOptions{URL: repoURL}
	if token != "" {
		cloneOpts.Auth = &githttp.BasicAuth{Username: "x-access-token", Password: token}
	}
	repo, err := git.PlainCloneContext(ctx, dir, false, cloneOpts)
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("api: clone %q: %w", repoURL, err)
	}

	// ResolveRevision (not plumbing.NewHash) so ref can be a branch name,
	// tag, or short/full commit hash: NewHash would silently zero-pad
	// anything that isn't already valid 40-character hex, turning a
	// branch name like "main" into a nonexistent hash instead of a clear
	// error.
	hash, err := repo.ResolveRevision(plumbing.Revision(ref))
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("api: resolve ref %q in %q: %w", ref, repoURL, err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("api: get worktree for %q: %w", repoURL, err)
	}

	if err := wt.Checkout(&git.CheckoutOptions{Hash: *hash}); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("api: checkout %q at ref %q (resolved %s): %w", repoURL, ref, hash, err)
	}

	return dir, cleanup, nil
}

// triggerBuildBuildInput is the build.* sub-object of
// triggerBuildRequest, matching the same fields app.yaml's own build:
// block has (internal/spec.Build): type, path, and image.
type triggerBuildBuildInput struct {
	// Type defaults to spec.BuildDockerfile when empty. handleTriggerBuild
	// accepts every build.type internal/deploy.Pipeline.Deploy has a real
	// case for (dockerfile, railpack, static, image); only compose (and
	// any unrecognized value) is rejected, since Pipeline's own switch
	// statement still returns a clear "not yet supported" error for that
	// one.
	Type string `json:"type,omitempty"`
	// Path is only meaningful for build.type: dockerfile (the Dockerfile
	// location, relative to the checkout root) and build.type: static
	// (the built output directory to serve, relative to the checkout
	// root; see internal/deploy/static.go's deployStatic). It has no
	// meaning for build.type: railpack, whose own build.RailpackRequest
	// carries no path field at all (Railpack's provider detection always
	// runs against the checkout root), so handleTriggerBuild rejects a
	// railpack request that sets one rather than silently ignoring it.
	Path string `json:"path,omitempty"`
	// BaseDirectory scopes the build context (and Path, for dockerfile)
	// to a subdirectory of the checkout, for a monorepo where this
	// service doesn't live at the repo root. Meaningful for dockerfile,
	// railpack, and static; rejected for image below.
	BaseDirectory string `json:"base_directory,omitempty"`
	// Image is required for, and only meaningful for, build.type: image:
	// a prebuilt registry reference deployed as-is, no git checkout or
	// build at all (internal/deploy.Pipeline.deployImage). repo_url/ref
	// are not required on the request for this build type either, since
	// nothing gets cloned.
	Image string `json:"image,omitempty"`
	// Args are Dockerfile build-time ARG values, passed through to
	// BuildKit as --build-arg equivalents. Only meaningful for
	// build.type: dockerfile; handleTriggerBuild rejects a non-empty
	// value for any other build type, the same "fail loudly on a
	// meaningless field" pattern Path's own doc comment establishes for
	// railpack.
	Args map[string]string `json:"args,omitempty"`
}

// triggerBuildRequest is POST /api/v1/apps/{name}/builds's body: a git
// source (repo_url, ref) plus how to build it, everything
// internal/deploy.Pipeline needs beyond what the app's own already-saved
// desired state (store.DesiredService) already supplies. See
// specServiceFromDesired's doc comment for exactly which fields come
// from where and why: store.DesiredService has no column for any of
// repo_url/ref/build type/build path (confirmed via internal/store,
// internal/spec: no app in this codebase has a stored git or build
// config today), so this request body is the only way to supply them,
// every call, rather than a one-time stored setting a later call could
// omit.
type triggerBuildRequest struct {
	// RepoURL is the git remote to clone, e.g.
	// "https://github.com/org/app.git". Required for every build.type
	// except image, which clones nothing (build.image is already a real,
	// already-pushed image reference).
	RepoURL string `json:"repo_url"`
	// Ref is the branch, tag, or commit hash to build, resolved via
	// gitCheckout's ResolveRevision call. Required for every build.type
	// except image, the same exception RepoURL's own doc comment gives.
	Ref string `json:"ref"`
	// ImageRepo is the image name without a tag, the same meaning
	// deploy.Request.ImageRepo already documents. Defaults to the app's
	// own name when empty, since (like RepoURL/Ref) nothing else stores
	// a per-app image repo naming policy today.
	ImageRepo string                 `json:"image_repo,omitempty"`
	Build     triggerBuildBuildInput `json:"build,omitempty"`
}

// triggerBuildResponse is POST /api/v1/apps/{name}/builds's success
// body: just the deploy_attempts row's id (same "id" tag
// deployAttemptResource uses), since the build hasn't run yet by the
// time this returns. Poll GET .../deploy-attempts or tail
// GET .../deploys/{id}/logs for progress and outcome.
type triggerBuildResponse struct {
	ID string `json:"id,omitempty"`
}

// isGitHubHTTPSRepoURL reports whether repoURL is an https remote on
// instanceHost: the only shape a GitHub App installation token can
// authenticate a clone against. instanceHost is github.com for the
// public GitHub App, or a GitHub Enterprise Server host for a
// self-hosted connection (store.GitHubAppConnection.InstanceURL).
func isGitHubHTTPSRepoURL(repoURL, instanceHost string) bool {
	u, err := url.Parse(repoURL)
	return err == nil && u.Scheme == "https" && u.Host == instanceHost
}

// tokenForRepo mints a GitHub App installation token for repoURL when
// possible, falling back to an empty (unauthenticated) token for any
// other host or a minting failure: the plain public-repo clone path must
// keep working regardless. Checks the connected instance's own host
// (GHE or github.com) rather than assuming github.com, so a private
// repo on a GitHub Enterprise Server installation authenticates too.
func (rt *Router) tokenForRepo(ctx context.Context, repoURL string) string {
	if rt.githubAppSecrets == nil {
		return ""
	}
	conn, err := rt.githubApp.GetGitHubAppConnection(ctx)
	if err != nil {
		return ""
	}
	instanceHost := "github.com"
	if u, parseErr := url.Parse(conn.InstanceURL); parseErr == nil && u.Host != "" {
		instanceHost = u.Host
	}
	if !isGitHubHTTPSRepoURL(repoURL, instanceHost) {
		return ""
	}

	_, token, err := rt.mintGitHubAppInstallationToken(ctx)
	if err != nil {
		if !errors.Is(err, errGitHubAppNotConnected) && !errors.Is(err, errGitHubAppNotInstalled) {
			rt.logger.Warn("api: trigger build: mint github app installation token failed, falling back to an unauthenticated clone", slog.String("error", err.Error()))
		}
		return ""
	}
	return token
}

// handleTriggerBuild handles POST /api/v1/apps/{name}/builds: builds an
// image from a git source and points the app's desired state at the
// result, through the same internal/deploy.Pipeline
// internal/webhook.Handler.ServeHTTP uses for a git-push-triggered
// deploy (see Builder's own doc comment). build.type: image is the one
// exception: no git source and no build at all, just build.image
// deployed as-is (internal/deploy.Pipeline.deployImage).
//
// Asynchronous: validates the request, records a deploy_attempts row
// (beginBuildDeployAttempt), and returns 202 with its id immediately,
// the same shape handleTriggerBackup already establishes for
// long-running work. The fetch and build run in a goroutine against
// context.Background(), not r.Context(), which is cancelled the moment
// this handler returns.
//
// See specServiceFromDesired's own doc comment for the fidelity losses
// this reconstruction carries versus the original app.yaml.
func (rt *Router) handleTriggerBuild(w http.ResponseWriter, r *http.Request) {
	if rt.builder == nil {
		writeError(w, http.StatusNotImplemented, "manual build trigger is not configured on this control plane")
		return
	}

	name := r.PathValue("name")

	existing, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: trigger build: load app failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	var req triggerBuildRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	buildType := req.Build.Type
	if buildType == "" {
		buildType = spec.BuildDockerfile
	}
	switch buildType {
	case spec.BuildDockerfile, spec.BuildRailpack, spec.BuildStatic, spec.BuildImage:
		// Supported: internal/deploy.Pipeline.Deploy has a real case for
		// each of these four.
	case spec.BuildCompose:
		// Fails before any git I/O: a manual build trigger rebuilds one
		// already-existing single service, but a compose file always
		// declares its own set of services, which can only ever be
		// expanded into a real multi-service deploy (POST
		// /api/v1/apps/{name}/deploy-spec, internal/deploy.Pipeline.
		// DeploySpec's own expandComposeServices), never a single one, so
		// there's nothing worth cloning first to find out.
		writeError(w, http.StatusBadRequest, fmt.Sprintf("build.type %q is only supported via a multi-service deploy (POST /api/v1/apps/{name}/deploy-spec), not a manual build trigger", buildType))
		return
	default:
		writeError(w, http.StatusBadRequest, fmt.Sprintf("build.type %q is not recognized", buildType))
		return
	}

	if buildType == spec.BuildImage {
		if req.Build.Image == "" {
			writeError(w, http.StatusBadRequest, "build.image is required for build.type \"image\"")
			return
		}
		if req.Build.BaseDirectory != "" {
			writeError(w, http.StatusBadRequest, "build.base_directory is not meaningful for build.type \"image\"")
			return
		}
	} else {
		if req.RepoURL == "" {
			writeError(w, http.StatusBadRequest, "repo_url is required")
			return
		}
		if req.Ref == "" {
			writeError(w, http.StatusBadRequest, "ref is required")
			return
		}
	}
	if buildType == spec.BuildRailpack && req.Build.Path != "" {
		// See triggerBuildBuildInput.Path's own doc comment: railpack has
		// no path concept at all, so a caller-supplied one here is a
		// mistake worth failing loudly on, not a value that would
		// otherwise be silently discarded by deployRailpack.
		writeError(w, http.StatusBadRequest, "build.path is not meaningful for build.type \"railpack\"")
		return
	}
	if buildType != spec.BuildDockerfile && len(req.Build.Args) > 0 {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("build.args is not meaningful for build.type %q", buildType))
		return
	}

	imageRepo := req.ImageRepo
	if imageRepo == "" {
		imageRepo = name
	}

	buildCfg := spec.Build{Type: buildType, Path: req.Build.Path, BaseDirectory: req.Build.BaseDirectory, Args: req.Build.Args}
	if buildType == spec.BuildImage {
		buildCfg = spec.Build{Type: buildType, Image: req.Build.Image}
	}
	svcSpec := specServiceFromDesired(*existing, buildCfg)

	buildReq := deploy.Request{
		ServiceName: name,
		Service:     svcSpec,
		CommitSHA:   req.Ref,
		ImageRepo:   imageRepo,
	}

	id, progress, finishAttempt := rt.beginBuildDeployAttempt(r.Context(), buildReq, store.DeployAttemptSourceManual)

	// AbilityDeploy alone (this route's own gate) is not enough to
	// authorize minting a live GitHub App installation token: repoURL is
	// fully caller-controlled and need not have anything to do with this
	// app, so an unscoped mint here would let any AbilityDeploy-only
	// caller (e.g. a CI token meant only to trigger builds) read any
	// private repo the org's installation can reach, the exact class of
	// disclosure handleListGitHubAppRepos already gates at
	// AbilityReadSensitive for the same reason. Checked once, in request
	// scope, before the goroutine below outlives r.
	allowPrivateRepoAuth := rt.callerHasAbility(r, AbilityReadSensitive)

	repoURL, ref := req.RepoURL, req.Ref
	go func() { //nolint:gosec // deliberately not r.Context(): it is cancelled the moment this handler returns, which would abort the fetch and build within microseconds of starting them; see this handler's own doc comment
		ctx := context.Background()
		if buildType != spec.BuildImage {
			var token string
			if allowPrivateRepoAuth {
				token = rt.tokenForRepo(ctx, repoURL)
			}
			sourceDir, cleanup, err := rt.fetch(ctx, repoURL, ref, token)
			if err != nil {
				rt.logger.Error("api: trigger build: fetch source failed", slog.String("error", err.Error()), slog.String("name", name), slog.String("repo_url", repoURL), slog.String("ref", ref))
				finishAttempt(err)
				return
			}
			defer cleanup()
			buildReq.SourceDir = sourceDir
		}

		tag, err := rt.builder.Deploy(ctx, buildReq, progress)
		finishAttempt(err)
		if err != nil {
			// Non-leaky, matching internal/webhook.Handler.ServeHTTP's own
			// choice for an identical failure (its own doc comment: "500
			// with a non-leaky message if the deploy itself fails"): a
			// build failure can carry internal detail (daemon socket
			// paths, registry hostnames) that has no business reaching an
			// HTTP response body, logged here instead. There's no HTTP
			// response left to write by this point anyway; the deploy
			// attempt row (already marked failed by finishAttempt above)
			// is how a caller learns the outcome.
			rt.logger.Error("api: trigger build failed", slog.String("error", err.Error()), slog.String("name", name), slog.String("repo_url", repoURL), slog.String("ref", ref))
			return
		}
		rt.logger.Info("api: manual build triggered", slog.String("name", name), slog.String("repo_url", repoURL), slog.String("ref", ref), slog.String("build_type", buildType), slog.String("tag", tag))
	}()

	writeJSON(w, http.StatusAccepted, triggerBuildResponse{ID: id})
}

// specServiceFromDesired reconstructs the parts of a spec.Service that
// store.DesiredService can represent, so a manual build's
// internal/deploy.Pipeline call has a full spec.Service to build
// against, the same input shape internal/webhook already gives it from a
// parsed app.yaml. buildCfg is passed straight through as the
// reconstructed Service's Build block, since store.DesiredService has no
// build.type/build.path column at all: confirmed via internal/store (no
// RepoURL/GitRef/Build fields anywhere in service.go) and
// internal/api/router.go's own package doc comment ("Deploy trigger... doesn't
// build anything"). This reverses internal/deploy's own toDesiredService
// (internal/deploy/translate.go), field for field, wherever a reverse is
// possible.
//
// Two known, deliberate fidelity losses versus the original app.yaml:
//
//   - Env vars marked { secret: true } lose their original
//     { required: true } flag: store.DesiredService.SecretEnv only ever
//     persists the name (see that field's own doc comment: "a name is
//     not a secret, only the value is"), never whether it was required.
//     Reconstructed here as Required: false, the permissive direction: a
//     missing secret value will not block this manual build the way it
//     would have blocked the original app.yaml-driven deploy.
//     internal/reconcile/application's own container-create step still
//     simply omits an unset optional secret var either way, so this
//     never produces a container silently missing a value that WAS set,
//     only a looser pre-build check than app.yaml's own `required: true`
//     would have enforced.
//   - { from: ... } cross-resource references can never be reconstructed:
//     store.DesiredService.Env only ever holds already-resolved literal
//     values (internal/deploy's own literalEnv), so a service that
//     originally declared { from: postgres.main.url } has no trace of
//     that left to rebuild. Not a new gap this endpoint introduces: the
//     same already-resolved-only shape already applies to every other
//     reader of DesiredService in this codebase (e.g. GET
//     /api/v1/apps/{name}).
func specServiceFromDesired(svc store.DesiredService, buildCfg spec.Build) spec.Service {
	out := spec.Service{
		Build:   buildCfg,
		Domains: svc.Domains,
		Port:    svc.Port,
		Labels:  svc.Labels,
	}
	if len(svc.Env) > 0 || len(svc.SecretEnv) > 0 {
		out.Env = make(map[string]spec.EnvVar, len(svc.Env)+len(svc.SecretEnv))
		for k, v := range svc.Env {
			out.Env[k] = spec.EnvVar{Value: v}
		}
		for _, k := range svc.SecretEnv {
			out.Env[k] = spec.EnvVar{Secret: true}
		}
	}
	if svc.Resources != nil {
		out.Resources = specResourcesFromStore(*svc.Resources)
	}
	if svc.Health != nil {
		out.Health = specHealthFromStore(*svc.Health)
	}
	return out
}

func specResourcesFromStore(r store.ServiceResources) *spec.Resources {
	out := &spec.Resources{}
	if r.MemoryBytes > 0 {
		out.Memory = formatMemoryBytes(r.MemoryBytes)
	}
	if r.NanoCPUs > 0 {
		out.CPU = float64(r.NanoCPUs) / 1e9
	}
	return out
}

// formatMemoryBytes reverses internal/deploy's own parseMemoryBytes
// (internal/deploy/translate.go), which only ever accepts app.yaml's
// "512Mi"/"1Gi" shape. Prefers Gi when bytes divides evenly into it,
// matching how an operator would most naturally have written a large
// limit by hand; falls back to Mi via integer division otherwise, which
// is exact for every value this codebase's own UI can ever have produced
// (web/src/components/ResourceLimitsEditor.tsx's own comment: "a
// whole-number MiB field... chosen specifically to keep the byte
// round-trip lossless"), and only lossy for a byte count nothing in this
// codebase's UI can actually create (e.g. one written directly through
// PUT /api/v1/apps/{name} by hand).
func formatMemoryBytes(bytes int64) string {
	const mi = 1024 * 1024
	const gi = mi * 1024
	if bytes%gi == 0 {
		return fmt.Sprintf("%dGi", bytes/gi)
	}
	return fmt.Sprintf("%dMi", bytes/mi)
}

func specHealthFromStore(h store.ServiceHealth) *spec.Health {
	out := &spec.Health{}
	if h.Readiness != nil {
		p := specProbeFromStore(*h.Readiness)
		out.Readiness = &p
	}
	if h.Liveness != nil {
		p := specProbeFromStore(*h.Liveness)
		out.Liveness = &p
	}
	return out
}

// specProbeFromStore reverses internal/deploy's own toServiceProbe:
// time.Duration's own String method (e.g. 5*time.Second -> "5s") is a
// real inverse of time.ParseDuration, which is exactly what
// internal/deploy/translate.go's parseDurationOrZero parses this string
// back through on the next forward pass.
func specProbeFromStore(p store.ServiceProbe) spec.Probe {
	out := spec.Probe{Path: p.Path, Failures: p.Failures}
	if p.Interval > 0 {
		out.Interval = p.Interval.String()
	}
	if p.Timeout > 0 {
		out.Timeout = p.Timeout.String()
	}
	return out
}

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
// directory, the full commit hash ref resolved to, and a cleanup func
// that removes it. ref is a general git revision (branch, tag, or commit
// hash, resolved via gitCheckout's own ResolveRevision call), unlike
// internal/webhook's own always-a-full-SHA fetchFunc, so the resolved
// hash is the only thing callers can safely tag a built image with: a
// branch name moves, and reusing it as a tag silently retags it onto
// newer content, orphaning the previous build as a rollback target.
type fetchFunc func(ctx context.Context, repoURL, ref, token string) (dir string, commit string, cleanup func(), err error)

// gitCheckout is the real fetchFunc implementation, the manual-build
// counterpart to internal/webhook's own cloneAndCheckout (see that
// function's own doc comment for why a full clone, not shallow). token
// authenticates the same way gitCheckoutWithToken does (git_webhook.go):
// GitHub's "any username, token as password" scheme, empty meaning an
// unauthenticated clone.
func gitCheckout(ctx context.Context, repoURL, ref, token string) (dir string, commit string, cleanup func(), err error) {
	dir, err = os.MkdirTemp("", "levelrail-build-*")
	if err != nil {
		return "", "", nil, fmt.Errorf("api: create temp checkout dir: %w", err)
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
		return "", "", nil, fmt.Errorf("api: clone %q: %w", repoURL, err)
	}

	// ResolveRevision (not plumbing.NewHash) so ref can be a branch name,
	// tag, or short/full commit hash: NewHash would silently zero-pad
	// anything that isn't already valid 40-character hex, turning a
	// branch name like "main" into a nonexistent hash instead of a clear
	// error.
	hash, err := repo.ResolveRevision(plumbing.Revision(ref))
	if err != nil {
		cleanup()
		return "", "", nil, fmt.Errorf("api: resolve ref %q in %q: %w", ref, repoURL, err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		cleanup()
		return "", "", nil, fmt.Errorf("api: get worktree for %q: %w", repoURL, err)
	}

	if err := wt.Checkout(&git.CheckoutOptions{Hash: *hash}); err != nil {
		cleanup()
		return "", "", nil, fmt.Errorf("api: checkout %q at ref %q (resolved %s): %w", repoURL, ref, hash, err)
	}

	return dir, hash.String(), cleanup, nil
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
	// The built image is tagged with the resolved commit hash, not with
	// this string (see fetchFunc).
	Ref string `json:"ref"`
	// ImageRepo is the image name without a tag, the same meaning
	// deploy.Request.ImageRepo already documents. Defaults to the app's
	// own name when empty, since (like RepoURL/Ref) nothing else stores
	// a per-app image repo naming policy today.
	ImageRepo string                 `json:"image_repo,omitempty"`
	Build     triggerBuildBuildInput `json:"build,omitempty"`
	// DetectedFramework is the human-readable framework name the wizard's
	// own pre-flight call to POST /api/v1/build/detect already reported
	// for this exact repo/ref, passed through here purely to be stored on
	// the resulting deploy_attempts row (see handleDetectFramework's own
	// doc comment): this handler never re-runs detection itself.
	DetectedFramework string `json:"detected_framework,omitempty"`
	freezeOverride
}

// triggerBuildResponse is POST /api/v1/apps/{name}/builds's success
// body: just the deploy_attempts row's id (same "id" tag
// deployAttemptResource uses), since the build hasn't run yet by the
// time this returns. Poll GET .../deploy-attempts or tail
// GET .../deploys/{id}/logs for progress and outcome.
type triggerBuildResponse struct {
	ID string `json:"id,omitempty"`
	// Status is "queued" when the build waits behind another deploy, with
	// QueuePosition and WaitReason saying why; empty when it started at once.
	Status        string `json:"status,omitempty"`
	QueuePosition int    `json:"queue_position,omitempty"`
	WaitReason    string `json:"wait_reason,omitempty"`
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
//
// A minting failure past the two expected sentinels (not connected / not
// installed, e.g. the App got suspended on GitHub's side) is logged at
// Error, not Warn: repoURL matched the connected instance's own host, so
// this is very likely a private repo about to fail an unauthenticated
// clone anyway, and that later clone error alone won't say why.
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
			rt.logger.Error("api: mint github app installation token failed, falling back to an unauthenticated clone that will likely fail for a private repo",
				slog.String("error", err.Error()), slog.String("repo_url", repoURL))
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
	freezeNote, ok := rt.freezeGate(w, r, name, req.freezeOverride)
	if !ok {
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
		if err := requireHTTPOrHTTPSScheme(req.RepoURL); err != nil {
			writeError(w, http.StatusBadRequest, "repo_url must use http or https")
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

	// AbilityDeploy alone (this route's own gate) is not enough to
	// authorize minting a live GitHub App installation token: repoURL is
	// fully caller-controlled, so an unscoped mint would let any
	// deploy-only caller read any private repo the org's installation can
	// reach. Checked once, in request scope, before the goroutine outlives r.
	allowPrivateRepoAuth := rt.callerHasAbility(r, AbilityReadSensitive)

	buildReq := manualBuildRequest(name, *existing, req, buildType)
	source := store.DeployAttemptSourceManual

	rt.buildStartMu.Lock()
	if rt.deploySafety != nil {
		if rt.deployBlocker(r.Context(), name) {
			resp, err := rt.enqueueManualBuild(r.Context(), *existing, req, buildReq, freezeNote, allowPrivateRepoAuth)
			rt.buildStartMu.Unlock()
			if err != nil {
				rt.logger.Error("api: trigger build: queue failed", slog.String("error", err.Error()), slog.String("name", name))
				writeError(w, http.StatusInternalServerError, "internal error")
				return
			}
			writeJSON(w, http.StatusAccepted, resp)
			return
		}
	} else if rt.hasRunningDeployAttempt(r.Context(), name) {
		rt.buildStartMu.Unlock()
		writeError(w, http.StatusConflict, "a deploy for this app is already running")
		return
	}
	run := rt.beginManualBuild(r.Context(), *existing, buildReq, buildType, source, req.DetectedFramework, freezeNote, allowPrivateRepoAuth)
	rt.buildStartMu.Unlock()

	run.repoURL, run.ref = req.RepoURL, req.Ref
	go rt.runManualBuild(run) //nolint:gosec // the build outlives the request; runManualBuild derives its own context

	writeJSON(w, http.StatusAccepted, triggerBuildResponse{ID: run.id})
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
// One known, deliberate fidelity loss versus the original app.yaml:
//
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
	if len(svc.Env) > 0 || len(svc.SecretEnv) > 0 || len(svc.VaultEnv) > 0 {
		out.Env = make(map[string]spec.EnvVar, len(svc.Env)+len(svc.SecretEnv)+len(svc.VaultEnv))
		for k, v := range svc.Env {
			out.Env[k] = spec.EnvVar{Value: v}
		}
		// Required carries through from store.DesiredService.SecretEnv
		// (SecretEnvRef.Required), not just the name: a required-but-unset
		// secret must reject this build the same way it already rejects a
		// fresh app.yaml-driven deploy (internal/deploy.Pipeline.
		// validateEnv), see SecretEnvRef's own doc comment for why this
		// used to be lossy here.
		for _, ref := range svc.SecretEnv {
			out.Env[ref.Name] = spec.EnvVar{Secret: true, Required: ref.Required}
		}
		// VaultEnv reconstructs exactly, unlike SecretEnv above: it stores
		// the full { path, key } reference, not just a name, so there is
		// no fidelity loss to call out here.
		for k, ref := range svc.VaultEnv {
			out.Env[k] = spec.EnvVar{Vault: &spec.VaultRef{Path: ref.Path, Key: ref.Key}}
		}
	}
	if svc.Resources != nil {
		out.Resources = specResourcesFromStore(*svc.Resources)
	}
	if svc.Health != nil {
		out.Health = specHealthFromStore(*svc.Health)
	}
	if svc.Egress != nil {
		out.Egress = specEgressFromStore(*svc.Egress)
	}
	return out
}

// specEgressFromStore reverses internal/deploy's own toServiceEgressPolicy
// exactly: unlike SecretEnv above, ServiceEgressPolicy stores every field
// an app.yaml egress: block can express, so there's no fidelity loss to
// call out here either. Without this, a build-triggered redeploy
// (specServiceFromDesired's own caller) would silently wipe out an
// already-configured egress policy the next time it runs
// toDesiredService/SaveDesiredService, since that call replaces the
// whole record.
func specEgressFromStore(e store.ServiceEgressPolicy) *spec.Egress {
	allow := make([]spec.EgressAllow, len(e.Allow))
	for i, a := range e.Allow {
		allow[i] = spec.EgressAllow{Host: a.Host, Port: a.Port}
	}
	return &spec.Egress{Mode: e.Mode, Allow: allow}
}

func specResourcesFromStore(r store.ServiceResources) *spec.Resources {
	out := &spec.Resources{}
	if r.MemoryBytes > 0 {
		out.Memory = formatMemoryBytes(r.MemoryBytes)
	}
	if r.NanoCPUs > 0 {
		out.CPU = float64(r.NanoCPUs) / 1e9
	}
	if r.SwapMemoryBytes > 0 {
		out.SwapMemory = formatMemoryBytes(r.SwapMemoryBytes)
	}
	out.CPUSet = r.CPUSetCPUs
	if r.GPU != nil {
		out.GPU = &spec.GPU{Count: r.GPU.Count, Devices: r.GPU.DeviceIDs}
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
	if h.ReadyTimeout > 0 {
		out.ReadyTimeout = h.ReadyTimeout.String()
	}
	return out
}

// specProbeFromStore reverses internal/deploy's toServiceProbe.
func specProbeFromStore(p store.ServiceProbe) spec.Probe {
	out := spec.Probe{
		Path:            p.Path,
		Scheme:          p.Scheme,
		Host:            p.Host,
		TLSSkipVerify:   p.TLSSkipVerify,
		FollowRedirects: p.FollowRedirects,
		ExpectedStatus:  spec.StatusCodes(p.ExpectedStatus),
		Exec:            spec.ExecCommand(p.Exec),
		Failures:        p.Failures,
	}
	if p.Interval > 0 {
		out.Interval = p.Interval.String()
	}
	if p.Timeout > 0 {
		out.Timeout = p.Timeout.String()
	}
	return out
}

// hasRunningDeployAttempt reports whether name already has a deploy
// attempt in flight. A history read failure counts as "none running" so a
// broken history table never blocks deploys.
func (rt *Router) hasRunningDeployAttempt(ctx context.Context, name string) bool {
	attempts, err := rt.deployAttempts.ListDeployAttempts(ctx, name)
	if err != nil {
		rt.logger.Error("api: trigger build: list deploy attempts failed", slog.String("error", err.Error()), slog.String("name", name))
		return false
	}
	for _, a := range attempts {
		if a.Status == store.DeployAttemptStatusRunning {
			return true
		}
	}
	return false
}

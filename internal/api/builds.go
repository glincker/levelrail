package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"

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
}

// fetchFunc fetches repoURL at ref to a local directory, returning the
// directory and a cleanup func that removes it. Mirrors
// internal/webhook's own fetchFunc seam exactly (same signature shape,
// same "overridable in tests, defaults to a real git implementation"
// story), except ref here is a general git revision (branch, tag, or
// commit hash, resolved via gitCheckout's own ResolveRevision call)
// rather than webhook's always-a-full-commit-SHA payload field, since a
// manual build trigger's most common input is "build my main branch",
// not a hand-typed SHA.
type fetchFunc func(ctx context.Context, repoURL, ref string) (dir string, cleanup func(), err error)

// gitCheckout is the real fetchFunc implementation, the manual-build
// counterpart to internal/webhook's own cloneAndCheckout. Deliberately
// duplicated rather than shared through a new common package: the two
// differ in exactly one meaningful way (ref resolution, see fetchFunc's
// own doc comment above), and internal/webhook is explicitly this task's
// reference implementation to read, not a dependency to extend, per this
// task's own scope note to keep changes to reconciler-adjacent,
// webhook-adjacent code minimal. See cloneAndCheckout's own doc comment
// for the full "why a full clone, not shallow" reasoning, which applies
// unchanged here.
func gitCheckout(ctx context.Context, repoURL, ref string) (dir string, cleanup func(), err error) {
	dir, err = os.MkdirTemp("", "levelrail-build-*")
	if err != nil {
		return "", nil, fmt.Errorf("api: create temp checkout dir: %w", err)
	}
	cleanup = func() {
		if rmErr := os.RemoveAll(dir); rmErr != nil {
			slog.Default().Error("api: failed to remove temp checkout dir", slog.String("dir", dir), slog.String("error", rmErr.Error()))
		}
	}

	repo, err := git.PlainCloneContext(ctx, dir, false, &git.CloneOptions{
		URL: repoURL,
	})
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
// triggerBuildRequest, matching the same two fields app.yaml's own
// build: block has (internal/spec.Build): type and path.
type triggerBuildBuildInput struct {
	// Type defaults to spec.BuildDockerfile when empty: this is the only
	// build type internal/deploy.Pipeline.Deploy actually performs a
	// build for today (its own switch statement returns a clear "not yet
	// supported" error for compose/railpack/static), so defaulting to
	// the one that works is more useful than forcing every caller to
	// spell it out.
	Type string `json:"type,omitempty"`
	Path string `json:"path,omitempty"`
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
	// "https://github.com/org/app.git". Always required: unlike
	// internal/webhook.Config.RepoURL (fixed, operator-configured
	// server-side per app), there is nowhere else this could come from
	// for an arbitrary app today.
	RepoURL string `json:"repo_url"`
	// Ref is the branch, tag, or commit hash to build, resolved via
	// gitCheckout's ResolveRevision call. Always required.
	Ref string `json:"ref"`
	// ImageRepo is the image name without a tag, the same meaning
	// deploy.Request.ImageRepo already documents. Defaults to the app's
	// own name when empty, since (like RepoURL/Ref) nothing else stores
	// a per-app image repo naming policy today.
	ImageRepo string                 `json:"image_repo,omitempty"`
	Build     triggerBuildBuildInput `json:"build,omitempty"`
}

// triggerBuildResponse is POST /api/v1/apps/{name}/builds's success body:
// the full image reference that was built (internal/deploy.Pipeline.Deploy's
// own return value) plus the app's resulting desired state, the same
// resource shape GET /api/v1/apps/{name} and handleTriggerDeploy's own
// response already return.
type triggerBuildResponse struct {
	Image string      `json:"image"`
	App   appResource `json:"app"`
}

// handleTriggerBuild handles POST /api/v1/apps/{name}/builds: builds an
// image from a git source and points the app's desired state at the
// result, through the exact same internal/deploy.Pipeline
// internal/webhook.Handler.ServeHTTP already calls for a git-push-triggered
// deploy (see Builder's own doc comment). This is the gap TASKS.md /
// router.go's own package doc comment names directly: "Wiring an actual
// 'build then deploy' endpoint is a follow-up once internal/deploy's API
// is stable" -- for an operator with no working git webhook configured
// (internal/webhook.Config's own doc comment: deliberately static,
// single-app configuration), this is the only path from the dashboard
// into a real build.
//
// Synchronous and blocking, exactly like internal/webhook.Handler.ServeHTTP's
// own call to Deploy: this handler's HTTP request does not return until
// the build itself finishes, and build progress is only logged via slog
// (build.SlogProgress), never streamed back to the caller. This mirrors
// the reference implementation deliberately rather than inventing new
// plumbing: web/src/hooks/useDeployLogStream.ts's own doc comment
// documents that its SSE contract is "assumed," not backed by any real
// internal/api route, and TASKS-v2.md's own build log lists the
// deploy-attempt-log design (a deploy_attempts store table plus a
// dedicated SSE endpoint) as a real, store-schema-sized follow-up that
// still needs founder sign-off, not something to improvise here. A user
// triggering a manual build gets a blocking request while it builds
// (the same UX the webhook path already has, just from a browser
// instead of GitHub) rather than a progress bar backed by
// infrastructure that doesn't exist yet.
//
// See specServiceFromDesired's own doc comment for the two known,
// deliberate fidelity losses this reconstruction carries versus the
// original app.yaml (secret env vars' `required` flag, `{ from: ... }`
// cross-resource references).
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
	if req.RepoURL == "" {
		writeError(w, http.StatusBadRequest, "repo_url is required")
		return
	}
	if req.Ref == "" {
		writeError(w, http.StatusBadRequest, "ref is required")
		return
	}

	buildType := req.Build.Type
	if buildType == "" {
		buildType = spec.BuildDockerfile
	}
	if buildType != spec.BuildDockerfile {
		// Fails before any git I/O: a build type internal/deploy.Pipeline
		// can never build is a fixed fact about this control plane's
		// current capability, not something a valid repo_url/ref could
		// ever fix, so there's nothing worth cloning first to find out.
		writeError(w, http.StatusNotImplemented, fmt.Sprintf("build.type %q is not yet supported for a manual build trigger", buildType))
		return
	}

	imageRepo := req.ImageRepo
	if imageRepo == "" {
		imageRepo = name
	}

	svcSpec := specServiceFromDesired(*existing, spec.Build{Type: buildType, Path: req.Build.Path})

	sourceDir, cleanup, err := rt.fetch(r.Context(), req.RepoURL, req.Ref)
	if err != nil {
		rt.logger.Error("api: trigger build: fetch source failed", slog.String("error", err.Error()), slog.String("name", name), slog.String("repo_url", req.RepoURL), slog.String("ref", req.Ref))
		writeError(w, http.StatusBadRequest, "fetching source failed: check repo_url and ref")
		return
	}
	defer cleanup()

	buildReq := deploy.Request{
		ServiceName: name,
		Service:     svcSpec,
		SourceDir:   sourceDir,
		CommitSHA:   req.Ref,
		ImageRepo:   imageRepo,
	}

	tag, err := rt.builder.Deploy(r.Context(), buildReq, build.SlogProgress(rt.logger))
	if err != nil {
		// Non-leaky, matching internal/webhook.Handler.ServeHTTP's own
		// choice for an identical failure (its own doc comment: "500
		// with a non-leaky message if the deploy itself fails"): a build
		// failure can carry internal detail (daemon socket paths,
		// registry hostnames) that has no business reaching an HTTP
		// response body, logged here instead.
		rt.logger.Error("api: trigger build failed", slog.String("error", err.Error()), slog.String("name", name), slog.String("repo_url", req.RepoURL), slog.String("ref", req.Ref))
		writeError(w, http.StatusInternalServerError, "build failed")
		return
	}

	updated, err := rt.apps.GetDesiredService(r.Context(), name)
	if err != nil {
		rt.logger.Error("api: trigger build: reload after build failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	rt.logger.Info("api: manual build triggered", slog.String("name", name), slog.String("repo_url", req.RepoURL), slog.String("ref", req.Ref), slog.String("tag", tag))
	writeJSON(w, http.StatusAccepted, triggerBuildResponse{Image: tag, App: toAppResource(*updated)})
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

package build

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"time"

	bkclient "github.com/moby/buildkit/client"
	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/session"
	"github.com/moby/buildkit/session/secrets/secretsprovider"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
	rpbuildkit "github.com/railwayapp/railpack/buildkit"
	"github.com/railwayapp/railpack/buildkit/build_llb"
	"github.com/railwayapp/railpack/core"
	rpapp "github.com/railwayapp/railpack/core/app"
	rpplan "github.com/railwayapp/railpack/core/plan"
	"github.com/tonistiigi/fsutil"
	"golang.org/x/sync/errgroup"
)

// This file: the Railpack integration, scoped to node, golang, java,
// python, ruby, and php (see supportedRailpackProviders). Railpack
// (github.com/railwayapp/railpack) is a real, embeddable Go library, not
// a CLI wrapped by exec.Command: core.GenerateBuildPlan inspects a source
// directory and returns Railpack's own build IR (a graph of named steps,
// not a Dockerfile), and buildkit.ConvertPlanToLLB turns that IR directly
// into a BuildKit llb.State plus an OCI image config. Both facts mean a
// Railpack build cannot reuse newSolveOpt (solve.go), which is built
// entirely around the dockerfile.v0 frontend: it needs its own solve
// path, built here, that hands BuildKit a marshaled llb.Definition
// instead of a Dockerfile frontend, and carries the image config Railpack
// computed as an exporter attribute since there's no Dockerfile FROM/CMD
// for BuildKit to read it from otherwise. A real third-party
// integration, unbindapp/unbind-api, does exactly this.
//
// Everything below reuses the existing hijacked-/grpc BuildKit connection
// (client.go's Client.bk) and the same relayProgress/loadImage helpers
// build.go already established for the Dockerfile path: a Railpack build
// still exports a docker-save-style tar and loads it into the local image
// store via the Docker Engine API, never a registry push, and still
// reports progress on the same build.ProgressEvent channel.

// RailpackRequest describes a single Railpack-detected build: no
// Dockerfile, no user-authored build steps, just a source directory
// Railpack inspects itself to determine how to build it.
type RailpackRequest struct {
	// SourceDir is the app's source root Railpack inspects to detect its
	// provider and generate a build plan, and the build context BuildKit
	// reads from. Required.
	SourceDir string

	// Tag is the image reference given to the built image, e.g.
	// "levelrail/thesvg:abc1234". Required.
	Tag string

	// CgroupParent places every build step in a pre-sized cgroup. Set by the Limiter; local builds only.
	CgroupParent string
	// NodeHeapMB, when positive, sets NODE_OPTIONS=--max-old-space-size for Node builds that did not set NODE_OPTIONS.
	NodeHeapMB int
	// Network is the RUN step policy; the Client fills it from APP_BUILD_NETWORK.
	Network NetworkMode
	// Env is build-time env, as Railpack's own --env does. Callers pass only
	// public-prefix literals; it reaches every build step as an env var.
	Env map[string]string
	// CacheID scopes Railpack cache mounts (.next/cache, package stores) to one app. Empty derives it from Tag.
	CacheID string

	perAppCache bool
}

// ErrRailpackSourceDirRequired and ErrRailpackTagRequired are returned by
// Validate for the two required fields, matching Request.Validate's own
// "distinguishable sentinel error" shape for the Dockerfile path.
var (
	ErrRailpackSourceDirRequired = errors.New("build: railpack: source dir is required")
	ErrRailpackTagRequired       = errors.New("build: railpack: tag is required")
)

// Validate checks the request is well-formed enough to attempt a build.
// Like Request.Validate, it does not check the filesystem: a missing
// source directory surfaces later, from Railpack's own app.NewApp call,
// which already produces a clear error for that case.
func (r RailpackRequest) Validate() error {
	if r.SourceDir == "" {
		return ErrRailpackSourceDirRequired
	}
	if r.Tag == "" {
		return ErrRailpackTagRequired
	}
	return nil
}

// supportedRailpackProviders is the allowlist of providers this codebase
// has verified. Python is verified live (TestClient_BuildRailpack_Live_Python);
// ruby and php are verified at plan generation only. Anything else
// Railpack detects (rust, deno, elixir, staticfile, ...) is rejected with
// UnsupportedProviderError instead of silently accepted.
var supportedRailpackProviders = map[string]bool{
	"node":   true,
	"golang": true,
	"java":   true,
	"python": true,
	"ruby":   true,
	"php":    true,
}

// UnsupportedProviderError is returned when Railpack's own detection
// (core.BuildResult.DetectedProviders) resolved to a provider outside
// this slice's scope (supportedRailpackProviders), including the empty
// string for "no provider matched at all". It is a distinct type, not a
// plain fmt.Errorf, so callers (internal/deploy in particular) can
// errors.As it to produce a caller-appropriate message rather than a
// generic build failure, the same "fail loudly, not silently" pattern
// deploy.go's validateEnv already establishes for unsupported env
// resolution.
type UnsupportedProviderError struct {
	// Provider is the provider name Railpack detected, or "" if it
	// detected none at all.
	Provider string
}

func (e *UnsupportedProviderError) Error() string {
	provider := e.Provider
	if provider == "" {
		provider = "(none detected)"
	}
	return "build: railpack: " + unsupportedDetail(provider)
}

// Detail is the message without the package prefix, for callers that
// add their own (internal/deploy).
func (e *UnsupportedProviderError) Detail() string {
	provider := e.Provider
	if provider == "" {
		provider = "(none detected)"
	}
	return unsupportedDetail(provider)
}

// unsupportedDetail keeps the supported list in one place; the docs
// anchor is the "Unsupported providers" section of importing-apps.md.
func unsupportedDetail(provider string) string {
	return fmt.Sprintf("detected provider %q, only node, golang, java, python, ruby, and php are supported; add a Dockerfile to the repository and build with the dockerfile build type (see importing-apps#unsupported-providers)", provider)
}

// generateRailpackPlan runs Railpack's own build-plan generation against
// req.SourceDir and enforces supportedRailpackProviders. It touches only
// the local filesystem (via Railpack's core.GenerateBuildPlan reading
// req.SourceDir), no network, no BuildKit connection, so it is exercised
// by plain table-driven tests against testdata fixtures, the same
// "pure, unit-testable" split newSolveOpt already establishes for the
// Dockerfile path relative to the live Build tests in build_test.go.
func generateRailpackPlan(req RailpackRequest) (*core.BuildResult, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	app, err := rpapp.NewApp(req.SourceDir)
	if err != nil {
		return nil, fmt.Errorf("build: railpack: read source dir %q: %w", req.SourceDir, err)
	}
	env := rpapp.NewEnvironment(&req.Env)

	result, err := core.GenerateBuildPlan(app, env, &core.GenerateBuildPlanOptions{
		RailpackVersion: "levelrail",
	})
	if err != nil {
		// core.GenerateBuildPlan's own doc comment: a non-nil error here
		// means a transient failure worth retrying (mise.IsTemporary),
		// not something about the app itself, so this is a real error,
		// not a "no supported provider" outcome.
		return nil, fmt.Errorf("build: railpack: generate build plan for %q: %w", req.SourceDir, err)
	}
	if !result.Success {
		return nil, fmt.Errorf("build: railpack: could not generate a build plan for %q: %s", req.SourceDir, railpackLogSummary(result))
	}

	provider := ""
	if len(result.DetectedProviders) > 0 {
		provider = result.DetectedProviders[0]
	}
	if !supportedRailpackProviders[provider] {
		return nil, &UnsupportedProviderError{Provider: provider}
	}
	if provider == "node" {
		applyNextStandalone(result.Plan, req.SourceDir)
	}

	return result, nil
}

// railpackLogSummary renders a core.BuildResult's diagnostic logs into
// one line for an error message, so a failed plan generation says why,
// not just that it failed.
func railpackLogSummary(result *core.BuildResult) string {
	if len(result.Logs) == 0 {
		return "no diagnostic logs"
	}
	msgs := make([]string, 0, len(result.Logs))
	for _, l := range result.Logs {
		msgs = append(msgs, string(l.Level)+": "+l.Msg)
	}
	return strings.Join(msgs, "; ")
}

// newRailpackSolveOpt turns a Railpack build plan into the Definition and
// SolveOpt BuildKit's client needs. Unlike newSolveOpt (solve.go), there
// is no dockerfile.v0 frontend involved: plan has already been converted
// (by the caller, via buildkit.ConvertPlanToLLB) into an llb.State and an
// OCI image config, so this only needs to marshal that state into a
// Definition and carry the image config as the "containerimage.config"
// export attribute, the same shape unbind-api's own Railpack integration
// uses (see the decision note, section 2).
func newRailpackSolveOpt(ctx context.Context, plan *rpplan.BuildPlan, req RailpackRequest, out io.WriteCloser) (*llb.Definition, *bkclient.SolveOpt, error) {
	contextFS, err := fsutil.NewFS(req.SourceDir)
	if err != nil {
		return nil, nil, fmt.Errorf("build: railpack: invalid source dir %q: %w", req.SourceDir, err)
	}

	cacheID := req.CacheID
	if !req.perAppCache {
		cacheID = ""
	}
	platform, err := rpbuildkit.ParsePlatformWithDefaults("")
	if err != nil {
		return nil, nil, fmt.Errorf("build: railpack: resolve build platform: %w", err)
	}

	// llbState is discarded and rebuilt below by deployStateWithoutMerge;
	// see that function's doc comment for why. image is unaffected.
	_, image, err := rpbuildkit.ConvertPlanToLLB(plan, rpbuildkit.ConvertPlanOptions{
		BuildPlatform: platform,
		SecretsHash:   railpackEnvHash(req.Env),
		CacheKey:      cacheID,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("build: railpack: convert plan to LLB: %w", err)
	}

	llbState, err := deployStateWithoutMerge(plan, platform, railpackEnvHash(req.Env), cacheID)
	if err != nil {
		return nil, nil, fmt.Errorf("build: railpack: assemble deploy image: %w", err)
	}

	if _, userSet := req.Env["NODE_OPTIONS"]; req.NodeHeapMB > 0 && !userSet {
		image.Config.Env = dropBuildHeapEnv(image.Config.Env, req.NodeHeapMB)
	}
	imageConfig, err := json.Marshal(image)
	if err != nil {
		return nil, nil, fmt.Errorf("build: railpack: marshal image config: %w", err)
	}

	def, err := llbState.Marshal(ctx, llb.Platform(platform))
	if err != nil {
		return nil, nil, fmt.Errorf("build: railpack: marshal LLB definition: %w", err)
	}
	if req.CgroupParent != "" {
		if def, err = withCgroupParent(def, req.CgroupParent); err != nil {
			return nil, nil, fmt.Errorf("build: railpack: apply build limits: %w", err)
		}
	}

	if def, err = req.Network.applyRailpack(def); err != nil {
		return nil, nil, err
	}

	opt := &bkclient.SolveOpt{
		LocalMounts: map[string]fsutil.FS{
			"context": contextFS,
		},
		Session: railpackSession(req.Env),
		Exports: []bkclient.ExportEntry{
			{
				Type: bkclient.ExporterDocker,
				Attrs: map[string]string{
					"name":                  req.Tag,
					"containerimage.config": string(imageConfig),
				},
				Output: func(map[string]string) (io.WriteCloser, error) {
					return out, nil
				},
			},
		},
	}
	return def, opt, nil
}

// deployStateWithoutMerge rebuilds Railpack's deploy-image filesystem the
// way rpbuildkit.ConvertPlanToLLB does internally, except the final layer
// assembly always uses sequential llb.Copy rather than letting Railpack's
// own build_llb.GetFullStateFromLayers pick llb.Merge for non-overlapping
// layers. That merge choice deadlocks creating a containerd lease against
// this codebase's BuildKit connection (a bare two-state llb.Merge against
// the same *Client.bk reproduces it with no Railpack involved at all,
// only unblocking on context cancellation). ConvertPlanOptions has no
// field to opt out, so this reimplements just that one step with the same
// exported build_llb building blocks ConvertPlanToLLB itself uses.
func deployStateWithoutMerge(plan *rpplan.BuildPlan, platform specs.Platform, secretsHash, cacheID string) (*llb.State, error) {
	localOpts := []llb.LocalOption{
		llb.SharedKeyHint("local"),
		llb.WithCustomName("loading ."),
	}
	if len(plan.Exclude) > 0 {
		localOpts = append(localOpts, llb.ExcludePatterns(plan.Exclude))
	}
	localState := llb.Local("context", localOpts...)

	graph, err := build_llb.NewBuildGraph(plan, &localState, build_llb.NewBuildKitCacheStore(cacheID), secretsHash, &platform, "", false)
	if err != nil {
		return nil, fmt.Errorf("build: railpack: build graph: %w", err)
	}
	// GenerateLLB also computes its own (merge-based) deploy state as part
	// of its return value; that part is never used below, only the step
	// states it populates on graph as a side effect.
	if _, err := graph.GenerateLLB(); err != nil {
		return nil, fmt.Errorf("build: railpack: generate step states: %w", err)
	}

	deployInputs := append([]rpplan.Layer{plan.Deploy.Base}, plan.Deploy.Inputs...)
	state := copyDeployLayers(graph, deployInputs).Dir(rpbuildkit.WorkingDir)
	return &state, nil
}

// copyDeployLayers and its helpers mirror build_llb's own unexported
// getCopyState/copyLayerPaths/resolvePaths (the fallback path
// GetFullStateFromLayers already uses for layers it doesn't trust
// merging), reimplemented here because that fallback isn't reachable
// through Railpack's public API.
func copyDeployLayers(graph *build_llb.BuildGraph, layers []rpplan.Layer) llb.State {
	state := graph.GetStateForLayer(layers[0])
	for _, input := range layers[1:] {
		inputState := graph.GetStateForLayer(input)
		state = copyLayerFiltered(state, inputState, input.Filter, input.Local)
	}
	return state
}

func copyLayerFiltered(destState, srcState llb.State, filter rpplan.Filter, isLocal bool) llb.State {
	for _, include := range filter.Include {
		srcPath, destPath := deployLayerPaths(include, isLocal)
		opts := []llb.ConstraintsOpt{}
		if srcPath == destPath {
			opts = append(opts, llb.WithCustomName("copy "+srcPath))
		}
		destState = destState.File(llb.Copy(srcState, srcPath, destPath, &llb.CopyInfo{
			CopyDirContentsOnly: true,
			CreateDestPath:      true,
			FollowSymlinks:      true,
			AllowWildcard:       true,
			AllowEmptyWildcard:  true,
			ExcludePatterns:     filter.Exclude,
		}), opts...)
	}
	return destState
}

func deployLayerPaths(include string, isLocal bool) (srcPath, destPath string) {
	if isLocal {
		return include, filepath.Join(rpbuildkit.WorkingDir, filepath.Base(include))
	}
	switch {
	case include == "." || include == rpbuildkit.WorkingDir || include == rpbuildkit.WorkingDir+"/":
		return rpbuildkit.WorkingDir, rpbuildkit.WorkingDir
	case filepath.IsAbs(include):
		return include, include
	default:
		return filepath.Join(rpbuildkit.WorkingDir, include), filepath.Join(rpbuildkit.WorkingDir, include)
	}
}

// BuildRailpack runs req end to end: generates a Railpack build plan for
// req.SourceDir, rejects it unless Railpack detected a supported
// provider (supportedRailpackProviders), converts the plan to a BuildKit
// LLB definition, solves it, and streams the resulting image into the
// local Docker Engine's /images/load endpoint, exactly like Build does
// for a Dockerfile. It never shells out to the railpack or docker CLI.
//
// progress behaves exactly as it does for Build: pass SlogProgress(logger)
// to log every update, or nil to discard them.
func (c *Client) BuildRailpack(ctx context.Context, req RailpackRequest, progress func(ProgressEvent)) (*Result, error) {
	if progress == nil {
		progress = func(ProgressEvent) {}
	}
	if err := c.checkEngine(ctx); err != nil {
		return nil, err
	}
	if err := checkDiskSpace(req.SourceDir); err != nil {
		return nil, err
	}
	return c.solveAndLoad(ctx, req.Tag, progress, func(solveCtx context.Context, out io.Writer) (*Result, error) {
		return c.solveRailpack(solveCtx, req, out, progress)
	})
}

// solveRailpack is solveDockerfile's Railpack counterpart: the same split
// between solving and loading, so a dispatched build can export the tar
// onto the wire instead (SolveRemote, remote.go).
func (c *Client) solveRailpack(ctx context.Context, req RailpackRequest, out io.Writer, progress func(ProgressEvent)) (*Result, error) {
	start := time.Now()
	var err error
	if req.Network, err = effectiveNetwork(ctx, req.Network); err != nil {
		return nil, err
	}

	req.perAppCache = perAppCacheEnabled(ctx)
	if req.perAppCache {
		req.CacheID = SanitizeCacheID(req.CacheID, req.Tag)
		progress(ProgressEvent{Step: "build cache", Log: c.cacheLogLine(ctx, req.CacheID), Stream: "stdout"})
	} else {
		progress(ProgressEvent{Step: "build cache", Log: "build cache: per app cache id disabled, using the shared cache", Stream: "stdout"})
	}

	result, err := generateRailpackPlan(req)
	if err != nil {
		return nil, err
	}

	if _, userSet := req.Env["NODE_OPTIONS"]; req.NodeHeapMB > 0 && !userSet && len(result.DetectedProviders) > 0 && result.DetectedProviders[0] == "node" {
		progress(ProgressEvent{Step: "memory guard", Log: applyNodeHeap(result.Plan, req.NodeHeapMB), Stream: "stdout"})
	}

	def, solveOpt, err := newRailpackSolveOpt(ctx, result.Plan, req, nopWriteCloser{out})
	if err != nil {
		return nil, err
	}

	statusCh := make(chan *bkclient.SolveStatus)
	eg, egCtx := errgroup.WithContext(ctx)

	var solveResp *bkclient.SolveResponse
	eg.Go(func() error {
		resp, err := c.bk.Solve(egCtx, def, *solveOpt, statusCh)
		if err != nil {
			return fmt.Errorf("build: railpack: solve %q: %w", req.Tag, err)
		}
		solveResp = resp
		return nil
	})
	eg.Go(func() error {
		relayProgress(statusCh, progress)
		return nil
	})

	if err := eg.Wait(); err != nil {
		return nil, err
	}
	return newResult(req.Tag, start, solveResp), nil
}

// railpackEnvHash changes whenever a build env value does, so a cached
// layer built with an old NEXT_PUBLIC_ value is not reused.
func railpackEnvHash(env map[string]string) string {
	if len(env) == 0 {
		return ""
	}
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	h := sha256.New()
	for _, k := range keys {
		_, _ = fmt.Fprintf(h, "%s=%s\n", k, env[k])
	}
	return hex.EncodeToString(h.Sum(nil))
}

// railpackSession serves the env values as BuildKit secrets, the channel
// Railpack's plan reads them from.
func railpackSession(env map[string]string) []session.Attachable {
	if len(env) == 0 {
		return nil
	}
	m := make(map[string][]byte, len(env))
	for k, v := range env {
		m[k] = []byte(v)
	}
	return []session.Attachable{secretsprovider.FromMap(m)}
}

// dropBuildHeapEnv removes the build-time heap cap from the image env so it never limits the running app.
func dropBuildHeapEnv(env []string, heapMB int) []string {
	injected := fmt.Sprintf("NODE_OPTIONS=--max-old-space-size=%d", heapMB)
	out := env[:0:0]
	for _, e := range env {
		if e != injected {
			out = append(out, e)
		}
	}
	return out
}

// applyNodeHeap sets NODE_OPTIONS on the install and build steps unless the plan already sets it, and returns the log line (keys only).
func applyNodeHeap(plan *rpplan.BuildPlan, heapMB int) string {
	for i := range plan.Steps {
		st := &plan.Steps[i]
		if st.Name != "install" && st.Name != "build" {
			continue
		}
		if _, set := st.Variables["NODE_OPTIONS"]; set {
			return "build memory: NODE_OPTIONS is already set for this build, left unchanged"
		}
	}
	for i := range plan.Steps {
		st := &plan.Steps[i]
		if st.Name != "install" && st.Name != "build" {
			continue
		}
		if st.Variables == nil {
			st.Variables = map[string]string{}
		}
		st.Variables["NODE_OPTIONS"] = fmt.Sprintf("--max-old-space-size=%d", heapMB)
	}
	return fmt.Sprintf("build memory: NODE_OPTIONS set to --max-old-space-size=%d to stay inside the build memory limit; set NODE_OPTIONS yourself to override", heapMB)
}

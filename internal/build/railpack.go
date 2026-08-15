package build

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	bkclient "github.com/moby/buildkit/client"
	"github.com/moby/buildkit/client/llb"
	rpbuildkit "github.com/railwayapp/railpack/buildkit"
	"github.com/railwayapp/railpack/core"
	rpapp "github.com/railwayapp/railpack/core/app"
	rpplan "github.com/railwayapp/railpack/core/plan"
	"github.com/tonistiigi/fsutil"
	"golang.org/x/sync/errgroup"
)

// This file: TASKS.md's Railpack integration, scoped to exactly two
// providers (node, golang), per
// docs-local/research/railpack-integration-decision.md. Railpack
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
// for BuildKit to read it from otherwise. See the decision note for the
// full evidence trail (a real third-party integration, unbindapp/
// unbind-api, doing exactly this).
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

// supportedRailpackProviders is this slice's entire scope: Node.js and
// Go, the two providers docs-local/research/
// railpack-integration-decision.md's recommendation names explicitly.
// Every other provider Railpack itself supports (python, ruby, php,
// java, rust, deno, elixir, gleam, dotnet, cpp, staticfile, ...) is
// deliberately deferred, not silently accepted: see
// UnsupportedProviderError.
var supportedRailpackProviders = map[string]bool{
	"node":   true,
	"golang": true,
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
	return fmt.Sprintf("build: railpack: detected provider %q, only node and golang are supported yet", provider)
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
	env := rpapp.NewEnvironment(nil)

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

	platform, err := rpbuildkit.ParsePlatformWithDefaults("")
	if err != nil {
		return nil, nil, fmt.Errorf("build: railpack: resolve build platform: %w", err)
	}

	llbState, image, err := rpbuildkit.ConvertPlanToLLB(plan, rpbuildkit.ConvertPlanOptions{
		BuildPlatform: platform,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("build: railpack: convert plan to LLB: %w", err)
	}

	imageConfig, err := json.Marshal(image)
	if err != nil {
		return nil, nil, fmt.Errorf("build: railpack: marshal image config: %w", err)
	}

	def, err := llbState.Marshal(ctx, llb.Platform(platform))
	if err != nil {
		return nil, nil, fmt.Errorf("build: railpack: marshal LLB definition: %w", err)
	}

	opt := &bkclient.SolveOpt{
		LocalMounts: map[string]fsutil.FS{
			"context": contextFS,
		},
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
	start := time.Now()

	result, err := generateRailpackPlan(req)
	if err != nil {
		return nil, err
	}

	pipeR, pipeW := io.Pipe()

	def, solveOpt, err := newRailpackSolveOpt(ctx, result.Plan, req, pipeW)
	if err != nil {
		_ = pipeW.Close()
		_ = pipeR.Close()
		return nil, err
	}

	statusCh := make(chan *bkclient.SolveStatus)

	eg, egCtx := errgroup.WithContext(ctx)

	var solveResp *bkclient.SolveResponse
	eg.Go(func() error {
		defer func() { _ = pipeW.Close() }()
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

	eg.Go(func() error {
		defer func() { _ = pipeR.Close() }()
		return loadImage(egCtx, c.docker, pipeR, req.Tag, progress)
	})

	if err := eg.Wait(); err != nil {
		return nil, err
	}

	res := &Result{
		Tag:      req.Tag,
		Duration: time.Since(start),
	}
	if solveResp != nil {
		res.ExporterResponse = solveResp.ExporterResponse
	}
	return res, nil
}

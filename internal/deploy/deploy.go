// Package deploy is TASKS.md 1.4's build integration: it connects
// internal/spec's build declaration to internal/build's BuildKit client,
// and a successful build's output to internal/store's desired state,
// closing the loop the application controller (internal/reconcile/
// application, TASKS.md 1.3) reads from on every reconcile.
package deploy

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

// ImageBuilder is the narrow surface this package needs from
// internal/build, so tests can fake it without a real daemon or
// BuildKit connection. *build.Client satisfies this.
type ImageBuilder interface {
	Build(ctx context.Context, req build.Request, progress func(build.ProgressEvent)) (*build.Result, error)

	// BuildRailpack builds a service whose build.Type is
	// spec.BuildRailpack: Railpack's own provider detection replaces a
	// user-authored Dockerfile, scoped to node and golang only (see
	// deployRailpack). A separate method, not a build.Request field,
	// because internal/build's own solve path is genuinely different
	// (Definition-based, not dockerfile.v0-frontend-based; see
	// internal/build/railpack.go's package doc comment).
	BuildRailpack(ctx context.Context, req build.RailpackRequest, progress func(build.ProgressEvent)) (*build.Result, error)
}

// ServiceStore is the narrow surface this package needs from
// internal/store. *store.DB satisfies this.
type ServiceStore interface {
	SaveDesiredService(ctx context.Context, svc store.DesiredService) error
}

// SecretChecker is the narrow surface this package needs from
// internal/secrets.Manager: whether a value has been set for a
// { secret: true } env var, never the value itself. A deploy never
// needs to see a secret's plaintext, only confirm one exists (or
// doesn't, for a required one) before proceeding.
type SecretChecker interface {
	Exists(ctx context.Context, serviceName, envKey string) (bool, error)
}

// BuildMetricsRecorder is the narrow surface this package needs to
// record TASKS.md 2.1's build-duration metric. *telemetry.DB satisfies
// this structurally; not imported directly to avoid a dependency this
// package doesn't otherwise need, the same reasoning ImageBuilder/
// ServiceStore/SecretChecker above already establish.
type BuildMetricsRecorder interface {
	RecordBuildDuration(ctx context.Context, serviceName string, d time.Duration, at time.Time) error
}

// Option configures optional Pipeline behavior.
type Option func(*Pipeline)

// WithSecretChecker enables { secret: true } env vars to pass through
// deployment instead of being rejected outright. Without one configured
// (the default), a service declaring any secret-backed env var fails to
// deploy with an explicit error, the same as before TASKS.md 1.7's
// secret storage existed: an operator running Levelrail without a
// master key configured should get a clear failure, not a container
// silently missing a variable it declared as required.
func WithSecretChecker(checker SecretChecker) Option {
	return func(p *Pipeline) { p.secrets = checker }
}

// WithBuildMetricsRecorder enables recording build.Result.Duration as
// TASKS.md 2.1's build_duration_seconds metric after a successful
// build. Without one configured (the default), a deploy still succeeds
// exactly as before, it just isn't measured: a metrics-store outage
// must never block a real deploy.
func WithBuildMetricsRecorder(recorder BuildMetricsRecorder) Option {
	return func(p *Pipeline) { p.metrics = recorder }
}

// WithLogger overrides the logger used to report a non-fatal metrics
// recording failure. Defaults to slog.Default().
func WithLogger(logger *slog.Logger) Option {
	return func(p *Pipeline) { p.logger = logger }
}

// Request is one deploy attempt for a single service.
type Request struct {
	// ServiceName identifies the service, matching the name the
	// application controller (1.3) reconciles under.
	ServiceName string
	// Service is the app.yaml service block: build config, port, env,
	// resources, health.
	Service spec.Service
	// SourceDir is the local checkout root, the build context.
	SourceDir string
	// CommitSHA tags the built image, and is what a future rollback
	// command points desired.Image back at.
	CommitSHA string
	// ImageRepo is the image name without a tag, e.g. "levelrail/thesvg".
	// Naming policy (namespacing, registry prefix) is the caller's
	// decision, not this package's.
	ImageRepo string
}

// Pipeline builds a service (when its build type requires a build) and
// writes the result as that service's new desired state.
type Pipeline struct {
	builder ImageBuilder
	store   ServiceStore
	secrets SecretChecker        // nil is valid: secret-backed env vars are rejected without one
	metrics BuildMetricsRecorder // nil is valid: build duration just isn't recorded
	logger  *slog.Logger

	// staticSites and staticRootDir back build.type: static (static.go);
	// both nil/empty is valid, the same "fail loudly without config"
	// shape secrets above already establishes for a different optional
	// feature.
	staticSites   StaticSiteStore
	staticRootDir string
}

// New builds a Pipeline.
func New(builder ImageBuilder, svcStore ServiceStore, opts ...Option) *Pipeline {
	p := &Pipeline{builder: builder, store: svcStore, logger: slog.Default()}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Deploy runs req's build (if its build.Type needs one) and saves the
// resulting desired state. For every build.Type except spec.BuildStatic,
// that means the full image reference that was built, ready for the
// application controller to converge on its next reconcile, which is
// also what's returned. spec.BuildStatic has no image and no container
// for the application controller to converge to at all (see static.go's
// package doc comment): its desired state is a store.StaticSite instead
// of a store.DesiredService, consumed directly by
// internal/reconcile/ingress rather than the application controller, and
// the string this returns is the local directory now being served, not
// an image reference.
//
// progress, if non-nil, receives build progress as it happens; see
// build.ProgressEvent and build.SlogProgress. A static deploy never
// invokes it: there is no build step to report progress on.
func (p *Pipeline) Deploy(ctx context.Context, req Request, progress func(build.ProgressEvent)) (string, error) {
	switch req.Service.Build.Type {
	case spec.BuildDockerfile:
		return p.deployDockerfile(ctx, req, progress)
	case spec.BuildStatic:
		// No image, no build.ProgressEvent stream: see static.go's
		// package doc comment for why this is a directory copy plus a
		// store.StaticSite save, not a build.
		return p.deployStatic(ctx, req)
	case spec.BuildCompose:
		return "", fmt.Errorf("deploy: service %q: build.type %q is not yet supported", req.ServiceName, spec.BuildCompose)
	case spec.BuildRailpack:
		return p.deployRailpack(ctx, req, progress)
	default:
		return "", fmt.Errorf("deploy: service %q: unrecognized build.type %q", req.ServiceName, req.Service.Build.Type)
	}
}

func (p *Pipeline) deployDockerfile(ctx context.Context, req Request, progress func(build.ProgressEvent)) (string, error) {
	if err := p.validateEnv(ctx, req.ServiceName, req.Service.Env); err != nil {
		return "", fmt.Errorf("deploy: service %q: %w", req.ServiceName, err)
	}

	tag := req.ImageRepo + ":" + req.CommitSHA

	dockerfilePath := ""
	if req.Service.Build.Path != "" {
		dockerfilePath = filepath.Join(req.SourceDir, req.Service.Build.Path)
	}

	res, err := p.builder.Build(ctx, build.Request{
		ContextDir:     req.SourceDir,
		DockerfilePath: dockerfilePath,
		Tag:            tag,
	}, progress)
	if err != nil {
		return "", fmt.Errorf("deploy: service %q: build: %w", req.ServiceName, err)
	}

	return p.finishDeploy(ctx, req, res)
}

// deployRailpack mirrors deployDockerfile's shape exactly, substituting
// Railpack's own provider detection for a user-authored Dockerfile: no
// build.Path to resolve, since there is no Dockerfile to point at. See
// docs-local/research/railpack-integration-decision.md for why this
// needs internal/build's separate BuildRailpack method (a different,
// Definition-based BuildKit solve path) rather than a variant
// build.Request fed through deployDockerfile's existing p.builder.Build
// call.
//
// This slice supports exactly two Railpack providers, node and golang
// (docs-local/research/railpack-integration-decision.md's recommendation
// section): any other provider Railpack itself detects fails loudly here
// with the service name and the detected provider named explicitly, the
// same "fail loudly, not silently" pattern validateEnv already
// establishes for unsupported env resolution, rather than a generic
// build failure that leaves an operator guessing why.
func (p *Pipeline) deployRailpack(ctx context.Context, req Request, progress func(build.ProgressEvent)) (string, error) {
	if err := p.validateEnv(ctx, req.ServiceName, req.Service.Env); err != nil {
		return "", fmt.Errorf("deploy: service %q: %w", req.ServiceName, err)
	}

	tag := req.ImageRepo + ":" + req.CommitSHA

	res, err := p.builder.BuildRailpack(ctx, build.RailpackRequest{
		SourceDir: req.SourceDir,
		Tag:       tag,
	}, progress)
	if err != nil {
		var unsupported *build.UnsupportedProviderError
		if errors.As(err, &unsupported) {
			provider := unsupported.Provider
			if provider == "" {
				provider = "(none detected)"
			}
			return "", fmt.Errorf("deploy: service %q: railpack detected provider %q, only node and golang are supported yet", req.ServiceName, provider)
		}
		return "", fmt.Errorf("deploy: service %q: build: %w", req.ServiceName, err)
	}

	return p.finishDeploy(ctx, req, res)
}

// finishDeploy is deployDockerfile and deployRailpack's shared tail once
// their respective build step has produced res: build type diverges up
// to this point (a Dockerfile solve vs. a Railpack Definition-based
// solve, see deployRailpack's own doc comment), but turning a successful
// build into saved desired state and a recorded build-duration metric is
// identical either way, so it exists exactly once here rather than
// duplicated in both callers.
func (p *Pipeline) finishDeploy(ctx context.Context, req Request, res *build.Result) (string, error) {
	desired, err := toDesiredService(req.ServiceName, res.Tag, req.Service)
	if err != nil {
		return "", fmt.Errorf("deploy: service %q: %w", req.ServiceName, err)
	}

	if err := p.store.SaveDesiredService(ctx, desired); err != nil {
		return "", fmt.Errorf("deploy: service %q: save desired state: %w", req.ServiceName, err)
	}

	// Best-effort, secondary to the deploy itself: the build already
	// succeeded and desired state is already saved by this point, so a
	// metrics-store hiccup is logged, not returned as this Deploy call's
	// error, the same "must never block the real operation" choice
	// reconcile.Engine.reconcileOne already makes for persisting
	// reconcile status.
	if p.metrics != nil {
		if err := p.metrics.RecordBuildDuration(ctx, req.ServiceName, res.Duration, time.Now()); err != nil {
			p.logger.Warn("deploy: record build duration metric failed",
				slog.String("service", req.ServiceName), slog.String("error", err.Error()))
		}
	}

	return res.Tag, nil
}

// validateEnv fails loudly rather than silently deploying a container
// missing values it needs.
//
// { from: ... } (cross-resource references like "postgres.main.url")
// still isn't resolvable: it needs a managed database's credentials,
// which are themselves blocked on secret storage (TASKS.md 1.8's own
// Postgres scope note), so this stays unsupported until that lands.
//
// { secret: true } is resolvable now that TASKS.md 1.7's secret storage
// exists, but only if this Pipeline has a SecretChecker configured
// (WithSecretChecker); without one, secret-backed env vars are rejected
// exactly like before, so a control plane running without a master key
// configured fails a deploy clearly rather than silently starting a
// container missing a variable it declared as required. When a checker
// is configured, a { secret: true, required: true } var with no value
// set yet still fails the deploy here, matching spec.EnvVar.Required's
// documented meaning ("fail the deploy... rather than starting the
// container with the variable unset"); an optional one with no value
// set is allowed through; the application controller's own resolution
// (internal/reconcile/application) simply omits it from the container's
// environment if it's still unset by the time of a later reconcile.
func (p *Pipeline) validateEnv(ctx context.Context, serviceName string, env map[string]spec.EnvVar) error {
	for name, v := range env {
		if v.From != "" {
			return fmt.Errorf("env var %q needs cross-resource resolution ({ from: ... }), not supported until managed database credentials exist", name)
		}
		if !v.Secret {
			continue
		}
		if p.secrets == nil {
			return fmt.Errorf("env var %q is a secret but no secret store is configured for this deploy pipeline", name)
		}
		exists, err := p.secrets.Exists(ctx, serviceName, name)
		if err != nil {
			return fmt.Errorf("env var %q: check secret value exists: %w", name, err)
		}
		if !exists && v.Required {
			return fmt.Errorf("env var %q is required but no secret value has been set for it yet", name)
		}
	}
	return nil
}

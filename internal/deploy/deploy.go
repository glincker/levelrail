// Package deploy is TASKS.md 1.4's build integration: it connects
// internal/spec's build declaration to internal/build's BuildKit client,
// and a successful build's output to internal/store's desired state,
// closing the loop the application controller (internal/reconcile/
// application, TASKS.md 1.3) reads from on every reconcile.
package deploy

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

// ImageBuilder is the narrow surface this package needs from
// internal/build, so tests can fake it without a real daemon or
// BuildKit connection. *build.Client satisfies this.
type ImageBuilder interface {
	Build(ctx context.Context, req build.Request, progress func(build.ProgressEvent)) (*build.Result, error)
}

// ServiceStore is the narrow surface this package needs from
// internal/store. *store.DB satisfies this.
type ServiceStore interface {
	SaveDesiredService(ctx context.Context, svc store.DesiredService) error
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
}

// New builds a Pipeline.
func New(builder ImageBuilder, svcStore ServiceStore) *Pipeline {
	return &Pipeline{builder: builder, store: svcStore}
}

// Deploy runs req's build (if its build.Type needs one) and saves the
// resulting desired state, ready for the application controller to
// converge on its next reconcile. It returns the full image reference
// that was built and saved.
//
// progress, if non-nil, receives build progress as it happens; see
// build.ProgressEvent and build.SlogProgress.
func (p *Pipeline) Deploy(ctx context.Context, req Request, progress func(build.ProgressEvent)) (string, error) {
	switch req.Service.Build.Type {
	case spec.BuildDockerfile:
		return p.deployDockerfile(ctx, req, progress)
	case spec.BuildStatic:
		return "", fmt.Errorf("deploy: service %q: build.type %q needs ingress integration first (TASKS.md 1.6), not yet supported", req.ServiceName, spec.BuildStatic)
	case spec.BuildCompose:
		return "", fmt.Errorf("deploy: service %q: build.type %q is not yet supported", req.ServiceName, spec.BuildCompose)
	case spec.BuildRailpack:
		return "", fmt.Errorf("deploy: service %q: build.type %q is not yet supported", req.ServiceName, spec.BuildRailpack)
	default:
		return "", fmt.Errorf("deploy: service %q: unrecognized build.type %q", req.ServiceName, req.Service.Build.Type)
	}
}

func (p *Pipeline) deployDockerfile(ctx context.Context, req Request, progress func(build.ProgressEvent)) (string, error) {
	if err := requireNoUnresolvedEnv(req.Service.Env); err != nil {
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

	desired, err := toDesiredService(req.ServiceName, res.Tag, req.Service)
	if err != nil {
		return "", fmt.Errorf("deploy: service %q: %w", req.ServiceName, err)
	}

	if err := p.store.SaveDesiredService(ctx, desired); err != nil {
		return "", fmt.Errorf("deploy: service %q: save desired state: %w", req.ServiceName, err)
	}

	return res.Tag, nil
}

// requireNoUnresolvedEnv fails loudly rather than silently deploying a
// container missing values it needs: secret-sourced and from-referenced
// env vars (CLAUDE.md 4.10's secrets, and cross-resource references like
// "postgres.main.url") aren't resolvable yet, since neither secrets
// (TASKS.md 1.7) nor managed databases (1.8) exist. Only plain literal
// values can be carried through today.
func requireNoUnresolvedEnv(env map[string]spec.EnvVar) error {
	for name, v := range env {
		if v.Secret || v.From != "" {
			return fmt.Errorf("env var %q needs secrets or cross-resource resolution, not supported until TASKS.md 1.7/1.8 land", name)
		}
	}
	return nil
}

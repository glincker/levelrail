package deploy

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/pkgregistry"
)

// BuildSecretSource supplies the private package registry config a
// service's Dockerfile build can mount as BuildKit secrets.
type BuildSecretSource interface {
	BuildSecrets(ctx context.Context, serviceName string) (pkgregistry.Output, error)
}

// WithBuildSecretSource enables private package registry secret mounts on
// Dockerfile builds. Without one, builds receive no secrets.
func WithBuildSecretSource(s BuildSecretSource) Option {
	return func(p *Pipeline) { p.buildSecrets = s }
}

// applyBuildSecrets adds the registry secret mounts and non-secret build
// args to req and returns a progress wrapper that masks the secret values,
// so they cannot reach build logs or the deploy history through it.
func (p *Pipeline) applyBuildSecrets(ctx context.Context, serviceName string, req *build.Request, progress func(build.ProgressEvent)) (func(build.ProgressEvent), error) {
	if p.buildSecrets == nil {
		return progress, nil
	}
	out, err := p.buildSecrets.BuildSecrets(ctx, serviceName)
	if err != nil {
		return nil, fmt.Errorf("package registry secrets: %w", err)
	}
	if len(out.Files) == 0 {
		return progress, nil
	}
	req.Secrets = out.Files
	if len(out.Args) > 0 {
		merged := make(map[string]string, len(req.BuildArgs)+len(out.Args))
		for k, v := range out.Args {
			merged[k] = v
		}
		for k, v := range req.BuildArgs {
			merged[k] = v
		}
		req.BuildArgs = merged
	}
	mask := pkgregistry.Redactor(out.Redact)
	if progress == nil {
		return nil, nil
	}
	return func(ev build.ProgressEvent) {
		ev.Step, ev.Log, ev.Error = mask(ev.Step), mask(ev.Log), mask(ev.Error)
		progress(ev)
	}, nil
}

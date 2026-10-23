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

// BuildSecretValues reads the app's encrypted secrets for build.secrets.
// internal/secrets.Manager satisfies it.
type BuildSecretValues interface {
	Exists(ctx context.Context, serviceName, envKey string) (bool, error)
	Resolve(ctx context.Context, serviceName, envKey string) (string, error)
}

// WithBuildSecretValues enables build.secrets on Dockerfile builds.
func WithBuildSecretValues(v BuildSecretValues) Option {
	return func(p *Pipeline) { p.buildSecretValues = v }
}

// WithBuildSecretSource enables private package registry secret mounts on
// Dockerfile builds. Without one, builds receive no secrets.
func WithBuildSecretSource(s BuildSecretSource) Option {
	return func(p *Pipeline) { p.buildSecrets = s }
}

// applyBuildSecrets adds the registry and build.secrets mounts and the
// non-secret build args to req and returns a progress wrapper that masks
// the secret values, so they cannot reach build logs or deploy history.
func (p *Pipeline) applyBuildSecrets(ctx context.Context, serviceName string, names []string, req *build.Request, progress func(build.ProgressEvent)) (func(build.ProgressEvent), error) {
	var out pkgregistry.Output
	if p.buildSecrets != nil {
		var err error
		out, err = p.buildSecrets.BuildSecrets(ctx, serviceName)
		if err != nil {
			return nil, fmt.Errorf("package registry secrets: %w", err)
		}
	}
	files := make(map[string][]byte, len(out.Files)+len(names))
	for id, b := range out.Files {
		files[id] = b
	}
	redact := append([]string(nil), out.Redact...)
	for _, name := range names {
		val, err := p.resolveBuildSecret(ctx, serviceName, name)
		if err != nil {
			return nil, err
		}
		files[name] = []byte(val)
		redact = append(redact, val)
	}
	if len(files) == 0 {
		return progress, nil
	}
	req.Secrets = files
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
	mask := pkgregistry.Redactor(redact)
	if progress == nil {
		return nil, nil
	}
	return func(ev build.ProgressEvent) {
		ev.Step, ev.Log, ev.Error = mask(ev.Step), mask(ev.Log), mask(ev.Error)
		progress(ev)
	}, nil
}

// resolveBuildSecret reads one build.secrets value. Errors name the secret
// but never carry its value.
func (p *Pipeline) resolveBuildSecret(ctx context.Context, serviceName, name string) (string, error) {
	if p.buildSecretValues == nil {
		return "", fmt.Errorf("build.secrets %q: no secret store is configured for this deploy pipeline", name)
	}
	ok, err := p.buildSecretValues.Exists(ctx, serviceName, name)
	if err != nil {
		return "", fmt.Errorf("build.secrets %q: check secret exists: %w", name, err)
	}
	if !ok {
		return "", fmt.Errorf("build.secrets %q: no secret with that name exists for this app, create it under the app's secrets and redeploy", name)
	}
	val, err := p.buildSecretValues.Resolve(ctx, serviceName, name)
	if err != nil {
		return "", fmt.Errorf("build.secrets %q: read secret: %w", name, err)
	}
	return val, nil
}

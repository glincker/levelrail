package deploy

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/spec"
)

// SecretScanHook scans a deploy's build context and plain env values before
// anything is built. A non-nil error stops the deploy and the previous
// release keeps serving.
type SecretScanHook interface {
	BeforeDeploy(ctx context.Context, app, attemptID, root string, env map[string]string) error
}

// WithSecretScan enables deploy-time secret scanning.
func WithSecretScan(h SecretScanHook) Option {
	return func(p *Pipeline) { p.secretScan = h }
}

// plainEnv returns the literal env values: references, secrets and vault
// values are resolved elsewhere and are not credentials committed in the spec.
func plainEnv(env map[string]spec.EnvVar) map[string]string {
	out := make(map[string]string, len(env))
	for k, v := range env {
		if v.Value != "" && !v.Secret && v.From == "" && v.Vault == nil {
			out[k] = v.Value
		}
	}
	return out
}

func (p *Pipeline) scanSecrets(ctx context.Context, req Request) error {
	if p.secretScan == nil {
		return nil
	}
	root := ""
	if req.Service.Build.Type != spec.BuildImage && req.SourceDir != "" {
		if r, err := resolveBuildRoot(req.SourceDir, req.Service.Build.BaseDirectory); err == nil {
			root = r
		}
	}
	if err := p.secretScan.BeforeDeploy(ctx, req.ServiceName, req.AttemptID, root, plainEnv(req.Service.Env)); err != nil {
		return fmt.Errorf("deploy: service %q: %w", req.ServiceName, err)
	}
	return nil
}

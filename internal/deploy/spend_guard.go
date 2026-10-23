package deploy

import (
	"context"
	"fmt"
)

// SpendGuardHook refuses a new deploy of an app held at its spend limit. A
// non-nil error stops the deploy and the running release keeps serving.
type SpendGuardHook interface {
	BeforeDeploy(ctx context.Context, app string) error
}

// WithSpendGuard enables the block_deploys spend cap guard.
func WithSpendGuard(h SpendGuardHook) Option {
	return func(p *Pipeline) { p.spendGuard = h }
}

func (p *Pipeline) checkSpendGuard(ctx context.Context, req Request) error {
	if p.spendGuard == nil {
		return nil
	}
	if err := p.spendGuard.BeforeDeploy(ctx, req.ServiceName); err != nil {
		return fmt.Errorf("deploy: service %q: %w", req.ServiceName, err)
	}
	return nil
}

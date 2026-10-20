package deploy

import (
	"context"
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/spendcap"
)

// SpendCapStore persists a service's budget: block from app.yaml.
// *store.DB satisfies this structurally.
type SpendCapStore interface {
	SaveSpendCap(ctx context.Context, c spendcap.Cap, ev spendcap.Event, now time.Time) error
}

// WithSpendCapStore makes a deploy persist the spec's budget: block. A spec
// with no block leaves any dashboard or CLI configured budget alone.
func WithSpendCapStore(s SpendCapStore) Option {
	return func(p *Pipeline) { p.spendCaps = s }
}

func (p *Pipeline) saveSpendCap(ctx context.Context, req Request) error {
	if p.spendCaps == nil || req.Service.Budget == nil {
		return nil
	}
	c, err := req.Service.Budget.Cap(req.ServiceName)
	if err != nil {
		return fmt.Errorf("deploy: service %q: %w", req.ServiceName, err)
	}
	now := time.Now()
	ev := spendcap.Event{App: req.ServiceName, Period: spendcap.Period(now), Kind: spendcap.EventConfigured, At: now, Detail: "set from app.yaml"}
	if err := p.spendCaps.SaveSpendCap(ctx, c, ev, now); err != nil {
		return fmt.Errorf("deploy: service %q: save budget: %w", req.ServiceName, err)
	}
	return nil
}

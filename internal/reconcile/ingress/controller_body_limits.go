package ingress

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/store"
)

// AppIngressLimitLister is optional on ServiceStore: without it every app
// uses the instance request body cap.
type AppIngressLimitLister interface {
	ListAppIngressLimits(ctx context.Context) ([]store.AppIngressLimit, error)
}

func (c *Controller) appBodyLimits(ctx context.Context) (map[string]int64, error) {
	l, ok := c.store.(AppIngressLimitLister)
	if !ok {
		return nil, nil
	}
	rows, err := l.ListAppIngressLimits(ctx)
	if err != nil {
		return nil, fmt.Errorf("list app ingress limits: %w", err)
	}
	out := make(map[string]int64, len(rows))
	for _, r := range rows {
		out[r.App] = r.MaxBodyBytes
	}
	return out, nil
}

package ingress

import (
	"context"
	"time"

	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/store"
)

// DomainLimitsStore is optional on ServiceStore: a store without it (most test
// fakes) leaves every route on the app and instance limits.
type DomainLimitsStore interface {
	ListDomainLimits(ctx context.Context) ([]store.DomainLimits, error)
}

// applyDomainLimits copies each domain's own body cap and upstream timeouts
// onto its route. A domain cap beats the app cap; zero fields change nothing.
func (c *Controller) applyDomainLimits(ctx context.Context, routes []ingress.ProxyRoute) ([]ingress.ProxyRoute, error) {
	ls, ok := c.store.(DomainLimitsStore)
	if !ok {
		return routes, nil
	}
	rows, err := ls.ListDomainLimits(ctx)
	if err != nil {
		return routes, err
	}
	if len(rows) == 0 {
		return routes, nil
	}
	return attachDomainLimits(routes, rows), nil
}

func attachDomainLimits(routes []ingress.ProxyRoute, rows []store.DomainLimits) []ingress.ProxyRoute {
	byDomain := make(map[string]store.DomainLimits, len(rows))
	for _, l := range rows {
		byDomain[l.Domain] = l
	}
	out := make([]ingress.ProxyRoute, 0, len(routes))
	for _, r := range routes {
		var rest []string
		for _, h := range r.Hosts {
			l, ok := byDomain[h]
			if !ok {
				rest = append(rest, h)
				continue
			}
			own := r
			own.Hosts = []string{h}
			if l.MaxBodyBytes > 0 {
				own.MaxBodyBytes = l.MaxBodyBytes
			}
			own.ReadTimeout = time.Duration(l.ReadTimeoutSeconds) * time.Second
			own.WriteTimeout = time.Duration(l.WriteTimeoutSeconds) * time.Second
			out = append(out, own)
		}
		if len(rest) > 0 {
			r.Hosts = rest
			out = append(out, r)
		}
	}
	return out
}

package api

import (
	"context"
	"time"

	"github.com/GLINCKER/levelrail/internal/ingress"
)

// DomainRequests counts one domain's requests in a range for the domain_traffic_drop alert.
// ok is false when no edge access source is configured, so the alert skips instead of firing on nothing.
func (rt *Router) DomainRequests(ctx context.Context, app, domain string, since, until time.Time) (int, bool) {
	src, ok := rt.access.(VisitorSource)
	if !ok {
		return 0, false
	}
	sum := src.AppVisitors(ctx, app, ingress.VisitorSummaryOptions{Domain: domain, Since: since, Until: until, TopN: 1})
	return sum.Requests, true
}

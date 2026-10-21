package api

import (
	"context"

	"github.com/GLINCKER/levelrail/internal/ingress"
)

// fallbackResolver reads the provider choices once, for callers that derive hostnames in a loop.
func (rt *Router) fallbackResolver(ctx context.Context) ingress.FallbackResolver {
	return ingress.LoadFallbackResolver(ctx, rt.ingressSettings, rt.publicHost)
}

// autoHost is the automatic hostname of one app under its effective DNS provider.
func (rt *Router) autoHost(ctx context.Context, app string) (string, bool) {
	return rt.fallbackResolver(ctx).Domain(app)
}

package ingress

import (
	"context"
	"log/slog"

	"github.com/GLINCKER/levelrail/internal/ingress"
)

const modelRouteOwner = "ai model"

// ModelHostSource lists the hostnames AI models are served on. They are
// routed to the control plane's own listener, whose model gateway
// authenticates and proxies each request.
type ModelHostSource interface {
	ModelHosts(ctx context.Context) ([]string, error)
}

// WithModelHosts routes every model hostname to the dashboard dial.
// Without it, or without WithDashboardDial, no model route is built.
func WithModelHosts(src ModelHostSource) Option {
	return func(c *Controller) { c.modelHosts = src }
}

func (c *Controller) modelRoutes(ctx context.Context, claimedHosts map[string]string) ([]ingress.ProxyRoute, error) {
	if c.modelHosts == nil || c.dashboardDial == "" {
		return nil, nil
	}
	hosts, err := c.modelHosts.ModelHosts(ctx)
	if err != nil {
		return nil, err
	}
	var free []string
	for _, h := range hosts {
		if owner, _, dup := firstDuplicateHost(modelRouteOwner, []string{h}, claimedHosts); dup {
			c.logger.WarnContext(ctx, "ingress: model host is already routed to another resource, skipping",
				slog.String("domain", h), slog.String("already_routed_to", owner))
			continue
		}
		claimedHosts[h] = modelRouteOwner
		free = append(free, h)
	}
	if len(free) == 0 {
		return nil, nil
	}
	return []ingress.ProxyRoute{{Hosts: free, BackendDial: c.dashboardDial}}, nil
}

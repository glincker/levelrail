package ingress

import (
	"context"
	"log/slog"

	"github.com/GLINCKER/levelrail/internal/canary"
	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/store"
)

// CanarySource lists in-flight canaries. nil disables traffic splitting.
type CanarySource interface {
	ListCanaryReleases(ctx context.Context) ([]store.CanaryRelease, error)
}

// WithCanaries splits traffic between an app and its canary clone.
func WithCanaries(src CanarySource) Option {
	return func(c *Controller) { c.canarySource = src }
}

// WithCanaryDials records which upstream serves each canary and its stable
// release, so the canary reconciler can compare their request stats.
func WithCanaryDials(d *canary.Dials) Option {
	return func(c *Controller) { c.canaryDials = d }
}

func (c *Controller) noteCanaryDials(app string, pair *canary.Pair) {
	if c.canaryDials == nil {
		return
	}
	if pair == nil {
		c.canaryDials.Delete(app)
		return
	}
	c.canaryDials.Set(app, *pair)
}

// canaryPlan is one app's canary together with its hidden clone service.
type canaryPlan struct {
	release store.CanaryRelease
	service store.DesiredService
}

func (c *Controller) activeCanaries(ctx context.Context, services []store.DesiredService) map[string]canaryPlan {
	if c.canarySource == nil {
		return nil
	}
	releases, err := c.canarySource.ListCanaryReleases(ctx)
	if err != nil {
		c.logger.WarnContext(ctx, "ingress: list canaries failed, routing to stable only", slog.String("error", err.Error()))
		return nil
	}
	byName := make(map[string]store.DesiredService, len(releases))
	for _, svc := range services {
		byName[svc.Name] = svc
	}
	out := make(map[string]canaryPlan, len(releases))
	for _, rel := range releases {
		if svc, ok := byName[rel.CanaryService]; ok {
			out[rel.ServiceName] = canaryPlan{release: rel, service: svc}
		}
	}
	return out
}

// canaryRoute returns a weighted pool of the stable dial and the canary's
// dial, or nil (all traffic to stable) when the canary is paused, absent or
// not ready, so a bad canary can never take the app down.
func (c *Controller) canaryRoute(ctx context.Context, stableDial string, plan canaryPlan, ready map[string]bool) *ingress.LBRoute {
	if plan.release.ServiceName == "" || plan.release.Weight <= 0 {
		c.noteCanaryDials(plan.release.ServiceName, nil)
		return nil
	}
	canaryDial, ok := c.dialForService(ctx, plan.service, ready[plan.service.Name])
	if !ok {
		c.noteCanaryDials(plan.release.ServiceName, nil)
		return nil
	}
	c.noteCanaryDials(plan.release.ServiceName, &canary.Pair{Stable: stableDial, Canary: canaryDial})
	weight := min(plan.release.Weight, 99)
	return &ingress.LBRoute{
		Upstreams: []string{stableDial, canaryDial},
		Weights:   []int{100 - weight, weight},
		Policy:    ingress.LBPolicyWeighted,
	}
}

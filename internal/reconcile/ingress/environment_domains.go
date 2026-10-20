package ingress

import (
	"context"
	"log/slog"

	"github.com/GLINCKER/levelrail/internal/store"
)

// EnvironmentDomainSource lists per-environment domain sets (migration 0385).
// The ServiceStore may implement it; without it every app routes its default
// set, which is the behavior from before environment domains existed.
type EnvironmentDomainSource interface {
	ListAllServiceEnvironmentDomains(ctx context.Context) (map[string]map[string][]string, error)
}

// environmentDomainSets reads fresh every pass. A read failure degrades to
// default sets only, never blocks the whole reconcile.
func (c *Controller) environmentDomainSets(ctx context.Context) map[string]map[string][]string {
	src, ok := c.store.(EnvironmentDomainSource)
	if !ok {
		return nil
	}
	sets, err := src.ListAllServiceEnvironmentDomains(ctx)
	if err != nil {
		c.logger.WarnContext(ctx, "ingress: list environment domains failed, routing default domain sets this pass", slog.String("error", err.Error()))
		return nil
	}
	return sets
}

// routedDomains is the host set svc routes this pass: its active
// environment's own set when it has one, else its default domains.
func routedDomains(svc store.DesiredService, sets map[string]map[string][]string) []string {
	return store.EffectiveServiceDomains(svc.Domains, svc.EnvironmentID, sets[svc.Name])
}

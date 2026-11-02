package api

import (
	"context"
	"time"

	"github.com/GLINCKER/levelrail/internal/certflow"
)

// annotateDomainEnvAndExpiry adds the owning app's environment and the covering
// certificate's days left. Best effort: a store error leaves the fields empty.
func (rt *Router) annotateDomainEnvAndExpiry(ctx context.Context, rows []domainResource) {
	if svcs, err := rt.apps.ListDesiredServices(ctx); err == nil {
		names := rt.environmentNames(ctx, svcs)
		envOf := make(map[string]string, len(svcs))
		for _, s := range svcs {
			envOf[s.Name] = s.EnvironmentID
		}
		for i := range rows {
			if id := envOf[rows[i].ServiceName]; id != "" {
				rows[i].EnvironmentID, rows[i].Environment = id, names[id]
			}
		}
	}
	now := time.Now()
	certs, err := rt.certTruthRows(ctx, now)
	if err != nil {
		return
	}
	for i := range rows {
		var best *certTruthRow
		for j := range certs {
			if certInfoCovers(certs[j].Info, rows[i].Domain) && (best == nil || certs[j].Info.NotAfter.After(best.Info.NotAfter)) {
				best = &certs[j]
			}
		}
		if best != nil {
			days := certflow.DaysLeft(best.Info.NotAfter, now)
			rows[i].TLSDaysLeft = &days
		}
	}
}

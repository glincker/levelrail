package ingress

import "github.com/GLINCKER/levelrail/internal/ingress"

// publishRequestHostOwners tells the request stats aggregator which app owns
// each routed host, leaving out the platform's own dashboard, registry and
// model routes.
func (c *Controller) publishRequestHostOwners(claimedHosts map[string]string) {
	owners := make(map[string]string, len(claimedHosts))
	for host, owner := range claimedHosts {
		switch owner {
		case dashboardRouteOwner, registryRouteOwner, modelRouteOwner, standaloneRouteOwner:
			continue
		}
		owners[host] = owner
	}
	// A host that left or changed hands must not show its old rows to the new owner.
	for _, key := range ingress.DefaultRequestStats().ReplaceHostOwners(owners) {
		ingress.DefaultAccessLog().Purge(key)
	}
}

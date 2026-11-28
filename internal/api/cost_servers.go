package api

import (
	"context"
	"time"

	"github.com/glincker/levelrail/internal/runtimesettings"
	"github.com/glincker/levelrail/internal/usagecost"
)

// costServerShares shares each node's bill out per app. Node prices come from the
// usage cost nodes, falling back to the advisor's advisor.node_prices; a node
// with neither stays unpriced and its shares are null.
func (rt *Router) costServerShares(ctx context.Context, rows []usagecost.DailyCost, nodes map[string]usagecost.NodePrice, s usagecost.Settings, now time.Time) []usagecost.ServerShare {
	merged := make(map[string]usagecost.NodePrice, len(nodes))
	for id, n := range nodes {
		merged[rt.advisorNodeOf(id)] = n
	}
	if rt.runtimeSettings != nil {
		prices, _ := runtimesettings.ParseNodePrices(rt.runtimeSettings.String(ctx, runtimesettings.KeyAdvisorNodePrices))
		for id, price := range prices {
			if n, ok := merged[id]; ok && n.Priced() {
				continue
			}
			n := merged[id]
			n.NodeID, n.MonthlyPrice = id, price
			if n.VCPUs <= 0 || n.MemoryGiB <= 0 {
				v, m := rt.usageNodeCapacity(ctx, id, now)
				if n.VCPUs <= 0 {
					n.VCPUs = v
				}
				if n.MemoryGiB <= 0 {
					n.MemoryGiB = m
				}
			}
			merged[id] = n
		}
	}
	norm := make([]usagecost.DailyCost, len(rows))
	for i, r := range rows {
		r.NodeID = rt.advisorNodeOf(r.NodeID)
		norm[i] = r
	}
	savings := map[string]float64{}
	for _, f := range rt.advisorFindingsCached() {
		if f.Impact.MonthlySaving != nil {
			savings[f.Resource.String()] += *f.Impact.MonthlySaving
		}
	}
	return usagecost.ServerShares(usagecost.ServerSharesInput{Rows: norm, Nodes: merged, Settings: s, Savings: savings})
}

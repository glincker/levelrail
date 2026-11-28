package usagecost

import (
	"sort"
)

// ServerAppShare is one app or database's slice of a node's monthly bill.
// Share and Saving are nil when the node has no price, never zero.
type ServerAppShare struct {
	Kind   string   `json:"kind"`
	Name   string   `json:"name"`
	Share  *float64 `json:"share"`
	Pct    *float64 `json:"share_pct"`
	Saving *float64 `json:"monthly_saving"`
}

// ServerShare is one node's bill shared out per app, with the unused rest.
type ServerShare struct {
	NodeID       string           `json:"node_id"`
	MonthlyPrice *float64         `json:"monthly_price"`
	Idle         *float64         `json:"idle_share"`
	Apps         []ServerAppShare `json:"apps"`
}

// ServerSharesInput carries usage rows already restricted to the caller and
// node ids already normalized to the same names the price map uses.
type ServerSharesInput struct {
	Rows     []DailyCost
	Nodes    map[string]NodePrice
	Settings Settings
	// Savings is the advisor's monthly saving per "kind:name"; absent means unknown.
	Savings map[string]float64
}

type serverKey struct{ kind, name string }

// ServerShares splits each priced node's monthly price by the measured
// average share of its CPU and memory (CPUWeightPct against the rest, as in
// node-mode allocation). Usage over capacity is scaled down, so shares never
// exceed the price. Unpriced nodes list their apps with null shares.
func ServerShares(in ServerSharesInput) []ServerShare {
	s := in.Settings.normalized()
	days := map[string]map[string]bool{}
	weight := map[string]map[serverKey]float64{}
	for _, r := range in.Rows {
		if r.Kind != KindApp && r.Kind != KindDatabase {
			continue
		}
		if weight[r.NodeID] == nil {
			weight[r.NodeID], days[r.NodeID] = map[serverKey]float64{}, map[string]bool{}
		}
		days[r.NodeID][r.Day] = true
		weight[r.NodeID][serverKey{r.Kind, r.Name}] += usageWeight(r.DailyUsage, in.Nodes[r.NodeID], s)
	}
	ids := make([]string, 0, len(weight))
	for id := range weight {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]ServerShare, 0, len(ids))
	for _, id := range ids {
		out = append(out, oneServer(id, in, weight[id], float64(len(days[id]))))
	}
	return out
}

func oneServer(id string, in ServerSharesInput, weight map[serverKey]float64, n float64) ServerShare {
	np := in.Nodes[id]
	srv := ServerShare{NodeID: id, Apps: []ServerAppShare{}}
	sum := 0.0
	for _, w := range weight {
		sum += w / n
	}
	scale := 1.0
	if sum > 1 {
		scale = 1 / sum
	}
	if np.Priced() {
		price := np.MonthlyPrice
		idle := price * (1 - sum*scale)
		srv.MonthlyPrice, srv.Idle = &price, &idle
	}
	for k, w := range weight {
		a := ServerAppShare{Kind: k.kind, Name: k.name}
		if np.Priced() {
			frac := w / n * scale
			share, pct := np.MonthlyPrice*frac, frac*100
			a.Share, a.Pct = &share, &pct
			if v, ok := in.Savings[k.kind+":"+k.name]; ok {
				a.Saving = &v
			}
		}
		srv.Apps = append(srv.Apps, a)
	}
	sort.Slice(srv.Apps, func(i, j int) bool {
		a, b := srv.Apps[i], srv.Apps[j]
		if a.Share != nil && b.Share != nil && *a.Share != *b.Share {
			return *a.Share > *b.Share
		}
		return a.Kind+a.Name < b.Kind+b.Name
	})
	return srv
}

// usageWeight is the fraction of the node one resource held over one day.
func usageWeight(u DailyUsage, np NodePrice, s Settings) float64 {
	cpuW, memW := s.CPUWeightPct/100, 1-s.CPUWeightPct/100
	if np.VCPUs <= 0 {
		cpuW, memW = 0, 1
	}
	if np.MemoryGiB <= 0 {
		cpuW, memW = 1, 0
	}
	w := 0.0
	if np.VCPUs > 0 {
		w += cpuW * u.cores(s.Basis) * u.UpSeconds / secondsPerDay / np.VCPUs
	}
	if np.MemoryGiB > 0 {
		w += memW * u.memBytes(s.Basis) / gib * u.UpSeconds / secondsPerDay / np.MemoryGiB
	}
	return w
}

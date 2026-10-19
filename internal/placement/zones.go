package placement

import "sort"

// Well-known node label keys that name a failure domain.
const (
	LabelRegion   = "region"
	LabelZone     = "zone"
	LabelProvider = "provider"
)

// ZoneOf returns the node's failure domain: its zone label, or "" when unlabeled.
func ZoneOf(labels map[string]string) string { return labels[LabelZone] }

// zoneSpread is the per-zone view used to rank candidates. It is only
// active when at least one eligible node carries a zone label.
type zoneSpread struct {
	active   bool
	siblings map[string]int
	load     map[string]int
}

func newZoneSpread(cands []Candidate, evals []Evaluation, peers []string) zoneSpread {
	zones := make(map[string]string, len(cands))
	z := zoneSpread{siblings: map[string]int{}, load: map[string]int{}}
	for _, c := range cands {
		zone := ZoneOf(c.Labels)
		zones[c.ID] = zone
		z.load[zone] += c.Load
	}
	for _, e := range evals {
		if e.Eligible && zones[e.NodeID] != "" {
			z.active = true
		}
	}
	for _, id := range peers {
		z.siblings[zones[id]]++
	}
	return z
}

// applyZones stamps each evaluation with its zone and region.
func applyZones(cands []Candidate, evals []Evaluation) {
	byID := make(map[string]map[string]string, len(cands))
	for _, c := range cands {
		byID[c.ID] = c.Labels
	}
	for i := range evals {
		evals[i].Zone = ZoneOf(byID[evals[i].NodeID])
		evals[i].Region = byID[evals[i].NodeID][LabelRegion]
	}
}

// rankZones orders eligible evaluations: the zone holding the fewest peer
// replicas first, then the least loaded zone, then the usual headroom order.
func rankZones(evals []Evaluation, z zoneSpread) []Evaluation {
	out := rank(evals)
	if !z.active {
		return out
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if z.siblings[a.Zone] != z.siblings[b.Zone] {
			return z.siblings[a.Zone] < z.siblings[b.Zone]
		}
		if z.load[a.Zone] != z.load[b.Zone] {
			return z.load[a.Zone] < z.load[b.Zone]
		}
		return betterThan(a, b)
	})
	return out
}

// ZoneSummary is one failure domain with the workloads it carries.
type ZoneSummary struct {
	Zone      string   `json:"zone"`
	Region    string   `json:"region,omitempty"`
	Providers []string `json:"providers,omitempty"`
	NodeIDs   []string `json:"node_ids"`
	Online    int      `json:"online_nodes"`
	Workloads int      `json:"workloads"`
}

// Zones groups candidates by zone (unlabeled nodes share the "" zone).
func Zones(cands []Candidate) []ZoneSummary {
	idx := map[string]*ZoneSummary{}
	provs := map[string]map[string]bool{}
	var order []string
	for _, c := range cands {
		zone := ZoneOf(c.Labels)
		s, ok := idx[zone]
		if !ok {
			s = &ZoneSummary{Zone: zone, NodeIDs: []string{}}
			idx[zone] = s
			provs[zone] = map[string]bool{}
			order = append(order, zone)
		}
		if s.Region == "" {
			s.Region = c.Labels[LabelRegion]
		}
		if p := c.Labels[LabelProvider]; p != "" {
			provs[zone][p] = true
		}
		s.NodeIDs = append(s.NodeIDs, c.ID)
		s.Workloads += c.Load
		if c.Online {
			s.Online++
		}
	}
	sort.Strings(order)
	out := make([]ZoneSummary, 0, len(order))
	for _, zone := range order {
		s := idx[zone]
		for p := range provs[zone] {
			s.Providers = append(s.Providers, p)
		}
		sort.Strings(s.Providers)
		sort.Strings(s.NodeIDs)
		out = append(out, *s)
	}
	return out
}

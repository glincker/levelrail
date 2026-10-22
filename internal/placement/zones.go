package placement

import (
	"os"
	"sort"
	"strconv"
)

// EnvRegionSpreadWeight names the env var weighting region spread in replica
// scoring. 0 turns region preference off.
const EnvRegionSpreadWeight = "APP_PLACEMENT_REGION_SPREAD_WEIGHT"

// EnvProviderSpreadWeight names the env var weighting provider spread in
// replica scoring. 0 turns provider preference off.
const EnvProviderSpreadWeight = "APP_PLACEMENT_PROVIDER_SPREAD_WEIGHT"

const (
	defaultRegionSpreadWeight   = 1
	defaultProviderSpreadWeight = 1
)

func regionSpreadWeight() int { return spreadWeight(EnvRegionSpreadWeight, defaultRegionSpreadWeight) }

func providerSpreadWeight() int {
	return spreadWeight(EnvProviderSpreadWeight, defaultProviderSpreadWeight)
}

func spreadWeight(name string, def int) int {
	v, ok := os.LookupEnv(name)
	if !ok {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return def
	}
	return n
}

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
	regions  map[string]int
	weight   int
	provs    map[string]int
	provW    int
}

func newZoneSpread(cands []Candidate, evals []Evaluation, peers []string) zoneSpread {
	zones := make(map[string]string, len(cands))
	regions := make(map[string]string, len(cands))
	provs := make(map[string]string, len(cands))
	z := zoneSpread{siblings: map[string]int{}, load: map[string]int{}, regions: map[string]int{}, weight: regionSpreadWeight(), provs: map[string]int{}, provW: providerSpreadWeight()}
	for _, c := range cands {
		zone := ZoneOf(c.Labels)
		zones[c.ID] = zone
		regions[c.ID] = c.Labels[LabelRegion]
		provs[c.ID] = c.Labels[LabelProvider]
		z.load[zone] += c.Load
	}
	for _, e := range evals {
		if e.Eligible && (zones[e.NodeID] != "" || (z.weight > 0 && regions[e.NodeID] != "") || (z.provW > 0 && provs[e.NodeID] != "")) {
			z.active = true
		}
	}
	for _, id := range peers {
		z.siblings[zones[id]]++
		z.regions[regions[id]]++
		z.provs[provs[id]]++
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
		evals[i].Provider = byID[evals[i].NodeID][LabelProvider]
	}
}

// regionScore is the weighted peer count of a region; unlabeled regions score
// 0 so they never outrank a labeled one by accident.
func (z zoneSpread) regionScore(region string) int {
	if region == "" || z.weight == 0 {
		return 0
	}
	return z.regions[region] * z.weight
}

func (z zoneSpread) providerScore(provider string) int {
	if provider == "" || z.provW == 0 {
		return 0
	}
	return z.provs[provider] * z.provW
}

// rankZones orders eligible evaluations: the zone holding the fewest peer
// replicas first, then the region and then the provider holding the fewest, then the least loaded zone, then the usual headroom order.
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
		if ra, rb := z.regionScore(a.Region), z.regionScore(b.Region); ra != rb {
			return ra < rb
		}
		if pa, pb := z.providerScore(a.Provider), z.providerScore(b.Provider); pa != pb {
			return pa < pb
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

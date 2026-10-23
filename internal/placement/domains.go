package placement

import (
	"fmt"
	"sort"
	"strings"
)

// Domain risk kinds.
const (
	RiskReplicasOneZone     = "replicas_one_zone"
	RiskReplicasOneProvider = "replicas_one_provider"
)

// Risk levels.
const (
	LevelWarn = "warn"
	LevelInfo = "info"
)

// DomainGroup is the capacity of every node sharing one provider, region or
// zone. Capacity sums only what nodes reported; Unverified counts the nodes
// that reported nothing, so a total is never mistaken for the whole fleet.
type DomainGroup struct {
	Name          string   `json:"name"`
	Region        string   `json:"region,omitempty"`
	Providers     []string `json:"providers,omitempty"`
	NodeIDs       []string `json:"node_ids"`
	Nodes         int      `json:"nodes"`
	Online        int      `json:"online_nodes"`
	Schedulable   int      `json:"schedulable_nodes"`
	Workloads     int      `json:"workloads"`
	CPUMilli      int64    `json:"cpu_milli"`
	MemoryBytes   int64    `json:"memory_bytes"`
	DiskFreeBytes int64    `json:"disk_free_bytes"`
	Unverified    int      `json:"unverified_nodes"`
}

// DomainOverview is the fleet grouped three ways. Unlabeled nodes are counted
// separately instead of being hidden in an empty-named group.
type DomainOverview struct {
	Providers []DomainGroup `json:"providers"`
	Regions   []DomainGroup `json:"regions"`
	Zones     []DomainGroup `json:"zones"`
	Unlabeled int           `json:"unlabeled_nodes"`
	Total     int           `json:"total_nodes"`
}

// Domains groups candidates by provider, region and zone. Zones are keyed by
// region and zone together because zone names repeat across regions.
func Domains(cands []Candidate) DomainOverview {
	out := DomainOverview{Total: len(cands)}
	provs := newGrouper()
	regs := newGrouper()
	zones := newGrouper()
	for _, c := range cands {
		p, r, z := c.Labels[LabelProvider], c.Labels[LabelRegion], ZoneOf(c.Labels)
		if p == "" && r == "" && z == "" {
			out.Unlabeled++
		}
		provs.add(c, p, "", "")
		regs.add(c, r, r, p)
		zk := z
		if z != "" && r != "" {
			zk = r + "/" + z
		}
		zones.add(c, zk, r, p)
	}
	out.Providers, out.Regions, out.Zones = provs.groups(), regs.groups(), zones.groups()
	return out
}

type grouper struct {
	idx   map[string]*DomainGroup
	provs map[string]map[string]bool
}

func newGrouper() *grouper {
	return &grouper{idx: map[string]*DomainGroup{}, provs: map[string]map[string]bool{}}
}

func (g *grouper) add(c Candidate, name, region, provider string) {
	if name == "" {
		return
	}
	d, ok := g.idx[name]
	if !ok {
		d = &DomainGroup{Name: name, Region: region, NodeIDs: []string{}}
		g.idx[name] = d
		g.provs[name] = map[string]bool{}
	}
	if provider != "" {
		g.provs[name][provider] = true
	}
	d.NodeIDs = append(d.NodeIDs, c.ID)
	d.Nodes++
	d.Workloads += c.Load
	if c.Online {
		d.Online++
	}
	if c.Schedulable && c.Online {
		d.Schedulable++
	}
	if c.Capacity.CPUMilli.Known && c.Capacity.MemoryBytes.Known {
		d.CPUMilli += c.Capacity.CPUMilli.Value
		d.MemoryBytes += c.Capacity.MemoryBytes.Value
		if c.Capacity.DiskFreeBytes.Known {
			d.DiskFreeBytes += c.Capacity.DiskFreeBytes.Value
		}
	} else {
		d.Unverified++
	}
}

func (g *grouper) groups() []DomainGroup {
	names := make([]string, 0, len(g.idx))
	for n := range g.idx {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]DomainGroup, 0, len(names))
	for _, n := range names {
		d := *g.idx[n]
		for p := range g.provs[n] {
			d.Providers = append(d.Providers, p)
		}
		sort.Strings(d.Providers)
		sort.Strings(d.NodeIDs)
		out = append(out, d)
	}
	return out
}

// DomainRisk says a workload's replicas share one failure domain although
// another domain could host a copy. All replicas of a workload run on one
// node today, so a workload with two or more replicas always shares fate.
type DomainRisk struct {
	Kind         string   `json:"kind"`
	Level        string   `json:"level"`
	Name         string   `json:"name"`
	NodeID       string   `json:"node_id"`
	Replicas     int      `json:"replicas"`
	Zone         string   `json:"zone,omitempty"`
	Region       string   `json:"region,omitempty"`
	Provider     string   `json:"provider,omitempty"`
	Alternatives []string `json:"alternatives"`
	Message      string   `json:"message"`
}

// RiskInput is one workload for DomainRisks.
type RiskInput struct {
	Name     string
	NodeID   string
	Replicas int
	Selector Selector
}

// DomainRisks lists workloads with two or more replicas whose node sits in a
// labeled zone or provider while an eligible node exists in a different one.
// The result is sorted by name so the same fleet always gives the same list.
func DomainRisks(cands []Candidate, in []RiskInput) []DomainRisk {
	byID := make(map[string]Candidate, len(cands))
	for _, c := range cands {
		byID[c.ID] = c
	}
	var out []DomainRisk
	for _, w := range in {
		c, ok := byID[w.NodeID]
		if !ok || w.Replicas < 2 {
			continue
		}
		zone, provider := ZoneOf(c.Labels), c.Labels[LabelProvider]
		if r, ok := riskFor(cands, w, c, RiskReplicasOneZone, zone, ZoneOf, LevelWarn, "zone"); ok {
			out = append(out, r)
		}
		if r, ok := riskFor(cands, w, c, RiskReplicasOneProvider, provider, func(l map[string]string) string { return l[LabelProvider] }, LevelInfo, "provider"); ok {
			out = append(out, r)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].Kind < out[j].Kind
	})
	return out
}

func riskFor(cands []Candidate, w RiskInput, home Candidate, kind, homeValue string, domainOf func(map[string]string) string, level, noun string) (DomainRisk, bool) {
	if homeValue == "" {
		return DomainRisk{}, false
	}
	seen := map[string]bool{}
	for _, c := range cands {
		d := domainOf(c.Labels)
		if d == "" || d == homeValue || c.ID == home.ID {
			continue
		}
		if Filter(c, Request{Selector: w.Selector}) == "" {
			seen[d] = true
		}
	}
	if len(seen) == 0 {
		return DomainRisk{}, false
	}
	alts := make([]string, 0, len(seen))
	for d := range seen {
		alts = append(alts, d)
	}
	sort.Strings(alts)
	return DomainRisk{
		Kind: kind, Level: level, Name: w.Name, NodeID: w.NodeID, Replicas: w.Replicas,
		Zone: ZoneOf(home.Labels), Region: home.Labels[LabelRegion], Provider: home.Labels[LabelProvider],
		Alternatives: alts,
		Message:      fmt.Sprintf("all %d replicas of %s run in %s %s, so losing it takes the app down; %s has an eligible node", w.Replicas, w.Name, noun, homeValue, strings.Join(alts, ", ")),
	}, true
}

package placement

import (
	"fmt"
	"sort"
	"strings"
)

// Evaluation is how one node fared for one placement request.
type Evaluation struct {
	NodeID     string      `json:"node_id"`
	Name       string      `json:"name"`
	Zone       string      `json:"zone,omitempty"`
	Region     string      `json:"region,omitempty"`
	Provider   string      `json:"provider,omitempty"`
	Eligible   bool        `json:"eligible"`
	Reason     string      `json:"reason,omitempty"`
	Capacity   bool        `json:"capacity_blocked,omitempty"`
	Unverified bool        `json:"unverified,omitempty"`
	Headroom   *int        `json:"headroom_permille,omitempty"`
	Load       int         `json:"load"`
	Dimensions []Dimension `json:"dimensions"`
}

// Evaluate checks c against req: the structural filter first, then each
// resource. An unknown dimension never fails the node by itself, it marks
// the node Unverified (or fails it when the policy rejects unknowns).
func Evaluate(c Candidate, req Request) Evaluation {
	pol := req.Policy.withDefaults()
	e := Evaluation{NodeID: c.ID, Name: c.Name, Load: c.Load}
	e.Dimensions = []Dimension{
		dimension(DimCPU, c.Capacity.CPUMilli, pol.CPUOvercommitPercent, c.Requested.CPUMilli, req.Needs.CPUMilli),
		dimension(DimMemory, c.Capacity.MemoryBytes, pol.MemoryOvercommitPercent, c.Requested.MemoryBytes, req.Needs.MemoryBytes),
		diskDimension(c.Capacity, pol.MinFreeDiskPercent, req.Needs.DiskBytes),
	}
	if reason := Filter(c, req); reason != "" {
		e.Reason = reason
		return e
	}
	var short, unknown []string
	for _, d := range e.Dimensions {
		switch {
		case d.Allocatable == nil && (d.Need > 0 || d.Name == DimDisk):
			unknown = append(unknown, d.Name)
		case d.Shortfall > 0:
			short = append(short, d.Describe())
		}
	}
	switch {
	case len(short) > 0:
		e.Reason, e.Capacity = "insufficient capacity: "+strings.Join(short, "; "), true
		return e
	case len(unknown) > 0 && pol.RejectUnknown:
		e.Reason = "capacity unknown for " + strings.Join(unknown, ", ") + " and the policy rejects unverifiable nodes"
		return e
	}
	e.Eligible = true
	e.Unverified = len(unknown) > 0
	e.Headroom = headroom(e.Dimensions)
	return e
}

// headroom is the share of its tightest known resource a node keeps free
// after the placement, in permille, so integer math decides ties.
func headroom(dims []Dimension) *int {
	var best *int
	for _, d := range dims {
		if d.Allocatable == nil || *d.Allocatable <= 0 {
			continue
		}
		p := int((*d.Free - d.Need) * 1000 / *d.Allocatable)
		if best == nil || p < *best {
			best = &p
		}
	}
	return best
}

func betterThan(a, b Evaluation) bool {
	if a.Unverified != b.Unverified {
		return !a.Unverified
	}
	switch {
	case a.Headroom != nil && b.Headroom != nil && *a.Headroom != *b.Headroom:
		return *a.Headroom > *b.Headroom
	case (a.Headroom == nil) != (b.Headroom == nil):
		return a.Headroom != nil
	case a.Load != b.Load:
		return a.Load < b.Load
	}
	return a.NodeID < b.NodeID
}

func rank(evals []Evaluation) []Evaluation {
	out := make([]Evaluation, 0, len(evals))
	for _, e := range evals {
		if e.Eligible {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return betterThan(out[i], out[j]) })
	return out
}

// explainWinner states why w was picked among ranked.
func explainWinner(ranked []Evaluation, prefer, zoned bool) string {
	w := ranked[0]
	switch {
	case prefer:
		return "preferred node: it serves ingress locally and it fits"
	case len(ranked) == 1:
		return "the only node that fits"
	case zoned && w.Zone != "":
		return "spread across failure domains: zone " + w.Zone + " carries the fewest replicas"
	case w.Unverified:
		return "no node could be verified, so the least loaded one was chosen"
	case w.Headroom != nil:
		return fmt.Sprintf("most headroom: %d%% of its tightest resource stays free", *w.Headroom/10)
	}
	return "least loaded node, ties broken by id"
}

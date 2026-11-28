package nodeinsight

import (
	"sort"

	"github.com/glincker/levelrail/kit/semver"
)

// Skew classes, stable so the dashboard can translate them.
const (
	SkewCurrent = "current"
	SkewBehind  = "behind"
	SkewAhead   = "ahead"
	SkewUnknown = "unknown"
)

// NodeVersion is what the control plane knows about one agent.
type NodeVersion struct {
	ID, Name, Version, OS, Arch string
	Online, Schedulable         bool
	Local                       bool
}

// NodeSkew is one node's position against the control plane version.
type NodeSkew struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Version     string `json:"version,omitempty"`
	OS          string `json:"os,omitempty"`
	Arch        string `json:"arch,omitempty"`
	Online      bool   `json:"online"`
	Class       string `json:"class"`
	NeedsUpdate bool   `json:"needs_update"`
}

// PlanStep is one node in the suggested upgrade order.
type PlanStep struct {
	Order  int    `json:"order"`
	NodeID string `json:"node_id"`
	Name   string `json:"name"`
	From   string `json:"from,omitempty"`
	To     string `json:"to"`
	Note   string `json:"note,omitempty"`
}

// SkewReport is the fleet view plus the upgrade plan.
type SkewReport struct {
	ControlPlaneVersion string     `json:"control_plane_version"`
	Nodes               []NodeSkew `json:"nodes"`
	Current             int        `json:"current"`
	Behind              int        `json:"behind"`
	Ahead               int        `json:"ahead"`
	Unknown             int        `json:"unknown"`
	Plan                []PlanStep `json:"plan"`
	// AutoUpdate mirrors the nodes.agent_auto_update runtime setting.
	AutoUpdate bool `json:"auto_update"`
}

// Classify compares an agent version with the control plane version.
func Classify(agent, controlPlane string) string {
	if agent == "" {
		return SkewUnknown
	}
	cmp, ok := semver.Compare(agent, controlPlane)
	switch {
	case !ok:
		return SkewUnknown
	case cmp < 0:
		return SkewBehind
	case cmp > 0:
		return SkewAhead
	}
	return SkewCurrent
}

// BuildSkew classifies every node and orders the upgrades: cordoned nodes
// first (no live placements to disturb), then online ones, offline last
// because they cannot take the new agent until they reconnect.
func BuildSkew(controlPlane string, nodes []NodeVersion) SkewReport {
	rep := SkewReport{ControlPlaneVersion: controlPlane, Nodes: []NodeSkew{}, Plan: []PlanStep{}}
	type cand struct {
		n    NodeVersion
		rank int
	}
	var todo []cand
	for _, n := range nodes {
		class := Classify(n.Version, controlPlane)
		if n.Local {
			class = SkewCurrent
		}
		rep.Nodes = append(rep.Nodes, NodeSkew{ID: n.ID, Name: n.Name, Version: n.Version, OS: n.OS, Arch: n.Arch, Online: n.Online, Class: class, NeedsUpdate: class == SkewBehind})
		switch class {
		case SkewCurrent:
			rep.Current++
		case SkewBehind:
			rep.Behind++
			rank := 1
			switch {
			case !n.Online:
				rank = 2
			case !n.Schedulable:
				rank = 0
			}
			todo = append(todo, cand{n, rank})
		case SkewAhead:
			rep.Ahead++
		default:
			rep.Unknown++
		}
	}
	sort.SliceStable(todo, func(i, j int) bool { return todo[i].rank < todo[j].rank })
	for i, c := range todo {
		note := ""
		switch c.rank {
		case 0:
			note = "cordoned"
		case 2:
			note = "offline"
		}
		rep.Plan = append(rep.Plan, PlanStep{Order: i + 1, NodeID: c.n.ID, Name: c.n.Name, From: c.n.Version, To: controlPlane, Note: note})
	}
	return rep
}

package build

// This file: the node-selection piece that routes a deploy's build step
// to a build-capable node, using the same "" == local-node convention
// migrations/0009_node_placement.sql established for service and
// database placement. Router (router.go) is what acts on the answer.

import (
	"errors"
	"sort"
)

// NodeInfo is the minimal node shape SelectBuildNode needs. Deliberately
// not store.Node: internal/build has no dependency on internal/store
// today, and build routing only ever cares about three of a node's
// fields, the same narrow, consumer-defined interface convention every
// other package boundary in this codebase already follows (e.g.
// internal/deploy's ImageBuilder/ServiceStore/SecretChecker).
type NodeInfo struct {
	// ID is the node's identifier, matching store.Node.ID.
	ID string
	// AcceptsBuildWorkloads mirrors store.Node.AcceptsBuildWorkloads
	// (migrations/0010_node_workloads.sql): whether this node has opted
	// in to running build work at all.
	AcceptsBuildWorkloads bool
	// Online reports whether this node is currently reachable through
	// the agent transport, i.e. whether
	// agent.Registry.Get for this node's ID currently succeeds. A
	// build-capable node the control plane cannot currently reach is
	// not a usable candidate, the same reasoning resolveNodeTransport
	// (cmd/levelrail/main.go) already applies to service/database
	// placement.
	Online bool
}

// ErrNoBuildNodeAvailable is returned by SelectBuildNode when at least
// one node is marked AcceptsBuildWorkloads but none of them is
// currently Online. This is distinct from "no node was ever configured
// as build-capable at all", which is not an error (SelectBuildNode
// returns "" for the control plane's own local node in that case): an
// operator who deliberately dedicated capacity to builds almost
// certainly does not want a deploy to silently fall back onto the
// control plane's own resources the moment that capacity blips offline.
// That surprise is worse than a clear, loud failure a caller can retry
// or alert on.
var ErrNoBuildNodeAvailable = errors.New("build: no build-capable node is currently online")

// SelectBuildNode picks which node a build should run on, given every
// known node.
//
//   - No node in nodes has AcceptsBuildWorkloads set: returns "" (this
//     control plane's own local node) and a nil error. This is the
//     default, zero-configuration behavior every deployment already had
//     before dedicated build nodes, matching the "" == local convention
//     migrations/0009_node_placement.sql established for service and
//     database placement: an operator who has never configured a
//     dedicated build node keeps building locally, unchanged.
//   - At least one node has AcceptsBuildWorkloads set: the Online one
//     with the lexicographically smallest ID is selected, a
//     deterministic, easy-to-reason-about tie-break rather than
//     anything load-aware; real load-based scheduling is explicitly out
//     of scope, this project's stated non-goal against building a
//     scheduler with bin-packing, affinity rules, or autoscaling in v1,
//     and this is the build-node equivalent of that same non-goal.
//   - At least one node has AcceptsBuildWorkloads set but none is
//     Online: returns ErrNoBuildNodeAvailable rather than silently
//     falling back to the local node; see the sentinel's own doc
//     comment for why.
func SelectBuildNode(nodes []NodeInfo) (string, error) {
	candidates := make([]NodeInfo, 0, len(nodes))
	for _, n := range nodes {
		if n.AcceptsBuildWorkloads {
			candidates = append(candidates, n)
		}
	}
	if len(candidates) == 0 {
		return "", nil
	}

	sort.Slice(candidates, func(i, j int) bool { return candidates[i].ID < candidates[j].ID })
	for _, n := range candidates {
		if n.Online {
			return n.ID, nil
		}
	}
	return "", ErrNoBuildNodeAvailable
}

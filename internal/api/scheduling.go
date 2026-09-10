package api

import (
	"context"
	"encoding/json"
	"sort"

	"github.com/GLINCKER/levelrail/internal/store"
)

// nodePlacementLoad is one eligible node's current app+database placement
// count, the input selectLeastLoadedNode ranks.
type nodePlacementLoad struct {
	NodeID string
	Count  int
}

// selectLeastLoadedNode returns the node with the fewest resources
// already placed on it, tie-broken by the lexicographically smallest
// node ID, the same deterministic tie-break build.SelectBuildNode already
// uses for the same reason (this project's stated non-goal against
// bin-packing or affinity-aware scheduling in v1). Returns "" (the
// local-node sentinel) when candidates is empty.
func selectLeastLoadedNode(candidates []nodePlacementLoad) string {
	if len(candidates) == 0 {
		return ""
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Count != candidates[j].Count {
			return candidates[i].Count < candidates[j].Count
		}
		return candidates[i].NodeID < candidates[j].NodeID
	})
	return candidates[0].NodeID
}

// autoPlaceNode picks a placement target for a new app or database whose
// create request omitted node_id entirely: the least-loaded registered
// node that is schedulable (not cordoned) and online, or "" (the local
// node) when auto-placement is disabled (WithAutoPlacement), no other
// node is registered, or none is eligible. This is the "simple spread
// scheduling" half of Phase 3 placement (see CLAUDE.md section 6);
// manual assignment (an explicit node_id, or PUT .../node) already
// covers the other half and always takes priority over this.
func (rt *Router) autoPlaceNode(ctx context.Context) (string, error) {
	if !rt.autoPlacementEnabled {
		return "", nil
	}
	nodes, err := rt.nodes.ListNodes(ctx)
	if err != nil {
		return "", err
	}
	if len(nodes) == 0 {
		return "", nil
	}

	services, err := rt.apps.ListDesiredServices(ctx)
	if err != nil {
		return "", err
	}
	databases, err := rt.databases.ListDesiredDatabases(ctx)
	if err != nil {
		return "", err
	}
	counts := make(map[string]int, len(nodes))
	for _, s := range services {
		counts[s.NodeID]++
	}
	for _, d := range databases {
		counts[d.NodeID]++
	}

	candidates := make([]nodePlacementLoad, 0, len(nodes))
	for _, n := range nodes {
		if !n.Schedulable || n.Status != store.NodeStatusOnline {
			continue
		}
		candidates = append(candidates, nodePlacementLoad{NodeID: n.ID, Count: counts[n.ID]})
	}
	return selectLeastLoadedNode(candidates), nil
}

// nodeIDKeyPresent reports whether body's top-level JSON object includes
// a "node_id" key at all (any value, including null or ""), the
// distinction handleCreateApp/handleCreateDatabase need between an
// explicit placement override and a genuinely omitted field: appResource/
// databaseResource's own NodeID field is a plain string, which on its
// own can't tell "the caller sent an empty string" apart from "the
// caller sent nothing." A malformed body reports false, matching the
// caller's own primary json.Unmarshal, which will have already rejected
// it by the time this is consulted.
func nodeIDKeyPresent(body []byte) bool {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(body, &probe); err != nil {
		return false
	}
	_, ok := probe["node_id"]
	return ok
}

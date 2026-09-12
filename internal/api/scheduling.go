package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
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

// selectLeastLoadedNodeExcluding is autoPlaceNode's per-resource sibling
// for handleDrainNode: same schedulable+online eligibility and the same
// least-loaded ranking, but excludes excludeNodeID (the node being
// drained) so a resource is never "moved" back onto itself. counts is
// read, not mutated: callers bump the picked node's count themselves so
// repeated calls within one drain spread resources across candidates
// instead of all landing on the same one.
func selectLeastLoadedNodeExcluding(nodes []store.Node, counts map[string]int, excludeNodeID string) string {
	candidates := make([]nodePlacementLoad, 0, len(nodes))
	for _, n := range nodes {
		if n.ID == excludeNodeID || !n.Schedulable || n.Status != store.NodeStatusOnline {
			continue
		}
		candidates = append(candidates, nodePlacementLoad{NodeID: n.ID, Count: counts[n.ID]})
	}
	return selectLeastLoadedNode(candidates)
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

// resolveCreateNodePlacement is handleCreateApp/handleCreateDatabase's
// shared node_id resolution: an explicit node_id in the body (even "")
// is validated as a placement override and left untouched in *nodeID/
// *autoPlaced; node_id omitted entirely lets autoPlaceNode pick one via
// simple spread scheduling, writing its result into both. logContext
// names the calling handler ("api: create app"/"api: create database")
// for its own error log lines. ok is false once it has already written
// the full HTTP response itself; the caller should return immediately.
func (rt *Router) resolveCreateNodePlacement(w http.ResponseWriter, r *http.Request, body []byte, nodeID *string, autoPlaced *bool, logContext string) (ok bool) {
	if nodeIDKeyPresent(body) {
		if err := rt.validatePlacementTarget(r.Context(), *nodeID); err != nil {
			rt.respondPlacementValidationError(w, err, *nodeID, logContext+": validate node failed")
			return false
		}
		return true
	}

	placed, err := rt.autoPlaceNode(r.Context())
	if err != nil {
		rt.logger.Error(logContext+": auto-place node failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return false
	}
	*nodeID = placed
	*autoPlaced = placed != ""
	return true
}

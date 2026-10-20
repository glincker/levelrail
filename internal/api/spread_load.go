package api

import "sort"

// nodePlacementLoad is one candidate node's placement count, the input
// selectLeastLoadedNode ranks for GPU spreading.
type nodePlacementLoad struct {
	NodeID string
	Count  int
}

// selectLeastLoadedNode returns the node with the fewest placements, ties
// broken by the smallest node ID, or "" (local) when candidates is empty.
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

package api

import (
	"github.com/GLINCKER/levelrail/internal/placement"
)

// selectorViolationWarnings flags a workload whose running node no longer
// matches its selector. Setting a selector never moves anything, so this is
// the only hint that the next move or drain will relocate it.
func selectorViolationWarnings(sel placement.Selector, cands []placement.Candidate, nodeID string) []string {
	if nodeID == "" || len(sel) == 0 {
		return nil
	}
	for _, c := range cands {
		if c.ID != nodeID {
			continue
		}
		if sel.Matches(c.Labels) {
			return nil
		}
		name := c.Name
		if name == "" {
			name = c.ID
		}
		return []string{"running node " + name + " does not match the selector " + sel.String() + "; nothing moves until the next move or drain"}
	}
	return nil
}

package alerting

import (
	"context"
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// EvaluateNodeOffline runs one KindNodeOffline rule: it fires while any
// node's status is offline and resolves once every node is back. Like
// the other platform-wide node kinds it has no debounce, since the
// heartbeat sweep already applies its own grace before marking a node
// offline. LastValue is the count of offline nodes.
func EvaluateNodeOffline(ctx context.Context, nodes NodeSource, r Rule, now time.Time) (Rule, []string, error) {
	all, err := nodes.ListNodes(ctx)
	if err != nil {
		return r, nil, fmt.Errorf("alerting: evaluate rule %q: list nodes: %w", r.ID, err)
	}

	var notices []string
	for _, n := range all {
		if n.Status != store.NodeStatusOffline {
			continue
		}
		name := n.Name
		if name == "" {
			name = n.ID
		}
		notices = append(notices, name+": offline")
	}

	next := r
	next.LastEvaluatedAt = &now
	v := float64(len(notices))
	next.LastValue = &v
	return advanceState(next, r, len(notices) > 0, 0, now), notices, nil
}

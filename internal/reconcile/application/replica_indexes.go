package application

import (
	"context"
	"fmt"

	"github.com/glincker/levelrail/internal/reconcile"
)

// replicaIndexFilter narrows a controller to some replica indexes of its
// service. The zero value means every replica, today's behavior.
type replicaIndexFilter struct {
	set     bool
	indexes []int
}

// WithReplicaIndexes limits the controller to the given replica indexes,
// keeping each one's original index for naming, hooks and rollout records.
// An empty list makes the controller only remove what its node holds.
func WithReplicaIndexes(indexes []int) Option {
	return func(c *Controller) {
		c.replicaIndexes = replicaIndexFilter{set: true, indexes: append([]int(nil), indexes...)}
	}
}

func (f replicaIndexFilter) targets(replicas int, name func(int) string) []string {
	if !f.set {
		out := make([]string, replicas)
		for i := range out {
			out[i] = name(i)
		}
		return out
	}
	out := make([]string, 0, len(f.indexes))
	for _, i := range f.indexes {
		if i >= 0 && i < replicas {
			out = append(out, name(i))
		}
	}
	return out
}

// original maps a position in the filtered target list back to its replica index.
func (f replicaIndexFilter) original(pos int) int {
	if f.set && pos >= 0 && pos < len(f.indexes) {
		return f.indexes[pos]
	}
	return pos
}

// holdsPrimary reports whether this controller owns replica 0, which alone
// runs hooks and writes the rollout record.
func (f replicaIndexFilter) holdsPrimary() bool {
	if !f.set {
		return true
	}
	for _, i := range f.indexes {
		if i == 0 {
			return true
		}
	}
	return false
}

// cleanSpreadNode removes everything this service left on the controller's node.
func (c *Controller) cleanSpreadNode(ctx context.Context) (reconcile.Result, error) {
	if err := c.removeStale(ctx, nil); err != nil {
		return notReady("SpreadCleanupFailed", err), fmt.Errorf("application/%s: spread cleanup: %w", c.serviceName, err)
	}
	if err := c.removeAllEgressSidecars(ctx); err != nil {
		return notReady("SpreadCleanupFailed", err), fmt.Errorf("application/%s: spread cleanup: %w", c.serviceName, err)
	}
	return ready("SpreadCleaned"), nil
}

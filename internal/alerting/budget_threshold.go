package alerting

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/spendcap"
	"github.com/GLINCKER/levelrail/internal/usagecost"
)

// BudgetSource reports each budget's month to date position. The api
// Router satisfies it.
type BudgetSource interface {
	BudgetStatuses(ctx context.Context) ([]usagecost.BudgetStatus, error)
}

// SpendCapSource is optional on a BudgetSource: per-app spend caps add their
// crossed thresholds to the same rule.
type SpendCapSource interface {
	SpendCapBreaches(ctx context.Context) ([]spendcap.Breach, error)
}

// EvaluateBudgetThreshold fires while any budget is over its warn level, its limit, or a forecast overrun.
// It returns one notice per breach and the breach keys used to decide when to notify again.
func EvaluateBudgetThreshold(ctx context.Context, src BudgetSource, r Rule, now time.Time) (Rule, []string, []string, error) {
	statuses, err := src.BudgetStatuses(ctx)
	if err != nil {
		return r, nil, nil, fmt.Errorf("alerting: evaluate rule %q: budget statuses: %w", r.ID, err)
	}
	var notices, keys []string
	var worst float64
	for _, s := range statuses {
		for _, b := range s.Breaches() {
			notices = append(notices, b.Text)
			keys = append(keys, b.Key)
		}
		if s.SpentPct > worst {
			worst = s.SpentPct
		}
	}
	if caps, ok := src.(SpendCapSource); ok {
		breaches, err := caps.SpendCapBreaches(ctx)
		if err != nil {
			return r, nil, nil, fmt.Errorf("alerting: evaluate rule %q: spend caps: %w", r.ID, err)
		}
		for _, b := range breaches {
			notices = append(notices, b.Text)
			keys = append(keys, b.Key)
		}
	}
	next := r
	next.LastEvaluatedAt = &now
	if len(statuses) > 0 {
		v := worst
		next.LastValue = &v
	}
	return advanceState(next, r, len(notices) > 0, 0, now), notices, keys, nil
}

// budgetTracker remembers which breach keys a rule already notified, so a
// rule that stays firing still announces a newly crossed threshold.
type budgetTracker struct {
	mu   sync.Mutex
	seen map[string]map[string]bool
}

func newBudgetTracker() *budgetTracker {
	return &budgetTracker{seen: map[string]map[string]bool{}}
}

// record stores keys as notified for ruleID and reports whether any was new.
// The first sighting after a restart only seeds, so a rule that was already
// firing does not announce itself again.
func (t *budgetTracker) record(ruleID string, keys []string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	prev, known := t.seen[ruleID]
	cur := make(map[string]bool, len(keys))
	fresh := false
	for _, k := range keys {
		cur[k] = true
		if !prev[k] {
			fresh = true
		}
	}
	t.seen[ruleID] = cur
	return fresh && known
}

func (t *budgetTracker) forget(ruleID string) {
	t.mu.Lock()
	delete(t.seen, ruleID)
	t.mu.Unlock()
}

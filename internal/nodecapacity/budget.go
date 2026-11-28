package nodecapacity

import "github.com/glincker/levelrail/internal/janitor"

// BudgetPlan compares a node's unused images with its disk budget. It never removes anything.
type BudgetPlan struct {
	BudgetBytes int64 `json:"budget_bytes"`
	UnusedBytes int64 `json:"unused_bytes"`
	OverBytes   int64 `json:"over_bytes"`
	// Oldest lists unused images, oldest first, until they cover OverBytes.
	Oldest []janitor.Item `json:"oldest"`
	// WouldFreeBytes counts only the listed items a safe cleanup is allowed to remove.
	WouldFreeBytes int64 `json:"would_free_bytes"`
}

// Over reports whether unused images exceed the budget.
func (b BudgetPlan) Over() bool { return b.OverBytes > 0 }

// PlanBudget reads the dangling and rollback classes of a preview, oldest first as the agent lists them.
// A budget of 0 or less means no budget and plans nothing.
func PlanBudget(p Preview, budgetBytes int64) BudgetPlan {
	plan := BudgetPlan{BudgetBytes: budgetBytes, Oldest: []janitor.Item{}}
	type cand struct {
		it        janitor.Item
		deletable bool
	}
	var cands []cand
	for _, name := range []string{ClassDanglingImages, ClassRollbackImages} {
		for _, c := range p.Classes {
			if c.Name != name {
				continue
			}
			for _, it := range c.Items {
				cands = append(cands, cand{it, c.Deletable})
				plan.UnusedBytes += max(it.SizeBytes, 0)
			}
		}
	}
	if budgetBytes <= 0 || plan.UnusedBytes <= budgetBytes {
		return plan
	}
	plan.OverBytes = plan.UnusedBytes - budgetBytes
	var covered int64
	for _, c := range cands {
		if covered >= plan.OverBytes {
			break
		}
		covered += max(c.it.SizeBytes, 0)
		plan.Oldest = append(plan.Oldest, c.it)
		if c.deletable {
			plan.WouldFreeBytes += max(c.it.SizeBytes, 0)
		}
	}
	return plan
}

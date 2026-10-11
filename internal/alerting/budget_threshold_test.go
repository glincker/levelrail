package alerting

import (
	"context"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/usagecost"
)

type fakeBudgets struct{ statuses []usagecost.BudgetStatus }

func (f fakeBudgets) BudgetStatuses(context.Context) ([]usagecost.BudgetStatus, error) {
	return f.statuses, nil
}

func TestEvaluateBudgetThreshold(t *testing.T) {
	b := usagecost.Budget{ID: "b1", Scope: usagecost.ScopeTeam, ScopeID: "t", Monthly: 100}
	tests := []struct {
		name       string
		spent      float64
		forecast   float64
		wantFiring bool
		wantKeys   int
	}{
		{"healthy", 10, 40, false, 0},
		{"warn", 85, 95, true, 1},
		{"over", 120, 150, true, 1},
		{"forecast only", 30, 140, true, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := fakeBudgets{[]usagecost.BudgetStatus{usagecost.NewStatus(b, "ops", "USD", tt.spent, tt.forecast)}}
			next, notices, keys, err := EvaluateBudgetThreshold(context.Background(), src, Rule{ID: "r", Kind: KindBudgetThreshold}, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			if next.Firing != tt.wantFiring || len(keys) != tt.wantKeys || len(notices) != tt.wantKeys {
				t.Errorf("firing=%v keys=%v notices=%v", next.Firing, keys, notices)
			}
		})
	}
}

func TestBudgetTrackerNotifiesOnlyNewThresholds(t *testing.T) {
	tr := newBudgetTracker()
	if tr.record("r", []string{"b1:warn"}) {
		t.Error("first sighting after restart must only seed")
	}
	if tr.record("r", []string{"b1:warn"}) {
		t.Error("unchanged breach notified again")
	}
	if !tr.record("r", []string{"b1:warn", "b1:forecast"}) {
		t.Error("new forecast breach was not announced")
	}
	if tr.record("r", []string{"b1:forecast"}) {
		t.Error("a lower level notified")
	}
	tr.forget("r")
	if tr.record("r", []string{"b1:over"}) {
		t.Error("record after forget must seed")
	}
}

package cloudiac

import (
	"fmt"
	"sort"
	"strings"
)

// PriceTable maps a resource type to a monthly list price in micro units.
// Implementations are static: no network lookups.
type PriceTable interface {
	// Monthly returns the monthly price for one resource and whether it is priced.
	Monthly(resourceType string) (micros int64, ok bool)
	// AsOf names the table version date shown next to every estimate.
	AsOf() string
}

// StaticPrices is an in-memory PriceTable.
type StaticPrices struct {
	Date   string
	Prices map[string]int64
}

// Monthly implements PriceTable.
func (s StaticPrices) Monthly(t string) (int64, bool) {
	v, ok := s.Prices[t]
	return v, ok
}

// AsOf implements PriceTable.
func (s StaticPrices) AsOf() string { return s.Date }

// CostPolicy bounds the estimated monthly spend of one account.
type CostPolicy struct {
	// MonthlyLimitMicros is the account limit. Zero means nothing billable is allowed.
	MonthlyLimitMicros int64
	// AllowUnpriced lets resources without a price through, not estimated.
	AllowUnpriced bool
}

// NewCostPolicy builds a cost policy from runtime setting values read by the
// caller. requireFullEstimate true blocks resources without a price.
func NewCostPolicy(limitMicros int64, requireFullEstimate bool) CostPolicy {
	return CostPolicy{MonthlyLimitMicros: limitMicros, AllowUnpriced: !requireFullEstimate}
}

// CostEstimate is the outcome of pricing a change set.
type CostEstimate struct {
	MonthlyMicros int64
	Unpriced      []string
	AsOf          string
	Refusals      []Refusal
}

// Allowed reports whether the estimate has no refusals.
func (e CostEstimate) Allowed() bool { return len(e.Refusals) == 0 }

// Estimate prices the resources a change set leaves in place plus the existing
// monthly spend, and returns the verdict against the account limit.
func (c CostPolicy) Estimate(table PriceTable, changes []ResourceChange, existingMicros int64) CostEstimate {
	est := CostEstimate{MonthlyMicros: existingMicros, AsOf: table.AsOf()}
	seen := map[string]bool{}
	for _, ch := range changes {
		var sign int64
		switch ch.Action {
		case ActionCreate:
			sign = 1
		case ActionDelete:
			sign = -1
		default:
			continue
		}
		price, ok := table.Monthly(ch.Type)
		if !ok {
			if !seen[ch.Type] {
				seen[ch.Type] = true
				est.Unpriced = append(est.Unpriced, ch.Type)
			}
			continue
		}
		est.MonthlyMicros += sign * price
	}
	if est.MonthlyMicros < 0 {
		est.MonthlyMicros = 0
	}
	sort.Strings(est.Unpriced)
	if len(est.Unpriced) > 0 && !c.AllowUnpriced {
		est.Refusals = append(est.Refusals, Refusal{Code: CodeCostUnpriced,
			Message: "no price for " + strings.Join(est.Unpriced, ", ")})
	}
	if est.MonthlyMicros > c.MonthlyLimitMicros {
		est.Refusals = append(est.Refusals, Refusal{Code: CodeCostLimit,
			Message: fmt.Sprintf("estimated %d exceeds limit %d (micro units, list price as of %s)",
				est.MonthlyMicros, c.MonthlyLimitMicros, est.AsOf)})
	}
	return est
}

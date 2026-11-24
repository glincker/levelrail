// Package doctor holds control plane self checks that turn live measurements into findings.
package doctor

import (
	"sort"
	"time"
)

// CheckReconcilerLagging is the stable id of the lag check; the dashboard and CLI localize it.
const CheckReconcilerLagging = "reconciler_lagging"

// Status of one check.
type Status string

// Status values.
const (
	StatusOK   Status = "ok"
	StatusWarn Status = "warn"
)

// Lagging is one reconciler over the warn threshold.
type Lagging struct {
	Reconciler string        `json:"reconciler"`
	Lag        time.Duration `json:"lag_ns"`
}

// Check is the result of the reconciler lag check.
type Check struct {
	ID        string        `json:"id"`
	Status    Status        `json:"status"`
	Threshold time.Duration `json:"threshold_ns"`
	Lagging   []Lagging     `json:"lagging,omitempty"`
}

// ReconcilerLag flags every reconciler whose oldest unobserved change is at or
// above warn, worst first. A non-positive warn disables the check.
func ReconcilerLag(lag map[string]time.Duration, warn time.Duration) Check {
	c := Check{ID: CheckReconcilerLagging, Status: StatusOK, Threshold: warn}
	if warn <= 0 {
		return c
	}
	for k, d := range lag {
		if d >= warn {
			c.Lagging = append(c.Lagging, Lagging{Reconciler: k, Lag: d})
		}
	}
	sort.Slice(c.Lagging, func(i, j int) bool {
		if c.Lagging[i].Lag != c.Lagging[j].Lag {
			return c.Lagging[i].Lag > c.Lagging[j].Lag
		}
		return c.Lagging[i].Reconciler < c.Lagging[j].Reconciler
	})
	if len(c.Lagging) > 0 {
		c.Status = StatusWarn
	}
	return c
}

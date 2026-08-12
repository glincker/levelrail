package reconcile

import "time"

// ConditionStatus mirrors Kubernetes' tri-state convention deliberately:
// "Unknown" is a real, distinct state from "False" — a controller that
// hasn't finished its first reconcile yet is not the same as one that has
// confirmed failure.
type ConditionStatus string

const (
	ConditionTrue    ConditionStatus = "True"
	ConditionFalse   ConditionStatus = "False"
	ConditionUnknown ConditionStatus = "Unknown"
)

// Condition is a single named status a controller reports after each
// reconcile, always with a machine-readable Reason. CLAUDE.md 4.2: "Every
// reconcile emits a status condition with a reason string."
type Condition struct {
	Type               string
	Status             ConditionStatus
	Reason             string
	Message            string
	LastTransitionTime time.Time
}

// Result is what a Controller returns from one Reconcile call.
type Result struct {
	Conditions []Condition
}

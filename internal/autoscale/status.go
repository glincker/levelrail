package autoscale

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Condition is the controller's status for one app.
type Condition struct {
	Healthy bool   `json:"healthy"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

// Reasons only Status produces.
const (
	ReasonPending  = "Pending"
	ReasonFlapping = "Flapping"
)

// Status is one app's policy with its live position and condition.
type Status struct {
	Policy
	EffectiveMax int        `json:"effective_max"`
	Current      int        `json:"current"`
	Desired      int        `json:"desired"`
	Value        float64    `json:"value"`
	ValueKnown   bool       `json:"value_known"`
	BlockedBy    string     `json:"blocked_by"`
	Flaps1h      int        `json:"flaps_1h"`
	Flapping     bool       `json:"flapping"`
	LastScaleUp  *time.Time `json:"last_scale_up,omitempty"`
	LastScaleDn  *time.Time `json:"last_scale_down,omitempty"`
	EvaluatedAt  *time.Time `json:"evaluated_at,omitempty"`
	Condition    Condition  `json:"condition"`
}

var unhealthyReasons = map[string]bool{
	ReasonMetricsUnavailable: true, ReasonNoCapacity: true, ReasonSpendCapPaused: true,
	ReasonAtMaximum: true, ReasonScaleFailed: true,
}

// Status returns the policy, the live replica count and the last decision.
func (c *Controller) Status(ctx context.Context, app string) (Status, error) {
	p, err := c.Store.GetAutoscalePolicy(ctx, app)
	if err != nil {
		return Status{}, err
	}
	current, _, err := c.Store.AutoscaleReplicas(ctx, app)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return Status{}, fmt.Errorf("autoscale: load replicas: %w", err)
	}
	st, ok, err := c.Store.GetAutoscaleState(ctx, app)
	if err != nil {
		return Status{}, fmt.Errorf("autoscale: load state: %w", err)
	}
	events, err := c.Store.ListAutoscaleEvents(ctx, app, flapScanLimit)
	if err != nil {
		return Status{}, fmt.Errorf("autoscale: load history: %w", err)
	}
	current = max(current, 1)
	out := Status{Policy: p, EffectiveMax: p.MaxReplicas, Current: current, Desired: current}
	if c.Ceiling > 0 && c.Ceiling < out.EffectiveMax {
		out.EffectiveMax = c.Ceiling
	}
	out.Flaps1h = Flaps(events, c.now(), flapWindow)
	threshold := c.FlapThreshold
	if threshold < 1 {
		threshold = DefaultFlapThreshold
	}
	out.Flapping = out.Flaps1h >= threshold
	if !ok {
		out.Condition = Condition{Reason: ReasonPending, Message: "Waiting for the first evaluation."}
		return out, nil
	}
	out.Desired, out.Value, out.ValueKnown, out.BlockedBy = st.Desired, st.Value, st.ValueKnown, st.BlockedBy
	out.LastScaleUp, out.LastScaleDn = optional(st.LastScaleUp), optional(st.LastScaleDown)
	out.EvaluatedAt = optional(st.UpdatedAt)
	out.Condition = Condition{Healthy: !unhealthyReasons[st.Reason] && st.BlockedBy != BlockedNoCapacity, Reason: st.Reason, Message: st.Message}
	if out.Flapping && out.Condition.Healthy {
		out.Condition = Condition{Reason: ReasonFlapping, Message: fmt.Sprintf(
			"The count reversed direction %d times in the last hour. Widen the tolerances or lengthen the cooldowns.", out.Flaps1h)}
	}
	return out, nil
}

func optional(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

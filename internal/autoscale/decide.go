package autoscale

import (
	"fmt"
	"math"
	"time"
)

// Decision reasons, shown in the status condition and the history.
const (
	ReasonWithinTarget       = "WithinTarget"
	ReasonScaledUp           = "ScaledUp"
	ReasonScaledDown         = "ScaledDown"
	ReasonMetricsUnavailable = "MetricsUnavailable"
	ReasonCooldown           = "CooldownActive"
	ReasonStabilizing        = "Stabilizing"
	ReasonNoCapacity         = "NoCapacity"
	ReasonSpendCapPaused     = "SpendCapPaused"
	ReasonAtMaximum          = "AtMaximum"
	ReasonAtMinimum          = "AtMinimum"
	ReasonBoundsCorrected    = "BoundsCorrected"
	ReasonSuspended          = "Suspended"
)

// Blockers recorded when a wanted change did not happen, or only partly.
const (
	BlockedCooldown      = "cooldown"
	BlockedStabilization = "stabilization"
	BlockedSpendCap      = "spend_cap"
	BlockedNoCapacity    = "no_capacity"
	BlockedMaxReplicas   = "max_replicas"
	BlockedMinReplicas   = "min_replicas"
	BlockedGlobalCeiling = "global_ceiling"
	BlockedStepLimit     = "step_limit"
	BlockedNoMetrics     = "no_metrics"
	BlockedSuspended     = "suspended"
)

// Input is everything one decision depends on. Decide reads nothing else, so
// the same Input always produces the same Decision.
type Input struct {
	Now     time.Time
	Current int
	// Value is the metric in the units of the policy target; it is only
	// meaningful while ValueKnown is true.
	Value      float64
	ValueKnown bool
	// MetricNote says why the value is unknown.
	MetricNote string
	// Ceiling is the operator's global maximum; 0 means none.
	Ceiling       int
	LastScaleUp   time.Time
	LastScaleDown time.Time
	// DownSince and DownFloor carry the scale down stabilization window
	// between calls: when a lower count was first recommended and the highest
	// count recommended since.
	DownSince time.Time
	DownFloor int
	// SpendBlocked is true while a spend cap is being enforced.
	SpendBlocked bool
	// Headroom reports how many of extra more replicas the placement can
	// hold. A negative answer, or a nil func, means unknown and never blocks.
	Headroom func(extra int) int
}

// Decision is the outcome: the count to converge on and why.
type Decision struct {
	Current   int
	Desired   int
	Value     float64
	Reason    string
	BlockedBy string
	Message   string
	DownSince time.Time
	DownFloor int
}

// Changed reports whether the decision moves the replica count.
func (d Decision) Changed() bool { return d.Desired != d.Current }

// Decide computes the desired replica count. It is pure: no clock, no I/O.
//
// The count follows ceil(current * value / target), the same proportional rule
// Kubernetes uses, but only outside a dead band (scale up above target plus the
// up tolerance, scale down below target minus the down tolerance), so a
// reading near the target never moves anything. Scale up is fast (short
// cooldown); scale down waits for a stabilization window and a longer cooldown,
// and lands on the highest count recommended inside that window.
func Decide(p Policy, in Input) Decision {
	d := Decision{Current: in.Current, Desired: in.Current, Value: in.Value, DownSince: in.DownSince, DownFloor: in.DownFloor}
	hi, capBy := p.MaxReplicas, BlockedMaxReplicas
	if in.Ceiling > 0 && in.Ceiling < hi {
		hi, capBy = in.Ceiling, BlockedGlobalCeiling
	}
	lo := min(p.MinReplicas, hi)

	switch {
	case in.Current < lo:
		d.resetDown()
		return d.growTo(in, lo, ReasonBoundsCorrected, fmt.Sprintf("running %d, below the minimum of %d", in.Current, lo))
	case in.Current > hi:
		d.resetDown()
		d.Desired, d.Reason = hi, ReasonBoundsCorrected
		d.Message = fmt.Sprintf("running %d, above the maximum of %d", in.Current, hi)
		return d
	}
	if !in.ValueKnown {
		d.resetDown()
		return d.hold(ReasonMetricsUnavailable, BlockedNoMetrics, "holding at "+fmt.Sprint(in.Current)+": "+in.MetricNote)
	}

	raw := int(math.Ceil(float64(in.Current)*in.Value/p.Target - 1e-9))
	upper := p.Target * (1 + float64(p.ScaleUpTolerancePct)/100)
	lower := p.Target * (1 - float64(p.ScaleDownTolerancePct)/100)
	reading := fmt.Sprintf("%s %.1f against a target of %.1f", p.Metric, in.Value, p.Target)

	switch {
	case in.Value > upper && raw > in.Current:
		d.resetDown()
		return d.up(p, in, raw, hi, capBy, reading)
	case in.Value < lower && raw < in.Current:
		return d.down(p, in, max(raw, lo), lo, reading)
	}
	d.resetDown()
	return d.hold(ReasonWithinTarget, "", fmt.Sprintf("%s, inside the %.1f to %.1f band", reading, lower, upper))
}

func (d *Decision) resetDown() { d.DownSince, d.DownFloor = time.Time{}, 0 }

func (d Decision) hold(reason, blocked, msg string) Decision {
	d.Reason, d.BlockedBy, d.Message = reason, blocked, msg
	return d
}

func (d Decision) up(p Policy, in Input, raw, hi int, capBy, reading string) Decision {
	if in.Current >= hi {
		return d.hold(ReasonAtMaximum, capBy, fmt.Sprintf("%s: wants %d, limited to %d", reading, raw, hi))
	}
	want := min(raw, hi)
	step := min(want, in.Current+p.MaxStepUp)
	switch {
	case in.SpendBlocked:
		return d.hold(ReasonSpendCapPaused, BlockedSpendCap, fmt.Sprintf("%s: wants %d, but a spend cap is in force so no replicas are added", reading, raw))
	case !in.LastScaleUp.IsZero() && in.Now.Sub(in.LastScaleUp) < p.ScaleUpCooldown():
		wait := p.ScaleUpCooldown() - in.Now.Sub(in.LastScaleUp)
		return d.hold(ReasonCooldown, BlockedCooldown, fmt.Sprintf("%s: wants %d, waiting %s after the last scale up", reading, raw, wait.Round(time.Second)))
	}
	grown := d.growTo(in, step, ReasonScaledUp, fmt.Sprintf("%s: wants %d", reading, raw))
	switch {
	case grown.BlockedBy != "" || !grown.Changed():
	case grown.Desired < step:
		grown.BlockedBy = BlockedNoCapacity
	case step < want:
		grown.BlockedBy = BlockedStepLimit
	case want < raw:
		grown.BlockedBy = capBy
	}
	return grown
}

// growTo raises the count toward target, limited by placement headroom and
// the spend cap. It does not apply cooldowns or step limits: callers do.
func (d Decision) growTo(in Input, target int, reason, msg string) Decision {
	if in.SpendBlocked {
		return d.hold(ReasonSpendCapPaused, BlockedSpendCap, msg+", but a spend cap is in force so no replicas are added")
	}
	extra := target - in.Current
	if in.Headroom != nil {
		if h := in.Headroom(extra); h >= 0 && h < extra {
			extra = h
		}
	}
	if extra <= 0 {
		return d.hold(ReasonNoCapacity, BlockedNoCapacity, msg+", but no node has room for another replica")
	}
	d.Desired, d.Reason = in.Current+extra, reason
	d.Message = fmt.Sprintf("%s, adding %d (%d to %d)", msg, extra, in.Current, d.Desired)
	if in.Current+extra < target {
		d.Message += fmt.Sprintf(", capacity allows only %d", extra)
	}
	return d
}

func (d Decision) down(p Policy, in Input, want, lo int, reading string) Decision {
	if in.Current <= lo {
		d.resetDown()
		return d.hold(ReasonAtMinimum, BlockedMinReplicas, fmt.Sprintf("%s: could run fewer, but the minimum is %d", reading, lo))
	}
	if d.DownSince.IsZero() {
		d.DownSince, d.DownFloor = in.Now, want
	} else {
		d.DownFloor = max(d.DownFloor, want)
	}
	if held := in.Now.Sub(d.DownSince); held < p.Stabilization() {
		return d.hold(ReasonStabilizing, BlockedStabilization, fmt.Sprintf("%s: could run %d, confirming for another %s", reading, want, (p.Stabilization()-held).Round(time.Second)))
	}
	if d.DownFloor >= in.Current {
		d.resetDown()
		return d.hold(ReasonWithinTarget, "", fmt.Sprintf("%s: load in the last %s needs the current count", reading, p.Stabilization()))
	}
	last := in.LastScaleUp
	if in.LastScaleDown.After(last) {
		last = in.LastScaleDown
	}
	if !last.IsZero() && in.Now.Sub(last) < p.ScaleDownCooldown() {
		wait := p.ScaleDownCooldown() - in.Now.Sub(last)
		return d.hold(ReasonCooldown, BlockedCooldown, fmt.Sprintf("%s: could run %d, waiting %s after the last change", reading, d.DownFloor, wait.Round(time.Second)))
	}
	target := max(d.DownFloor, in.Current-p.MaxStepDown)
	floor := d.DownFloor
	d.Desired, d.Reason = target, ReasonScaledDown
	d.Message = fmt.Sprintf("%s: removing %d (%d to %d)", reading, in.Current-target, in.Current, target)
	if target > floor {
		d.BlockedBy = BlockedStepLimit
	}
	d.resetDown()
	return d
}

package autoscale

import (
	"testing"
	"time"
)

var t0 = time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)

func testPolicy() Policy {
	return Policy{
		App: "web", MinReplicas: 2, MaxReplicas: 10, Metric: MetricCPUPercent, Target: 60,
	}.Normalize()
}

func known(cur int, value float64) Input {
	return Input{Now: t0, Current: cur, Value: value, ValueKnown: true}
}

func TestDecide(t *testing.T) {
	long := t0.Add(-time.Hour)
	tests := []struct {
		name    string
		mod     func(p *Policy)
		in      Input
		desired int
		reason  string
		blocked string
	}{
		{"inside the band holds", nil, known(4, 60), 4, ReasonWithinTarget, ""},
		{"just above target but under the up tolerance holds", nil, known(4, 65), 4, ReasonWithinTarget, ""},
		{"just below target but over the down tolerance holds", nil, known(4, 45), 4, ReasonWithinTarget, ""},
		{"overload scales proportionally", nil, known(4, 90), 6, ReasonScaledUp, ""},
		{"step limit caps a big jump", nil, known(2, 300), 6, ReasonScaledUp, BlockedStepLimit},
		{"max replicas caps the target", nil, known(8, 120), 10, ReasonScaledUp, BlockedMaxReplicas},
		{"global ceiling beats a higher max", nil, Input{Now: t0, Current: 8, Value: 120, ValueKnown: true, Ceiling: 9}, 9, ReasonScaledUp, BlockedGlobalCeiling},
		{"already at max reports it", nil, known(10, 200), 10, ReasonAtMaximum, BlockedMaxReplicas},
		{"already at the global ceiling reports it", nil, Input{Now: t0, Current: 9, Value: 200, ValueKnown: true, Ceiling: 9}, 9, ReasonAtMaximum, BlockedGlobalCeiling},
		{"scale up cooldown holds", nil, Input{Now: t0, Current: 4, Value: 90, ValueKnown: true, LastScaleUp: t0.Add(-30 * time.Second)}, 4, ReasonCooldown, BlockedCooldown},
		{"scale up after cooldown proceeds", nil, Input{Now: t0, Current: 4, Value: 90, ValueKnown: true, LastScaleUp: t0.Add(-2 * time.Minute)}, 6, ReasonScaledUp, ""},
		{"a recent scale down does not delay a scale up", nil, Input{Now: t0, Current: 4, Value: 90, ValueKnown: true, LastScaleDown: t0.Add(-5 * time.Second)}, 6, ReasonScaledUp, ""},
		{"spend cap beats scale up", nil, Input{Now: t0, Current: 4, Value: 90, ValueKnown: true, SpendBlocked: true}, 4, ReasonSpendCapPaused, BlockedSpendCap},
		{"no capacity holds", nil, Input{Now: t0, Current: 4, Value: 90, ValueKnown: true, Headroom: func(int) int { return 0 }}, 4, ReasonNoCapacity, BlockedNoCapacity},
		{"partial capacity scales part way", nil, Input{Now: t0, Current: 4, Value: 90, ValueKnown: true, Headroom: func(int) int { return 1 }}, 5, ReasonScaledUp, BlockedNoCapacity},
		{"unknown capacity never blocks", nil, Input{Now: t0, Current: 4, Value: 90, ValueKnown: true, Headroom: func(int) int { return -1 }}, 6, ReasonScaledUp, ""},
		{"unknown metric holds", nil, Input{Now: t0, Current: 4, MetricNote: "no samples"}, 4, ReasonMetricsUnavailable, BlockedNoMetrics},
		{"unknown metric still corrects the floor", nil, Input{Now: t0, Current: 1, MetricNote: "no samples"}, 2, ReasonBoundsCorrected, ""},
		{"unknown metric still corrects the ceiling", nil, Input{Now: t0, Current: 12, MetricNote: "no samples"}, 10, ReasonBoundsCorrected, ""},
		{"below min is raised without a cooldown", nil, Input{Now: t0, Current: 1, Value: 10, ValueKnown: true, LastScaleUp: t0.Add(-time.Second)}, 2, ReasonBoundsCorrected, ""},
		{"below min is still blocked by a spend cap", nil, Input{Now: t0, Current: 1, SpendBlocked: true}, 1, ReasonSpendCapPaused, BlockedSpendCap},
		{"light load waits out stabilization", nil, Input{Now: t0, Current: 6, Value: 12, ValueKnown: true}, 6, ReasonStabilizing, BlockedStabilization},
		{"light load after stabilization steps down by one", nil, Input{Now: t0, Current: 6, Value: 12, ValueKnown: true, DownSince: long, DownFloor: 2}, 5, ReasonScaledDown, BlockedStepLimit},
		{"scale down lands on the window's highest recommendation", nil, Input{Now: t0, Current: 6, Value: 12, ValueKnown: true, DownSince: long, DownFloor: 5}, 5, ReasonScaledDown, ""},
		{"scale down cooldown holds after a recent change", nil, Input{Now: t0, Current: 6, Value: 12, ValueKnown: true, DownSince: long, DownFloor: 2, LastScaleUp: t0.Add(-time.Minute)}, 6, ReasonCooldown, BlockedCooldown},
		{"scale down respects min", nil, Input{Now: t0, Current: 2, Value: 5, ValueKnown: true, DownSince: long, DownFloor: 2}, 2, ReasonAtMinimum, BlockedMinReplicas},
		{"step down can be raised by the policy", func(p *Policy) { p.MaxStepDown = 3 }, Input{Now: t0, Current: 8, Value: 12, ValueKnown: true, DownSince: long, DownFloor: 3}, 5, ReasonScaledDown, BlockedStepLimit},
		{"window high enough to need current count cancels scale down", nil, Input{Now: t0, Current: 6, Value: 12, ValueKnown: true, DownSince: long, DownFloor: 6}, 6, ReasonWithinTarget, ""},
		{"requests metric uses the same math", func(p *Policy) { p.Metric = MetricRequestsPerReplica; p.Target = 100 }, known(3, 200), 6, ReasonScaledUp, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := testPolicy()
			if tt.mod != nil {
				tt.mod(&p)
			}
			d := Decide(p, tt.in)
			if d.Desired != tt.desired || d.Reason != tt.reason || d.BlockedBy != tt.blocked {
				t.Fatalf("Decide = desired %d reason %s blocked %q (%s), want %d %s %q", d.Desired, d.Reason, d.BlockedBy, d.Message, tt.desired, tt.reason, tt.blocked)
			}
			if d.Message == "" {
				t.Fatal("decision has no message")
			}
		})
	}
}

func TestDecideIsDeterministic(t *testing.T) {
	in := Input{Now: t0, Current: 6, Value: 12, ValueKnown: true, DownSince: t0.Add(-time.Hour), DownFloor: 4}
	first := Decide(testPolicy(), in)
	for i := 0; i < 5; i++ {
		if got := Decide(testPolicy(), in); got != first {
			t.Fatalf("run %d = %+v, want %+v", i, got, first)
		}
	}
}

func TestStabilizationWindowTracksTheHighestRecommendation(t *testing.T) {
	p := testPolicy()
	cur, since, floor := 8, time.Time{}, 0
	step := func(at time.Time, value float64) Decision {
		d := Decide(p, Input{Now: at, Current: cur, Value: value, ValueKnown: true, DownSince: since, DownFloor: floor})
		since, floor = d.DownSince, d.DownFloor
		return d
	}
	if d := step(t0, 12); d.Reason != ReasonStabilizing || d.DownFloor != 2 {
		t.Fatalf("first low reading = %+v", d)
	}
	if d := step(t0.Add(time.Minute), 36); d.DownFloor != 5 {
		t.Fatalf("a busier reading should raise the floor to 5, got %+v", d)
	}
	if d := step(t0.Add(2*time.Minute), 12); d.DownFloor != 5 {
		t.Fatalf("a quiet reading must not lower the floor, got %+v", d)
	}
	d := step(t0.Add(6*time.Minute), 12)
	if d.Reason != ReasonScaledDown || d.Desired != 7 {
		t.Fatalf("after the window one step down expected, got %+v", d)
	}
	if !d.DownSince.IsZero() {
		t.Fatal("scaling down restarts the stabilization window")
	}
	if d := step(t0.Add(7*time.Minute), 60); d.Reason != ReasonWithinTarget || !d.DownSince.IsZero() {
		t.Fatalf("a reading back in the band cancels the window, got %+v", d)
	}
}

func TestNoFlappingOnNoisyLoad(t *testing.T) {
	p := testPolicy()
	cur := 4
	var lastUp, lastDown, since time.Time
	floor := 0
	changes := 0
	load := []float64{48, 64, 55, 65, 50, 62, 58, 60, 44, 61}
	for i := 0; i < 120; i++ {
		now := t0.Add(time.Duration(i) * 30 * time.Second)
		value := load[i%len(load)] * float64(4) / float64(cur)
		d := Decide(p, Input{Now: now, Current: cur, Value: value, ValueKnown: true,
			LastScaleUp: lastUp, LastScaleDown: lastDown, DownSince: since, DownFloor: floor})
		since, floor = d.DownSince, d.DownFloor
		if d.Changed() {
			changes++
			if d.Desired > cur {
				lastUp = now
			} else {
				lastDown = now
			}
			cur = d.Desired
		}
	}
	if changes != 0 {
		t.Fatalf("noisy load around the target moved the count %d times", changes)
	}
}

func TestScaleDownDoesNotImmediatelyTriggerScaleUp(t *testing.T) {
	p := testPolicy()
	in := Input{Now: t0, Current: 8, Value: 20, ValueKnown: true, DownSince: t0.Add(-time.Hour), DownFloor: 4}
	p.MaxStepDown = 8
	d := Decide(p, in)
	if d.Desired >= 8 {
		t.Fatalf("expected a scale down, got %+v", d)
	}
	projected := in.Value * float64(in.Current) / float64(d.Desired)
	if upper := p.Target * (1 + float64(p.ScaleUpTolerancePct)/100); projected > upper {
		t.Fatalf("after scaling to %d the metric would be %.1f, above the scale up threshold %.1f", d.Desired, projected, upper)
	}
}

func TestStepResponseAndRecovery(t *testing.T) {
	p := testPolicy()
	cur := 2
	var lastUp, lastDown, since time.Time
	floor := 0
	// total demand in "replica-equivalents at target": 2 -> 8 -> 1.
	demand := func(i int) float64 {
		switch {
		case i < 10:
			return 1.2
		case i < 40:
			return 8
		default:
			return 1
		}
	}
	peak := 0
	for i := 0; i < 400; i++ {
		now := t0.Add(time.Duration(i) * 30 * time.Second)
		value := p.Target * demand(i) / float64(cur)
		d := Decide(p, Input{Now: now, Current: cur, Value: value, ValueKnown: true,
			LastScaleUp: lastUp, LastScaleDown: lastDown, DownSince: since, DownFloor: floor})
		since, floor = d.DownSince, d.DownFloor
		if d.Changed() {
			if d.Desired > cur {
				lastUp = now
			} else {
				lastDown = now
			}
			cur = d.Desired
		}
		peak = max(peak, cur)
	}
	if peak < 8 {
		t.Fatalf("the step load should have raised replicas to at least 8, peak was %d", peak)
	}
	if cur != p.MinReplicas {
		t.Fatalf("after the load is gone replicas should settle at the minimum %d, got %d", p.MinReplicas, cur)
	}
}

func TestPolicyValidate(t *testing.T) {
	good := testPolicy()
	tests := []struct {
		name    string
		mod     func(p *Policy)
		ceiling int
		ok      bool
	}{
		{"valid", func(*Policy) {}, 20, true},
		{"min zero", func(p *Policy) { p.MinReplicas = 0 }, 20, false},
		{"max below min", func(p *Policy) { p.MaxReplicas = 1 }, 20, false},
		{"max required", func(p *Policy) { p.MaxReplicas = 0 }, 20, false},
		{"max above ceiling", func(p *Policy) { p.MaxReplicas = 30 }, 20, false},
		{"unknown metric", func(p *Policy) { p.Metric = "latency" }, 20, false},
		{"cpu target over 100", func(p *Policy) { p.Target = 150 }, 20, false},
		{"requests target may exceed 100", func(p *Policy) { p.Metric = MetricRequestsPerReplica; p.Target = 500 }, 20, true},
		{"zero target", func(p *Policy) { p.Target = 0 }, 20, false},
		{"tolerance too large", func(p *Policy) { p.ScaleDownTolerancePct = 100 }, 20, false},
		{"cooldown too long", func(p *Policy) { p.ScaleUpCooldownSecs = 90000 }, 20, false},
		{"step too large", func(p *Policy) { p.MaxStepUp = 500 }, 20, false},
		{"no app", func(p *Policy) { p.App = "" }, 20, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := good
			tt.mod(&p)
			if err := p.Validate(tt.ceiling); (err == nil) != tt.ok {
				t.Fatalf("Validate = %v, want ok=%v", err, tt.ok)
			}
		})
	}
}

func TestNormalizeKeepsHysteresisOn(t *testing.T) {
	p := Policy{App: "a", MinReplicas: 1, MaxReplicas: 3, Metric: MetricCPUPercent, Target: 50}.Normalize()
	if p.ScaleUpTolerancePct == 0 || p.ScaleDownTolerancePct == 0 || p.ScaleDownCooldownSecs <= p.ScaleUpCooldownSecs || p.StabilizationSecs == 0 {
		t.Fatalf("defaults must be non-zero and slow to scale down: %+v", p)
	}
	if err := p.Validate(20); err != nil {
		t.Fatal(err)
	}
}

func TestEnvSettings(t *testing.T) {
	env := map[string]string{EnvMaxReplicasCeiling: "8", EnvInterval: "5s", EnvWindow: "bad", EnvFlapThreshold: "0"}
	look := func(k string) string { return env[k] }
	if got := CeilingFromEnv(look); got != 8 {
		t.Fatalf("ceiling = %d", got)
	}
	if got := IntervalFromEnv(look); got != 5*time.Second {
		t.Fatalf("interval = %s", got)
	}
	if got := WindowFromEnv(look); got != DefaultWindow {
		t.Fatalf("an invalid window should fall back, got %s", got)
	}
	if got := FlapThresholdFromEnv(look); got != DefaultFlapThreshold {
		t.Fatalf("flap threshold = %d", got)
	}
	if got := MetricMaxAgeFromEnv(look); got != DefaultMetricMaxAge {
		t.Fatalf("max age = %s", got)
	}
}

package alerting

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/telemetry"
)

type fakeMetricsSource struct {
	samples []telemetry.Sample
	err     error
}

func (f *fakeMetricsSource) QueryMetrics(_ context.Context, _, _ string, _, _ time.Time) ([]telemetry.Sample, error) {
	return f.samples, f.err
}

func baseRule() Rule {
	return Rule{
		ID: "r1", Name: "high cpu", Kind: KindThreshold, ResourceID: "service:web",
		Metric: "cpu_percent", Comparator: GreaterThan, Threshold: 80,
		ForDuration: 2 * time.Minute, Enabled: true,
	}
}

func TestEvaluateThreshold_NoSamples_NotFiring(t *testing.T) {
	r := baseRule()
	source := &fakeMetricsSource{}
	now := time.Now()

	got, err := EvaluateThreshold(context.Background(), source, r, now)
	if err != nil {
		t.Fatalf("EvaluateThreshold() error = %v", err)
	}
	if got.Firing || got.PendingSince != nil {
		t.Errorf("got = %+v, want not firing and not pending with no data", got)
	}
}

func TestEvaluateThreshold_ConditionNotMet_NotFiring(t *testing.T) {
	r := baseRule()
	now := time.Now()
	source := &fakeMetricsSource{samples: []telemetry.Sample{{Timestamp: now, Value: 10}}} // well under threshold of 80

	got, err := EvaluateThreshold(context.Background(), source, r, now)
	if err != nil {
		t.Fatalf("EvaluateThreshold() error = %v", err)
	}
	if got.Firing || got.PendingSince != nil {
		t.Errorf("got = %+v, want not firing", got)
	}
	if got.LastValue == nil || *got.LastValue != 10 {
		t.Errorf("LastValue = %v, want 10", got.LastValue)
	}
}

func TestEvaluateThreshold_FirstTickOverThreshold_BecomesPendingNotFiring(t *testing.T) {
	r := baseRule() // ForDuration = 2m
	now := time.Now()
	source := &fakeMetricsSource{samples: []telemetry.Sample{{Timestamp: now, Value: 95}}}

	got, err := EvaluateThreshold(context.Background(), source, r, now)
	if err != nil {
		t.Fatalf("EvaluateThreshold() error = %v", err)
	}
	if got.Firing {
		t.Error("Firing = true on the very first over-threshold tick, want pending only (ForDuration not yet elapsed)")
	}
	if got.PendingSince == nil || !got.PendingSince.Equal(now) {
		t.Errorf("PendingSince = %v, want %v", got.PendingSince, now)
	}
}

func TestEvaluateThreshold_ZeroForDuration_FiresImmediately(t *testing.T) {
	r := baseRule()
	r.ForDuration = 0
	now := time.Now()
	source := &fakeMetricsSource{samples: []telemetry.Sample{{Timestamp: now, Value: 95}}}

	got, err := EvaluateThreshold(context.Background(), source, r, now)
	if err != nil {
		t.Fatalf("EvaluateThreshold() error = %v", err)
	}
	if !got.Firing {
		t.Error("Firing = false with ForDuration=0, want immediate firing on first over-threshold tick")
	}
	if got.FiringSince == nil || !got.FiringSince.Equal(now) {
		t.Errorf("FiringSince = %v, want %v", got.FiringSince, now)
	}
}

func TestEvaluateThreshold_PendingLongEnough_GraduatesToFiring(t *testing.T) {
	r := baseRule() // ForDuration = 2m
	pendingStart := time.Now().Add(-3 * time.Minute)
	r.PendingSince = &pendingStart
	now := time.Now()
	source := &fakeMetricsSource{samples: []telemetry.Sample{{Timestamp: now, Value: 95}}}

	got, err := EvaluateThreshold(context.Background(), source, r, now)
	if err != nil {
		t.Fatalf("EvaluateThreshold() error = %v", err)
	}
	if !got.Firing {
		t.Error("Firing = false, want true: pending for 3m against a 2m ForDuration should graduate")
	}
	if got.FiringSince == nil || !got.FiringSince.Equal(pendingStart) {
		t.Errorf("FiringSince = %v, want %v (the original pending start, not now)", got.FiringSince, pendingStart)
	}
}

func TestEvaluateThreshold_PendingNotLongEnough_StaysPending(t *testing.T) {
	r := baseRule() // ForDuration = 2m
	pendingStart := time.Now().Add(-30 * time.Second)
	r.PendingSince = &pendingStart
	now := time.Now()
	source := &fakeMetricsSource{samples: []telemetry.Sample{{Timestamp: now, Value: 95}}}

	got, err := EvaluateThreshold(context.Background(), source, r, now)
	if err != nil {
		t.Fatalf("EvaluateThreshold() error = %v", err)
	}
	if got.Firing {
		t.Error("Firing = true, want false: only pending 30s against a 2m ForDuration")
	}
	if got.PendingSince == nil || !got.PendingSince.Equal(pendingStart) {
		t.Errorf("PendingSince = %v, want unchanged at %v", got.PendingSince, pendingStart)
	}
}

func TestEvaluateThreshold_AlreadyFiring_StaysFiring(t *testing.T) {
	r := baseRule()
	firingStart := time.Now().Add(-10 * time.Minute)
	r.Firing = true
	r.FiringSince = &firingStart
	now := time.Now()
	source := &fakeMetricsSource{samples: []telemetry.Sample{{Timestamp: now, Value: 95}}}

	got, err := EvaluateThreshold(context.Background(), source, r, now)
	if err != nil {
		t.Fatalf("EvaluateThreshold() error = %v", err)
	}
	if !got.Firing || got.FiringSince == nil || !got.FiringSince.Equal(firingStart) {
		t.Errorf("got = %+v, want still firing since the original %v", got, firingStart)
	}
}

func TestEvaluateThreshold_FiringThenRecovers_ClearsImmediately(t *testing.T) {
	r := baseRule()
	firingStart := time.Now().Add(-10 * time.Minute)
	r.Firing = true
	r.FiringSince = &firingStart
	now := time.Now()
	source := &fakeMetricsSource{samples: []telemetry.Sample{{Timestamp: now, Value: 10}}} // back under threshold

	got, err := EvaluateThreshold(context.Background(), source, r, now)
	if err != nil {
		t.Fatalf("EvaluateThreshold() error = %v", err)
	}
	if got.Firing || got.FiringSince != nil || got.PendingSince != nil {
		t.Errorf("got = %+v, want fully cleared on a single recovering sample (no resolve-debounce)", got)
	}
}

func TestEvaluateThreshold_QueryError_Propagates(t *testing.T) {
	r := baseRule()
	source := &fakeMetricsSource{err: errors.New("store unreachable")}

	_, err := EvaluateThreshold(context.Background(), source, r, time.Now())
	if err == nil {
		t.Fatal("EvaluateThreshold() error = nil, want the query error surfaced")
	}
}

func TestSatisfies(t *testing.T) {
	tests := []struct {
		value, threshold float64
		c                Comparator
		want             bool
	}{
		{10, 5, GreaterThan, true},
		{5, 10, GreaterThan, false},
		{5, 5, GreaterThan, false},
		{5, 10, LessThan, true},
		{5, 5, GreaterOrEqual, true},
		{5, 5, LessOrEqual, true},
		{5, 5, "unknown", false},
	}
	for _, tt := range tests {
		if got := satisfies(tt.value, tt.c, tt.threshold); got != tt.want {
			t.Errorf("satisfies(%v, %q, %v) = %v, want %v", tt.value, tt.c, tt.threshold, got, tt.want)
		}
	}
}

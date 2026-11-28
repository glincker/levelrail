package alerting

import (
	"context"
	"testing"
	"time"

	"github.com/glincker/levelrail/internal/telemetry"
)

type fakeAcks struct{ read bool }

func (f fakeAcks) LatestFiredAcknowledged(context.Context, string) (bool, error) { return f.read, nil }

func TestEngine_Renotify_RepeatsUntilRead(t *testing.T) {
	since := time.Now().Add(-time.Hour)
	r := Rule{ID: "r1", Kind: KindThreshold, ResourceID: "service:web", Metric: "cpu_percent",
		Comparator: GreaterThan, Threshold: 80, Enabled: true, Firing: true, FiringSince: &since}
	rules := newFakeRuleStore(r)
	metrics := &fakeMetricsSource{samples: []telemetry.Sample{{Timestamp: time.Now(), Value: 95}}}
	spy := &spyNotifier{}
	engine := newTestEngine(rules, metrics, nil, nil, spy)
	clock := time.Now()
	engine.clock = func() time.Time { return clock }
	acks := fakeAcks{}
	engine.SetRenotify(func(context.Context) time.Duration { return 2 * time.Hour }, &acks)

	tick := func(advance time.Duration) int {
		clock = clock.Add(advance)
		if err := engine.Tick(context.Background()); err != nil {
			t.Fatalf("Tick() error = %v", err)
		}
		return len(spy.calls())
	}
	if n := tick(0); n != 0 {
		t.Fatalf("first sighting notified %d times, want 0", n)
	}
	if n := tick(time.Hour); n != 0 {
		t.Fatalf("inside the interval notified %d times, want 0", n)
	}
	if n := tick(2 * time.Hour); n != 1 {
		t.Fatalf("after the interval notified %d times, want 1", n)
	}
	acks.read = true
	if n := tick(3 * time.Hour); n != 1 {
		t.Fatalf("a read alert notified again: %d calls, want 1", n)
	}
}

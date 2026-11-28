package alerting

import (
	"context"
	"testing"
	"time"

	"github.com/glincker/levelrail/internal/telemetry"
)

type escalationStore struct {
	*fakeRuleStore
	ch NotificationChannel
}

func (s escalationStore) GetNotificationChannel(context.Context, string) (*NotificationChannel, error) {
	return &s.ch, nil
}

func TestEngine_Escalation_SecondChannelOnceUnlessRead(t *testing.T) {
	since := time.Now().Add(-time.Minute)
	r := Rule{ID: "r1", Kind: KindThreshold, ResourceID: "service:web", Metric: "cpu_percent",
		Comparator: GreaterThan, Threshold: 80, Enabled: true, Firing: true, FiringSince: &since,
		ChannelID: "chn_a", EscalateChannelID: "chn_b", EscalateAfter: 10 * time.Minute}
	store := escalationStore{newFakeRuleStore(r), NotificationChannel{ID: "chn_b", Kind: NotifySlack, NotifyURL: "https://hooks.example.test/b", Enabled: true}}
	metrics := &fakeMetricsSource{samples: []telemetry.Sample{{Timestamp: time.Now(), Value: 95}}}
	spy := &spyNotifier{}
	engine := NewEngine(store, metrics, nil, nil, nil, nil, 0, 0, nil, 0, 0, nil, 0, 0, nil, nil, 0, nil, 0, func(Rule) Notifier { return spy }, nil)
	clock := time.Now()
	engine.clock = func() time.Time { return clock }
	acks := fakeAcks{}
	engine.acks = &acks

	tick := func(advance time.Duration) int {
		clock = clock.Add(advance)
		if err := engine.Tick(context.Background()); err != nil {
			t.Fatalf("Tick() error = %v", err)
		}
		return len(spy.calls())
	}
	if n := tick(0); n != 0 {
		t.Fatalf("before the delay sent %d, want 0", n)
	}
	if n := tick(10 * time.Minute); n != 1 {
		t.Fatalf("after the delay sent %d, want 1", n)
	}
	if got := spy.calls()[0].Rule; got.ChannelID != "chn_b" || got.NotifyKind != NotifySlack {
		t.Fatalf("escalation went to channel %q kind %q, want chn_b slack", got.ChannelID, got.NotifyKind)
	}
	if n := tick(time.Hour); n != 1 {
		t.Fatalf("escalated again: %d calls, want 1", n)
	}
}

func TestEngine_Escalation_SkippedWhenRead(t *testing.T) {
	since := time.Now().Add(-time.Hour)
	r := Rule{ID: "r2", Kind: KindThreshold, ResourceID: "service:web", Metric: "cpu_percent",
		Comparator: GreaterThan, Threshold: 80, Enabled: true, Firing: true, FiringSince: &since,
		EscalateChannelID: "chn_b", EscalateAfter: time.Minute}
	store := escalationStore{newFakeRuleStore(r), NotificationChannel{ID: "chn_b", Kind: NotifySlack, Enabled: true}}
	metrics := &fakeMetricsSource{samples: []telemetry.Sample{{Timestamp: time.Now(), Value: 95}}}
	spy := &spyNotifier{}
	engine := NewEngine(store, metrics, nil, nil, nil, nil, 0, 0, nil, 0, 0, nil, 0, 0, nil, nil, 0, nil, 0, func(Rule) Notifier { return spy }, nil)
	engine.acks = fakeAcks{read: true}
	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error = %v", err)
	}
	if n := len(spy.calls()); n != 0 {
		t.Fatalf("a read alert escalated: %d calls, want 0", n)
	}
}

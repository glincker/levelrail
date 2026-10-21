package alerting

import (
	"context"
	"sync"
	"testing"
	"time"
)

type tickClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *tickClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *tickClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func certLoopFixture(t *testing.T, certs *fakeCertSource) (*Engine, *DB, *tickClock) {
	t.Helper()
	var rules []Rule
	for _, d := range []string{"14", "7", "1"} {
		days := map[string]time.Duration{"14": 14 * oneDay, "7": 7 * oneDay, "1": oneDay}[d]
		rules = append(rules, Rule{
			ID: "cert-" + d, Name: "Certificate expires within " + d + " days", Kind: KindCertExpiry,
			ForDuration: days, Severity: SeverityCritical, Enabled: true,
		})
	}
	clock := &tickClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	spy := &spyNotifier{}
	e := NewEngine(newFakeRuleStore(rules...), nil, nil, nil, certs, nil, 0, 0, nil, 0, 0, nil, 0, 0, nil, nil, 0, nil, 0, func(Rule) Notifier { return spy }, nil)
	e.clock = clock.now
	e.SetCertExpiryPolicy(testPolicy(nil, 14*oneDay, 7*oneDay, oneDay))
	db := newTestDB(t)
	e.SetNoiseControl(NewNoiseControl(NoiseConfig{}, db, nil, nil))
	return e, db, clock
}

func countEvents(t *testing.T, db *DB, event string) int {
	t.Helper()
	n := 0
	for _, h := range history(t, db) {
		if h.Event == event {
			n++
		}
	}
	return n
}

func tickN(t *testing.T, e *Engine, c *tickClock) {
	t.Helper()
	for i := 0; i < 20; i++ {
		if err := e.Tick(context.Background()); err != nil {
			t.Fatal(err)
		}
		c.advance(45 * time.Second)
	}
}

func TestCertAlerts_InternalCertProducesNoHistory(t *testing.T) {
	certs := newFakeCertSource()
	e, db, clock := certLoopFixture(t, certs)
	certs.seed(t, "localhost", clock.now().Add(-oneDay), clock.now().Add(12*time.Hour))

	tickN(t, e, clock)

	if h := history(t, db); len(h) != 0 {
		t.Fatalf("history rows = %d, want 0: %+v", len(h), h)
	}
}

func TestCertAlerts_ExpiringCertFiresOnceThenResolvesOnce(t *testing.T) {
	certs := newFakeCertSource()
	e, db, clock := certLoopFixture(t, certs)
	seedACME(t, certs, clock.now().Add(-60*oneDay), clock.now().Add(2*oneDay))

	tickN(t, e, clock)
	if f, r := countEvents(t, db, EventFired), countEvents(t, db, EventResolved); f != 1 || r != 0 {
		t.Fatalf("before renewal fired=%d resolved=%d, want 1 and 0", f, r)
	}

	seedACME(t, certs, clock.now().Add(-oneDay), clock.now().Add(60*oneDay))
	tickN(t, e, clock)
	if f, r := countEvents(t, db, EventFired), countEvents(t, db, EventResolved); f != 1 || r != 1 {
		t.Fatalf("after renewal fired=%d resolved=%d, want 1 and 1", f, r)
	}
}

func TestCertAlerts_StaleFiringStateNeverDeliveredResolvesNothing(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	if err := db.SeedDefaults(ctx, DefaultsConfig{}); err != nil {
		t.Fatal(err)
	}
	certs := newFakeCertSource()
	clock := &tickClock{t: time.Now()}
	e := NewEngine(db, nil, nil, nil, certs, nil, 0, 0, nil, 0, 0, nil, 0, 0, nil, nil, 0, nil, 0, func(Rule) Notifier { return &spyNotifier{} }, nil)
	e.clock = clock.now
	e.SetCertExpiryPolicy(CertPolicyFromEnv(func(string) string { return "" }))
	e.SetNoiseControl(NewNoiseControl(NoiseConfig{ConsecutiveFailures: 1}, db, nil, nil))
	certs.seed(t, "localhost", clock.now().Add(-oneDay), clock.now().Add(20*time.Hour))
	since := clock.now()
	if err := db.UpdateState(ctx, "default-cert-expiry-1d", nil, &since, true, since, nil); err != nil {
		t.Fatal(err)
	}

	tickN(t, e, clock)

	if h := history(t, db); len(h) != 0 {
		t.Fatalf("history rows = %d, want 0: %+v", len(h), h)
	}
}

func TestRecordHistory_ResolvedIsRecordedOnce(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rows := []struct {
		event string
		want  int
	}{{EventFired, 1}, {EventResolved, 2}, {EventResolved, 2}, {EventResolved, 2}, {EventFired, 3}, {EventResolved, 4}}
	for i, r := range rows {
		err := db.RecordHistory(ctx, HistoryEntry{At: base.Add(time.Duration(i) * time.Minute), RuleID: "r1", RuleKind: string(KindCertExpiry), Event: r.event, Outcome: OutcomeSent})
		if err != nil {
			t.Fatal(err)
		}
		if got := len(history(t, db)); got != r.want {
			t.Fatalf("after row %d (%s) history rows = %d, want %d", i, r.event, got, r.want)
		}
	}
}

package alerting

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// fakeRuleStore is an in-memory RuleStore, the same hand-written-fake
// pattern every other package in this codebase uses instead of a
// mocking framework.
type fakeRuleStore struct {
	mu    sync.Mutex
	rules map[string]Rule
}

func newFakeRuleStore(rules ...Rule) *fakeRuleStore {
	m := make(map[string]Rule, len(rules))
	for _, r := range rules {
		m[r.ID] = r
	}
	return &fakeRuleStore{rules: m}
}

func (f *fakeRuleStore) ListEnabledRules(_ context.Context) ([]Rule, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []Rule
	for _, r := range f.rules {
		if r.Enabled {
			out = append(out, r)
		}
	}
	return out, nil
}

func (f *fakeRuleStore) UpdateState(_ context.Context, id string, pendingSince, firingSince *time.Time, firing bool, evaluatedAt time.Time, value *float64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	r := f.rules[id]
	r.PendingSince, r.FiringSince, r.Firing, r.LastEvaluatedAt, r.LastValue = pendingSince, firingSince, firing, &evaluatedAt, value
	f.rules[id] = r
	return nil
}

func (f *fakeRuleStore) get(id string) Rule {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.rules[id]
}

// fakeLogsSource supports crashloop log-line attachment tests.
type fakeLogsSource struct {
	entries []telemetry.LogEntry
	err     error
}

func (f *fakeLogsSource) QueryLogs(_ context.Context, _ string, _, _ time.Time, _ string) ([]telemetry.LogEntry, error) {
	return f.entries, f.err
}

// spyNotifier records every Notify call, for assertions on exactly
// which events an Engine.Tick dispatched.
type spyNotifier struct {
	mu     sync.Mutex
	events []Event
	err    error
}

func (s *spyNotifier) Notify(_ context.Context, ev Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, ev)
	return s.err
}

func (s *spyNotifier) calls() []Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Event(nil), s.events...)
}

func newTestEngine(rules *fakeRuleStore, metrics MetricsSource, logs LogsSource, tracker *RestartTracker, spy *spyNotifier) *Engine {
	return NewEngine(rules, metrics, logs, tracker, func(Rule) Notifier { return spy }, nil)
}

func TestEngine_Tick_ThresholdFires_NotifiesOnce(t *testing.T) {
	r := Rule{ID: "r1", Name: "high cpu", Kind: KindThreshold, ResourceID: "service:web",
		Metric: "cpu_percent", Comparator: GreaterThan, Threshold: 80, Enabled: true}
	rules := newFakeRuleStore(r)
	metrics := &fakeMetricsSource{samples: []telemetry.Sample{{Timestamp: time.Now(), Value: 95}}}
	spy := &spyNotifier{}
	engine := newTestEngine(rules, metrics, nil, nil, spy)

	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error = %v", err)
	}

	calls := spy.calls()
	if len(calls) != 1 {
		t.Fatalf("Notify called %d times, want 1", len(calls))
	}
	if calls[0].Resolved {
		t.Error("Resolved = true on the first firing tick, want false")
	}
	if !rules.get("r1").Firing {
		t.Error("persisted state Firing = false, want true")
	}
}

func TestEngine_Tick_StaysFiring_DoesNotRenotify(t *testing.T) {
	firingSince := time.Now().Add(-time.Hour)
	r := Rule{ID: "r1", Kind: KindThreshold, ResourceID: "service:web", Metric: "cpu_percent",
		Comparator: GreaterThan, Threshold: 80, Enabled: true, Firing: true, FiringSince: &firingSince}
	rules := newFakeRuleStore(r)
	metrics := &fakeMetricsSource{samples: []telemetry.Sample{{Timestamp: time.Now(), Value: 95}}}
	spy := &spyNotifier{}
	engine := newTestEngine(rules, metrics, nil, nil, spy)

	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error = %v", err)
	}

	if calls := spy.calls(); len(calls) != 0 {
		t.Errorf("Notify called %d times, want 0: a rule that was already firing and stays firing must not renotify", len(calls))
	}
}

func TestEngine_Tick_Recovers_SendsResolvedNotification(t *testing.T) {
	firingSince := time.Now().Add(-time.Hour)
	r := Rule{ID: "r1", Kind: KindThreshold, ResourceID: "service:web", Metric: "cpu_percent",
		Comparator: GreaterThan, Threshold: 80, Enabled: true, Firing: true, FiringSince: &firingSince}
	rules := newFakeRuleStore(r)
	metrics := &fakeMetricsSource{samples: []telemetry.Sample{{Timestamp: time.Now(), Value: 10}}} // back under threshold
	spy := &spyNotifier{}
	engine := newTestEngine(rules, metrics, nil, nil, spy)

	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error = %v", err)
	}

	calls := spy.calls()
	if len(calls) != 1 || !calls[0].Resolved {
		t.Fatalf("calls = %+v, want exactly one Resolved=true notification", calls)
	}
	if rules.get("r1").Firing {
		t.Error("persisted state Firing = true, want false after recovering")
	}
}

func TestEngine_Tick_DisabledRule_Skipped(t *testing.T) {
	r := Rule{ID: "r1", Kind: KindThreshold, ResourceID: "service:web", Metric: "cpu_percent",
		Comparator: GreaterThan, Threshold: 80, Enabled: false}
	rules := newFakeRuleStore(r)
	metrics := &fakeMetricsSource{samples: []telemetry.Sample{{Timestamp: time.Now(), Value: 95}}}
	spy := &spyNotifier{}
	engine := newTestEngine(rules, metrics, nil, nil, spy)

	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error = %v", err)
	}
	if calls := spy.calls(); len(calls) != 0 {
		t.Errorf("Notify called %d times for a disabled rule, want 0", len(calls))
	}
}

func TestEngine_Tick_CrashloopFires_AttachesLogLines(t *testing.T) {
	r := Rule{ID: "cl1", Kind: KindCrashloop, ResourceID: "service:web",
		RestartCountThreshold: 1, RestartWindow: time.Hour, Enabled: true}
	rules := newFakeRuleStore(r)
	tracker := NewRestartTracker()
	tracker.Observe("service:web", "web-h1", time.Now())
	tracker.Observe("service:web", "web-h1", time.Now()) // 1 restart, meets threshold of 1

	logs := &fakeLogsSource{entries: []telemetry.LogEntry{
		{Message: "panic: out of memory"},
		{Message: "goroutine 1 [running]:"},
	}}
	spy := &spyNotifier{}
	engine := newTestEngine(rules, nil, logs, tracker, spy)

	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error = %v", err)
	}

	calls := spy.calls()
	if len(calls) != 1 {
		t.Fatalf("Notify called %d times, want 1", len(calls))
	}
	if len(calls[0].LogLines) != 2 || calls[0].LogLines[0] != "panic: out of memory" {
		t.Errorf("LogLines = %v, want the two fake log entries' messages", calls[0].LogLines)
	}
}

func TestEngine_Tick_CrashloopResolved_NoLogLines(t *testing.T) {
	firingSince := time.Now().Add(-time.Hour)
	r := Rule{ID: "cl1", Kind: KindCrashloop, ResourceID: "service:web",
		RestartCountThreshold: 5, RestartWindow: time.Minute, Enabled: true, Firing: true, FiringSince: &firingSince}
	rules := newFakeRuleStore(r)
	tracker := NewRestartTracker() // no restarts recorded: count is 0, well under threshold, so this resolves
	logs := &fakeLogsSource{entries: []telemetry.LogEntry{{Message: "should not be fetched"}}}
	spy := &spyNotifier{}
	engine := newTestEngine(rules, nil, logs, tracker, spy)

	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error = %v", err)
	}

	calls := spy.calls()
	if len(calls) != 1 || !calls[0].Resolved {
		t.Fatalf("calls = %+v, want one Resolved=true event", calls)
	}
	if len(calls[0].LogLines) != 0 {
		t.Errorf("LogLines = %v, want none attached to a resolved event", calls[0].LogLines)
	}
}

func TestEngine_Tick_TruncatesToLast200LogLines(t *testing.T) {
	r := Rule{ID: "cl1", Kind: KindCrashloop, ResourceID: "service:web",
		RestartCountThreshold: 1, RestartWindow: time.Hour, Enabled: true}
	rules := newFakeRuleStore(r)
	tracker := NewRestartTracker()
	tracker.Observe("service:web", "web-h1", time.Now())
	tracker.Observe("service:web", "web-h1", time.Now())

	entries := make([]telemetry.LogEntry, 250)
	for i := range entries {
		entries[i] = telemetry.LogEntry{Message: string(rune('a' + i%26))}
	}
	entries[249].Message = "the-last-line"
	logs := &fakeLogsSource{entries: entries}
	spy := &spyNotifier{}
	engine := newTestEngine(rules, nil, logs, tracker, spy)

	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error = %v", err)
	}

	calls := spy.calls()
	if len(calls[0].LogLines) != crashloopLogLines {
		t.Fatalf("LogLines length = %d, want %d (truncated to the most recent 200)", len(calls[0].LogLines), crashloopLogLines)
	}
	if got := calls[0].LogLines[len(calls[0].LogLines)-1]; got != "the-last-line" {
		t.Errorf("last line = %q, want the actual most recent entry preserved", got)
	}
}

func TestEngine_Tick_MetricsQueryError_OtherRulesStillEvaluated(t *testing.T) {
	broken := Rule{ID: "r1", Kind: KindThreshold, ResourceID: "service:broken", Metric: "cpu_percent", Comparator: GreaterThan, Threshold: 80, Enabled: true}
	// Second rule uses the crashloop path so it's unaffected by the
	// metrics source's error, proving one rule's failure doesn't stop
	// the tick from evaluating the rest.
	crashloop := Rule{ID: "cl1", Kind: KindCrashloop, ResourceID: "service:web", RestartCountThreshold: 1, RestartWindow: time.Hour, Enabled: true}
	rules := newFakeRuleStore(broken, crashloop)
	metrics := &fakeMetricsSource{err: errors.New("query failed")}
	tracker := NewRestartTracker()
	tracker.Observe("service:web", "web-h1", time.Now())
	tracker.Observe("service:web", "web-h1", time.Now())
	spy := &spyNotifier{}
	engine := newTestEngine(rules, metrics, &fakeLogsSource{}, tracker, spy)

	err := engine.Tick(context.Background())
	if err == nil {
		t.Fatal("Tick() error = nil, want the broken rule's error surfaced")
	}

	if !rules.get("cl1").Firing {
		t.Error("crashloop rule was not evaluated/fired despite the threshold rule's query failure")
	}
}

// TestEngine_Tick_MixedLegacyAndChannelRules_BothDispatch proves a
// legacy (no ChannelID) rule and a channel-attached rule both fire from
// one Tick, each resolving its own NotifyURL correctly, mirroring
// TestDeployDispatcher_Dispatch_MixedLegacyAndChannelTargets. Uses the
// real *DB as RuleStore, not fakeRuleStore, since the channel resolution
// under test happens in the SQL join, not in Go logic a fake could
// stand in for.
func TestEngine_Tick_MixedLegacyAndChannelRules_BothDispatch(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	if err := db.SaveNotificationChannel(ctx, NotificationChannel{ID: "chn_1", Name: "Team Slack", Kind: NotifySlack, NotifyURL: "https://hooks.slack.com/x", Enabled: true}); err != nil {
		t.Fatalf("seed channel: %v", err)
	}
	legacy := Rule{ID: "r_legacy", Kind: KindThreshold, ResourceID: "service:web", Metric: "cpu_percent",
		Comparator: GreaterThan, Threshold: 80, NotifyURL: "https://legacy.example.com/hook", NotifyKind: NotifyDiscord, Enabled: true}
	channelAttached := Rule{ID: "r_channel", Kind: KindThreshold, ResourceID: "service:web", Metric: "cpu_percent",
		Comparator: GreaterThan, Threshold: 80, ChannelID: "chn_1", Enabled: true}
	if err := db.SaveRule(ctx, legacy); err != nil {
		t.Fatalf("seed legacy rule: %v", err)
	}
	if err := db.SaveRule(ctx, channelAttached); err != nil {
		t.Fatalf("seed channel-attached rule: %v", err)
	}

	metrics := &fakeMetricsSource{samples: []telemetry.Sample{{Timestamp: time.Now(), Value: 95}}}
	spy := &spyNotifier{}
	engine := NewEngine(db, metrics, nil, nil, func(Rule) Notifier { return spy }, nil)

	if err := engine.Tick(ctx); err != nil {
		t.Fatalf("Tick() error = %v", err)
	}

	calls := spy.calls()
	if len(calls) != 2 {
		t.Fatalf("Notify called %d times, want 2 (one per rule)", len(calls))
	}

	var gotLegacyURL, gotChannelURL string
	for _, ev := range calls {
		switch ev.Rule.ID {
		case "r_legacy":
			gotLegacyURL = ev.Rule.NotifyURL
		case "r_channel":
			gotChannelURL = ev.Rule.NotifyURL
		}
	}
	if gotLegacyURL != "https://legacy.example.com/hook" {
		t.Errorf("legacy rule NotifyURL = %q, want its own column unchanged", gotLegacyURL)
	}
	if gotChannelURL != "https://hooks.slack.com/x" {
		t.Errorf("channel-attached rule NotifyURL = %q, want resolved from chn_1", gotChannelURL)
	}
}

// TestEngine_Tick_DisabledChannel_SilencesRule proves a rule attached to
// a disabled channel still evaluates (state is tracked) but never
// dispatches, mirroring the deploy-target silencing property at the
// evaluator level rather than just the DB-read level.
func TestEngine_Tick_DisabledChannel_SilencesRule(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	if err := db.SaveNotificationChannel(ctx, NotificationChannel{ID: "chn_1", Name: "Paused", Kind: NotifyGeneric, NotifyURL: "https://example.com/hook", Enabled: false}); err != nil {
		t.Fatalf("seed channel: %v", err)
	}
	r := Rule{ID: "r1", Kind: KindThreshold, ResourceID: "service:web", Metric: "cpu_percent",
		Comparator: GreaterThan, Threshold: 80, ChannelID: "chn_1", Enabled: true}
	if err := db.SaveRule(ctx, r); err != nil {
		t.Fatalf("seed rule: %v", err)
	}

	metrics := &fakeMetricsSource{samples: []telemetry.Sample{{Timestamp: time.Now(), Value: 95}}}
	spy := &spyNotifier{}
	engine := NewEngine(db, metrics, nil, nil, func(Rule) Notifier { return spy }, nil)

	if err := engine.Tick(ctx); err != nil {
		t.Fatalf("Tick() error = %v", err)
	}
	if calls := spy.calls(); len(calls) != 0 {
		t.Errorf("Notify called %d times for a rule attached to a disabled channel, want 0", len(calls))
	}
	got, err := db.GetRule(ctx, "r1")
	if err != nil {
		t.Fatalf("GetRule() error = %v", err)
	}
	if !got.Firing {
		t.Error("Firing = false, want true: evaluation state must still be tracked even while silenced")
	}
}

func TestEngine_Run_StopsOnContextCancel(t *testing.T) {
	rules := newFakeRuleStore()
	engine := newTestEngine(rules, &fakeMetricsSource{}, &fakeLogsSource{}, NewRestartTracker(), &spyNotifier{})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	err := engine.Run(ctx, 10*time.Millisecond)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Run() error = %v, want context.DeadlineExceeded", err)
	}
}

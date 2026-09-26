package alerting

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

var t0 = time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)

func TestSilenceMatcher_Matches(t *testing.T) {
	ac := AlertContext{RuleID: "r1", Kind: "threshold", App: "web", NodeID: "n1", NodeName: "edge-1",
		Labels: map[string]string{"team": "core", "env": "prod"}, Severity: SeverityCritical}
	tests := []struct {
		name string
		m    SilenceMatcher
		want bool
	}{
		{"rule id", SilenceMatcher{RuleIDs: []string{"r1"}}, true},
		{"other rule id", SilenceMatcher{RuleIDs: []string{"r2"}}, false},
		{"kind or", SilenceMatcher{Kinds: []string{"crashloop", "threshold"}}, true},
		{"app case-insensitive", SilenceMatcher{Apps: []string{"WEB"}}, true},
		{"app miss", SilenceMatcher{Apps: []string{"api"}}, false},
		{"node by name", SilenceMatcher{Nodes: []string{"edge-1"}}, true},
		{"node by id", SilenceMatcher{Nodes: []string{"n1"}}, true},
		{"node miss", SilenceMatcher{Nodes: []string{"n2"}}, false},
		{"labels subset", SilenceMatcher{Labels: map[string]string{"team": "core"}}, true},
		{"labels value miss", SilenceMatcher{Labels: map[string]string{"team": "data"}}, false},
		{"labels key miss", SilenceMatcher{Labels: map[string]string{"zone": "a"}}, false},
		{"severity", SilenceMatcher{Severities: []string{SeverityCritical}}, true},
		{"severity miss", SilenceMatcher{Severities: []string{SeverityInfo}}, false},
		{"AND across fields", SilenceMatcher{Apps: []string{"web"}, Kinds: []string{"crashloop"}}, false},
		{"AND all match", SilenceMatcher{Apps: []string{"web"}, Kinds: []string{"threshold"}, Nodes: []string{"n1"}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.m.Matches(ac); got != tt.want {
				t.Errorf("Matches() = %v, want %v", got, tt.want)
			}
		})
	}

	platform := AlertContext{RuleID: "r9", Kind: "cert_expiry"}
	if (SilenceMatcher{Apps: []string{"web"}}).Matches(platform) {
		t.Error("app matcher must not match a platform-wide alert with no app")
	}
	if (SilenceMatcher{Nodes: []string{"n1"}}).Matches(platform) {
		t.Error("node matcher must not match an alert with no node")
	}
}

func TestSilenceMatcher_Validate(t *testing.T) {
	if err := (SilenceMatcher{}).Validate(); err == nil {
		t.Error("empty matcher must be rejected")
	}
	if err := (SilenceMatcher{Severities: []string{"loud"}}).Validate(); err == nil {
		t.Error("unknown severity must be rejected")
	}
	if err := (SilenceMatcher{Apps: []string{"web"}}).Validate(); err != nil {
		t.Errorf("valid matcher rejected: %v", err)
	}
}

func TestSilence_Status(t *testing.T) {
	early := t0.Add(30 * time.Minute)
	tests := []struct {
		name string
		s    Silence
		now  time.Time
		want string
	}{
		{"pending", Silence{StartsAt: t0.Add(time.Hour), EndsAt: t0.Add(2 * time.Hour)}, t0, SilencePending},
		{"active", Silence{StartsAt: t0.Add(-time.Hour), EndsAt: t0.Add(time.Hour)}, t0, SilenceActive},
		{"active at start", Silence{StartsAt: t0, EndsAt: t0.Add(time.Hour)}, t0, SilenceActive},
		{"expired at end", Silence{StartsAt: t0.Add(-time.Hour), EndsAt: t0}, t0, SilenceExpired},
		{"expired early", Silence{StartsAt: t0.Add(-time.Hour), EndsAt: t0.Add(time.Hour), ExpiredAt: &early}, t0.Add(time.Hour), SilenceExpired},
		{"early expiry not yet", Silence{StartsAt: t0.Add(-time.Hour), EndsAt: t0.Add(time.Hour), ExpiredAt: &early}, t0, SilenceActive},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.s.Status(tt.now); got != tt.want {
				t.Errorf("Status() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMaintenanceWindow_StateAt(t *testing.T) {
	utc := MaintenanceWindow{Cron: "0 3 * * *", Duration: 2 * time.Hour, Timezone: "UTC", Scope: ScopeAll}
	ny := MaintenanceWindow{Cron: "0 22 * * *", Duration: 4 * time.Hour, Timezone: "America/New_York", Scope: ScopeAll}
	weekly := MaintenanceWindow{Cron: "30 1 * * 0", Duration: time.Hour, Scope: ScopeAll}
	tests := []struct {
		name       string
		w          MaintenanceWindow
		now        time.Time
		wantActive bool
	}{
		{"before start", utc, time.Date(2026, 9, 25, 2, 59, 0, 0, time.UTC), false},
		{"at start", utc, time.Date(2026, 9, 25, 3, 0, 0, 0, time.UTC), true},
		{"inside", utc, time.Date(2026, 9, 25, 4, 59, 0, 0, time.UTC), true},
		{"at end excluded", utc, time.Date(2026, 9, 25, 5, 0, 0, 0, time.UTC), false},
		// 22:00 New York (EDT, UTC-4) is 02:00 UTC next day.
		{"tz inside", ny, time.Date(2026, 9, 26, 3, 0, 0, 0, time.UTC), true},
		{"tz before in utc terms", ny, time.Date(2026, 9, 26, 1, 59, 0, 0, time.UTC), false},
		{"tz crosses local midnight", ny, time.Date(2026, 9, 26, 5, 59, 0, 0, time.UTC), true},
		{"tz after", ny, time.Date(2026, 9, 26, 6, 0, 0, 0, time.UTC), false},
		{"weekly sunday hit", weekly, time.Date(2026, 9, 27, 1, 45, 0, 0, time.UTC), true},
		{"weekly monday miss", weekly, time.Date(2026, 9, 28, 1, 45, 0, 0, time.UTC), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st, err := tt.w.StateAt(tt.now)
			if err != nil {
				t.Fatalf("StateAt() error = %v", err)
			}
			if st.Active != tt.wantActive {
				t.Errorf("Active = %v, want %v (state %+v)", st.Active, tt.wantActive, st)
			}
			if st.Active && !st.End.After(tt.now) {
				t.Errorf("End %v must be after now %v", st.End, tt.now)
			}
			if !st.Active && !st.NextStart.After(tt.now) {
				t.Errorf("NextStart %v must be after now %v", st.NextStart, tt.now)
			}
		})
	}
}

func TestMaintenanceWindow_Validate(t *testing.T) {
	ok := MaintenanceWindow{Name: "n", Cron: "0 3 * * *", Duration: time.Hour, Scope: ScopeAll}
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid window rejected: %v", err)
	}
	bad := map[string]func(*MaintenanceWindow){
		"no name":        func(w *MaintenanceWindow) { w.Name = "" },
		"bad cron":       func(w *MaintenanceWindow) { w.Cron = "nope" },
		"zero duration":  func(w *MaintenanceWindow) { w.Duration = 0 },
		"huge duration":  func(w *MaintenanceWindow) { w.Duration = MaxMaintenanceDuration + time.Second },
		"bad tz":         func(w *MaintenanceWindow) { w.Timezone = "Mars/Base" },
		"bad scope":      func(w *MaintenanceWindow) { w.Scope = "galaxy" },
		"app no targets": func(w *MaintenanceWindow) { w.Scope = ScopeApp },
	}
	for name, mutate := range bad {
		t.Run(name, func(t *testing.T) {
			w := ok
			mutate(&w)
			if err := w.Validate(); err == nil {
				t.Error("expected validation error")
			}
		})
	}
}

func TestMaintenanceWindow_Covers(t *testing.T) {
	ac := AlertContext{App: "web", NodeID: "n1", NodeName: "edge-1"}
	tests := []struct {
		w    MaintenanceWindow
		want bool
	}{
		{MaintenanceWindow{Scope: ScopeAll}, true},
		{MaintenanceWindow{Scope: ScopeApp, Targets: []string{"web"}}, true},
		{MaintenanceWindow{Scope: ScopeApp, Targets: []string{"api"}}, false},
		{MaintenanceWindow{Scope: ScopeNode, Targets: []string{"edge-1"}}, true},
		{MaintenanceWindow{Scope: ScopeNode, Targets: []string{"n2"}}, false},
	}
	for _, tt := range tests {
		if got := tt.w.Covers(ac); got != tt.want {
			t.Errorf("Covers(%+v) = %v, want %v", tt.w, got, tt.want)
		}
	}
}

func TestStreakTracker(t *testing.T) {
	s := newStreakTracker()
	prev := Rule{ID: "r"}
	firing := Rule{ID: "r", Firing: true, FiringSince: &t0}
	quiet := Rule{ID: "r"}

	got := s.apply(prev, firing, 3, t0)
	if got.Firing || got.PendingSince == nil {
		t.Fatalf("tick 1: want pending, got %+v", got)
	}
	got = s.apply(Rule{ID: "r", PendingSince: &t0}, firing, 3, t0.Add(30*time.Second))
	if got.Firing {
		t.Fatalf("tick 2: want still pending, got %+v", got)
	}
	got = s.apply(Rule{ID: "r", PendingSince: &t0}, firing, 3, t0.Add(time.Minute))
	if !got.Firing {
		t.Fatalf("tick 3: want firing, got %+v", got)
	}

	s.apply(prev, quiet, 3, t0)
	if s.counts["r"] != 0 {
		t.Errorf("streak must reset when the condition clears, got %d", s.counts["r"])
	}
	got = s.apply(prev, firing, 3, t0)
	if got.Firing {
		t.Error("after reset the streak must start over")
	}
	if got := s.apply(Rule{ID: "r", Firing: true}, firing, 3, t0); !got.Firing {
		t.Error("an already-firing rule is never held back")
	}
	if got := newStreakTracker().apply(prev, firing, 1, t0); !got.Firing {
		t.Error("need=1 must fire immediately")
	}
}

func TestFlapTracker_EnterAndExitWithHysteresis(t *testing.T) {
	f := newFlapTracker()
	fire := Event{Rule: Rule{ID: "r"}}
	resolve := Event{Rule: Rule{ID: "r"}, Resolved: true}
	window := 10 * time.Minute

	var entered bool
	now := t0
	for i := 0; i < 3; i++ {
		fl, en := f.observe(fire, 3, window, now)
		if fl || en {
			t.Fatalf("fire %d must not flap yet", i+1)
		}
		f.observe(resolve, 3, window, now.Add(10*time.Second))
		now = now.Add(time.Minute)
	}
	_, entered = f.observe(fire, 3, window, now)
	if !entered {
		t.Fatal("4th fire inside the window must enter flapping")
	}
	if fl, en := f.observe(resolve, 3, window, now.Add(time.Second)); !fl || en {
		t.Errorf("resolve while flapping: flapping=%v entered=%v, want true,false", fl, en)
	}
	if got := f.sweep(now.Add(time.Minute)); len(got) != 0 {
		t.Errorf("must stay flapping while fires are still in the window, got %d exits", len(got))
	}
	ended := f.sweep(now.Add(window + time.Minute))
	if len(ended) != 1 || !ended[0].Resolved {
		t.Fatalf("expected one exit carrying the last event, got %+v", ended)
	}
	if _, still := f.flapping["r"]; still {
		t.Error("rule must no longer be flapping after exit")
	}
}

func TestFlapTracker_Disabled(t *testing.T) {
	f := newFlapTracker()
	for i := 0; i < 20; i++ {
		if fl, _ := f.observe(Event{Rule: Rule{ID: "r"}}, 0, time.Hour, t0); fl {
			t.Fatal("threshold 0 disables flapping")
		}
	}
}

func TestRateLimiter(t *testing.T) {
	r := newRateLimiter()
	for i := 0; i < 2; i++ {
		if !r.allow("c", 2, time.Minute, t0.Add(time.Duration(i)*time.Second)) {
			t.Fatalf("send %d should be allowed", i+1)
		}
	}
	if r.allow("c", 2, time.Minute, t0.Add(5*time.Second)) {
		t.Error("third send inside the window must be limited")
	}
	if !r.allow("other", 2, time.Minute, t0.Add(5*time.Second)) {
		t.Error("limits are per key")
	}
	if !r.allow("c", 2, time.Minute, t0.Add(time.Minute+2*time.Second)) {
		t.Error("window must slide")
	}
	if !r.allow("c", 0, time.Minute, t0) {
		t.Error("limit 0 disables limiting")
	}
}

func TestGroupBuffer(t *testing.T) {
	g := newGroupBuffer()
	a := Event{Rule: Rule{ID: "a", Name: "A", Kind: KindThreshold, ResourceID: "service:web"}}
	b := Event{Rule: Rule{ID: "b", Name: "B", Kind: KindCrashloop, ResourceID: "service:web"}}
	g.add("k", a, t0)
	g.add("k", b, t0.Add(time.Second))
	g.add("k", a, t0.Add(2*time.Second))

	if got := g.due(time.Minute, t0.Add(30*time.Second)); len(got) != 0 {
		t.Fatal("group must wait for its window")
	}
	due := g.due(time.Minute, t0.Add(time.Minute))
	if len(due) != 1 || len(due[0]) != 2 {
		t.Fatalf("want one group of 2 (deduplicated), got %+v", due)
	}
	combined := combineGroup(due[0])
	if combined.GroupCount != 2 || len(combined.GroupNotices) != 2 {
		t.Errorf("combined = %+v", combined)
	}
	if summaryText(combined) == "" || combined.Headline == "" {
		t.Error("combined event needs a headline")
	}
	if one := combineGroup(due[0][:1]); one.GroupCount != 0 {
		t.Error("a single alert is sent as-is")
	}

	g.add("k", a, t0)
	if !g.remove("a") {
		t.Error("remove must find the buffered alert")
	}
	if len(g.groups) != 0 {
		t.Error("empty group must be dropped")
	}
}

type fakePlacement struct {
	nodeOf map[string]string
	nodes  []store.Node
}

func (f *fakePlacement) GetDesiredService(_ context.Context, name string) (*store.DesiredService, error) {
	id, ok := f.nodeOf[name]
	if !ok {
		return nil, store.ErrServiceNotFound
	}
	return &store.DesiredService{NodeID: id}, nil
}

func (f *fakePlacement) ListNodes(_ context.Context) ([]store.Node, error) { return f.nodes, nil }

type sendSpy struct {
	events []Event
	err    error
}

func (s *sendSpy) send(_ context.Context, ev Event) error {
	s.events = append(s.events, ev)
	return s.err
}

func newRouteFixture(t *testing.T, cfg NoiseConfig, pl PlacementSource) (*NoiseControl, *DB, *sendSpy) {
	t.Helper()
	db := newTestDB(t)
	return NewNoiseControl(cfg, db, pl, nil), db, &sendSpy{}
}

func history(t *testing.T, db *DB) []HistoryEntry {
	t.Helper()
	h, err := db.ListHistory(context.Background(), HistoryFilter{})
	if err != nil {
		t.Fatalf("ListHistory() error = %v", err)
	}
	return h
}

func webRule(id string) Rule {
	return Rule{ID: id, Name: "rule " + id, Kind: KindThreshold, ResourceID: "service:web", Enabled: true, Severity: SeverityWarning}
}

func TestRoute_SentAndRecorded(t *testing.T) {
	nc, db, spy := newRouteFixture(t, NoiseConfig{}, nil)
	nc.Route(context.Background(), Event{Rule: webRule("r1")}, t0, spy.send)
	if len(spy.events) != 1 {
		t.Fatalf("sent %d, want 1", len(spy.events))
	}
	h := history(t, db)
	if len(h) != 1 || h[0].Outcome != OutcomeSent || h[0].Event != EventFired || h[0].App != "web" {
		t.Fatalf("history = %+v", h)
	}
}

func TestRoute_SilencedFireAndResolve_ThenReleasedOnExpiry(t *testing.T) {
	ctx := context.Background()
	nc, db, spy := newRouteFixture(t, NoiseConfig{}, nil)
	if err := db.CreateSilence(ctx, Silence{ID: "sil_1", Matchers: SilenceMatcher{Apps: []string{"web"}}, StartsAt: t0.Add(-time.Minute), EndsAt: t0.Add(time.Hour), CreatedAt: t0}); err != nil {
		t.Fatal(err)
	}

	r := webRule("r1")
	nc.Route(ctx, Event{Rule: r}, t0, spy.send)
	if len(spy.events) != 0 {
		t.Fatal("silenced alert must not notify")
	}

	// Still firing after the silence ends: the held notification is released.
	nc.Recheck(ctx, r, t0.Add(30*time.Minute), spy.send)
	if len(spy.events) != 0 {
		t.Fatal("must not release while the silence is active")
	}
	nc.Recheck(ctx, r, t0.Add(2*time.Hour), spy.send)
	if len(spy.events) != 1 || spy.events[0].Resolved {
		t.Fatalf("expected the held fire to be released, got %+v", spy.events)
	}

	h := history(t, db)
	if h[0].Outcome != OutcomeSent || h[0].Detail == "" || h[1].Outcome != OutcomeSilenced || h[1].SilenceID != "sil_1" {
		t.Fatalf("history = %+v", h)
	}
}

func TestRoute_SilencedFireResolvedBeforeRelease_NoResolvedNotice(t *testing.T) {
	ctx := context.Background()
	nc, db, spy := newRouteFixture(t, NoiseConfig{}, nil)
	_ = db.CreateSilence(ctx, Silence{ID: "sil_1", Matchers: SilenceMatcher{RuleIDs: []string{"r1"}}, StartsAt: t0.Add(-time.Minute), EndsAt: t0.Add(time.Hour), CreatedAt: t0})

	r := webRule("r1")
	nc.Route(ctx, Event{Rule: r}, t0, spy.send)
	nc.Route(ctx, Event{Rule: r, Resolved: true}, t0.Add(10*time.Minute), spy.send)
	if len(spy.events) != 0 {
		t.Fatalf("operator never heard the fire, so no resolved notice either: %+v", spy.events)
	}
	h := history(t, db)
	if len(h) != 2 || h[0].Event != EventResolved || h[0].Outcome != OutcomeSilenced {
		t.Fatalf("history = %+v", h)
	}
}

func TestRoute_MaintenanceWindowSilences(t *testing.T) {
	ctx := context.Background()
	nc, db, spy := newRouteFixture(t, NoiseConfig{}, nil)
	if err := db.SaveMaintenanceWindow(ctx, MaintenanceWindow{ID: "mw_1", Name: "nightly", Cron: "0 12 * * *", Duration: time.Hour, Scope: ScopeApp, Targets: []string{"web"}, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	nc.Route(ctx, Event{Rule: webRule("r1")}, t0.Add(10*time.Minute), spy.send)
	if len(spy.events) != 0 {
		t.Fatal("alert inside a maintenance window must not notify")
	}
	nc.Route(ctx, Event{Rule: webRule("r2")}, t0.Add(2*time.Hour), spy.send)
	if len(spy.events) != 1 {
		t.Fatal("alert outside the window must notify")
	}
	other := webRule("r3")
	other.ResourceID = "service:api"
	nc.Route(ctx, Event{Rule: other}, t0.Add(10*time.Minute), spy.send)
	if len(spy.events) != 2 {
		t.Fatal("window scoped to another app must not apply")
	}
}

func TestRoute_NodeOfflineInhibitsAppAlerts_NotNodeRules(t *testing.T) {
	ctx := context.Background()
	pl := &fakePlacement{nodeOf: map[string]string{"web": "n1"}, nodes: []store.Node{{ID: "n1", Name: "edge-1", Status: store.NodeStatusOffline}}}
	nc, db, spy := newRouteFixture(t, NoiseConfig{}, pl)

	nc.Route(ctx, Event{Rule: webRule("r1")}, t0, spy.send)
	if len(spy.events) != 0 {
		t.Fatal("app alert on an offline node must be inhibited")
	}
	off := Rule{ID: "r2", Name: "node down", Kind: KindNodeOffline, ResourceID: "service:web", Enabled: true}
	nc.Route(ctx, Event{Rule: off}, t0, spy.send)
	if len(spy.events) != 1 {
		t.Fatal("the node offline rule itself must still notify")
	}

	pl.nodes[0].Status = store.NodeStatusOnline
	nc.Recheck(ctx, webRule("r1"), t0.Add(time.Minute), spy.send)
	if len(spy.events) != 2 {
		t.Fatal("inhibited alert must be released once the node is back and the app is still firing")
	}
	var inhibited bool
	for _, h := range history(t, db) {
		if h.Outcome == OutcomeInhibited && h.Node == "edge-1" {
			inhibited = true
		}
	}
	if !inhibited {
		t.Error("inhibition must be recorded in history with the node")
	}
}

func TestRoute_SilenceMatchesByNode(t *testing.T) {
	ctx := context.Background()
	pl := &fakePlacement{nodeOf: map[string]string{"web": "n1"}, nodes: []store.Node{{ID: "n1", Name: "edge-1", Status: store.NodeStatusOnline}}}
	nc, db, spy := newRouteFixture(t, NoiseConfig{}, pl)
	_ = db.CreateSilence(ctx, Silence{ID: "sil_n", Matchers: SilenceMatcher{Nodes: []string{"edge-1"}}, StartsAt: t0.Add(-time.Minute), EndsAt: t0.Add(time.Hour), CreatedAt: t0})
	nc.Route(ctx, Event{Rule: webRule("r1")}, t0, spy.send)
	if len(spy.events) != 0 {
		t.Fatal("node silence must cover apps placed on that node")
	}
}

func TestRoute_GroupingSendsOneMessageWithCount(t *testing.T) {
	ctx := context.Background()
	nc, db, spy := newRouteFixture(t, NoiseConfig{GroupWindow: time.Minute}, nil)
	for _, id := range []string{"r1", "r2", "r3"} {
		r := webRule(id)
		r.NotifyURL = "https://hook"
		nc.Route(ctx, Event{Rule: r}, t0, spy.send)
	}
	if len(spy.events) != 0 {
		t.Fatal("nothing is sent before the group window closes")
	}
	nc.Sweep(ctx, t0.Add(30*time.Second), spy.send)
	if len(spy.events) != 0 {
		t.Fatal("window not yet closed")
	}
	nc.Sweep(ctx, t0.Add(time.Minute), spy.send)
	if len(spy.events) != 1 || spy.events[0].GroupCount != 3 {
		t.Fatalf("want one grouped message of 3, got %+v", spy.events)
	}
	for _, h := range history(t, db) {
		if h.Outcome != OutcomeGrouped {
			t.Errorf("history outcome = %q, want grouped", h.Outcome)
		}
	}
}

func TestRoute_GroupedFireResolvedBeforeFlush_SendsNothing(t *testing.T) {
	ctx := context.Background()
	nc, _, spy := newRouteFixture(t, NoiseConfig{GroupWindow: time.Minute}, nil)
	r := webRule("r1")
	nc.Route(ctx, Event{Rule: r}, t0, spy.send)
	nc.Route(ctx, Event{Rule: r, Resolved: true}, t0.Add(10*time.Second), spy.send)
	nc.Sweep(ctx, t0.Add(2*time.Minute), spy.send)
	if len(spy.events) != 0 {
		t.Fatalf("a blip that resolved inside the group window must send nothing: %+v", spy.events)
	}
}

func TestRoute_FlushAllOnShutdown(t *testing.T) {
	ctx := context.Background()
	nc, _, spy := newRouteFixture(t, NoiseConfig{GroupWindow: time.Hour}, nil)
	nc.Route(ctx, Event{Rule: webRule("r1")}, t0, spy.send)
	nc.FlushAll(ctx, t0, spy.send)
	if len(spy.events) != 1 {
		t.Fatalf("FlushAll must not lose buffered alerts, sent %d", len(spy.events))
	}
}

func TestRoute_FlappingNotifiesOnceWithSummary(t *testing.T) {
	ctx := context.Background()
	nc, db, spy := newRouteFixture(t, NoiseConfig{FlapThreshold: 2, FlapWindow: 10 * time.Minute}, nil)
	r := webRule("r1")
	now := t0
	for i := 0; i < 6; i++ {
		nc.Route(ctx, Event{Rule: r}, now, spy.send)
		nc.Route(ctx, Event{Rule: r, Resolved: true}, now.Add(10*time.Second), spy.send)
		now = now.Add(time.Minute)
	}
	// fire1, resolve1, fire2, resolve2 go out (4), then fire3 flaps: one summary, everything after is held.
	if len(spy.events) != 5 {
		t.Fatalf("sent %d, want 5 (4 normal + 1 flapping summary)", len(spy.events))
	}
	summary := spy.events[4]
	if summary.Headline == "" || summary.Resolved {
		t.Fatalf("5th message must be the flapping summary, got %+v", summary)
	}
	counts := map[string]int{}
	for _, h := range history(t, db) {
		counts[h.Outcome+"/"+h.Event]++
	}
	if counts["sent/flapping"] != 1 || counts["flapping/fired"] == 0 || counts["flapping/resolved"] == 0 {
		t.Fatalf("history outcome counts = %v", counts)
	}

	nc.Sweep(ctx, now.Add(20*time.Minute), spy.send)
	last := spy.events[len(spy.events)-1]
	if len(spy.events) != 6 || last.Headline == "" {
		t.Fatalf("expected one stable notice after calming, got %d events, last %+v", len(spy.events), last)
	}
}

func TestRoute_SendFailureRecordedAsFailed(t *testing.T) {
	nc, db, spy := newRouteFixture(t, NoiseConfig{}, nil)
	spy.err = errors.New("webhook down")
	nc.Route(context.Background(), Event{Rule: webRule("r1")}, t0, spy.send)
	h := history(t, db)
	if len(h) != 1 || h[0].Outcome != OutcomeFailed || h[0].Error != "webhook down" {
		t.Fatalf("history = %+v", h)
	}
}

func TestRoute_GroupFlushFailureRecordsFailedForEveryMember(t *testing.T) {
	ctx := context.Background()
	nc, db, spy := newRouteFixture(t, NoiseConfig{GroupWindow: time.Minute}, nil)
	spy.err = errors.New("boom")
	nc.Route(ctx, Event{Rule: webRule("r1")}, t0, spy.send)
	nc.Route(ctx, Event{Rule: webRule("r2")}, t0, spy.send)
	nc.Sweep(ctx, t0.Add(time.Minute), spy.send)
	h := history(t, db)
	if len(h) != 2 || h[0].Outcome != OutcomeFailed || h[1].Outcome != OutcomeFailed {
		t.Fatalf("history = %+v", h)
	}
}

func TestRoute_RateLimitPerChannel(t *testing.T) {
	ctx := context.Background()
	nc, db, spy := newRouteFixture(t, NoiseConfig{RateLimit: 2, RateWindow: time.Minute}, nil)
	for i, id := range []string{"r1", "r2", "r3"} {
		r := webRule(id)
		r.ChannelID = "ch1"
		nc.Route(ctx, Event{Rule: r}, t0.Add(time.Duration(i)*time.Second), spy.send)
	}
	if len(spy.events) != 2 {
		t.Fatalf("sent %d, want 2", len(spy.events))
	}
	if h := history(t, db); h[0].Outcome != OutcomeRateLimited {
		t.Errorf("newest history = %+v, want ratelimited", h[0])
	}
}

func TestSweep_PrunesHistoryByRetention(t *testing.T) {
	ctx := context.Background()
	nc, db, spy := newRouteFixture(t, NoiseConfig{HistoryRetention: 24 * time.Hour}, nil)
	_ = db.RecordHistory(ctx, HistoryEntry{At: t0.Add(-48 * time.Hour), RuleID: "old", Event: EventFired, Outcome: OutcomeSent})
	_ = db.RecordHistory(ctx, HistoryEntry{At: t0.Add(-time.Hour), RuleID: "new", Event: EventFired, Outcome: OutcomeSent})
	nc.Sweep(ctx, t0, spy.send)
	h := history(t, db)
	if len(h) != 1 || h[0].RuleID != "new" {
		t.Fatalf("history after prune = %+v", h)
	}
}

func TestSilenceStore_ExpireKeepsHistory(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	s := Silence{ID: "sil_1", Matchers: SilenceMatcher{Apps: []string{"web"}}, StartsAt: t0.Add(-time.Hour), EndsAt: t0.Add(time.Hour), CreatedBy: "alice", Reason: "deploy", CreatedAt: t0}
	if err := db.CreateSilence(ctx, s); err != nil {
		t.Fatal(err)
	}
	if active, _ := db.ListActiveSilences(ctx, t0); len(active) != 1 {
		t.Fatalf("active = %d, want 1", len(active))
	}
	if _, err := db.ExpireSilence(ctx, "sil_1", t0); err != nil {
		t.Fatal(err)
	}
	if active, _ := db.ListActiveSilences(ctx, t0); len(active) != 0 {
		t.Fatal("expired silence must not be active")
	}
	all, _ := db.ListSilences(ctx, t0, true, 10)
	if len(all) != 1 || all[0].Status(t0) != SilenceExpired || all[0].CreatedBy != "alice" {
		t.Fatalf("expired silence must stay in history: %+v", all)
	}
	if noExp, _ := db.ListSilences(ctx, t0, false, 10); len(noExp) != 0 {
		t.Fatal("expired silences are hidden unless requested")
	}
	if _, err := db.ExpireSilence(ctx, "missing", t0); !errors.Is(err, ErrSilenceNotFound) {
		t.Errorf("err = %v, want ErrSilenceNotFound", err)
	}
}

func TestRuleNoiseFieldsRoundTrip(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	r := webRule("r1")
	r.Labels = map[string]string{"team": "core"}
	r.Severity = SeverityCritical
	r.ConsecutiveFailures = 3
	r.FlapThreshold = 4
	r.FlapWindow = 15 * time.Minute
	r.Metric, r.Comparator = "cpu", GreaterThan
	if err := db.SaveRule(ctx, r); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetRule(ctx, "r1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Severity != SeverityCritical || got.Labels["team"] != "core" || got.ConsecutiveFailures != 3 || got.FlapThreshold != 4 || got.FlapWindow != 15*time.Minute {
		t.Fatalf("round trip = %+v", got)
	}
}

func TestEngine_Tick_ConsecutiveFailuresHoldsFirstTicks(t *testing.T) {
	r := Rule{ID: "r1", Name: "cpu", Kind: KindThreshold, ResourceID: "service:web", Metric: "cpu_percent",
		Comparator: GreaterThan, Threshold: 80, Enabled: true, ConsecutiveFailures: 3}
	rules := newFakeRuleStore(r)
	metrics := &fakeMetricsSource{samples: telemetrySample(95)}
	spy := &spyNotifier{}
	engine := newTestEngine(rules, metrics, nil, nil, spy)
	db := newTestDB(t)
	engine.SetNoiseControl(NewNoiseControl(NoiseConfig{}, db, nil, nil))

	for i := 1; i <= 3; i++ {
		if err := engine.Tick(context.Background()); err != nil {
			t.Fatal(err)
		}
		wantFiring := i == 3
		if got := rules.get("r1").Firing; got != wantFiring {
			t.Fatalf("after tick %d firing = %v, want %v", i, got, wantFiring)
		}
	}
	if len(spy.calls()) != 1 {
		t.Fatalf("notified %d times, want 1", len(spy.calls()))
	}
}

func telemetrySample(v float64) []telemetry.Sample {
	return []telemetry.Sample{{Timestamp: time.Now(), Value: v}}
}

func TestRoute_GroupedAlertMutedByLateSilence_NotSent(t *testing.T) {
	ctx := context.Background()
	nc, db, spy := newRouteFixture(t, NoiseConfig{GroupWindow: time.Minute}, nil)
	nc.Route(ctx, Event{Rule: webRule("r1")}, t0, spy.send)
	if err := db.CreateSilence(ctx, Silence{ID: "sil_late", Matchers: SilenceMatcher{Apps: []string{"web"}}, StartsAt: t0.Add(10 * time.Second), EndsAt: t0.Add(time.Hour), CreatedAt: t0}); err != nil {
		t.Fatal(err)
	}
	nc.Sweep(ctx, t0.Add(time.Minute), spy.send)
	if len(spy.events) != 0 {
		t.Fatalf("a silence that began before the flush must mute the group: %+v", spy.events)
	}
	h := history(t, db)
	if len(h) != 1 || h[0].Outcome != OutcomeSilenced || h[0].SilenceID != "sil_late" {
		t.Fatalf("history = %+v", h)
	}
}

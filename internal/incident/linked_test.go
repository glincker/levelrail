package incident

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/monitor"
	"github.com/GLINCKER/levelrail/internal/store"
)

type linkStep struct {
	state  string
	streak int
	after  time.Duration
}

func TestLinkerStateMachine(t *testing.T) {
	down := func(n int) linkStep { return linkStep{state: monitor.StateDown, streak: n} }
	up := func(after time.Duration) linkStep { return linkStep{state: monitor.StateUp, after: after} }
	wobble := linkStep{state: monitor.StateSuspect, streak: 1}
	cases := []struct {
		name     string
		spec     monitor.Spec
		mute     string
		manual   bool
		steps    []linkStep
		wantOpen int
		wantAll  int
	}{
		{name: "down opens one incident and repeats are idempotent", spec: monitor.Spec{StatusLink: true},
			steps: []linkStep{down(3), down(4), down(5)}, wantOpen: 1, wantAll: 1},
		{name: "below the per monitor confirmation count stays quiet", spec: monitor.Spec{StatusLink: true, LinkConfirmFailures: 3},
			steps: []linkStep{down(2)}, wantOpen: 0, wantAll: 0},
		{name: "confirmation count reached opens", spec: monitor.Spec{StatusLink: true, LinkConfirmFailures: 3},
			steps: []linkStep{down(2), down(3)}, wantOpen: 1, wantAll: 1},
		{name: "recovery after the period resolves", spec: monitor.Spec{StatusLink: true},
			steps: []linkStep{down(3), up(0), up(3 * time.Minute)}, wantOpen: 0, wantAll: 1},
		{name: "flap inside the recovery period keeps one incident open", spec: monitor.Spec{StatusLink: true},
			steps: []linkStep{down(3), up(0), up(time.Minute), wobble, up(0), up(time.Minute), down(3)}, wantOpen: 1, wantAll: 1},
		{name: "recovery clock restarts after a wobble then resolves", spec: monitor.Spec{StatusLink: true},
			steps: []linkStep{down(3), up(0), up(time.Minute), wobble, up(0), up(3 * time.Minute)}, wantOpen: 0, wantAll: 1},
		{name: "silenced outage opens nothing", spec: monitor.Spec{StatusLink: true}, mute: "silence s1",
			steps: []linkStep{down(3)}, wantOpen: 0, wantAll: 0},
		{name: "operator incident already open on the component", spec: monitor.Spec{StatusLink: true}, manual: true,
			steps: []linkStep{down(3)}, wantOpen: 0, wantAll: 0},
		{name: "link off changes nothing", spec: monitor.Spec{},
			steps: []linkStep{down(3)}, wantOpen: 0, wantAll: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			db, err := store.Open(ctx, filepath.Join(t.TempDir(), "state.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			m := monitor.Monitor{ID: "m1", Name: "api", Kind: monitor.KindTCP, Interval: time.Minute, Timeout: time.Second, Enabled: true,
				Spec: tc.spec, CreatedAt: time.Now()}
			m.Spec.Host, m.Spec.Port = "localhost", 80
			must(t, db.SaveMonitor(ctx, m))
			must(t, db.SaveStatusComponent(ctx, store.StatusComponent{ID: "c1", Kind: "monitor", Target: "m1", DisplayName: "API"}))
			if tc.manual {
				must(t, db.SaveStatusIncident(ctx, store.StatusIncident{ID: "man", Kind: "incident", Title: "Manual", Status: "investigating",
					Impact: "minor", ComponentIDs: []string{"c1"}, StartsAt: time.Now()}))
			}
			fm := &fakeMonitors{status: map[string]monitor.Status{}}
			var audited []string
			l := NewLinker(LinkConfig{ConfirmFailures: 1, RecoveryPeriod: 2 * time.Minute, SweepInterval: time.Minute, Impact: "major", QueueMax: 4},
				db, &fakeGuard{mute: tc.mute}, fm, func(_ context.Context, action, _, _ string) { audited = append(audited, action) }, nil)
			clock := time.Now().UTC()
			l.now = func() time.Time { return clock }
			for _, s := range tc.steps {
				clock = clock.Add(s.after)
				fm.status["m1"] = monitor.Status{State: s.state, FailStreak: s.streak}
				l.Reconcile(ctx, "m1")
			}
			open, err := db.ListOpenLinkedIncidents(ctx)
			must(t, err)
			if len(open) != tc.wantOpen {
				t.Fatalf("open = %d, want %d", len(open), tc.wantOpen)
			}
			all := 0
			for _, a := range audited {
				if a == AuditLinkOpened {
					all++
				}
			}
			if all != tc.wantAll {
				t.Fatalf("opened = %d, want %d (audit %v)", all, tc.wantAll, audited)
			}
			if tc.wantAll > 0 && tc.wantOpen == 0 {
				ups, err := db.ListPublicStatusIncidentUpdates(ctx)
				must(t, err)
				if len(ups) < 2 || ups[len(ups)-1].Status != "resolved" {
					t.Fatalf("want a resolved timeline entry, got %+v", ups)
				}
			}
		})
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestLinkerPublishesOpenAndResolve(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "state.db"))
	must(t, err)
	t.Cleanup(func() { _ = db.Close() })
	m := monitor.Monitor{ID: "m1", Name: "api", Kind: monitor.KindTCP, Interval: time.Minute, Timeout: time.Second, Enabled: true,
		Spec: monitor.Spec{StatusLink: true, Host: "localhost", Port: 80}, CreatedAt: time.Now()}
	must(t, db.SaveMonitor(ctx, m))
	fm := &fakeMonitors{status: map[string]monitor.Status{"m1": {State: monitor.StateDown, FailStreak: 2}}}
	l := NewLinker(LinkConfig{ConfirmFailures: 1, RecoveryPeriod: time.Minute, SweepInterval: time.Minute, Impact: "major", QueueMax: 4}, db, nil, fm, nil, nil)
	clock := time.Now().UTC()
	l.now = func() time.Time { return clock }
	var events []bool
	l.SetPublisher(func(_ context.Context, _ string, opened bool) { events = append(events, opened) })
	l.Reconcile(ctx, "m1")
	fm.status["m1"] = monitor.Status{State: monitor.StateUp}
	l.Reconcile(ctx, "m1")
	clock = clock.Add(2 * time.Minute)
	l.Reconcile(ctx, "m1")
	if len(events) != 2 || !events[0] || events[1] {
		t.Fatalf("want opened then resolved events, got %v", events)
	}
}

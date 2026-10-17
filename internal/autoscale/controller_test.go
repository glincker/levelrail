package autoscale

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeStore struct {
	policies  map[string]Policy
	states    map[string]State
	events    []Event
	replicas  map[string]int
	suspended map[string]bool

	failSetReplicas error
	failSaveState   error
	setCalls        int
}

func newFakeStore(p Policy, replicas int) *fakeStore {
	return &fakeStore{
		policies: map[string]Policy{p.App: p}, states: map[string]State{},
		replicas: map[string]int{p.App: replicas}, suspended: map[string]bool{},
	}
}

func (f *fakeStore) ListAutoscalePolicies(context.Context) ([]Policy, error) {
	var out []Policy
	for _, p := range f.policies {
		out = append(out, p)
	}
	return out, nil
}

func (f *fakeStore) GetAutoscalePolicy(_ context.Context, app string) (Policy, error) {
	p, ok := f.policies[app]
	if !ok {
		return p, ErrNotFound
	}
	return p, nil
}

func (f *fakeStore) SaveAutoscalePolicy(_ context.Context, p Policy, ev Event, _ time.Time) error {
	f.policies[p.App] = p
	f.events = append(f.events, ev)
	return nil
}

func (f *fakeStore) DeleteAutoscalePolicy(_ context.Context, app string, ev Event) error {
	delete(f.policies, app)
	f.events = append(f.events, ev)
	return nil
}

func (f *fakeStore) GetAutoscaleState(_ context.Context, app string) (State, bool, error) {
	st, ok := f.states[app]
	return st, ok, nil
}

func (f *fakeStore) SaveAutoscaleState(_ context.Context, st State, events []Event) error {
	if f.failSaveState != nil {
		return f.failSaveState
	}
	f.states[st.App] = st
	f.events = append(f.events, events...)
	return nil
}

func (f *fakeStore) ListAutoscaleEvents(_ context.Context, app string, _ int) ([]Event, error) {
	var out []Event
	for i := len(f.events) - 1; i >= 0; i-- {
		if f.events[i].App == app {
			out = append(out, f.events[i])
		}
	}
	return out, nil
}

func (f *fakeStore) AutoscaleReplicas(_ context.Context, app string) (int, bool, error) {
	n, ok := f.replicas[app]
	if !ok {
		return 0, false, ErrNotFound
	}
	return n, f.suspended[app], nil
}

func (f *fakeStore) SetAutoscaleReplicas(_ context.Context, app string, n int) error {
	f.setCalls++
	if f.failSetReplicas != nil {
		return f.failSetReplicas
	}
	f.replicas[app] = n
	return nil
}

type fakeSignals struct {
	reading  Reading
	err      error
	spend    string
	spendErr error
	headroom int
}

func (s *fakeSignals) Metric(context.Context, Policy, int, time.Time) (Reading, error) {
	return s.reading, s.err
}

func (s *fakeSignals) Headroom(context.Context, string, int) (int, error) { return s.headroom, nil }

func (s *fakeSignals) SpendEnforced(context.Context, string) (string, error) {
	return s.spend, s.spendErr
}

type fakeRecorder struct{ got []Gauge }

func (r *fakeRecorder) RecordGauges(_ context.Context, s []Gauge, _ time.Time) error {
	r.got = append(r.got, s...)
	return nil
}

func newController(f *fakeStore, sig *fakeSignals, now *time.Time) (*Controller, *int) {
	nudges := 0
	return &Controller{
		Store: f, Signals: sig, Nudge: func() { nudges++ }, Ceiling: 20,
		Now: func() time.Time { return *now },
	}, &nudges
}

func hot() *fakeSignals {
	return &fakeSignals{reading: Reading{Value: 90, Known: true}, headroom: -1}
}

func TestTickScalesUpWritesReplicasAndRecordsWhy(t *testing.T) {
	f, now := newFakeStore(testPolicy(), 4), t0
	c, nudges := newController(f, hot(), &now)
	rec := &fakeRecorder{}
	c.Recorder = rec
	if err := c.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.replicas["web"] != 6 || *nudges != 1 {
		t.Fatalf("replicas = %d nudges = %d, want 6 and 1", f.replicas["web"], *nudges)
	}
	st := f.states["web"]
	if st.Reason != ReasonScaledUp || st.Desired != 6 || st.Current != 4 || !st.LastScaleUp.Equal(t0) || st.Value != 90 {
		t.Fatalf("state = %+v", st)
	}
	if len(f.events) != 1 || f.events[0].Kind != EventScaledUp || f.events[0].Detail == "" {
		t.Fatalf("events = %+v", f.events)
	}
	if len(rec.got) != 1 || rec.got[0].Desired != 6 {
		t.Fatalf("gauges = %+v", rec.got)
	}
}

func TestTickIsIdempotentInsideTheCooldown(t *testing.T) {
	f, now := newFakeStore(testPolicy(), 4), t0
	c, _ := newController(f, hot(), &now)
	_ = c.Tick(context.Background())
	for i := 1; i <= 3; i++ {
		now = t0.Add(time.Duration(i) * 10 * time.Second)
		if err := c.Tick(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if f.replicas["web"] != 6 || f.setCalls != 1 {
		t.Fatalf("replicas = %d after %d writes, want one write to 6", f.replicas["web"], f.setCalls)
	}
	if st := f.states["web"]; st.Reason != ReasonCooldown || st.BlockedBy != BlockedCooldown {
		t.Fatalf("state = %+v", st)
	}
	held := 0
	for _, e := range f.events {
		if e.Kind == EventHeld {
			held++
		}
	}
	if held != 1 {
		t.Fatalf("a repeated hold should be one history entry, got %d", held)
	}
}

func TestTickHoldsOnUnknownMetrics(t *testing.T) {
	cases := []struct {
		name string
		sig  *fakeSignals
	}{
		{"stale", &fakeSignals{reading: Reading{Note: "the newest sample is 10m old"}, headroom: -1}},
		{"query error", &fakeSignals{err: errors.New("db locked"), headroom: -1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, now := newFakeStore(testPolicy(), 4), t0
			c, _ := newController(f, tc.sig, &now)
			if err := c.Tick(context.Background()); err != nil {
				t.Fatal(err)
			}
			st := f.states["web"]
			if f.replicas["web"] != 4 || f.setCalls != 0 || st.Reason != ReasonMetricsUnavailable || st.BlockedBy != BlockedNoMetrics || st.ValueKnown {
				t.Fatalf("replicas = %d writes = %d state = %+v", f.replicas["web"], f.setCalls, st)
			}
			if st.Message == "" {
				t.Fatal("the hold must say why")
			}
		})
	}
}

func TestTickSpendCapWinsOverScaleUp(t *testing.T) {
	cases := []struct {
		name string
		sig  *fakeSignals
	}{
		{"paused", &fakeSignals{reading: Reading{Value: 90, Known: true}, spend: "pause", headroom: -1}},
		{"throttled", &fakeSignals{reading: Reading{Value: 90, Known: true}, spend: "throttle", headroom: -1}},
		{"state unreadable", &fakeSignals{reading: Reading{Value: 90, Known: true}, spendErr: errors.New("boom"), headroom: -1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, now := newFakeStore(testPolicy(), 4), t0
			c, _ := newController(f, tc.sig, &now)
			if err := c.Tick(context.Background()); err != nil {
				t.Fatal(err)
			}
			if f.replicas["web"] != 4 || f.states["web"].Reason != ReasonSpendCapPaused {
				t.Fatalf("replicas = %d state = %+v", f.replicas["web"], f.states["web"])
			}
		})
	}
}

func TestTickSpendCapDoesNotStopScaleDown(t *testing.T) {
	f, now := newFakeStore(testPolicy(), 6), t0
	f.states["web"] = State{App: "web", DownSince: t0.Add(-time.Hour), DownFloor: 2, Reason: ReasonStabilizing}
	sig := &fakeSignals{reading: Reading{Value: 10, Known: true}, spend: "pause", headroom: -1}
	c, _ := newController(f, sig, &now)
	if err := c.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.replicas["web"] != 5 {
		t.Fatalf("replicas = %d, want 5", f.replicas["web"])
	}
}

func TestTickNoCapacityIsRecordedAndAlertable(t *testing.T) {
	f, now := newFakeStore(testPolicy(), 4), t0
	sig := hot()
	sig.headroom = 0
	c, _ := newController(f, sig, &now)
	rec := &fakeRecorder{}
	c.Recorder = rec
	if err := c.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	st := f.states["web"]
	if f.replicas["web"] != 4 || st.Reason != ReasonNoCapacity || st.BlockedBy != BlockedNoCapacity {
		t.Fatalf("replicas = %d state = %+v", f.replicas["web"], st)
	}
	if len(rec.got) != 1 || !rec.got[0].NoCapacity {
		t.Fatalf("the no capacity gauge must be set: %+v", rec.got)
	}
	status, err := c.Status(context.Background(), "web")
	if err != nil || status.Condition.Healthy || status.Condition.Reason != ReasonNoCapacity {
		t.Fatalf("status = %+v err = %v", status.Condition, err)
	}
}

func TestTickHalfSucceeded(t *testing.T) {
	t.Run("replica write fails: cooldown is given back", func(t *testing.T) {
		f, now := newFakeStore(testPolicy(), 4), t0
		f.failSetReplicas = errors.New("disk full")
		c, nudges := newController(f, hot(), &now)
		if err := c.Tick(context.Background()); err == nil {
			t.Fatal("expected the write failure to surface")
		}
		st := f.states["web"]
		if !st.LastScaleUp.IsZero() || st.Reason != ReasonScaleFailed || st.Desired != 4 || *nudges != 0 {
			t.Fatalf("state = %+v nudges = %d", st, *nudges)
		}
		last := f.events[len(f.events)-1]
		if last.Kind != EventFailed {
			t.Fatalf("history should end with a failure, got %+v", last)
		}
		f.failSetReplicas = nil
		now = t0.Add(time.Second)
		if err := c.Tick(context.Background()); err != nil {
			t.Fatal(err)
		}
		if f.replicas["web"] != 6 {
			t.Fatalf("the retry should scale at once, replicas = %d", f.replicas["web"])
		}
	})
	t.Run("state save fails: nothing is scaled", func(t *testing.T) {
		f, now := newFakeStore(testPolicy(), 4), t0
		f.failSaveState = errors.New("readonly")
		c, _ := newController(f, hot(), &now)
		if err := c.Tick(context.Background()); err == nil {
			t.Fatal("expected the save failure to surface")
		}
		if f.setCalls != 0 || f.replicas["web"] != 4 {
			t.Fatalf("replicas must not change when intent was not recorded: %d writes", f.setCalls)
		}
	})
}

func TestTickSkipsSuspendedAndMissingApps(t *testing.T) {
	f, now := newFakeStore(testPolicy(), 4), t0
	f.suspended["web"] = true
	c, _ := newController(f, hot(), &now)
	if err := c.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.replicas["web"] != 4 || f.states["web"].Reason != ReasonSuspended {
		t.Fatalf("replicas = %d state = %+v", f.replicas["web"], f.states["web"])
	}
	delete(f.replicas, "web")
	if err := c.Tick(context.Background()); err != nil {
		t.Fatalf("an app that vanished must not fail the tick: %v", err)
	}
}

func TestTickCorrectsBoundsAfterAPolicyChange(t *testing.T) {
	f, now := newFakeStore(testPolicy(), 14), t0
	sig := &fakeSignals{reading: Reading{Value: 60, Known: true}, headroom: -1}
	c, _ := newController(f, sig, &now)
	if err := c.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.replicas["web"] != 10 || f.states["web"].Reason != ReasonBoundsCorrected {
		t.Fatalf("replicas = %d state = %+v", f.replicas["web"], f.states["web"])
	}
}

func TestOneFailingAppDoesNotStopTheOthers(t *testing.T) {
	f, now := newFakeStore(testPolicy(), 4), t0
	other := testPolicy()
	other.App = "api"
	f.policies["api"] = other
	f.replicas["api"] = 4
	c, _ := newController(f, hot(), &now)
	f.failSaveState = errors.New("boom")
	if err := c.Tick(context.Background()); err == nil {
		t.Fatal("expected errors")
	}
	f.failSaveState = nil
	if err := c.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.replicas["web"] != 6 || f.replicas["api"] != 6 {
		t.Fatalf("replicas = %v", f.replicas)
	}
}

func TestStatusBeforeFirstEvaluationAndFlapping(t *testing.T) {
	f, now := newFakeStore(testPolicy(), 3), t0
	c, _ := newController(f, hot(), &now)
	st, err := c.Status(context.Background(), "web")
	if err != nil || st.Condition.Reason != ReasonPending || st.Current != 3 {
		t.Fatalf("status = %+v err = %v", st, err)
	}
	if _, err := c.Status(context.Background(), "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	f.states["web"] = State{App: "web", Current: 3, Desired: 3, Reason: ReasonWithinTarget, Message: "ok", UpdatedAt: t0}
	kinds := []string{EventScaledUp, EventScaledDown, EventScaledUp, EventScaledDown}
	for i, k := range kinds {
		f.events = append(f.events, Event{App: "web", Kind: k, At: t0.Add(-time.Duration(len(kinds)-i) * 5 * time.Minute)})
	}
	st, err = c.Status(context.Background(), "web")
	if err != nil || !st.Flapping || st.Flaps1h != 3 || st.Condition.Reason != ReasonFlapping || st.Condition.Healthy {
		t.Fatalf("status = %+v err = %v", st, err)
	}
}

func TestFlaps(t *testing.T) {
	at := func(m int) time.Time { return t0.Add(-time.Duration(m) * time.Minute) }
	tests := []struct {
		name   string
		events []Event
		want   int
	}{
		{"none", nil, 0},
		{"one direction", []Event{{Kind: EventScaledUp, At: at(5)}, {Kind: EventScaledUp, At: at(2)}}, 0},
		{"one reversal, newest first", []Event{{Kind: EventScaledDown, At: at(2)}, {Kind: EventScaledUp, At: at(5)}}, 1},
		{"holds do not count", []Event{{Kind: EventHeld, At: at(3)}, {Kind: EventScaledUp, At: at(5)}, {Kind: EventScaledDown, At: at(1)}}, 1},
		{"old events fall out of the window", []Event{{Kind: EventScaledUp, At: at(200)}, {Kind: EventScaledDown, At: at(1)}}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Flaps(tt.events, t0, time.Hour); got != tt.want {
				t.Fatalf("Flaps = %d, want %d", got, tt.want)
			}
		})
	}
}

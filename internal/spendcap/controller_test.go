package spendcap

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeStore struct {
	caps     map[string]Cap
	states   map[string]State
	events   []Event
	failSave int
}

func newFakeStore(caps ...Cap) *fakeStore {
	f := &fakeStore{caps: map[string]Cap{}, states: map[string]State{}}
	for _, c := range caps {
		f.caps[c.App] = c.Normalize()
	}
	return f
}

func (f *fakeStore) ListSpendCaps(context.Context) ([]Cap, error) {
	var out []Cap
	for _, c := range f.caps {
		out = append(out, c)
	}
	return out, nil
}

func (f *fakeStore) GetSpendCap(_ context.Context, app string) (Cap, error) {
	c, ok := f.caps[app]
	if !ok {
		return c, ErrNotFound
	}
	return c, nil
}

func (f *fakeStore) SaveSpendCap(_ context.Context, c Cap, ev Event, _ time.Time) error {
	f.caps[c.App] = c
	f.events = append(f.events, ev)
	return nil
}

func (f *fakeStore) DeleteSpendCap(_ context.Context, app string, ev Event) error {
	delete(f.caps, app)
	delete(f.states, app)
	f.events = append(f.events, ev)
	return nil
}

func (f *fakeStore) GetSpendCapState(_ context.Context, app string) (State, bool, error) {
	s, ok := f.states[app]
	return s, ok, nil
}

func (f *fakeStore) SaveSpendCapState(_ context.Context, st State, events []Event) error {
	if f.failSave > 0 {
		f.failSave--
		return errors.New("disk full")
	}
	f.states[st.App] = st
	f.events = append(f.events, events...)
	return nil
}

func (f *fakeStore) ListSpendCapEvents(context.Context, string, int) ([]Event, error) {
	return f.events, nil
}

type fakeUsage struct {
	use map[string]Usage
	err error
}

func (u *fakeUsage) SpendCapUsage(context.Context, time.Time) (map[string]Usage, error) {
	return u.use, u.err
}

const gib = int64(1) << 30

func kinds(evs []Event) []string {
	var out []string
	for _, e := range evs {
		out = append(out, e.Kind)
	}
	return out
}

func newController(st *fakeStore, u *fakeUsage, now *time.Time, nudges *int, nudgeErr *error) *Controller {
	return &Controller{
		Store: st, Usage: u, Now: func() time.Time { return *now },
		Nudge: func(context.Context) error {
			*nudges++
			return *nudgeErr
		},
	}
}

func TestTickEnforcesAndReleases(t *testing.T) {
	st := newFakeStore(Cap{App: "web", MonthlyEgressBytes: 10 * gib, Action: ActionPause})
	u := &fakeUsage{use: map[string]Usage{"web": {EgressBytes: 6 * gib}}}
	now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	var nudges int
	var nudgeErr error
	c := newController(st, u, &now, &nudges, &nudgeErr)
	ctx := context.Background()

	if err := c.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if got := st.states["web"]; got.Enforced != "" || got.Highest != 50 || nudges != 0 {
		t.Fatalf("at 60%%: %+v nudges=%d", got, nudges)
	}

	u.use["web"] = Usage{EgressBytes: 11 * gib}
	if err := c.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if got := st.states["web"]; got.Enforced != ActionPause || got.Nudged != ActionPause || got.Highest != 100 || nudges != 1 {
		t.Fatalf("at 110%%: %+v nudges=%d", got, nudges)
	}

	if err := c.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if nudges != 1 {
		t.Errorf("level-triggered tick re-nudged an applied state: %d", nudges)
	}
	events := len(st.events)
	if err := c.Tick(ctx); err != nil || len(st.events) != events {
		t.Errorf("steady state wrote events: %v", kinds(st.events))
	}

	st.caps["web"] = Cap{App: "web", MonthlyEgressBytes: 100 * gib, Thresholds: []int{50, 80, 100}, Action: ActionPause}
	if err := c.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if got := st.states["web"]; got.Enforced != "" || got.Nudged != "" || nudges != 2 {
		t.Errorf("raising the limit did not release: %+v nudges=%d", got, nudges)
	}
	want := []string{EventThreshold, EventThreshold, EventEnforced, EventReleased}
	if got := kinds(st.events); len(got) != len(want) || got[2] != EventEnforced || got[3] != EventReleased {
		t.Errorf("history = %v", got)
	}
}

func TestTickNudgeFailureRetriesNextTick(t *testing.T) {
	st := newFakeStore(Cap{App: "web", MonthlyEgressBytes: gib, Action: ActionThrottle})
	u := &fakeUsage{use: map[string]Usage{"web": {EgressBytes: 2 * gib}}}
	now := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	var nudges int
	nudgeErr := errors.New("ingress busy")
	c := newController(st, u, &now, &nudges, &nudgeErr)
	ctx := context.Background()

	if err := c.Tick(ctx); err == nil {
		t.Fatal("nudge failure not reported")
	}
	got := st.states["web"]
	if got.Enforced != ActionThrottle || got.Nudged != "" {
		t.Fatalf("half-succeeded state = %+v", got)
	}
	if cond := Conditions(st.caps["web"], Evaluate(st.caps["web"], u.use["web"]), got.Enforced, got.Nudged); cond.Reason != ReasonEnforcementPending {
		t.Errorf("condition = %+v", cond)
	}

	nudgeErr = nil
	if err := c.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if got := st.states["web"]; got.Nudged != ActionThrottle || nudges != 2 {
		t.Errorf("retry did not converge: %+v nudges=%d", got, nudges)
	}
	enforced := 0
	for _, e := range st.events {
		if e.Kind == EventEnforced {
			enforced++
		}
	}
	if enforced != 1 {
		t.Errorf("enforced event recorded %d times", enforced)
	}
}

func TestTickSaveFailureLeavesNoPartialState(t *testing.T) {
	st := newFakeStore(Cap{App: "web", MonthlyEgressBytes: gib, Action: ActionPause})
	st.failSave = 1
	u := &fakeUsage{use: map[string]Usage{"web": {EgressBytes: 2 * gib}}}
	now := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	var nudges int
	var nudgeErr error
	c := newController(st, u, &now, &nudges, &nudgeErr)
	if err := c.Tick(context.Background()); err == nil {
		t.Fatal("save failure not reported")
	}
	if _, ok := st.states["web"]; ok || nudges != 0 {
		t.Fatalf("state=%v nudges=%d", st.states, nudges)
	}
	if err := c.Tick(context.Background()); err != nil || st.states["web"].Enforced != ActionPause {
		t.Fatalf("did not recover: %v %+v", err, st.states["web"])
	}
}

func TestTickUsageFailureKeepsEnforcement(t *testing.T) {
	st := newFakeStore(Cap{App: "web", MonthlyEgressBytes: gib, Action: ActionPause})
	now := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	st.states["web"] = State{App: "web", Period: "2026-10", Enforced: ActionPause, Nudged: ActionPause, Highest: 100}
	u := &fakeUsage{err: errors.New("telemetry down")}
	var nudges int
	var nudgeErr error
	c := newController(st, u, &now, &nudges, &nudgeErr)
	if err := c.Tick(context.Background()); err == nil {
		t.Fatal("usage failure not reported")
	}
	if got := st.states["web"]; got.Enforced != ActionPause || nudges != 0 {
		t.Errorf("enforcement changed on a usage outage: %+v", got)
	}
}

func TestTickMonthRollover(t *testing.T) {
	st := newFakeStore(Cap{App: "web", MonthlyEgressBytes: gib, Action: ActionPause})
	u := &fakeUsage{use: map[string]Usage{"web": {EgressBytes: 3 * gib}}}
	now := time.Date(2026, 10, 31, 23, 0, 0, 0, time.UTC)
	var nudges int
	var nudgeErr error
	c := newController(st, u, &now, &nudges, &nudgeErr)
	ctx := context.Background()
	if err := c.Tick(ctx); err != nil || st.states["web"].Enforced != ActionPause {
		t.Fatalf("october: %v %+v", err, st.states["web"])
	}

	now = time.Date(2026, 11, 1, 0, 5, 0, 0, time.UTC)
	u.use = map[string]Usage{"web": {EgressBytes: gib / 10}}
	if err := c.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	got := st.states["web"]
	if got.Period != "2026-11" || got.Enforced != "" || got.Nudged != "" || got.Highest != 0 {
		t.Fatalf("november state = %+v", got)
	}
	if nudges != 2 {
		t.Errorf("rollover must tell ingress the pause is gone, nudges=%d", nudges)
	}
	roll := false
	for _, e := range st.events {
		roll = roll || e.Kind == EventRollover && e.Period == "2026-11"
	}
	if !roll {
		t.Errorf("no rollover event in %v", kinds(st.events))
	}
}

func TestStatusAndBreaches(t *testing.T) {
	st := newFakeStore(Cap{App: "web", MonthlyEgressBytes: 10 * gib, Action: ActionNone})
	u := &fakeUsage{use: map[string]Usage{"web": {EgressBytes: 9 * gib}}}
	now := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	var nudges int
	var nudgeErr error
	c := newController(st, u, &now, &nudges, &nudgeErr)
	ctx := context.Background()
	if _, err := c.Status(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing app = %v", err)
	}
	if err := c.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	s, err := c.Status(ctx, "web")
	if err != nil || s.Pct != 90 || s.Enforced != ActionNone || s.Condition.Reason != ReasonThresholdReached {
		t.Fatalf("status = %+v err=%v", s, err)
	}
	if !s.ResetsAt.Equal(time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("resets_at = %v", s.ResetsAt)
	}
	b, err := c.Breaches(ctx)
	if err != nil || len(b) != 1 || b[0].Key != "spendcap:web:2026-10:80" {
		t.Fatalf("breaches = %+v err=%v", b, err)
	}

	u.use["web"] = Usage{EgressBytes: 10 * gib}
	if err := c.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	b, _ = c.Breaches(ctx)
	if len(b) != 1 || b[0].Key != "spendcap:web:2026-10:100" {
		t.Errorf("new threshold must change the key: %+v", b)
	}
	now = time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
	if b, _ = c.Breaches(ctx); len(b) != 0 {
		t.Errorf("last month's breach leaked into november: %+v", b)
	}
}

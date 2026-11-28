package spendcap

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeSleeper struct{ paused, resumed []string }

func (f *fakeSleeper) Pause(_ context.Context, app string) error {
	f.paused = append(f.paused, app)
	return nil
}

func (f *fakeSleeper) Resume(_ context.Context, app string) error {
	f.resumed = append(f.resumed, app)
	return nil
}

func TestCapPauseStopsAndResets(t *testing.T) {
	cases := []struct {
		name      string
		stopApps  bool
		wantPause int
	}{
		{"setting on stops the app", true, 1},
		{"setting off keeps the maintenance page only", false, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := newFakeStore(Cap{App: "web", MonthlyEgressBytes: 10 * gib, Action: ActionPause})
			u := &fakeUsage{use: map[string]Usage{"web": {EgressBytes: 11 * gib}}}
			now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
			var nudges int
			var nudgeErr error
			sl := &fakeSleeper{}
			c := newController(st, u, &now, &nudges, &nudgeErr)
			c.Sleeper, c.StopApps = sl, func(context.Context) bool { return tc.stopApps }
			ctx := context.Background()
			for i := 0; i < 2; i++ {
				if err := c.Tick(ctx); err != nil {
					t.Fatal(err)
				}
			}
			if len(sl.paused) != tc.wantPause {
				t.Fatalf("pauses = %v, want %d", sl.paused, tc.wantPause)
			}
			if tc.wantPause == 0 {
				return
			}
			if st.states["web"].StopState != StopPaused {
				t.Fatalf("stop state = %q", st.states["web"].StopState)
			}
			now = time.Date(2026, 11, 1, 0, 5, 0, 0, time.UTC)
			u.use["web"] = Usage{}
			if err := c.Tick(ctx); err != nil {
				t.Fatal(err)
			}
			if len(sl.resumed) != 1 || st.states["web"].StopState != "" {
				t.Fatalf("after reset: resumed=%v state=%+v", sl.resumed, st.states["web"])
			}
		})
	}
}

func TestManualResumeIsNotRepaused(t *testing.T) {
	st := newFakeStore(Cap{App: "web", MonthlyEgressBytes: 10 * gib, Action: ActionPause})
	u := &fakeUsage{use: map[string]Usage{"web": {EgressBytes: 11 * gib}}}
	now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	var nudges int
	var nudgeErr error
	sl := &fakeSleeper{}
	c := newController(st, u, &now, &nudges, &nudgeErr)
	c.Sleeper, c.StopApps = sl, func(context.Context) bool { return true }
	ctx := context.Background()
	if err := c.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if err := c.ResumeNow(ctx, "web", "root", "launch day"); err != nil {
		t.Fatal(err)
	}
	if err := c.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if len(sl.paused) != 1 || st.states["web"].StopState != StopResumed {
		t.Fatalf("paused=%v state=%+v", sl.paused, st.states["web"])
	}
	if err := c.ResumeNow(ctx, "web", "root", "again"); !errors.Is(err, ErrNotPaused) {
		t.Fatalf("second resume = %v", err)
	}
}

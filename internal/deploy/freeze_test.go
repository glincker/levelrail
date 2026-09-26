package deploy

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestWindowActive(t *testing.T) {
	// Friday 17:00 Berlin for 64h: the classic weekend freeze.
	weekend := store.DeployFreezeWindow{ID: "w", Cron: "0 17 * * 5", Duration: 64 * time.Hour, Timezone: "Europe/Berlin"}
	berlin, _ := time.LoadLocation("Europe/Berlin")
	tests := []struct {
		name    string
		w       store.DeployFreezeWindow
		now     time.Time
		want    bool
		wantEnd time.Time
	}{
		{name: "before the window", w: weekend, now: time.Date(2026, 9, 25, 16, 59, 0, 0, berlin)},
		{name: "at the start", w: weekend, now: time.Date(2026, 9, 25, 17, 0, 0, 0, berlin), want: true, wantEnd: time.Date(2026, 9, 28, 9, 0, 0, 0, berlin)},
		{name: "sunday inside", w: weekend, now: time.Date(2026, 9, 27, 12, 0, 0, 0, berlin), want: true, wantEnd: time.Date(2026, 9, 28, 9, 0, 0, 0, berlin)},
		{name: "exactly at the end is released", w: weekend, now: time.Date(2026, 9, 28, 9, 0, 0, 0, berlin)},
		{name: "same instant in UTC is still inside", w: weekend, now: time.Date(2026, 9, 25, 15, 30, 0, 0, time.UTC), want: true, wantEnd: time.Date(2026, 9, 28, 9, 0, 0, 0, berlin)},
		{name: "utc default", w: store.DeployFreezeWindow{Cron: "0 0 * * *", Duration: time.Hour}, now: time.Date(2026, 1, 1, 0, 30, 0, 0, time.UTC), want: true, wantEnd: time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, end, err := WindowActive(tt.w, tt.now)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want || (tt.want && !end.Equal(tt.wantEnd)) {
				t.Fatalf("active=%v end=%v, want %v %v", got, end, tt.want, tt.wantEnd)
			}
		})
	}
}

func TestValidateFreezeWindow(t *testing.T) {
	ok := store.DeployFreezeWindow{Cron: "0 17 * * 5", Duration: time.Hour, Timezone: "UTC"}
	if err := ValidateFreezeWindow(ok); err != nil {
		t.Fatal(err)
	}
	bad := []store.DeployFreezeWindow{
		{Cron: "not cron", Duration: time.Hour},
		{Cron: "0 * * * *", Duration: 0},
		{Cron: "0 * * * *", Duration: 40 * 24 * time.Hour},
		{Cron: "0 * * * *", Duration: time.Hour, Timezone: "Mars/Olympus"},
	}
	for _, w := range bad {
		if ValidateFreezeWindow(w) == nil {
			t.Errorf("window %+v should be rejected", w)
		}
	}
}

func TestActiveFreezePicksLatestEnd(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 30, 0, 0, time.UTC)
	st := ActiveFreeze([]store.DeployFreezeWindow{
		{ID: "a", Cron: "0 0 * * *", Duration: time.Hour, Reason: "short"},
		{ID: "b", Cron: "0 0 * * *", Duration: 3 * time.Hour, Reason: "long"},
		{ID: "c", Cron: "0 12 * * *", Duration: time.Hour},
	}, now)
	if !st.Frozen || st.WindowID != "b" || st.Reason != "long" {
		t.Fatalf("status = %+v", st)
	}
	if err := (&FrozenError{Status: st}); !errors.Is(err, ErrFrozen) {
		t.Fatal("FrozenError must match ErrFrozen")
	}
}

type fakeFreezeStore struct{ windows []store.DeployFreezeWindow }

func (f fakeFreezeStore) ListDeployFreezeWindows(_ context.Context, scopes ...string) ([]store.DeployFreezeWindow, error) {
	var out []store.DeployFreezeWindow
	for _, w := range f.windows {
		for _, s := range scopes {
			if w.Scope == s {
				out = append(out, w)
			}
		}
	}
	return out, nil
}

type fakeHeldStore struct {
	attempts map[string]*store.DeployAttempt
	order    []string
}

func newFakeHeldStore() *fakeHeldStore {
	return &fakeHeldStore{attempts: map[string]*store.DeployAttempt{}}
}

func (f *fakeHeldStore) SaveDeployAttempt(_ context.Context, a store.DeployAttempt) error {
	f.attempts[a.ID] = &a
	f.order = append(f.order, a.ID)
	return nil
}

func (f *fakeHeldStore) ListHeldDeployAttempts(context.Context) ([]store.DeployAttempt, error) {
	var out []store.DeployAttempt
	for _, id := range f.order {
		if a := f.attempts[id]; a.Status == store.DeployAttemptStatusHeld {
			out = append(out, *a)
		}
	}
	return out, nil
}

func (f *fakeHeldStore) TransitionDeployAttempt(_ context.Context, id, from, to, reason string) (bool, error) {
	a := f.attempts[id]
	if a == nil || a.Status != from {
		return false, nil
	}
	a.Status, a.Reason = to, reason
	return true, nil
}

func TestReleaserHoldsThenReleasesNewestOnly(t *testing.T) {
	ctx := context.Background()
	hs := newFakeHeldStore()
	fs := fakeFreezeStore{windows: []store.DeployFreezeWindow{{ID: "w", Scope: store.DeployFreezeScopeApp("web"), Cron: "0 0 * * *", Duration: time.Hour}}}
	inside := time.Date(2026, 1, 1, 0, 30, 0, 0, time.UTC)
	status, err := CheckFreeze(ctx, fs, "web", inside)
	if err != nil || !status.Frozen {
		t.Fatalf("status = %+v, %v", status, err)
	}
	older, _ := HoldDeploy(ctx, hs, "web", "web:a", store.DeployAttemptSourceWebhook, HeldRequest{Kind: HeldKindGit, CommitSHA: "a"}, status)
	newer, _ := HoldDeploy(ctx, hs, "web", "web:b", store.DeployAttemptSourceWebhook, HeldRequest{Kind: HeldKindGit, CommitSHA: "b"}, status)

	var replayed []string
	now := inside
	r := Releaser{Store: hs, Freeze: fs, Now: func() time.Time { return now }, Handlers: map[string]ReleaseHandler{
		HeldKindGit: func(_ context.Context, _ store.DeployAttempt, req HeldRequest) error {
			replayed = append(replayed, req.CommitSHA)
			return nil
		},
	}}
	if n, err := r.ReleaseDue(ctx); err != nil || n != 0 {
		t.Fatalf("inside the window released %d, %v", n, err)
	}
	if hs.attempts[older].Status != store.DeployAttemptStatusSuperseded || hs.attempts[newer].Status != store.DeployAttemptStatusHeld {
		t.Fatalf("older=%s newer=%s", hs.attempts[older].Status, hs.attempts[newer].Status)
	}
	now = inside.Add(time.Hour)
	if n, err := r.ReleaseDue(ctx); err != nil || n != 1 {
		t.Fatalf("after the window released %d, %v", n, err)
	}
	if len(replayed) != 1 || replayed[0] != "b" || hs.attempts[newer].Status != store.DeployAttemptStatusSucceeded {
		t.Fatalf("replayed %v, newer status %s", replayed, hs.attempts[newer].Status)
	}
	if n, _ := r.ReleaseDue(ctx); n != 0 || len(replayed) != 1 {
		t.Fatal("a released deploy must never run twice")
	}
}

// Half-succeeded: the replay itself fails after the attempt was claimed.
func TestReleaserRecordsFailedReplay(t *testing.T) {
	ctx := context.Background()
	hs := newFakeHeldStore()
	id, _ := HoldDeploy(ctx, hs, "web", "nginx:1", store.DeployAttemptSourceImage, HeldRequest{Kind: HeldKindImage, Image: "nginx:1"}, FreezeStatus{})
	r := Releaser{Store: hs, Freeze: fakeFreezeStore{}, Handlers: map[string]ReleaseHandler{
		HeldKindImage: func(context.Context, store.DeployAttempt, HeldRequest) error { return errors.New("registry down") },
	}}
	if _, err := r.ReleaseDue(ctx); err != nil {
		t.Fatal(err)
	}
	if a := hs.attempts[id]; a.Status != store.DeployAttemptStatusFailed || a.Reason != "Released: registry down" {
		t.Fatalf("attempt = %+v", a)
	}
}

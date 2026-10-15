package maintenance

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/store"
)

var t0 = time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)

func TestInputValidate(t *testing.T) {
	ok := Input{Title: "DB move", Scope: store.MaintenanceScopeAll, StartsAt: t0.Add(time.Hour), EndsAt: t0.Add(2 * time.Hour)}
	tests := []struct {
		name    string
		mutate  func(*Input)
		wantErr string
	}{
		{"valid", func(*Input) {}, ""},
		{"no title", func(i *Input) { i.Title = " " }, "title is required"},
		{"end before start", func(i *Input) { i.EndsAt = i.StartsAt }, "after starts_at"},
		{"past", func(i *Input) { i.StartsAt, i.EndsAt = t0.Add(-2*time.Hour), t0.Add(-time.Hour) }, "past"},
		{"too long", func(i *Input) { i.EndsAt = i.StartsAt.Add(31 * 24 * time.Hour) }, "at most"},
		{"app needs target", func(i *Input) { i.Scope = store.MaintenanceScopeApp }, "target is required"},
		{"bad scope", func(i *Input) { i.Scope = "node" }, "scope must be"},
		{"bad lead", func(i *Input) { i.AnnounceLeadMins = -1 }, "announce_lead_minutes"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := ok
			tt.mutate(&in)
			_, err := in.Validate(t0)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestStateAtBoundaries(t *testing.T) {
	w := store.MaintenanceWindow{StartsAt: t0, EndsAt: t0.Add(time.Hour)}
	tests := []struct {
		at   time.Time
		want string
	}{
		{t0.Add(-time.Second), StateScheduled},
		{t0, StateActive},
		{t0.Add(time.Hour - time.Second), StateActive},
		{t0.Add(time.Hour), StateEnded},
	}
	for _, tt := range tests {
		if got := StateAt(w, tt.at); got != tt.want {
			t.Errorf("StateAt(%s) = %s, want %s", tt.at, got, tt.want)
		}
	}
}

func TestAnnounceDue(t *testing.T) {
	announced := t0
	base := store.MaintenanceWindow{StartsAt: t0.Add(2 * time.Hour), EndsAt: t0.Add(3 * time.Hour), AnnounceLeadMins: 60}
	tests := []struct {
		name string
		w    store.MaintenanceWindow
		now  time.Time
		want bool
	}{
		{"before lead", base, t0, false},
		{"at lead", base, t0.Add(time.Hour), true},
		{"during window", base, t0.Add(2*time.Hour + time.Minute), true},
		{"after end", base, t0.Add(3 * time.Hour), false},
		{"already announced", func() store.MaintenanceWindow { w := base; w.AnnouncedAt = &announced; return w }(), t0.Add(time.Hour), false},
		{"no lead", func() store.MaintenanceWindow { w := base; w.AnnounceLeadMins = 0; return w }(), t0.Add(time.Hour), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AnnounceDue(tt.w, tt.now); got != tt.want {
				t.Fatalf("AnnounceDue = %v, want %v", got, tt.want)
			}
		})
	}
}

type fakeStore struct {
	windows []store.MaintenanceWindow
	marked  []string
}

func (f *fakeStore) ListMaintenanceWindows(context.Context, time.Time) ([]store.MaintenanceWindow, error) {
	return f.windows, nil
}

func (f *fakeStore) MarkMaintenanceAnnounced(_ context.Context, id string, _ time.Time) error {
	f.marked = append(f.marked, id)
	return nil
}

type fakeChannels []alerting.NotificationChannel

func (f fakeChannels) ListNotificationChannels(context.Context) ([]alerting.NotificationChannel, error) {
	return f, nil
}

type fakeNotifier struct {
	sent []string
	fail bool
}

func (f *fakeNotifier) SendNotice(_ context.Context, c alerting.NotificationChannel, _ string) error {
	if f.fail {
		return context.DeadlineExceeded
	}
	f.sent = append(f.sent, c.ID)
	return nil
}

func TestAnnouncerTickSendsOnceAndRetriesOnFailure(t *testing.T) {
	win := store.MaintenanceWindow{ID: "mw_1", Title: "x", StartsAt: t0.Add(30 * time.Minute),
		EndsAt: t0.Add(time.Hour), AnnounceLeadMins: 60}
	st := &fakeStore{windows: []store.MaintenanceWindow{win}}
	n := &fakeNotifier{fail: true}
	a := &Announcer{Store: st, Notifier: n, Now: func() time.Time { return t0 },
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Channels: fakeChannels{{ID: "c1", Enabled: true}, {ID: "c2", Enabled: false}}}

	if err := a.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(st.marked) != 0 {
		t.Fatalf("marked %v after a failed send, want none", st.marked)
	}
	n.fail = false
	if err := a.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(st.marked) != 1 || len(n.sent) != 1 || n.sent[0] != "c1" {
		t.Fatalf("marked=%v sent=%v, want one notice to enabled channel c1", st.marked, n.sent)
	}
}

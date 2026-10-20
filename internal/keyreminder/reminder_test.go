package keyreminder

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeSource struct {
	policy   store.SecretKeyPolicy
	statuses []store.ServiceDEKStatus
	master   time.Time
	marked   []time.Time
}

func (f *fakeSource) GetSecretKeyPolicy(context.Context) (store.SecretKeyPolicy, error) {
	return f.policy, nil
}

func (f *fakeSource) MarkSecretKeyReminded(_ context.Context, at time.Time) error {
	f.marked = append(f.marked, at)
	f.policy.LastRemindedAt = at
	return nil
}

func (f *fakeSource) ListServiceDEKStatus(context.Context) ([]store.ServiceDEKStatus, error) {
	return f.statuses, nil
}

func (f *fakeSource) GetMasterKeyRotatedAt(context.Context) (time.Time, bool, error) {
	return f.master, !f.master.IsZero(), nil
}

type fakeChannels []alerting.NotificationChannel

func (f fakeChannels) ListNotificationChannels(context.Context) ([]alerting.NotificationChannel, error) {
	return f, nil
}

type fakeNotifier struct {
	texts []string
	err   error
}

func (f *fakeNotifier) SendNotice(_ context.Context, _ alerting.NotificationChannel, text string) error {
	if f.err != nil {
		return f.err
	}
	f.texts = append(f.texts, text)
	return nil
}

func TestTick(t *testing.T) {
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	old := now.Add(-100 * 24 * time.Hour)
	fresh := now.Add(-2 * 24 * time.Hour)
	enabled := fakeChannels{{ID: "c1", Enabled: true}}
	on := store.SecretKeyPolicy{MaxKeyAgeDays: 90, RemindEveryDays: 7}

	tests := []struct {
		name      string
		src       fakeSource
		channels  fakeChannels
		notifyErr error
		wantSent  int
		wantMark  bool
	}{
		{"policy off", fakeSource{policy: store.SecretKeyPolicy{RemindEveryDays: 7}, statuses: []store.ServiceDEKStatus{{ServiceName: "web", CreatedAt: old}}}, enabled, nil, 0, false},
		{"all keys fresh", fakeSource{policy: on, statuses: []store.ServiceDEKStatus{{ServiceName: "web", CreatedAt: old, RotatedAt: fresh}}, master: fresh}, enabled, nil, 0, false},
		{"overdue app", fakeSource{policy: on, statuses: []store.ServiceDEKStatus{{ServiceName: "web", CreatedAt: old}}}, enabled, nil, 1, true},
		{"overdue master only", fakeSource{policy: on, master: old}, enabled, nil, 1, true},
		{"reminded recently", fakeSource{policy: store.SecretKeyPolicy{MaxKeyAgeDays: 90, RemindEveryDays: 7, LastRemindedAt: now.Add(-24 * time.Hour)}, master: old}, enabled, nil, 0, false},
		{"repeat due", fakeSource{policy: store.SecretKeyPolicy{MaxKeyAgeDays: 90, RemindEveryDays: 7, LastRemindedAt: now.Add(-8 * 24 * time.Hour)}, master: old}, enabled, nil, 1, true},
		{"no channels stays unmarked", fakeSource{policy: on, master: old}, nil, nil, 0, false},
		{"send failure stays unmarked", fakeSource{policy: on, master: old}, enabled, errors.New("down"), 0, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			n := &fakeNotifier{err: tc.notifyErr}
			src := tc.src
			r := &Reminder{Source: &src, Channels: tc.channels, Notifier: n, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Now: func() time.Time { return now }}
			if err := r.Tick(context.Background()); err != nil {
				t.Fatal(err)
			}
			if len(n.texts) != tc.wantSent || (len(src.marked) == 1) != tc.wantMark {
				t.Fatalf("sent %d marked %d, want sent %d marked %v", len(n.texts), len(src.marked), tc.wantSent, tc.wantMark)
			}
		})
	}
}

func TestNoticeTextNamesAppsButCapsThem(t *testing.T) {
	text := NoticeText(90, true, []string{"a", "b", "c", "d", "e", "f", "g"})
	for _, want := range []string{"the master key", "7 app data key(s)", "and 2 more", "90 day"} {
		if !strings.Contains(text, want) {
			t.Errorf("notice %q missing %q", text, want)
		}
	}
}

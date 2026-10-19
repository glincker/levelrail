package alerting

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type fakeDigestSender struct{ texts []string }

func (f *fakeDigestSender) SendNotice(_ context.Context, _ NotificationChannel, text string) error {
	f.texts = append(f.texts, text)
	return nil
}

func seedHistory(t *testing.T, db *DB, at time.Time, app, sev, event, outcome string) string {
	t.Helper()
	id, err := NewHistoryID()
	if err != nil {
		t.Fatal(err)
	}
	if err := db.RecordHistory(context.Background(), HistoryEntry{ID: id, At: at, RuleID: "r1", RuleName: "cpu", App: app, Severity: sev, Event: event, Outcome: outcome}); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestInboxDefaultsAreNoiseFree(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	now := time.Now()
	sentWarn := seedHistory(t, db, now.Add(-1*time.Minute), "web", SeverityWarning, EventFired, OutcomeSent)
	seedHistory(t, db, now.Add(-2*time.Minute), "web", SeverityInfo, EventFired, OutcomeSent)
	seedHistory(t, db, now.Add(-3*time.Minute), "web", SeverityCritical, EventFired, OutcomeSilenced)
	seedHistory(t, db, now.Add(-4*time.Minute), "web", SeverityCritical, EventFired, OutcomeRateLimited)

	items, err := db.ListInbox(ctx, "user:a", InboxFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != sentWarn || items[0].Read {
		t.Fatalf("items = %+v, want only the sent warning, unread", items)
	}
}

func TestInboxReadStateIsPerPrincipal(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	now := time.Now()
	a := seedHistory(t, db, now.Add(-2*time.Minute), "web", SeverityCritical, EventFired, OutcomeSent)
	b := seedHistory(t, db, now.Add(-1*time.Minute), "web", SeverityCritical, EventResolved, OutcomeSent)

	if err := db.MarkInboxRead(ctx, "user:a", []string{a}); err != nil {
		t.Fatal(err)
	}
	if n, _ := db.InboxUnreadCount(ctx, "user:a"); n != 1 {
		t.Fatalf("user:a unread = %d, want 1", n)
	}
	if n, _ := db.InboxUnreadCount(ctx, "user:b"); n != 2 {
		t.Fatalf("user:b unread = %d, want 2", n)
	}
	if err := db.MarkAllInboxRead(ctx, "user:a", now); err != nil {
		t.Fatal(err)
	}
	if n, _ := db.InboxUnreadCount(ctx, "user:a"); n != 0 {
		t.Fatalf("after mark all, unread = %d, want 0", n)
	}
	later := seedHistory(t, db, now.Add(time.Minute), "web", SeverityCritical, EventFired, OutcomeSent)
	unread, err := db.ListInbox(ctx, "user:a", InboxFilter{UnreadOnly: true})
	if err != nil || len(unread) != 1 || unread[0].ID != later {
		t.Fatalf("unread after cursor = %+v, %v; want only %s (b=%s)", unread, err, later, b)
	}
	if err := db.MarkInboxRead(ctx, "user:a", nil); !errors.Is(err, ErrInboxInvalid) {
		t.Fatalf("empty ids error = %v, want ErrInboxInvalid", err)
	}
}

func TestInboxRoutingRules(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	now := time.Now()
	seedHistory(t, db, now.Add(-2*time.Minute), "noisy", SeverityCritical, EventFired, OutcomeSent)
	keep := seedHistory(t, db, now.Add(-1*time.Minute), "web", SeverityInfo, EventFired, OutcomeSent)

	p := DefaultInboxPrefs("user:a")
	p.MinSeverity = SeverityInfo
	p.MutedApps = []string{"noisy"}
	if err := db.SaveInboxPrefs(ctx, p, now); err != nil {
		t.Fatal(err)
	}
	items, err := db.ListInbox(ctx, "user:a", InboxFilter{})
	if err != nil || len(items) != 1 || items[0].ID != keep {
		t.Fatalf("items = %+v, %v; want only %s", items, err, keep)
	}
	got, _ := db.GetInboxPrefs(ctx, "user:a")
	if got.MinSeverity != SeverityInfo || len(got.MutedApps) != 1 || got.MutedApps[0] != "noisy" {
		t.Fatalf("prefs round trip = %+v", got)
	}
}

func TestInboxPrefsValidate(t *testing.T) {
	tests := []struct {
		name string
		mut  func(*InboxPrefs)
	}{
		{"bad severity", func(p *InboxPrefs) { p.MinSeverity = "loud" }},
		{"bad hour", func(p *InboxPrefs) { p.DigestHour = 24 }},
		{"digest without channel", func(p *InboxPrefs) { p.DigestEnabled = true }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := DefaultInboxPrefs("user:a")
			tc.mut(&p)
			if err := p.Validate(); !errors.Is(err, ErrInboxInvalid) {
				t.Fatalf("Validate() = %v, want ErrInboxInvalid", err)
			}
		})
	}
}

func TestDigestRunnerSendsOncePerDay(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC)
	if err := db.SaveNotificationChannel(ctx, NotificationChannel{ID: "chn_1", Name: "ops", Kind: NotifyGeneric, NotifyURL: "https://example.test/hook", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	seedHistory(t, db, now.Add(-time.Hour), "web", SeverityCritical, EventFired, OutcomeSent)
	p := DefaultInboxPrefs("user:a")
	p.DigestEnabled, p.DigestChannelID, p.DigestHour = true, "chn_1", 8
	if err := db.SaveInboxPrefs(ctx, p, now); err != nil {
		t.Fatal(err)
	}
	sender := &fakeDigestSender{}
	r := &DigestRunner{DB: db, Sender: sender, Now: func() time.Time { return now }}
	for i := 0; i < 2; i++ {
		if _, err := r.RunOnce(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if len(sender.texts) != 1 || !strings.Contains(sender.texts[0], "1 critical") {
		t.Fatalf("digests = %q, want exactly one mentioning 1 critical", sender.texts)
	}
}

func TestFormatDigestEmpty(t *testing.T) {
	if got := FormatDigest(nil); got != "" {
		t.Fatalf("FormatDigest(nil) = %q, want empty", got)
	}
}

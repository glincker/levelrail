package alerting

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDeleteInboxRejectsDerivedItems(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	now := time.Now()
	id := seedHistory(t, db, now.Add(-time.Minute), "web", SeverityCritical, EventFired, OutcomeSent)

	for _, ids := range [][]string{{"deploy:x"}, {id, "approval:y"}} {
		if err := db.DeleteInbox(ctx, "user:a", ids, false, now); !errors.Is(err, ErrInboxNotDeletable) {
			t.Fatalf("ids %v: err = %v, want ErrInboxNotDeletable", ids, err)
		}
	}
	if items, _ := db.ListInbox(ctx, "user:a", InboxFilter{}); len(items) != 1 {
		t.Fatalf("rejected delete still hid items: %d", len(items))
	}
	if err := db.DeleteInbox(ctx, "user:a", []string{id}, false, now); err != nil {
		t.Fatal(err)
	}
	if items, _ := db.ListInbox(ctx, "user:a", InboxFilter{}); len(items) != 0 {
		t.Fatalf("deleted item listed: %d", len(items))
	}
	if items, _ := db.ListInbox(ctx, "user:b", InboxFilter{}); len(items) != 1 {
		t.Fatalf("delete leaked to another principal: %d", len(items))
	}
	if err := db.DeleteInbox(ctx, "user:a", []string{id}, true, now); err != nil {
		t.Fatal(err)
	}
	if items, _ := db.ListInbox(ctx, "user:a", InboxFilter{}); len(items) != 1 {
		t.Fatalf("undo did not bring the item back: %d", len(items))
	}
}

func TestClearInboxScopes(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	cases := []struct {
		name, action, scope string
		older               time.Duration
		dry                 bool
		want                int
		wantErr             bool
	}{
		{name: "archive resolved", action: InboxActionArchive, scope: InboxScopeResolved, want: 3},
		{name: "delete resolved", action: InboxActionDelete, scope: InboxScopeResolved, want: 3},
		{name: "archive read", action: InboxActionArchive, scope: InboxScopeRead, want: 1},
		{name: "older than skips fresh", action: InboxActionArchive, scope: InboxScopeResolved, older: time.Hour, want: 2},
		{name: "dry run counts only", action: InboxActionDelete, scope: InboxScopeResolved, dry: true, want: 3},
		{name: "archived scope with archive", action: InboxActionArchive, scope: InboxScopeArchived, wantErr: true},
		{name: "unknown scope", action: InboxActionDelete, scope: "all", wantErr: true},
		{name: "unknown action", action: "purge", scope: InboxScopeRead, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := newTestDB(t)
			seedHistory(t, db, now.Add(-3*time.Hour), "web", SeverityCritical, EventFired, OutcomeSent)
			seedHistory(t, db, now.Add(-2*time.Hour), "web", SeverityCritical, EventResolved, OutcomeSent)
			apiID, err := NewHistoryID()
			if err != nil {
				t.Fatal(err)
			}
			if err := db.RecordHistory(ctx, HistoryEntry{ID: apiID, At: now.Add(-time.Minute), RuleID: "r2", RuleName: "cpu", App: "api", Severity: SeverityWarning, Event: EventResolved, Outcome: OutcomeSent}); err != nil {
				t.Fatal(err)
			}
			readID := seedHistory(t, db, now.Add(-30*time.Second), "db", SeverityWarning, EventFired, OutcomeSent)
			if err := db.MarkInboxRead(ctx, "user:a", []string{readID}); err != nil {
				t.Fatal(err)
			}
			n, err := db.ClearInbox(ctx, "user:a", tc.action, InboxSelector{Scope: tc.scope, OlderThan: tc.older, Now: now}, tc.dry)
			if tc.wantErr {
				if !errors.Is(err, ErrInboxInvalid) {
					t.Fatalf("err = %v, want ErrInboxInvalid", err)
				}
				return
			}
			if err != nil || n != tc.want {
				t.Fatalf("n = %d, err = %v, want %d", n, err, tc.want)
			}
			left, _ := db.ListInbox(ctx, "user:a", InboxFilter{})
			wantLeft := 4
			if !tc.dry {
				wantLeft -= tc.want
			}
			if len(left) != wantLeft {
				t.Fatalf("items left = %d, want %d", len(left), wantLeft)
			}
		})
	}
}

func TestTidyRunnerArchivesResolvedThenPrunes(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	clock := time.Now()
	fired := seedHistory(t, db, clock.Add(-3*time.Hour), "web", SeverityCritical, EventFired, OutcomeSent)
	seedHistory(t, db, clock.Add(-2*time.Hour), "web", SeverityCritical, EventResolved, OutcomeSent)
	open := seedHistory(t, db, clock.Add(-2*time.Hour), "api", SeverityCritical, EventFired, OutcomeSent)
	if err := db.ArchiveInbox(ctx, "user:a", []string{"deploy:keep"}, true); err != nil {
		t.Fatal(err)
	}
	r := &TidyRunner{DB: db, Now: func() time.Time { return clock },
		Cfg: TidyConfig{ArchiveResolvedAfter: 30 * time.Minute, Retention: 24 * time.Hour, Interval: time.Minute}}

	res, err := r.RunOnce(ctx)
	if err != nil || res.Archived != 2 || res.Pruned != 0 {
		t.Fatalf("first sweep = %+v, %v", res, err)
	}
	active, _ := db.ListInbox(ctx, "user:a", InboxFilter{})
	if len(active) != 1 || active[0].ID != open {
		t.Fatalf("unresolved item must stay active: %+v", active)
	}

	clock = clock.Add(25 * time.Hour)
	res, err = r.RunOnce(ctx)
	if err != nil || res.Pruned != 2 {
		t.Fatalf("second sweep = %+v, %v", res, err)
	}
	archived, _ := db.ListInbox(ctx, "user:a", InboxFilter{Archived: true})
	if len(archived) != 0 {
		t.Fatalf("archived items survived retention: %d", len(archived))
	}
	st, _ := db.GetInboxState(ctx, "user:a")
	if len(st.ArchivedIDs) != 1 || st.ArchivedIDs[0] != "deploy:keep" {
		t.Fatalf("derived archive must never be pruned: %+v", st.ArchivedIDs)
	}
	if err := db.DeleteInbox(ctx, "user:a", []string{fired}, true, clock); err != nil {
		t.Fatal(err)
	}
}

func TestTidyConfigFromEnv(t *testing.T) {
	env := map[string]string{EnvInboxRetention: "0", EnvInboxAutoArchiveAfter: "bogus", EnvInboxTidyInterval: "2m"}
	c := TidyConfigFromEnv(func(k string) string { return env[k] })
	if c.Retention != 0 || c.ArchiveResolvedAfter != defaultInboxAutoArchiveAfter || c.Interval != 2*time.Minute {
		t.Fatalf("cfg = %+v", c)
	}
}

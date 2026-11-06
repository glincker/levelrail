package store

import (
	"context"
	"testing"
	"time"
)

func TestListAuditEntries_SinceUntilAndActor(t *testing.T) {
	db := openTestDB(t)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i, actor := range []string{"alice", "bob", "alice"} {
		e := testAuditEntry("aud_"+actor+string(rune('a'+i)), FormatAuditTime(base.Add(time.Duration(i)*time.Hour)))
		e.ActorName = actor
		e.ActorID = "id-" + actor
		if err := db.SaveAuditEntry(context.Background(), e); err != nil {
			t.Fatal(err)
		}
	}
	count := func(f AuditEntryFilter) int {
		t.Helper()
		got, err := db.ListAuditEntries(context.Background(), 50, nil, f)
		if err != nil {
			t.Fatal(err)
		}
		return len(got)
	}
	if n := count(AuditEntryFilter{Actor: "alice"}); n != 2 {
		t.Errorf("actor alice = %d, want 2", n)
	}
	if n := count(AuditEntryFilter{Actor: "id-alice"}); n != 0 {
		t.Errorf("actor matches actor_name only, got %d for an actor id", n)
	}
	if n := count(AuditEntryFilter{Since: base.Add(time.Hour)}); n != 2 {
		t.Errorf("since is inclusive: got %d, want 2", n)
	}
	if n := count(AuditEntryFilter{Until: base.Add(time.Hour)}); n != 1 {
		t.Errorf("until is exclusive: got %d, want 1", n)
	}
}

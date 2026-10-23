package alerting

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestInboxArchiveHidesAndRestores(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	id := seedHistory(t, db, time.Now().Add(-time.Minute), "web", SeverityCritical, EventFired, OutcomeSent)

	if err := db.ArchiveInbox(ctx, "user:a", []string{id, "deploy:x"}, true); err != nil {
		t.Fatal(err)
	}
	items, err := db.ListInbox(ctx, "user:a", InboxFilter{})
	if err != nil || len(items) != 0 {
		t.Fatalf("archived item listed: %v %v", items, err)
	}
	other, _ := db.ListInbox(ctx, "user:b", InboxFilter{})
	if len(other) != 1 {
		t.Fatalf("archive leaked to another principal: %d", len(other))
	}
	st, err := db.GetInboxState(ctx, "user:a")
	if err != nil || len(st.ArchivedIDs) != 2 {
		t.Fatalf("state = %+v %v", st, err)
	}
	if err := db.ArchiveInbox(ctx, "user:a", []string{id}, false); err != nil {
		t.Fatal(err)
	}
	if items, _ = db.ListInbox(ctx, "user:a", InboxFilter{}); len(items) != 1 {
		t.Fatalf("restored item missing: %d", len(items))
	}
	if err := db.ArchiveInbox(ctx, "user:a", nil, true); !errors.Is(err, ErrInboxInvalid) {
		t.Fatalf("empty ids err = %v", err)
	}
}

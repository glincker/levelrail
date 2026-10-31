package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestMailboxOwnership(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.ClaimMailDomain(ctx, MailDomain{Domain: "Example.test", Service: "mail-mox", TeamID: "team-a"}); err != nil {
		t.Fatal(err)
	}
	if err := db.ClaimMailDomain(ctx, MailDomain{Domain: "example.test", Service: "mail-mox", TeamID: "team-b"}); !errors.Is(err, ErrMailDomainTaken) {
		t.Fatalf("second team claim: %v", err)
	}
	m := Mailbox{Address: "Jo@example.test", Domain: "example.test", Localpart: "jo", Service: "mail-mox", TeamID: "team-a", LinkedUserID: "u1"}
	if err := db.CreateMailbox(ctx, m); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateMailbox(ctx, m); !errors.Is(err, ErrMailboxExists) {
		t.Fatalf("duplicate: %v", err)
	}
	if err := db.SetMailboxSuspendPending(ctx, "jo@example.test"); err != nil {
		t.Fatal(err)
	}
	if owed, err := db.ListMailboxesSuspendPending(ctx); err != nil || len(owed) != 1 || !owed[0].SuspendPending {
		t.Fatalf("pending %+v %v", owed, err)
	}
	if err := db.SetMailboxSuspended(ctx, "jo@example.test", true); err != nil {
		t.Fatal(err)
	}
	if owed, _ := db.ListMailboxesSuspendPending(ctx); len(owed) != 0 {
		t.Fatalf("suspending must settle the owed suspend: %+v", owed)
	}
	linked, err := db.ListMailboxesLinkedTo(ctx, "team-a", "u1")
	if err != nil || len(linked) != 1 || !linked[0].Suspended {
		t.Fatalf("linked %+v %v", linked, err)
	}
	b, _ := json.Marshal(linked[0])
	if strings.Contains(strings.ToLower(string(b)), "password") {
		t.Fatalf("mailbox record must not carry a password: %s", b)
	}
	if err := db.DeleteMailbox(ctx, "jo@example.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetMailbox(ctx, "jo@example.test"); !errors.Is(err, ErrMailboxNotFound) {
		t.Fatalf("after delete: %v", err)
	}
}

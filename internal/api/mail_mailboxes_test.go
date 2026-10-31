package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/mail"
	"github.com/GLINCKER/levelrail/internal/store"
)

var _ MailboxStore = (*store.DB)(nil)

func TestMailDecideTeamBoundary(t *testing.T) {
	teamB := &teamScope{roles: map[string]string{"team-b": store.TeamRoleOwner}}
	all := []string{AbilityRoot}
	res := []string{"mail:mailbox:jo@example.test", "mail:domain:example.test", "mail:service:mail-mox"}
	if got := mailDecide(teamB, all, nil, "team-a", AbilityRead, res...); got != http.StatusNotFound {
		t.Fatalf("team B on team A mailbox: got %d, want 404", got)
	}
	teamA := &teamScope{roles: map[string]string{"team-a": store.TeamRoleOwner}}
	if got := mailDecide(teamA, all, nil, "team-a", AbilityWriteSensitive, res...); got != 0 {
		t.Fatalf("team A owner: got %d", got)
	}
	if got := mailDecide(nil, []string{AbilityRead}, nil, "team-a", AbilityWriteSensitive, res...); got != http.StatusForbidden {
		t.Fatalf("read only caller writing: got %d, want 403", got)
	}
	deny := store.Policy{Document: `{"Statement":[{"Effect":"Deny","Action":["read"],"Resource":["mail:domain:example.test"]}]}`}
	if got := mailDecide(nil, all, []store.Policy{deny}, "team-a", AbilityRead, res...); got != http.StatusNotFound {
		t.Fatalf("deny on the domain must hide the mailbox: got %d", got)
	}
}

// TestMailResponsesCarryNoStoredSecret checks that only the show-once
// response has a password field, and that list, store and audit shapes do not.
func TestMailResponsesCarryNoStoredSecret(t *testing.T) {
	for _, v := range []any{mailboxResource{}, mailDomainResource{}, mailQueueResponse{}, mailQueueHoldResponse{}, mail.QueueEntry{}, mail.Alias{}, mail.AccountDetail{}, store.Mailbox{}, store.MailDomain{}, store.AuditEntry{}} {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		low := strings.ToLower(string(b))
		for _, bad := range []string{"password", "secret", "subject", "body", "message_id"} {
			if strings.Contains(low, bad) {
				t.Errorf("%T serializes %q: %s", v, bad, b)
			}
		}
	}
	if _, ok := reflect.TypeOf(mailboxSecretResponse{}).FieldByName("Password"); !ok {
		t.Fatal("the show-once response must carry the generated password")
	}
	spec := mail.AccountSpec{Name: "a", Address: "a@x", Password: "pw-should-not-print"} //nolint:gosec // fake value proving redaction
	if s := spec.String(); strings.Contains(s, "pw-should-not-print") {
		t.Fatalf("spec leaks password: %s", s)
	}
}

type fakeBoxes struct {
	MailboxStore
	boxes map[string]store.Mailbox
}

func (f *fakeBoxes) ListMailboxesLinkedTo(_ context.Context, teamID, userID string) ([]store.Mailbox, error) {
	var out []store.Mailbox
	for _, m := range f.boxes {
		if m.TeamID == teamID && m.LinkedUserID == userID {
			out = append(out, m)
		}
	}
	return out, nil
}

func (f *fakeBoxes) SetMailboxSuspended(_ context.Context, address string, s bool) error {
	m := f.boxes[address]
	m.Suspended = s
	f.boxes[address] = m
	return nil
}

type fakeMailEngine struct {
	mail.Engine
	disabled map[string]string
}

func (f *fakeMailEngine) SetLoginDisabled(_ context.Context, account, msg string) error {
	f.disabled[account] = msg
	return nil
}

func TestOffboardingSuspendsLinkedMailboxes(t *testing.T) {
	boxes := &fakeBoxes{boxes: map[string]store.Mailbox{
		"jo@example.test":  {Address: "jo@example.test", Service: "mail-mox", TeamID: "team-a", LinkedUserID: "u1"},
		"ops@example.test": {Address: "ops@example.test", Service: "mail-mox", TeamID: "team-a"},
		"jo@other.test":    {Address: "jo@other.test", Service: "mail-mox", TeamID: "team-b", LinkedUserID: "u1"},
	}}
	eng := &fakeMailEngine{disabled: map[string]string{}}
	rt := &Router{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	rt.mail.boxes = boxes
	rt.mail.engine = func(context.Context, string) (mail.Engine, error) { return eng, nil }
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/teams/team-a/members/u1", nil)
	if n := rt.offboardMailboxes(req, "team-a", "u1"); n != 1 {
		t.Fatalf("suspended %d, want 1", n)
	}
	if !boxes.boxes["jo@example.test"].Suspended || eng.disabled["jo@example.test"] == "" {
		t.Fatal("the linked mailbox must be suspended in the engine and the store")
	}
	if boxes.boxes["ops@example.test"].Suspended || boxes.boxes["jo@other.test"].Suspended {
		t.Fatal("only the leaving member's mailbox in that team is suspended")
	}
}

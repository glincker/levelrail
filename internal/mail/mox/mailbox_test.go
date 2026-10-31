package mox

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/mail"
)

func TestMailboxOps(t *testing.T) {
	var calls []string
	f := &fake{handler: func(fn string, _ []json.RawMessage) (any, *rpcError) {
		calls = append(calls, fn)
		switch fn {
		case "Account":
			return []any{map[string]any{"LoginDisabled": "x", "Destinations": map[string]any{"jo@example.test": map[string]any{}}}, 42}, nil
		case "DomainLocalparts":
			return []any{map[string]string{"jo": "jo"}, map[string]any{"team": map[string]any{"Addresses": []string{"jo@example.test"}}}}, nil
		case "QueueList":
			return []map[string]any{{"ID": 7, "SenderAccount": "jo", "RecipientDomainStr": "far.test", "Attempts": 2, "Subject": "secret plans", "MessageID": "<m@x>",
				"Results": []map[string]any{{"Success": false, "Error": "450 try later"}}}}, nil
		case "QueueHoldSet":
			return 1, nil
		case "SetPassword":
			return nil, &rpcError{Code: "user:error", Message: "bad password"}
		}
		return nil, nil
	}}
	c := newClient(t, f)
	ctx := context.Background()
	d, err := c.AccountDetail(ctx, "jo")
	if err != nil || !d.LoginDisabled || d.DiskUsageBytes != 42 || len(d.Addresses) != 1 {
		t.Fatalf("detail %+v %v", d, err)
	}
	if err := mail.DeleteAccount(ctx, c, "jo", "jo", false); !errors.Is(err, mail.ErrHasMail) {
		t.Fatalf("delete with mail must be refused: %v", err)
	}
	if err := mail.DeleteAccount(ctx, c, "jo", "nope", true); !errors.Is(err, mail.ErrInvalid) {
		t.Fatalf("delete without typed confirm: %v", err)
	}
	al, err := c.Aliases(ctx, "example.test")
	if err != nil || len(al) != 1 || al[0].Address != "team@example.test" {
		t.Fatalf("aliases %+v %v", al, err)
	}
	q, err := c.QueueList(ctx, 10)
	if err != nil || len(q) != 1 || q[0].LastError != "450 try later" || q[0].RecipientDomain != "far.test" {
		t.Fatalf("queue %+v %v", q, err)
	}
	if b, _ := json.Marshal(q); strings.Contains(string(b), "secret plans") || strings.Contains(string(b), "<m@x>") {
		t.Fatalf("queue entry leaks message content: %s", b)
	}
	if _, err := c.HoldMessages(ctx, nil, true); !errors.Is(err, mail.ErrInvalid) {
		t.Fatalf("empty id list must not hold the whole queue: %v", err)
	}
	if n, err := c.HoldMessages(ctx, []int64{7}, true); err != nil || n != 1 {
		t.Fatalf("hold %d %v", n, err)
	}
	if err := mail.SuspendAccount(ctx, c, "jo"); err != nil {
		t.Fatal(err)
	}
	calls = nil
	if _, err := mail.CreateAccount(ctx, c, "Jo@Example.test", 0); err == nil {
		t.Fatal("a failed password set must fail the create")
	}
	if strings.Join(calls, ",") != "AccountAdd,AccountSettingsSave,SetPassword,AccountRemove" {
		t.Fatalf("create must roll back a half-made account, calls %v", calls)
	}
}

// TestLiveMox runs against a throwaway mox v0.0.17 container when
// MOX_LIVE_URL and MOX_LIVE_PASSWORD_FILE are set; it is skipped otherwise.
func TestLiveMox(t *testing.T) {
	base, pwFile := os.Getenv("MOX_LIVE_URL"), os.Getenv("MOX_LIVE_PASSWORD_FILE")
	if base == "" || pwFile == "" {
		t.Skip("set MOX_LIVE_URL and MOX_LIVE_PASSWORD_FILE to run against a throwaway mox")
	}
	pw, err := os.ReadFile(pwFile) //nolint:gosec // operator supplied path for an opt-in live test
	if err != nil {
		t.Fatal(err)
	}
	c, err := New(Config{BaseURL: base, Credentials: creds{pw: strings.TrimSpace(string(pw))}, HTTPClient: &http.Client{}})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	addr := "live-proof@example.test"
	secret, err := mail.CreateAccount(ctx, c, addr, 0)
	if err != nil || len(secret) != mail.PasswordLength() {
		t.Fatalf("create: %v", err)
	}
	t.Cleanup(func() { _ = c.RemoveAccount(context.Background(), addr) })
	if _, err := mail.CreateAccount(ctx, c, addr, 0); !errors.Is(err, mail.ErrExists) {
		t.Fatalf("duplicate create: %v", err)
	}
	steps := []func() error{
		func() error { return mail.SuspendAccount(ctx, c, addr) },
		func() error {
			list, err := mail.ListAccounts(ctx, c)
			for _, a := range list {
				if a.Name == addr && !a.Disabled {
					return errors.New("suspended account not reported disabled")
				}
			}
			return err
		},
		func() error { return mail.ResumeAccount(ctx, c, addr) },
		func() error { _, err := mail.SetAccountPassword(ctx, c, addr); return err },
		func() error {
			return c.AddAlias(ctx, mail.Alias{Address: "live-team@example.test", Targets: []string{addr}})
		},
		func() error {
			al, err := mail.ListAliases(ctx, c, "example.test")
			for _, a := range al {
				if a.Address == "live-team@example.test" {
					return err
				}
			}
			return errors.New("alias not listed")
		},
		func() error { return c.RemoveAlias(ctx, "live-team@example.test") },
		func() error { _, err := mail.ListQueue(ctx, c, 10); return err },
		func() error { return c.HoldQueue(ctx) },
		func() error { return c.ReleaseQueue(ctx) },
		func() error { return mail.DeleteAccount(ctx, c, addr, addr, false) },
	}
	for i, s := range steps {
		if err := s(); err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
	}
	if _, err := c.AccountDetail(ctx, addr); !errors.Is(err, mail.ErrNotFound) {
		t.Fatalf("deleted account still readable: %v", err)
	}
}

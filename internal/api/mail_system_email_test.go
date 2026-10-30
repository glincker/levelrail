package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/mail/demo"
	"github.com/GLINCKER/levelrail/internal/store"
)

type sysEmailBoxes struct {
	fakeBoxes
	domains []store.MailDomain
}

func (f *sysEmailBoxes) GetMailbox(_ context.Context, address string) (store.Mailbox, error) {
	m, ok := f.boxes[address]
	if !ok {
		return m, store.ErrMailboxNotFound
	}
	return m, nil
}

func (f *sysEmailBoxes) CreateMailbox(_ context.Context, m store.Mailbox) error {
	if _, ok := f.boxes[m.Address]; ok {
		return errors.New("exists")
	}
	f.boxes[m.Address] = m
	return nil
}

func (f *sysEmailBoxes) SetMailboxSystem(_ context.Context, address string, system bool) error {
	m := f.boxes[address]
	m.System = system
	f.boxes[address] = m
	return nil
}

func (f *sysEmailBoxes) DeleteMailbox(_ context.Context, address string) error {
	delete(f.boxes, address)
	return nil
}

func (f *sysEmailBoxes) ListMailDomains(context.Context) ([]store.MailDomain, error) {
	return f.domains, nil
}

func TestSystemEmailNoreplyMailboxIsCreatedThenReused(t *testing.T) {
	rt := mailDemoRouter(t)
	boxes := &sysEmailBoxes{fakeBoxes: fakeBoxes{boxes: map[string]store.Mailbox{}}}
	rt.mail.boxes = boxes
	e := demo.NewEngine()
	d := store.MailDomain{Domain: "example.test", Service: demo.AppName, TeamID: "team_1"}
	r := httptest.NewRequest("POST", "/api/v1/mail/system-email/enable", nil)

	nb, err := rt.ensureNoreplyMailbox(r, e, d, "usr_1", "")
	addr, first := nb.address, nb.password
	if err != nil || addr != "noreply@example.test" || first == "" || !boxes.boxes[addr].System {
		t.Fatalf("create: addr=%q err=%v", addr, err)
	}
	if err := rt.sysEmailSuspend(r, demo.AppName, addr); err == nil && !boxes.boxes[addr].Suspended {
		t.Fatal("suspend did not record the state")
	}
	m := boxes.boxes[addr]
	m.Suspended = true
	boxes.boxes[addr] = m
	nb2, err := rt.ensureNoreplyMailbox(r, e, d, "usr_1", "")
	second := nb2.password
	if err != nil || second == "" || second == first {
		t.Fatalf("reuse must set a fresh password: err=%v", err)
	}
	if boxes.boxes[addr].Suspended {
		t.Fatal("reuse must resume a suspended noreply mailbox")
	}
}

func TestSystemEmailSnapshotRestoresPreviousCredentials(t *testing.T) {
	rt := mailDemoRouter(t)
	sec := newFakeEmailSecretsStore()
	rt.emailSecrets = sec
	ctx := context.Background()
	key := store.EmailSettingsSecretsKey()
	prev := "prev-" + strings.Repeat("x", 8)
	_ = sec.SetValue(ctx, key, store.EmailSecretResendAPIKey, prev)
	sealed, err := rt.sealedSlots(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_ = sec.SetValue(ctx, key, store.EmailSecretSMTPPassword, "generated-at-runtime")
	_ = sec.SetValue(ctx, key, store.EmailSecretSMTPTLSPin, "10.0.0.5:587 abc")
	if err := rt.restoreSealed(ctx, sealed); err != nil {
		t.Fatal(err)
	}
	if got, _ := sec.Resolve(ctx, key, store.EmailSecretResendAPIKey); got != prev {
		t.Fatalf("previous credential not restored: %q", got)
	}
	if ok, _ := sec.Exists(ctx, key, store.EmailSecretSMTPPassword); ok {
		t.Fatal("the noreply credential must be removed on disable")
	}
	if ok, _ := sec.Exists(ctx, key, store.EmailSecretSMTPTLSPin); ok {
		t.Fatal("the pin must be removed on disable")
	}
}

func TestSystemEmailViewCarriesNoSecret(t *testing.T) {
	rt := mailDemoRouter(t)
	rt.mail.boxes = &sysEmailBoxes{fakeBoxes: fakeBoxes{boxes: map[string]store.Mailbox{}}}
	pw := "pw-" + strings.Repeat("q", 12)
	st := sysEmailState{Enabled: true, Domain: demo.DefaultDomain, Mailbox: "noreply@" + demo.DefaultDomain, Addr: "mail.demo.invalid:587",
		Previous: &sysEmailPrevious{Sealed: map[string]string{store.EmailSecretSMTPPassword: pw}}}
	b, _ := json.Marshal(rt.sysEmailView(context.Background(), st))
	if strings.Contains(string(b), pw) || strings.Contains(string(b), "sealed") {
		t.Fatalf("view leaks sealed state: %s", b)
	}
	if !strings.Contains(string(b), `"has_previous_provider":true`) {
		t.Fatalf("view = %s", b)
	}
}

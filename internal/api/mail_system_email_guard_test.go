package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/mail"
	"github.com/GLINCKER/levelrail/internal/mail/demo"
	"github.com/GLINCKER/levelrail/internal/store"
)

func runtimeSecret(t *testing.T) string {
	t.Helper()
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(b)
}

// sysGuardRouter seeds a team domain with the system noreply mailbox and an ordinary one beside it.
func sysGuardRouter(t *testing.T) (*Router, *store.DB, *fakeEmailSecretsStore) {
	t.Helper()
	db := openTestDB(t)
	sec := newFakeEmailSecretsStore()
	rt := NewRouter(discardLogger(), testBrand(), db, WithEmailSecrets(sec))
	rt.mail.boxes = db
	ctx := context.Background()
	if err := db.ClaimMailDomain(ctx, store.MailDomain{Domain: "example.test", Service: "mail-mox", TeamID: "team-a", CreatedBy: "u"}); err != nil {
		t.Fatal(err)
	}
	for _, m := range []store.Mailbox{
		{Address: "noreply@example.test", Domain: "example.test", Localpart: "noreply", Service: "mail-mox", TeamID: "team-a", System: true},
		{Address: "ops@example.test", Domain: "example.test", Localpart: "ops", Service: "mail-mox", TeamID: "team-a"},
	} {
		if err := db.CreateMailbox(ctx, m); err != nil {
			t.Fatal(err)
		}
	}
	return rt, db, sec
}

// TestSystemMailboxRefusedOnTeamRoutes models a team admin (write:sensitive) and root trying to take the system account.
func TestSystemMailboxRefusedOnTeamRoutes(t *testing.T) {
	rt, db, _ := sysGuardRouter(t)
	admin := seedMatrixToken(t, db, "team-admin", []string{AbilityRead, AbilityWriteSensitive})
	root := seedMatrixToken(t, db, "root", []string{AbilityRoot})
	cases := []struct {
		name, token, method, path, body string
	}{
		{"password reset", admin, http.MethodPost, "/api/v1/mail/mailboxes/noreply@example.test/password", `{}`},
		{"suspend", admin, http.MethodPost, "/api/v1/mail/mailboxes/noreply@example.test/suspend", `{}`},
		{"resume", admin, http.MethodPost, "/api/v1/mail/mailboxes/noreply@example.test/resume", `{}`},
		{"delete as root", root, http.MethodDelete, "/api/v1/mail/mailboxes/noreply@example.test", `{"confirm":"noreply@example.test"}`},
		{"alias over it", admin, http.MethodPost, "/api/v1/mail/aliases", `{"address":"noreply@example.test","targets":["ops@example.test"]}`},
		{"alias into it", admin, http.MethodPost, "/api/v1/mail/aliases", `{"address":"team@example.test","targets":["noreply@example.test"]}`},
		{"alias remove", admin, http.MethodDelete, "/api/v1/mail/aliases/noreply@example.test", ``},
	}
	for _, tc := range cases {
		code, out := mailCall(t, rt, tc.token, tc.method, tc.path, tc.body)
		if tc.token == root && code == http.StatusForbidden {
			continue // the team role cap stops this token first; the store check below still proves nothing changed
		}
		if code != http.StatusConflict || !strings.Contains(asString(out["error"]), "mail system-email") {
			t.Errorf("%s: got %d %v, want 409 pointing at mail system-email", tc.name, code, out)
		}
	}
	if code, _ := mailCall(t, rt, admin, http.MethodPost, "/api/v1/mail/mailboxes/ops@example.test/password", `{}`); code == http.StatusConflict {
		t.Fatal("an ordinary mailbox must not be refused as a system one")
	}
	if m, err := db.GetMailbox(context.Background(), "noreply@example.test"); err != nil || m.Suspended {
		t.Fatal("the system mailbox changed state")
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/mail/mailboxes", nil)
	req.Header.Set("Authorization", "Bearer "+root)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "noreply@") || !strings.Contains(rec.Body.String(), "ops@") {
		t.Fatalf("list must hide the system mailbox: %d %s", rec.Code, rec.Body.String())
	}
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

// TestSystemEmailStatusNeedsRoot keeps the claimed domains, engine address and test recipient from non-root callers.
func TestSystemEmailStatusNeedsRoot(t *testing.T) {
	rt, db, _ := sysGuardRouter(t)
	admin := seedMatrixToken(t, db, "team-admin", []string{AbilityRead, AbilityWriteSensitive})
	if code, out := mailCall(t, rt, admin, http.MethodGet, "/api/v1/mail/system-email", ``); code != http.StatusForbidden || out["domains"] != nil {
		t.Fatalf("non-root status: %d %v", code, out)
	}
}

func TestSystemEmailActiveFollowsCurrentProvider(t *testing.T) {
	st := sysEmailState{Enabled: true, Mailbox: "noreply@example.test", Addr: "10.0.0.5:587"}
	ours := store.EmailSettings{Backend: store.EmailBackendSMTP, SMTPHost: "10.0.0.5", SMTPPort: 587, SMTPUsername: "noreply@example.test"}
	other := ours
	other.SMTPHost = "smtp.other.test"
	off := st
	off.Enabled = false
	for _, tc := range []struct {
		s    store.EmailSettings
		st   sysEmailState
		want bool
	}{{ours, st, true}, {other, st, false}, {store.EmailSettings{Backend: store.EmailBackendResend}, st, false}, {ours, off, false}} {
		if got := sysEmailActive(tc.s, tc.st); got != tc.want {
			t.Errorf("active(%+v, enabled=%v) = %v", tc.s, tc.st.Enabled, got)
		}
	}
}

func TestSystemEmailEngineLookupFailsClosed(t *testing.T) {
	rt := &Router{logger: discardLogger()}
	if _, err := rt.sysEmailSubmission(context.Background(), "mail-mox", 587); !errors.Is(err, errSysEmailNoIdentity) {
		t.Fatalf("no instance identity must refuse, got %v", err)
	}
	c := docker.ContainerState{Networks: []docker.ContainerNetwork{{Name: "bridge", IPAddress: "172.17.0.9"}, {Name: "lr-app-app_1", IPAddress: "10.9.0.4"}}}
	if ip := sysEmailAppNetworkIP(c, "app_1"); ip != "10.9.0.4" {
		t.Fatalf("app network ip = %q", ip)
	}
	if ip := sysEmailAppNetworkIP(c, "app_2"); ip != "" {
		t.Fatalf("another app's network must not count: %q", ip)
	}
	if ip := sysEmailAppNetworkIP(c, ""); ip != "" {
		t.Fatalf("no app id must not match: %q", ip)
	}
}

type pwEngine struct {
	mail.Engine
	pw   map[string]string
	fail error
}

func (e *pwEngine) SetPassword(_ context.Context, account, password string) error {
	if e.fail != nil {
		return e.fail
	}
	e.pw[account] = password
	return nil
}

func (e *pwEngine) SetLoginDisabled(_ context.Context, account, _ string) error {
	if e.fail != nil {
		return e.fail
	}
	e.pw[account+"#disabled"] = "1"
	return nil
}

// TestSystemEmailRotateNeverTouchesAnotherProvider: with a newer provider active, rotate writes only its own slot.
func TestSystemEmailRotateNeverTouchesAnotherProvider(t *testing.T) {
	rt, _, sec := sysGuardRouter(t)
	ctx := context.Background()
	key := store.EmailSettingsSecretsKey()
	other, old := runtimeSecret(t), runtimeSecret(t)
	_ = sec.SetValue(ctx, key, store.EmailSecretSMTPPassword, other)
	_ = sec.SetValue(ctx, sysEmailSecretsNS, store.EmailSecretSMTPPassword, old)
	st := sysEmailState{Enabled: true, Mailbox: "noreply@example.test", Addr: "10.0.0.5:587"}
	eng := &pwEngine{pw: map[string]string{}}
	pw := runtimeSecret(t)
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	if !rt.sysEmailRotateSealed(httptest.NewRecorder(), r, eng, st, pw) {
		t.Fatal("rotate failed")
	}
	if got, _ := sec.Resolve(ctx, key, store.EmailSecretSMTPPassword); got != other {
		t.Fatal("rotate overwrote the newer provider's password")
	}
	if got, _ := sec.Resolve(ctx, sysEmailSecretsNS, store.EmailSecretSMTPPassword); got != pw || eng.pw[st.Mailbox] != pw {
		t.Fatal("rotate did not commit the new password to its own slot and the engine")
	}
	if ok, _ := sec.Exists(ctx, sysEmailSecretsNS, sysEmailPendingKey); ok {
		t.Fatal("the pending value must be cleared")
	}
	eng.fail = errors.New("engine down")
	if rt.sysEmailRotateSealed(httptest.NewRecorder(), r, eng, st, runtimeSecret(t)) {
		t.Fatal("an engine failure must fail the rotate")
	}
	if got, _ := sec.Resolve(ctx, sysEmailSecretsNS, store.EmailSecretSMTPPassword); got != pw {
		t.Fatal("a failed rotate must keep the committed password")
	}
	if ok, _ := sec.Exists(ctx, sysEmailSecretsNS, sysEmailPendingKey); ok {
		t.Fatal("a failed rotate must drop the pending value")
	}
}

// TestSystemEmailDisableLeavesNewerProvider: disable after someone switched providers only suspends the mailbox.
func TestSystemEmailDisableLeavesNewerProvider(t *testing.T) {
	rt, db, sec := sysGuardRouter(t)
	ctx := context.Background()
	eng := &pwEngine{pw: map[string]string{}}
	rt.mail.engine = func(context.Context, string) (mail.Engine, error) { return eng, nil }
	key := store.EmailSettingsSecretsKey()
	other := runtimeSecret(t)
	_ = sec.SetValue(ctx, key, store.EmailSecretSMTPPassword, other)
	st := sysEmailState{Enabled: true, Domain: "example.test", Service: "mail-mox", Mailbox: "noreply@example.test", Addr: "10.0.0.5:587",
		Previous: &sysEmailPrevious{Settings: store.EmailSettings{Backend: store.EmailBackendResend}}}
	if err := rt.saveSysEmailState(ctx, st); err != nil {
		t.Fatal(err)
	}
	d := store.MailDomain{Domain: "example.test", Service: "mail-mox", TeamID: "team-a"}
	disable := func() int {
		rec := httptest.NewRecorder()
		rt.sysEmailDisableLocked(rec, httptest.NewRequest(http.MethodPost, "/api/v1/mail/system-email/disable", nil), st, d)
		return rec.Code
	}
	if code := disable(); code != http.StatusOK {
		t.Fatalf("disable: %d", code)
	}
	if got, _ := sec.Resolve(ctx, key, store.EmailSecretSMTPPassword); got != other {
		t.Fatal("disable touched the newer provider's credential")
	}
	if s, _ := db.GetEmailSettings(ctx); s.Backend == store.EmailBackendResend {
		t.Fatal("disable restored the stale previous provider over the newer one")
	}
	if m, _ := db.GetMailbox(ctx, "noreply@example.test"); !m.Suspended {
		t.Fatal("the noreply mailbox must be suspended")
	}
	got, _ := rt.loadSysEmailState(ctx)
	if got.Enabled || got.Previous != nil {
		t.Fatalf("state not cleared: %+v", got)
	}
	// A suspend failure keeps system email enabled with the error.
	_ = rt.saveSysEmailState(ctx, st)
	eng.fail = errors.New("engine down")
	if code := disable(); code != http.StatusOK {
		t.Fatalf("disable with failing suspend: %d", code)
	}
	if got, _ := rt.loadSysEmailState(ctx); !got.Enabled || got.Error == "" {
		t.Fatalf("a failed suspend must keep enabled-with-error: %+v", got)
	}
}

func TestSystemEmailFailedEnableUndoesMailbox(t *testing.T) {
	rt := mailDemoRouter(t)
	boxes := &sysEmailBoxes{fakeBoxes: fakeBoxes{boxes: map[string]store.Mailbox{
		"noreply@example.test": {Address: "noreply@example.test", Domain: "example.test", TeamID: "team_1", Suspended: true},
	}}}
	rt.mail.boxes = boxes
	eng := &pwEngine{pw: map[string]string{}}
	old := runtimeSecret(t)
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	nb, err := rt.ensureNoreplyMailbox(r, eng, store.MailDomain{Domain: "example.test", Service: demo.AppName, TeamID: "team_1"}, "u", old)
	if err != nil {
		t.Skipf("resume on the fake engine: %v", err)
	}
	if !boxes.boxes[nb.address].System {
		t.Fatal("enable must mark the mailbox as the system one")
	}
	nb.undo(context.Background())
	if m := boxes.boxes[nb.address]; m.System || !m.Suspended || eng.pw[nb.address] != old {
		t.Fatalf("undo must restore flag, suspension and the old password: %+v", m)
	}
}

func TestSystemEmailDNSIsCached(t *testing.T) {
	rt := mailDemoRouter(t)
	t.Setenv(envSysEmailDNSTTL, "300")
	first := rt.sysEmailDNSCached(context.Background(), demo.DefaultDomain)
	if second := rt.sysEmailDNSCached(context.Background(), demo.DefaultDomain); first != second {
		t.Fatal("a second status read inside the TTL must reuse the cached DNS answer")
	}
}

func TestReservedSecretNamespacesRefusedAsAppNames(t *testing.T) {
	for _, n := range []string{sysEmailSecretsNS, store.EmailSettingsSecretsKey(), store.VaultSecretsKey()} {
		if err := validateAppResource(appResource{Name: n, Image: "nginx:1"}); err == nil || !strings.Contains(err.Error(), "reserved") {
			t.Errorf("%s: %v", n, err)
		}
	}
}

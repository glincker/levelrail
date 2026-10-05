package authengine_test

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/GLINCKER/levelrail/internal/authengine"
	"github.com/GLINCKER/levelrail/internal/store"
)

const sessPassword = "correct horse battery"

type sessEnv struct {
	db     *store.DB
	eng    *authengine.Engine
	sess   *authengine.Sessions
	mu     sync.Mutex
	audits []authengine.AuditRecord
	mail   *authengine.MailRelay
}

func newSessEnv(t *testing.T) *sessEnv {
	t.Helper()
	t.Setenv(authengine.EnvEngine, authengine.EngineLibrary)
	t.Setenv(authengine.EnvAreas, string(authengine.AreaSessions))
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "levelrail.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	e := &sessEnv{db: db, mail: &authengine.MailRelay{}}
	eng, err := authengine.New(db.DB, authengine.Config{
		BaseURL:   "http://example.test",
		Directory: authengine.NewDirectory(db.DB),
		Sessions: authengine.SessionsHooks{Mail: e.mail, Audit: func(_ context.Context, r authengine.AuditRecord) {
			e.mu.Lock()
			e.audits = append(e.audits, r)
			e.mu.Unlock()
		}},
	})
	if err != nil {
		t.Fatalf("authengine.New: %v", err)
	}
	t.Cleanup(eng.Close)
	e.eng, e.sess = eng, eng.Sessions()
	if e.sess == nil {
		t.Fatal("sessions area not active")
	}
	return e
}

func (e *sessEnv) user(t *testing.T, id, email string) authengine.LegacyUser {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(sessPassword), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	h := string(hash)
	u := store.User{ID: id, Email: email, DisplayName: email, PasswordHash: &h, Abilities: []string{"read"}, CreatedAt: time.Now()}
	if err := e.db.CreateUser(context.Background(), u); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	return authengine.LegacyUser{ID: id, Email: email, DisplayName: email, PasswordHash: h, CreatedAt: u.CreatedAt}
}

func (e *sessEnv) login(t *testing.T, email, pw string) (string, error) {
	t.Helper()
	tok, _, err := e.sess.Login(context.Background(), authengine.LoginInput{Email: email, Password: pw, UserAgent: "test", IP: "198.51.100.9"})
	return tok, err
}

func (e *sessEnv) auditPaths() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	var out []string
	for _, a := range e.audits {
		out = append(out, fmt.Sprintf("%s %s %d %s", a.Method, a.Path, a.StatusCode, a.ActorType))
	}
	return out
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestSessionsLoginLookupRevoke(t *testing.T) {
	e := newSessEnv(t)
	ctx := context.Background()
	e.user(t, "user_a", "a@example.test")

	tok, err := e.login(t, "A@Example.test", sessPassword)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	info, ok := e.sess.Lookup(ctx, tok)
	if !ok || info.LegacyUserID != "user_a" {
		t.Fatalf("Lookup = %+v, %v; want user_a", info, ok)
	}
	if time.Until(info.ExpiresAt) < 23*time.Hour {
		t.Errorf("session expiry %v, want about 24h", info.ExpiresAt)
	}
	e.sess.Revoke(ctx, tok)
	if _, ok := e.sess.Lookup(ctx, tok); ok {
		t.Error("revoked session still resolves")
	}
	waitFor(t, "login and revoke audit rows", func() bool { return len(e.auditPaths()) >= 2 })
	got := e.auditPaths()
	want := []string{"POST /api/v1/auth/login 200 session", "POST /api/v1/auth/logout 204 session"}
	for _, w := range want {
		found := false
		for _, g := range got {
			found = found || g == w
		}
		if !found {
			t.Errorf("audit rows %v missing %q", got, w)
		}
	}
}

func TestSessionsLegacyBcryptRehashedAndMirrored(t *testing.T) {
	e := newSessEnv(t)
	e.user(t, "user_a", "a@example.test")
	if _, err := e.login(t, "a@example.test", sessPassword); err != nil {
		t.Fatalf("Login: %v", err)
	}
	var hash string
	if err := e.db.QueryRow(`SELECT p.password_hash FROM theauth_user_passwords p JOIN authengine_user_map m ON m.engine_id = p.user_id WHERE m.legacy_id = 'user_a'`).Scan(&hash); err != nil {
		t.Fatalf("read library hash: %v", err)
	}
	if len(hash) < 9 || hash[:9] != "$argon2id" {
		t.Errorf("library hash prefix = %q, want argon2id after rehash", hash)
	}
	got, err := e.db.GetUserByEmail(context.Background(), "a@example.test")
	if err != nil || got.PasswordHash == nil || (*got.PasswordHash)[:2] != "$2" {
		t.Errorf("platform hash must stay bcrypt for rollback, got %v err %v", got.PasswordHash, err)
	}
}

func TestSessionsThrottleAndLockoutAreConfigurable(t *testing.T) {
	t.Setenv(authengine.EnvLoginGraceFailures, "1")
	t.Setenv(authengine.EnvLoginUserMaxFailures, "2")
	t.Setenv(authengine.EnvLoginUserLockout, "1h")
	e := newSessEnv(t)
	e.user(t, "user_a", "a@example.test")
	var last error
	for i := 0; i < 4; i++ {
		_, last = e.login(t, "a@example.test", "wrong-password-1")
	}
	if !authengine.IsCode(last, authengine.CodeRateLimited) && !authengine.IsCode(last, authengine.CodeAccountLocked) {
		t.Fatalf("after repeated failures error = %v, want throttled or locked", last)
	}
	if _, err := e.login(t, "a@example.test", sessPassword); err == nil {
		t.Error("correct password accepted while the account is locked")
	}
}

func TestSessionsDeletedUserSessionAndLinkDieImmediately(t *testing.T) {
	e := newSessEnv(t)
	ctx := context.Background()
	u := e.user(t, "user_a", "a@example.test")
	tok, err := e.login(t, "a@example.test", sessPassword)
	if err != nil {
		t.Fatal(err)
	}
	link, err := e.sess.MintLink(ctx, u)
	if err != nil {
		t.Fatalf("MintLink: %v", err)
	}
	engineID, err := e.sess.EngineIDFor(ctx, "user_a")
	if err != nil || engineID == "" {
		t.Fatalf("EngineIDFor = %q, %v", engineID, err)
	}
	if err := e.db.DeleteUser(ctx, "user_a"); err != nil {
		t.Fatal(err)
	}
	if _, ok := e.sess.Lookup(ctx, tok); ok {
		t.Error("session of a deleted user still resolves before library cleanup")
	}
	if err := e.sess.DeleteUser(ctx, engineID); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	if _, _, ok, err := e.sess.ConsumeLink(ctx, link, "ua", "198.51.100.9"); ok || err != nil {
		t.Errorf("ConsumeLink after delete = ok %v err %v, want invalid", ok, err)
	}
	var n int
	_ = e.db.QueryRow(`SELECT COUNT(*) FROM theauth_users`).Scan(&n)
	if n != 0 {
		t.Errorf("library users left = %d, want 0", n)
	}
}

func TestSessionsLinkConsumedOnceAndBoundToUser(t *testing.T) {
	e := newSessEnv(t)
	ctx := context.Background()
	u := e.user(t, "user_a", "a@example.test")
	link, err := e.sess.MintLink(ctx, u)
	if err != nil {
		t.Fatal(err)
	}
	tok, legacy, ok, err := e.sess.ConsumeLink(ctx, link, "ua", "198.51.100.9")
	if err != nil || !ok || legacy != "user_a" {
		t.Fatalf("ConsumeLink = %q %q %v %v", tok, legacy, ok, err)
	}
	if _, ok := e.sess.Lookup(ctx, tok); !ok {
		t.Error("link session does not resolve")
	}
	if _, _, ok, _ := e.sess.ConsumeLink(ctx, link, "ua", "198.51.100.9"); ok {
		t.Error("link consumed twice")
	}
	if err := e.db.DeleteUser(ctx, "user_a"); err != nil {
		t.Fatal(err)
	}
	if _, ok := e.sess.Lookup(ctx, tok); ok {
		t.Error("link session survives user deletion")
	}
}

func TestSessionsRevokeAllExceptAndWatch(t *testing.T) {
	e := newSessEnv(t)
	ctx := context.Background()
	e.user(t, "user_a", "a@example.test")
	keep, _ := e.login(t, "a@example.test", sessPassword)
	other, _ := e.login(t, "a@example.test", sessPassword)
	wctx, cancel := e.sess.Watch(ctx, other, 20*time.Millisecond)
	defer cancel(nil)
	if err := e.sess.RevokeAllExcept(ctx, "user_a", keep); err != nil {
		t.Fatalf("RevokeAllExcept: %v", err)
	}
	if _, ok := e.sess.Lookup(ctx, keep); !ok {
		t.Error("kept session was revoked")
	}
	if _, ok := e.sess.Lookup(ctx, other); ok {
		t.Error("other session survived")
	}
	select {
	case <-wctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("stream watcher did not fire after revocation")
	}
}

func TestSessionsResetPasswordThroughLibrary(t *testing.T) {
	e := newSessEnv(t)
	ctx := context.Background()
	e.user(t, "user_a", "a@example.test")
	got := make(chan string, 1)
	e.mail.SetSender(func(_ context.Context, to, _, body string) error {
		if tok, ok := authengine.ResetTokenFromMail(body); ok && to == "a@example.test" {
			got <- tok
		}
		return nil
	})
	old, _ := e.login(t, "a@example.test", sessPassword)
	if err := e.sess.RequestReset(ctx, "a@example.test", "198.51.100.9"); err != nil {
		t.Fatalf("RequestReset: %v", err)
	}
	var token string
	select {
	case token = <-got:
	case <-time.After(2 * time.Second):
		t.Fatal("reset email was not relayed")
	}
	owner, err := e.sess.ResetPassword(ctx, token, "a-new-password-1", "198.51.100.9")
	if err != nil || owner != "user_a" {
		t.Fatalf("ResetPassword = %q, %v", owner, err)
	}
	if _, ok := e.sess.Lookup(ctx, old); ok {
		t.Error("session survived a password reset")
	}
	if _, err := e.sess.ResetPassword(ctx, token, "a-new-password-2", "198.51.100.9"); err == nil {
		t.Error("reset token redeemed twice")
	}
	if _, err := e.login(t, "a@example.test", "a-new-password-1"); err != nil {
		t.Errorf("login with reset password: %v", err)
	}
}

func TestSessionsRecoverAdminClearsSessionsAndLockout(t *testing.T) {
	t.Setenv(authengine.EnvLoginGraceFailures, "1")
	t.Setenv(authengine.EnvLoginUserMaxFailures, "2")
	e := newSessEnv(t)
	ctx := context.Background()
	u := e.user(t, "user_a", "a@example.test")
	tok, _ := e.login(t, "a@example.test", sessPassword)
	for i := 0; i < 4; i++ {
		_, _ = e.login(t, "a@example.test", "wrong-password-1")
	}
	if _, err := e.login(t, "a@example.test", sessPassword); err == nil {
		t.Fatal("expected the account to be locked before recovery")
	}
	if err := authengine.RecoverAdmin(ctx, e.db.DB, u, "recovered-pass-9"); err != nil {
		t.Fatalf("RecoverAdmin: %v", err)
	}
	if _, ok := e.sess.Lookup(ctx, tok); ok {
		t.Error("recover-admin left an old session alive")
	}
	if _, err := e.login(t, "a@example.test", "recovered-pass-9"); err != nil {
		t.Errorf("login after recovery: %v", err)
	}
}

func TestSessionsSyncHooksRaceFree(t *testing.T) {
	e := newSessEnv(t)
	ctx := context.Background()
	const n = 12
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		u := e.user(t, fmt.Sprintf("user_%02d", i), fmt.Sprintf("u%02d@example.test", i))
		for range 3 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if _, err := e.sess.SyncUser(ctx, u); err != nil {
					t.Errorf("SyncUser: %v", err)
				}
				if _, err := e.sess.SyncPassword(ctx, u.ID, u.PasswordHash); err != nil {
					t.Errorf("SyncPassword: %v", err)
				}
			}()
		}
	}
	wg.Wait()
	var maps, users int
	_ = e.db.QueryRow(`SELECT COUNT(*) FROM authengine_user_map`).Scan(&maps)
	_ = e.db.QueryRow(`SELECT COUNT(*) FROM theauth_users`).Scan(&users)
	if maps != n || users != n {
		t.Fatalf("maps %d users %d, want %d each", maps, users, n)
	}
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("user_%02d", i)
		engine, err := e.sess.EngineIDFor(ctx, id)
		if err != nil || engine == "" {
			t.Fatalf("EngineIDFor(%s) = %q, %v", id, engine, err)
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := e.sess.DeleteUser(ctx, engine); err != nil {
				t.Errorf("DeleteUser: %v", err)
			}
		}()
	}
	wg.Wait()
	_ = e.db.QueryRow(`SELECT COUNT(*) FROM theauth_users`).Scan(&users)
	if users != 0 {
		t.Errorf("library users after delete = %d, want 0", users)
	}
}

func TestSessionsRegisterRejectsPlainUsernameAndClosesAfterFirst(t *testing.T) {
	e := newSessEnv(t)
	ctx := context.Background()
	if _, _, err := e.sess.Register(ctx, authengine.RegisterInput{Email: "admin", Password: sessPassword, IP: "198.51.100.9"}); !errors.Is(err, authengine.ErrNotAnAddress) {
		t.Fatalf("Register(admin) error = %v, want ErrNotAnAddress", err)
	}
	tok, engineID, err := e.sess.Register(ctx, authengine.RegisterInput{Email: "first@example.test", Password: sessPassword, IP: "198.51.100.9"})
	if err != nil || tok == "" || engineID == "" {
		t.Fatalf("Register = %q %q %v", tok, engineID, err)
	}
	if _, _, err := e.sess.Register(ctx, authengine.RegisterInput{Email: "second@example.test", Password: sessPassword, IP: "198.51.100.9"}); err == nil {
		t.Error("library signup stayed open after the first user")
	}
}

package api

import (
	"context"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/authengine"
)

type capturedMail struct {
	mu   sync.Mutex
	mail []struct{ to, subject, body string }
}

func (c *capturedMail) Send(_ context.Context, to, subject, body string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.mail = append(c.mail, struct{ to, subject, body string }{to, subject, body})
	return nil
}

func (c *capturedMail) last() (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.mail) == 0 {
		return "", false
	}
	return c.mail[len(c.mail)-1].body, true
}

func TestAuthLibSessionsAreInTheLibraryTables(t *testing.T) {
	e := newGoldenEnv(t, true)
	seedStandard(t, e)
	_, cookie := e.do(t, call{step: "login", method: "POST", path: "/api/v1/auth/login", body: loginBody(goldenRootEmail, goldenRootPass)})
	var n int
	if err := e.db.QueryRow(`SELECT COUNT(*) FROM theauth_sessions WHERE revoked_at IS NULL`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("library sessions = %d, err %v; want 1", n, err)
	}
	if e.rt.sessions.lib == nil {
		t.Fatal("session store is not library-backed")
	}
	e.rt.sessions.mu.Lock()
	inMemory := len(e.rt.sessions.sessions)
	e.rt.sessions.mu.Unlock()
	if inMemory != 0 {
		t.Errorf("in-memory sessions = %d, want 0 in library mode", inMemory)
	}
	e.do(t, call{step: "logout", method: "POST", path: "/api/v1/auth/logout", cookie: cookie})
}

func TestAuthLibSessionsLegacyModeUnchanged(t *testing.T) {
	e := newGoldenEnv(t, false)
	if e.rt.libSessions != nil || e.rt.sessions.lib != nil {
		t.Fatal("legacy mode must not attach the library")
	}
	if _, ok := e.rt.auth.(*libSyncedAuth); ok {
		t.Fatal("legacy mode must not wrap the user store")
	}
}

func TestAuthLibSessionsAuditEvents(t *testing.T) {
	e := newGoldenEnv(t, true)
	seedStandard(t, e)
	e.do(t, call{step: "bad", method: "POST", path: "/api/v1/auth/login", body: loginBody(goldenRootEmail, "nope-nope-1")})
	_, cookie := e.do(t, call{step: "login", method: "POST", path: "/api/v1/auth/login", body: loginBody(goldenRootEmail, goldenRootPass)})
	e.do(t, call{step: "change", method: "PUT", path: "/api/v1/auth/password", cookie: cookie, body: `{"current_password":"` + goldenRootPass + `","new_password":"brand-new-pass-2"}`})
	e.do(t, call{step: "logout", method: "POST", path: "/api/v1/auth/logout", cookie: cookie})

	want := []string{
		"POST /api/v1/auth/login 200",
		"POST /api/v1/auth/login 401",
		"PUT /api/v1/auth/password 204",
		"POST /api/v1/auth/logout 204",
	}
	deadline := time.Now().Add(3 * time.Second)
	var got []string
	for time.Now().Before(deadline) {
		got = got[:0]
		for _, r := range e.audits.snapshot() {
			got = append(got, r.Method+" "+r.Path+" "+strconv.Itoa(r.StatusCode))
		}
		if hasAll(got, want) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	sort.Strings(got)
	t.Fatalf("audit rows %v missing some of %v", got, want)
}

func hasAll(got, want []string) bool {
	for _, w := range want {
		found := false
		for _, g := range got {
			found = found || g == w
		}
		if !found {
			return false
		}
	}
	return true
}

func TestAuthLibSessionsPasswordResetFlow(t *testing.T) {
	t.Setenv(authengine.EnvEngine, authengine.EngineLibrary)
	e := newGoldenEnv(t, true)
	mail := &capturedMail{}
	e.rt.emailSender = mail
	seedStandard(t, e)
	_, old := e.do(t, call{step: "login", method: "POST", path: "/api/v1/auth/login", body: loginBody(goldenRootEmail, goldenRootPass)})

	forgot, _ := e.do(t, call{step: "forgot", method: "POST", path: "/api/v1/auth/forgot-password", body: `{"email":"` + goldenRootEmail + `"}`})
	if forgot.Status != http.StatusNoContent {
		t.Fatalf("forgot status = %d, want 204", forgot.Status)
	}
	var body string
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if b, ok := mail.last(); ok {
			body = b
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	const marker = "/reset-password?token="
	_, rest, ok := strings.Cut(body, marker)
	if !ok {
		t.Fatalf("reset email %q has no platform reset link", body)
	}
	token := strings.Fields(rest)[0]

	short, _ := e.do(t, call{step: "short", method: "POST", path: "/api/v1/auth/reset-password", body: `{"token":"` + token + `","new_password":"short"}`})
	if short.Status != http.StatusBadRequest {
		t.Errorf("short password status = %d, want 400", short.Status)
	}
	reset, _ := e.do(t, call{step: "reset", method: "POST", path: "/api/v1/auth/reset-password", body: `{"token":"` + token + `","new_password":"after-reset-pass-3"}`})
	if reset.Status != http.StatusNoContent {
		t.Fatalf("reset status = %d body %q, want 204", reset.Status, reset.Body)
	}
	again, _ := e.do(t, call{step: "reuse", method: "POST", path: "/api/v1/auth/reset-password", body: `{"token":"` + token + `","new_password":"after-reset-pass-4"}`})
	if again.Status != http.StatusBadRequest || !strings.Contains(again.Body, "invalid or expired reset token") {
		t.Errorf("reused token = %d %q, want the legacy 400 body", again.Status, again.Body)
	}
	after, _ := e.do(t, call{step: "old session", method: "GET", path: "/api/v1/auth/session", cookie: old})
	if after.Status != http.StatusUnauthorized {
		t.Errorf("old session status = %d, want 401 after reset", after.Status)
	}
	relogin, _ := e.do(t, call{step: "relogin", method: "POST", path: "/api/v1/auth/login", body: loginBody(goldenRootEmail, "after-reset-pass-3")})
	if relogin.Status != http.StatusOK {
		t.Errorf("login with reset password = %d, want 200", relogin.Status)
	}
	hashed, err := e.db.GetUserByEmail(context.Background(), goldenRootEmail)
	if err != nil || hashed.PasswordHash == nil || !strings.HasPrefix(*hashed.PasswordHash, "$2") {
		t.Errorf("platform hash after reset = %v err %v, want bcrypt so rollback to legacy works", hashed.PasswordHash, err)
	}
}

func TestAuthLibSessionsPlainUsernameRegistration(t *testing.T) {
	e := newGoldenEnv(t, true)
	body := `{"username":"admin","password":"a-real-password","setup_token":"` + e.token + `"}`
	ok, cookie := e.do(t, call{step: "register", method: "POST", path: "/api/v1/auth/register", body: body})
	if ok.Status != http.StatusCreated || cookie == "" {
		t.Fatalf("register admin = %d %q, want 201 with a cookie", ok.Status, ok.Body)
	}
	session, _ := e.do(t, call{step: "session", method: "GET", path: "/api/v1/auth/session", cookie: cookie})
	if session.Status != http.StatusOK {
		t.Errorf("session after plain-username registration = %d, want 200", session.Status)
	}
	login, _ := e.do(t, call{step: "login", method: "POST", path: "/api/v1/auth/login", body: loginBody("admin", "a-real-password")})
	if login.Status != http.StatusOK {
		t.Errorf("login as plain username = %d, want 200", login.Status)
	}
}

func TestAuthLibSessionsUnknownUserTimingMatchesWrongPassword(t *testing.T) {
	e := newGoldenEnv(t, true)
	seedStandard(t, e)
	median := func(email string) time.Duration {
		var ds []time.Duration
		for i := 0; i < 3; i++ {
			start := time.Now()
			e.do(t, call{step: "t", method: "POST", path: "/api/v1/auth/login", body: loginBody(email, "nope-nope-1")})
			ds = append(ds, time.Since(start))
			if _, err := e.db.Exec(`DELETE FROM theauth_throttle_entries`); err != nil {
				t.Fatalf("clear throttle: %v", err)
			}
		}
		sort.Slice(ds, func(i, j int) bool { return ds[i] < ds[j] })
		return ds[1]
	}
	wrong := median(goldenRootEmail)
	unknown := median("ghost@example.test")
	if unknown < wrong/4 {
		t.Errorf("unknown user took %v vs wrong password %v: the dummy hash path is too cheap", unknown, wrong)
	}
}

func TestAuthLibStreamWatchFiresWhenSessionRevoked(t *testing.T) {
	e := newGoldenEnv(t, true)
	t.Setenv(authengine.EnvStreamWatchInterval, "20ms")
	seedStandard(t, e)
	_, cookie := e.do(t, call{step: "login", method: "POST", path: "/api/v1/auth/login", body: loginBody(goldenRootEmail, goldenRootPass)})

	req, _ := http.NewRequest(http.MethodGet, "/x", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: cookie}) //nolint:gosec // request cookie, attributes only apply to Set-Cookie
	fired := make(chan struct{}, 1)
	stop := e.rt.watchLibSession(req, func() { fired <- struct{}{} })
	defer stop()
	e.do(t, call{step: "logout", method: "POST", path: "/api/v1/auth/logout", cookie: cookie})
	select {
	case <-fired:
	case <-time.After(3 * time.Second):
		t.Fatal("stream watcher did not fire after the session was revoked")
	}
}

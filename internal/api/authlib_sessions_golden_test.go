package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/authengine"
	"github.com/GLINCKER/levelrail/internal/store"
	"golang.org/x/crypto/bcrypt"
)

const (
	goldenRootEmail = "root@example.test"
	goldenRootPass  = "root-password-1"
	goldenUserEmail = "dev@example.test"
	goldenUserPass  = "dev-password-1"
)

type goldenEnv struct {
	rt      *Router
	db      *store.DB
	dataDir string
	token   string
	audits  *auditCollector
}

type auditCollector struct {
	mu   sync.Mutex
	recs []authengine.AuditRecord
}

func (a *auditCollector) add(_ context.Context, r authengine.AuditRecord) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.recs = append(a.recs, r)
}

func (a *auditCollector) snapshot() []authengine.AuditRecord {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]authengine.AuditRecord(nil), a.recs...)
}

func newGoldenEnv(t *testing.T) *goldenEnv {
	t.Helper()
	db := openTestDB(t)
	dataDir := t.TempDir()
	token, _, err := EnsureSetupToken(context.Background(), db, dataDir)
	if err != nil {
		t.Fatalf("EnsureSetupToken() error = %v", err)
	}
	env := &goldenEnv{db: db, dataDir: dataDir, token: token, audits: &auditCollector{}}
	eng, err := authengine.New(db.DB, authengine.Config{
		BaseURL:   "http://example.test",
		Directory: authengine.NewDirectory(db.DB),
		Sessions:  authengine.SessionsHooks{Mail: &authengine.MailRelay{}, Audit: env.audits.add},
	})
	if err != nil {
		t.Fatalf("authengine.New() error = %v", err)
	}
	t.Cleanup(eng.Close)
	env.rt = NewRouter(discardLogger(), testBrand(), db, WithDataDir(dataDir), WithAuthEngine(eng))
	return env
}

func (e *goldenEnv) seed(t *testing.T, email, pass string, abilities []string, first bool) {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(pass), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	h := string(hash)
	id, err := randomOpaqueID("user_")
	if err != nil {
		t.Fatalf("id: %v", err)
	}
	u := store.User{ID: id, Email: email, DisplayName: email, PasswordHash: &h, Abilities: abilities, IsFirstUser: first, CreatedAt: time.Now()}
	if err := e.db.CreateUser(context.Background(), u); err != nil {
		t.Fatalf("seed user: %v", err)
	}
}

type exchange struct {
	Step    string
	Status  int
	Body    string
	Cookies []string
	Retry   bool
}

var volatileKeys = map[string]bool{"expires_at": true, "token": true, "url": true, "id": true}

func normalizeSessionBody(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return raw
	}
	for k := range volatileKeys {
		if _, ok := m[k]; ok {
			m[k] = "<volatile>"
		}
	}
	out, _ := json.Marshal(m)
	return string(out)
}

func cookieShape(c *http.Cookie) string {
	state := "set"
	if c.Value == "" {
		state = "cleared"
	}
	return fmt.Sprintf("%s=%s path=%s httponly=%t secure=%t samesite=%d expires=%t maxage_neg=%t",
		c.Name, state, c.Path, c.HttpOnly, c.Secure, c.SameSite, !c.Expires.IsZero(), c.MaxAge < 0)
}

type call struct {
	step, method, path, body, cookie string
}

func (e *goldenEnv) do(t *testing.T, c call) (exchange, string) {
	t.Helper()
	req := httptest.NewRequest(c.method, c.path, strings.NewReader(c.body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "203.0.113.7:4444"
	if c.cookie != "" {
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: c.cookie}) //nolint:gosec // request cookie, attributes only apply to Set-Cookie
	}
	rec := httptest.NewRecorder()
	e.rt.Handler().ServeHTTP(rec, req)
	res := rec.Result()
	body, _ := io.ReadAll(res.Body)
	ex := exchange{Step: c.step, Status: res.StatusCode, Body: normalizeSessionBody(string(body)), Retry: res.Header.Get("Retry-After") != ""}
	newCookie := ""
	for _, ck := range res.Cookies() {
		ex.Cookies = append(ex.Cookies, cookieShape(ck))
		if ck.Name == sessionCookieName && ck.Value != "" {
			newCookie = ck.Value
		}
	}
	sort.Strings(ex.Cookies)
	return ex, newCookie
}

func loginBody(email, pass string) string {
	b, _ := json.Marshal(map[string]string{"username": email, "password": pass})
	return string(b)
}

type scenario func(t *testing.T, e *goldenEnv) []exchange

func runSessionGolden(t *testing.T, name string, sc scenario) {
	t.Helper()
	t.Run(name, func(t *testing.T) {
		got := sc(t, newGoldenEnv(t))
		path := "testdata/session_golden_" + strings.ReplaceAll(name, " ", "_") + ".json"
		if os.Getenv("APP_UPDATE_GOLDEN") == "1" {
			raw, err := json.MarshalIndent(got, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, append(raw, '\n'), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		raw, err := os.ReadFile(path) //nolint:gosec // fixed testdata path
		if err != nil {
			t.Fatalf("read golden: %v", err)
		}
		var want []exchange
		if err := json.Unmarshal(raw, &want); err != nil {
			t.Fatalf("decode golden: %v", err)
		}
		if len(want) != len(got) {
			t.Fatalf("produced %d exchanges, want %d", len(got), len(want))
		}
		for i := range want {
			if !reflect.DeepEqual(want[i], got[i]) {
				t.Errorf("step %q differs\n  got:  %+v\n  want: %+v", want[i].Step, got[i], want[i])
			}
		}
	})
}

func seedStandard(t *testing.T, e *goldenEnv) {
	e.seed(t, goldenRootEmail, goldenRootPass, []string{AbilityRoot}, true)
	e.seed(t, goldenUserEmail, goldenUserPass, []string{AbilityRead}, false)
}

func TestAuthLibSessionsGolden(t *testing.T) {
	runSessionGolden(t, "login ok", func(t *testing.T, e *goldenEnv) []exchange {
		seedStandard(t, e)
		login, cookie := e.do(t, call{step: "login", method: "POST", path: "/api/v1/auth/login", body: loginBody(goldenRootEmail, goldenRootPass)})
		session, _ := e.do(t, call{step: "session", method: "GET", path: "/api/v1/auth/session", cookie: cookie})
		whoami, _ := e.do(t, call{step: "whoami", method: "GET", path: "/api/v1/auth/whoami", cookie: cookie})
		return []exchange{login, session, whoami}
	})

	runSessionGolden(t, "wrong password and unknown user", func(t *testing.T, e *goldenEnv) []exchange {
		seedStandard(t, e)
		wrong, _ := e.do(t, call{step: "wrong password", method: "POST", path: "/api/v1/auth/login", body: loginBody(goldenRootEmail, "nope-nope-1")})
		unknown, _ := e.do(t, call{step: "unknown user", method: "POST", path: "/api/v1/auth/login", body: loginBody("ghost@example.test", "nope-nope-1")})
		missing, _ := e.do(t, call{step: "missing fields", method: "POST", path: "/api/v1/auth/login", body: `{"username":"","password":""}`})
		badJSON, _ := e.do(t, call{step: "bad json", method: "POST", path: "/api/v1/auth/login", body: `{`})
		return []exchange{wrong, unknown, missing, badJSON}
	})

	runSessionGolden(t, "rate limited", func(t *testing.T, e *goldenEnv) []exchange {
		seedStandard(t, e)
		var out []exchange
		for i := 1; i <= 6; i++ {
			ex, _ := e.do(t, call{step: fmt.Sprintf("attempt %d", i), method: "POST", path: "/api/v1/auth/login", body: loginBody(goldenRootEmail, "nope-nope-1")})
			out = append(out, ex)
		}
		return out
	})

	runSessionGolden(t, "logout and session after logout", func(t *testing.T, e *goldenEnv) []exchange {
		seedStandard(t, e)
		_, cookie := e.do(t, call{step: "login", method: "POST", path: "/api/v1/auth/login", body: loginBody(goldenRootEmail, goldenRootPass)})
		logout, _ := e.do(t, call{step: "logout", method: "POST", path: "/api/v1/auth/logout", cookie: cookie})
		after, _ := e.do(t, call{step: "session after logout", method: "GET", path: "/api/v1/auth/session", cookie: cookie})
		again, _ := e.do(t, call{step: "logout again", method: "POST", path: "/api/v1/auth/logout", cookie: cookie})
		return []exchange{logout, after, again}
	})

	runSessionGolden(t, "setup token registration", func(t *testing.T, e *goldenEnv) []exchange {
		status1, _ := e.do(t, call{step: "setup status before", method: "GET", path: "/api/v1/auth/setup-status"})
		badToken, _ := e.do(t, call{step: "bad token", method: "POST", path: "/api/v1/auth/register", body: `{"username":"first@example.test","password":"a-real-password","setup_token":"wrong"}`})
		short, _ := e.do(t, call{step: "short password", method: "POST", path: "/api/v1/auth/register", body: `{"username":"first@example.test","password":"short","setup_token":"` + e.token + `"}`})
		ok, cookie := e.do(t, call{step: "register", method: "POST", path: "/api/v1/auth/register", body: `{"username":"first@example.test","password":"a-real-password","setup_token":"` + e.token + `"}`})
		session, _ := e.do(t, call{step: "session", method: "GET", path: "/api/v1/auth/session", cookie: cookie})
		second, _ := e.do(t, call{step: "second register", method: "POST", path: "/api/v1/auth/register", body: `{"username":"other@example.test","password":"a-real-password","setup_token":"` + e.token + `"}`})
		status2, _ := e.do(t, call{step: "setup status after", method: "GET", path: "/api/v1/auth/setup-status"})
		relogin, _ := e.do(t, call{step: "login with new account", method: "POST", path: "/api/v1/auth/login", body: loginBody("first@example.test", "a-real-password")})
		return []exchange{status1, badToken, short, ok, session, second, status2, relogin}
	})

	runSessionGolden(t, "session link consume", func(t *testing.T, e *goldenEnv) []exchange {
		seedStandard(t, e)
		_, root := e.do(t, call{step: "root login", method: "POST", path: "/api/v1/auth/login", body: loginBody(goldenRootEmail, goldenRootPass)})
		mint, _ := e.do(t, call{step: "mint", method: "POST", path: "/api/v1/auth/session-links", cookie: root})
		var minted mintSessionLinkResponse
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/session-links", nil)
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: root}) //nolint:gosec // request cookie, attributes only apply to Set-Cookie
		e.rt.Handler().ServeHTTP(rec, req)
		if err := json.Unmarshal(rec.Body.Bytes(), &minted); err != nil || minted.Token == "" {
			t.Fatalf("mint response = %q, err %v", rec.Body.String(), err)
		}
		consume, linked := e.do(t, call{step: "consume", method: "GET", path: "/api/v1/auth/session-links/" + minted.Token + "/consume"})
		session, _ := e.do(t, call{step: "linked session", method: "GET", path: "/api/v1/auth/session", cookie: linked})
		reuse, _ := e.do(t, call{step: "consume again", method: "GET", path: "/api/v1/auth/session-links/" + minted.Token + "/consume"})
		unknown, _ := e.do(t, call{step: "consume unknown", method: "GET", path: "/api/v1/auth/session-links/not-a-real-token/consume"})
		return []exchange{mint, consume, session, reuse, unknown}
	})

	runSessionGolden(t, "deleted user session rejected", func(t *testing.T, e *goldenEnv) []exchange {
		seedStandard(t, e)
		_, root := e.do(t, call{step: "root login", method: "POST", path: "/api/v1/auth/login", body: loginBody(goldenRootEmail, goldenRootPass)})
		_, dev := e.do(t, call{step: "dev login", method: "POST", path: "/api/v1/auth/login", body: loginBody(goldenUserEmail, goldenUserPass)})
		before, _ := e.do(t, call{step: "dev session before", method: "GET", path: "/api/v1/auth/session", cookie: dev})
		devUser, err := e.db.GetUserByEmail(context.Background(), goldenUserEmail)
		if err != nil {
			t.Fatalf("GetUserByEmail() error = %v", err)
		}
		del, _ := e.do(t, call{step: "delete", method: "DELETE", path: "/api/v1/users/" + devUser.ID, cookie: root})
		after, _ := e.do(t, call{step: "dev session after delete", method: "GET", path: "/api/v1/auth/session", cookie: dev})
		relogin, _ := e.do(t, call{step: "dev login after delete", method: "POST", path: "/api/v1/auth/login", body: loginBody(goldenUserEmail, goldenUserPass)})
		return []exchange{before, del, after, relogin}
	})

	runSessionGolden(t, "password change revokes other sessions", func(t *testing.T, e *goldenEnv) []exchange {
		seedStandard(t, e)
		_, a := e.do(t, call{step: "login a", method: "POST", path: "/api/v1/auth/login", body: loginBody(goldenRootEmail, goldenRootPass)})
		_, b := e.do(t, call{step: "login b", method: "POST", path: "/api/v1/auth/login", body: loginBody(goldenRootEmail, goldenRootPass)})
		change, _ := e.do(t, call{step: "change password", method: "PUT", path: "/api/v1/auth/password", cookie: a, body: `{"current_password":"` + goldenRootPass + `","new_password":"brand-new-pass-2"}`})
		aAfter, _ := e.do(t, call{step: "session a", method: "GET", path: "/api/v1/auth/session", cookie: a})
		bAfter, _ := e.do(t, call{step: "session b", method: "GET", path: "/api/v1/auth/session", cookie: b})
		oldPw, _ := e.do(t, call{step: "old password", method: "POST", path: "/api/v1/auth/login", body: loginBody(goldenRootEmail, goldenRootPass)})
		newPw, _ := e.do(t, call{step: "new password", method: "POST", path: "/api/v1/auth/login", body: loginBody(goldenRootEmail, "brand-new-pass-2")})
		return []exchange{change, aAfter, bAfter, oldPw, newPw}
	})
}

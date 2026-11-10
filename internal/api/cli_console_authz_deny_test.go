package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/cliconsole"
	"github.com/GLINCKER/levelrail/internal/experimental"
	"github.com/GLINCKER/levelrail/internal/store"
)

// authzDenyRoute is one registered route of a feature plus the weakest ability that may reach its handler.
type authzDenyRoute struct {
	method  string
	pattern string
	body    string
	ability string // AbilityRoot, AbilityRead, or "" for a route that is public by design
	stepUp  bool
}

func (r authzDenyRoute) key() string  { return r.method + " " + r.pattern }
func (r authzDenyRoute) path() string { return concretePathFor(r.pattern, "", "x1") }

// authzDenyActors are the callers every feature table is run against, built once per test on one router.
type authzDenyActors struct {
	rt                                      *Router
	db                                      *store.DB
	admin, reader, dev, nobody              *http.Cookie
	rootTok, readTok, devTok, deployOnlyTok string
	expiredTok, revokedTok                  string
	secrets                                 []string
	bodies                                  []string
}

func newAuthzDenyActors(t *testing.T) *authzDenyActors {
	t.Helper()
	experimental.Set(experimental.All()...)
	t.Cleanup(enableAllExperimental)
	rt, db := newTestRouter(t)
	a := &authzDenyActors{rt: rt, db: db, admin: loginTestSession(t, rt, db)}
	cookieFor := func(email string, abilities []string) *http.Cookie {
		return sessionCookieForTest(t, rt, storeUserWithAbilitiesForTest(t, db, email, abilities).ID)
	}
	a.reader = cookieFor("deny-reader@example.com", []string{AbilityRead})
	a.dev = cookieFor("deny-dev@example.com", []string{AbilityRead, AbilityWrite, AbilityDeploy})
	a.nobody = cookieFor("deny-nobody@example.com", []string{})
	past, future := time.Now().Add(-time.Hour), time.Now().Add(time.Hour)
	seed := func(id string, abilities []string, expires *time.Time) string {
		plain := "authz-deny-secret-" + id
		err := db.SaveAPIToken(context.Background(), store.APIToken{
			ID: id, Name: id, TokenHash: hashToken(plain), Abilities: abilities, ExpiresAt: expires,
		})
		if err != nil {
			t.Fatalf("seed token %s: %v", id, err)
		}
		a.secrets = append(a.secrets, plain)
		return plain
	}
	// RevokeAPIToken is the real revoke path; SaveAPIToken does not persist RevokedAt.
	revokeNow := func(id string) {
		if err := db.RevokeAPIToken(context.Background(), id); err != nil {
			t.Fatalf("revoke token %s: %v", id, err)
		}
	}
	a.rootTok = seed("deny-root", []string{AbilityRoot}, &future)
	a.readTok = seed("deny-read", []string{AbilityRead}, &future)
	a.devTok = seed("deny-dev", []string{AbilityRead, AbilityWrite, AbilityDeploy}, &future)
	a.deployOnlyTok = seed("deny-deploy", []string{AbilityDeploy}, &future)
	a.expiredTok = seed("deny-expired", []string{AbilityRoot}, &past)
	a.revokedTok = seed("deny-revoked", []string{AbilityRoot}, &future)
	revokeNow("deny-revoked")
	for _, c := range []*http.Cookie{a.admin, a.reader, a.dev, a.nobody} {
		a.secrets = append(a.secrets, c.Value)
	}
	return a
}

func (a *authzDenyActors) do(method, path, body string, cookie *http.Cookie, token, origin string) *httptest.ResponseRecorder {
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, path, nil)
	} else {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
	}
	r.Host = "example.com"
	r.RemoteAddr = "198.51.100.7:4321"
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	if cookie != nil {
		r.AddCookie(cookie)
	}
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	a.rt.Handler().ServeHTTP(rec, r)
	a.bodies = append(a.bodies, rec.Body.String())
	return rec
}

// assertNoSecrets proves no response body and no audit row carried a token, cookie or code value.
func (a *authzDenyActors) assertNoSecrets(t *testing.T) {
	t.Helper()
	entries, err := a.db.ListAuditEntries(context.Background(), 1000, nil, store.AuditEntryFilter{})
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	audit := fmt.Sprintf("%+v", entries)
	for _, s := range a.secrets {
		for _, b := range a.bodies {
			if strings.Contains(b, s) {
				t.Fatalf("a response body leaked a credential value (len %d): %.80s", len(s), b)
			}
		}
		if strings.Contains(audit, s) {
			t.Fatalf("an audit entry leaked a credential value (len %d)", len(s))
		}
	}
}

// authzDenyRegistered lists "METHOD pattern" for every registered route the matcher selects.
func authzDenyRegistered(t *testing.T, match func(method, pattern string) bool) []string {
	t.Helper()
	var out []string
	for _, r := range loadMatrixRoutes(t) {
		if match(r.method, r.pattern) {
			out = append(out, r.key())
		}
	}
	sort.Strings(out)
	return out
}

// authzDenyCovered fails when a registered route has no table row, or a row names a route that is gone.
func authzDenyCovered(t *testing.T, match func(method, pattern string) bool, table []authzDenyRoute) {
	t.Helper()
	have := map[string]bool{}
	for _, r := range table {
		have[r.key()] = true
	}
	reg := authzDenyRegistered(t, match)
	if len(reg) == 0 {
		t.Fatal("matcher selected no registered routes")
	}
	for _, k := range reg {
		if !have[k] {
			t.Errorf("registered route %q has no authz deny case: add it to the table", k)
		}
		delete(have, k)
	}
	for k := range have {
		t.Errorf("table row %q is not a registered route", k)
	}
}

type authzDenyFeature struct {
	name   string
	match  func(method, pattern string) bool
	routes []authzDenyRoute
	origin string
	stepUp func(t *testing.T, rt *Router, cookie *http.Cookie)
}

// runAuthzDeny drives one feature table through every caller class and then proves nothing leaked.
func runAuthzDeny(t *testing.T, f authzDenyFeature) {
	a := newAuthzDenyActors(t)
	t.Run("every registered route has a case", func(t *testing.T) { authzDenyCovered(t, f.match, f.routes) })

	refuse := func(t *testing.T, r authzDenyRoute, who string, rec *httptest.ResponseRecorder, want int) {
		t.Helper()
		if rec.Code != want {
			t.Errorf("%s as %s: status = %d, want %d, body = %.200s", r.key(), who, rec.Code, want, rec.Body.String())
		}
	}
	t.Run("unauthenticated or stale credentials get 401", func(t *testing.T) {
		for _, r := range f.routes {
			if r.ability == "" {
				if rec := a.do(r.method, r.path(), r.body, nil, "", f.origin); rec.Code >= 500 || rec.Code < 400 {
					t.Errorf("%s public route with no credential = %d, want a 4xx refusal", r.key(), rec.Code)
				}
				continue
			}
			refuse(t, r, "anonymous", a.do(r.method, r.path(), r.body, nil, "", f.origin), http.StatusUnauthorized)
			refuse(t, r, "expired token", a.do(r.method, r.path(), r.body, nil, a.expiredTok, f.origin), http.StatusUnauthorized)
			refuse(t, r, "revoked token", a.do(r.method, r.path(), r.body, nil, a.revokedTok, f.origin), http.StatusUnauthorized)
			refuse(t, r, "garbage token", a.do(r.method, r.path(), r.body, nil, "not-a-token", f.origin), http.StatusUnauthorized)
		}
	})
	t.Run("insufficient abilities get 403", func(t *testing.T) {
		for _, r := range f.routes {
			if r.ability == "" {
				continue
			}
			lacking := map[string]struct {
				cookie *http.Cookie
				token  string
			}{"no-ability session": {a.nobody, ""}, "deploy-only token": {nil, a.deployOnlyTok}}
			if r.ability == AbilityRoot {
				lacking["read session"] = struct {
					cookie *http.Cookie
					token  string
				}{a.reader, ""}
				lacking["developer session"] = struct {
					cookie *http.Cookie
					token  string
				}{a.dev, ""}
				lacking["read token"] = struct {
					cookie *http.Cookie
					token  string
				}{nil, a.readTok}
				lacking["developer token"] = struct {
					cookie *http.Cookie
					token  string
				}{nil, a.devTok}
			}
			for who, c := range lacking {
				refuse(t, r, who, a.do(r.method, r.path(), r.body, c.cookie, c.token, f.origin), http.StatusForbidden)
			}
		}
	})
	t.Run("step-up routes refuse a root caller without a fresh step-up", func(t *testing.T) {
		for _, r := range f.routes {
			if !r.stepUp {
				continue
			}
			refuse(t, r, "admin without step-up", a.do(r.method, r.path(), r.body, a.admin, "", f.origin), http.StatusForbidden)
			refuse(t, r, "root token without step-up", a.do(r.method, r.path(), r.body, nil, a.rootTok, f.origin), http.StatusForbidden)
		}
	})
	t.Run("admin reaches the handler", func(t *testing.T) {
		if f.stepUp != nil {
			f.stepUp(t, a.rt, a.admin)
		}
		for _, r := range f.routes {
			if r.ability == "" {
				continue
			}
			rec := a.do(r.method, r.path(), r.body, a.admin, "", f.origin)
			if rec.Code == http.StatusUnauthorized || rec.Code == http.StatusForbidden || rec.Code >= http.StatusInternalServerError {
				t.Errorf("%s as admin = %d, want a handler answer below 500 that is not 401 or 403, body = %.200s", r.key(), rec.Code, rec.Body.String())
			}
		}
	})
	a.assertNoSecrets(t)
}

func cliConsoleDenyFeature() authzDenyFeature {
	return authzDenyFeature{
		name:  "cli-console",
		match: func(_, p string) bool { return strings.HasPrefix(p, "/api/v1/system/cli-console/") },
		routes: []authzDenyRoute{
			{http.MethodGet, "/api/v1/system/cli-console/commands", "", AbilityRoot, true},
			{http.MethodGet, "/api/v1/system/cli-console/session", "", AbilityRoot, true},
		},
		origin: "http://example.com",
		stepUp: cliConsoleStepUp,
	}
}

func TestAuthzDeny_CLIConsole(t *testing.T) {
	t.Setenv(cliconsole.EnvBinary, "/nonexistent/cli")
	runAuthzDeny(t, cliConsoleDenyFeature())
}

// Flag off and the kill switch answer 409 before the origin or step-up gates do any work.
func TestAuthzDeny_CLIConsoleGateOff(t *testing.T) {
	t.Setenv(cliconsole.EnvBinary, "/nonexistent/cli")
	a := newAuthzDenyActors(t)
	for name, setup := range map[string]func(){
		"flag off":    func() { experimental.Set() },
		"kill switch": func() { experimental.Set(experimental.CLIConsole); t.Setenv(cliconsole.EnvKillSwitch, "off") },
	} {
		setup()
		for _, r := range cliConsoleDenyFeature().routes {
			rec := a.do(r.method, r.path(), "", a.admin, "", "http://example.com")
			if rec.Code != http.StatusConflict || errorCode(t, rec) != ExperimentalDisabledCode {
				t.Errorf("%s %s = %d %s, want 409 %s", name, r.key(), rec.Code, rec.Body.String(), ExperimentalDisabledCode)
			}
			if rec := a.do(r.method, r.path(), "", a.reader, "", "http://example.com"); rec.Code != http.StatusForbidden {
				t.Errorf("%s %s as reader = %d, want 403 before the gate", name, r.key(), rec.Code)
			}
		}
	}
	a.assertNoSecrets(t)
}

// An API token never opens the console, even a root one with a valid origin.
func TestAuthzDeny_CLIConsoleRefusesAPITokens(t *testing.T) {
	t.Setenv(cliconsole.EnvBinary, "/nonexistent/cli")
	a := newAuthzDenyActors(t)
	for _, r := range cliConsoleDenyFeature().routes {
		if rec := a.do(r.method, r.path(), "", nil, a.rootTok, "http://example.com"); rec.Code != http.StatusForbidden {
			t.Errorf("%s with a root API token = %d %s, want 403", r.key(), rec.Code, rec.Body.String())
		}
	}
	a.assertNoSecrets(t)
}

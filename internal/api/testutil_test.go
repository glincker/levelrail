package api

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/brand"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	testAdminUsername = "admin"
	testAdminPassword = "correct-horse-battery-staple"
)

// openTestDB opens a fresh temp-file SQLite store, the same pattern
// internal/store's own tests use (internal/store/store_test.go), not a
// mock: the project favors real behavior under test, and the store
// package is fast and local enough that a mock buys nothing here.
func openTestDB(t *testing.T) *store.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "levelrail.db")
	db, err := store.Open(context.Background(), path)
	if err != nil {
		t.Fatalf("store.Open(%q) error = %v", path, err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("closing test db: %v", err)
		}
	})
	return db
}

func testBrand() *brand.Brand {
	return &brand.Brand{Name: "Test Platform", BinaryName: "testplatform"}
}

// newTestRouter builds a Router over a fresh test store, with no admin
// bootstrapped yet: tests that need an authenticated session call
// bootstrapTestAdmin themselves, so tests exercising the "no admin
// exists" path don't need a workaround.
func newTestRouter(t *testing.T) (*Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	return NewRouter(logger, testBrand(), db), db
}

// discardWriter is a minimal io.Writer that throws everything away, so
// test output isn't cluttered by the router's own request logging.
type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }

// discardLogger is the standalone-logger counterpart to newTestRouter's
// own inline discardWriter usage, for tests (deploy_attempts_test.go's
// live-fan-out coverage) that need a *slog.Logger to hand to a component
// built outside NewRouter, e.g. a *deploylog.Recorder constructed
// directly by the test.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(discardWriter{}, nil))
}

func bootstrapTestAdmin(t *testing.T, db *store.DB) {
	t.Helper()
	if err := BootstrapAdmin(context.Background(), db, testAdminUsername, testAdminPassword); err != nil {
		t.Fatalf("BootstrapAdmin() error = %v", err)
	}
}

// loginTestSession bootstraps the admin account (if not already done by
// the caller) and logs in through the real handler, returning the
// session cookie subsequent authenticated requests need. Exercises
// handleLogin itself rather than reaching into the session store
// directly, so auth tests cover the real login path, not a shortcut
// around it.
func loginTestSession(t *testing.T, rt *Router, db *store.DB) *http.Cookie {
	t.Helper()
	bootstrapTestAdmin(t, db)

	body := strings.NewReader(`{"username":"` + testAdminUsername + `","password":"` + testAdminPassword + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", body)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("login setup: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	cookies := rec.Result().Cookies()
	for _, c := range cookies {
		if c.Name == sessionCookieName {
			return c
		}
	}
	t.Fatal("login setup: no session cookie returned")
	return nil
}

// storeUserForTest inserts a plain, no-password user row directly (not
// through a handler): the OAuth-only-account, multi-user, and
// account-linking tests all need a user that didn't come through
// handleRegister/handleLogin's password path.
func storeUserForTest(t *testing.T, db *store.DB, email string) store.User {
	t.Helper()
	id, err := randomOpaqueID("user_")
	if err != nil {
		t.Fatalf("randomOpaqueID() error = %v", err)
	}
	u := store.User{ID: id, Email: email, DisplayName: email, CreatedAt: time.Now()}
	if err := db.CreateUser(context.Background(), u); err != nil {
		t.Fatalf("CreateUser(%q) error = %v", email, err)
	}
	return u
}

// storeUserWithAbilitiesForTest is storeUserForTest plus an explicit
// Abilities value, for tests exercising requireAbility's session branch
// against a user who is deliberately not root.
func storeUserWithAbilitiesForTest(t *testing.T, db *store.DB, email string, abilities []string) store.User {
	t.Helper()
	id, err := randomOpaqueID("user_")
	if err != nil {
		t.Fatalf("randomOpaqueID() error = %v", err)
	}
	u := store.User{ID: id, Email: email, DisplayName: email, Abilities: abilities, CreatedAt: time.Now()}
	if err := db.CreateUser(context.Background(), u); err != nil {
		t.Fatalf("CreateUser(%q) error = %v", email, err)
	}
	return u
}

// sessionCookieForTest mints a real session for userID directly through
// rt.sessions (not through a login handler, since some test users have
// no password to log in with) and wraps it as the http.Cookie
// authedRequest expects.
func sessionCookieForTest(t *testing.T, rt *Router, userID string) *http.Cookie {
	t.Helper()
	token, err := rt.sessions.create(userID)
	if err != nil {
		t.Fatalf("sessions.create(%q) error = %v", userID, err)
	}
	return &http.Cookie{Name: sessionCookieName, Value: token} //nolint:gosec // request cookie, not a response Set-Cookie
}

// routeCase is one route an ability/auth test drives, shared across
// GitHub's, GitLab's, and Bitbucket's own app-connection and
// use-as-source route tests. body is optional: when non-empty the
// request carries it with Content-Type: application/json.
type routeCase struct {
	method string
	path   string
	body   string
}

// assertRoutesRequireAuth proves every route in routes rejects a
// completely unauthenticated request, the shared shape each git
// provider's own RequireAuth test establishes.
func assertRoutesRequireAuth(t *testing.T, rt *Router, routes []routeCase) {
	t.Helper()
	for _, r := range routes {
		var body io.Reader
		if r.body != "" {
			body = strings.NewReader(r.body)
		}
		req := httptest.NewRequest(r.method, r.path, body)
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s: status = %d, want 401 for an unauthenticated request", r.method, r.path, rec.Code)
		}
	}
}

// assertRoutesForbiddenForAbilities seeds a token scoped to abilities and
// proves every route in routes rejects a request bearing it: the shared
// "declared ability doesn't reach this route" shape used across GitHub's,
// GitLab's, and Bitbucket's own app-connection and use-as-source
// endpoints.
func assertRoutesForbiddenForAbilities(t *testing.T, rt *Router, db *store.DB, tokenID, plaintext string, abilities []string, routes []routeCase) {
	t.Helper()
	if err := db.SaveAPIToken(context.Background(), store.APIToken{
		ID: tokenID, Name: "test-token", TokenHash: hashToken(plaintext), Abilities: abilities,
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}
	for _, r := range routes {
		var body io.Reader
		if r.body != "" {
			body = strings.NewReader(r.body)
		}
		req := httptest.NewRequest(r.method, r.path, body)
		if r.body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		req.Header.Set("Authorization", "Bearer "+plaintext)
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s %s: status = %d, want 403 for a token whose ability doesn't reach this route", r.method, r.path, rec.Code)
		}
	}
}

// assertBodyContainsAll proves body contains every one of want, the
// shared multi-substring response-shape assertion used throughout the
// git-provider connect/status handler tests.
func assertBodyContainsAll(t *testing.T, body string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(body, w) {
			t.Errorf("body = %s, want it to contain %s", body, w)
		}
	}
}

// assertProviderStatusNotConnected proves the common "nothing connected
// yet" shape every per-provider status endpoint (GitHub, GitLab,
// Bitbucket) shares: a fresh router and session report connected:false.
func assertProviderStatusNotConnected(t *testing.T, path string) {
	t.Helper()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, path, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	assertBodyContainsAll(t, rec.Body.String(), `"connected":false`)
}

// assertGitSourceWebhookRegistered proves the common "use repo/project as
// source" success shape shared by GitHub's, GitLab's, and Bitbucket's own
// handlers: the fake client's CreateWebhook-equivalent was called with a
// URL ending in the generic push path, and the hook token it received
// matches the git source's own stored webhook secret.
func assertGitSourceWebhookRegistered(t *testing.T, gitSourceSecrets GitSourceSecrets, appName string, hookCalled bool, hookURL, hookToken string) {
	t.Helper()
	if !hookCalled {
		t.Fatal("CreateWebhook was not called")
	}
	if !strings.HasSuffix(hookURL, "/api/v1/webhooks/github/"+appName) {
		t.Errorf("hookURL = %q, want it to end with the generic git-push webhook path", hookURL)
	}
	storedSecret, err := gitSourceSecrets.Resolve(context.Background(), store.GitSourceSecretsKey(appName), gitSourceSecretKey)
	if err != nil {
		t.Fatalf("resolve stored git source webhook secret: %v", err)
	}
	if hookToken != storedSecret {
		t.Errorf("hook token = %q, want it to match the stored git-source webhook secret %q", hookToken, storedSecret)
	}
}

// assertGitSourceSurvivesWebhookFailure proves the "web" fixture app's git
// source stays connected after a webhook registration failure: the shared
// "connect first, webhook second" shape GitHub's, GitLab's, and
// Bitbucket's own use-as-source handlers all document. Every current
// caller seeds its git source under that same app name (seedApp(t, db,
// "web")).
func assertGitSourceSurvivesWebhookFailure(t *testing.T, db *store.DB) {
	t.Helper()
	if _, err := db.GetGitSource(context.Background(), "web"); err != nil {
		t.Errorf("GetGitSource() error = %v, want the git source to remain connected despite the webhook failure", err)
	}
}

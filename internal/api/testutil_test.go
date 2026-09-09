package api

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/brand"
	"github.com/GLINCKER/levelrail/internal/reconcile"
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

// routeCase is one method+target pair for assertRoutesRequireAuth.
type routeCase struct {
	method string
	target string
}

// assertRoutesRequireAuth proves every route in routes rejects an
// unauthenticated request with 401, the shared shape nearly every
// "*Routes_RequireAuth" test in this package already establishes
// individually.
func assertRoutesRequireAuth(t *testing.T, rt *Router, routes []routeCase) {
	t.Helper()
	for _, r := range routes {
		t.Run(r.method+" "+r.target, func(t *testing.T) {
			req := httptest.NewRequest(r.method, r.target, nil)
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
			}
		})
	}
}

// seedTriStateConditions upserts a True/"Running" condition for healthy and
// a False/"CrashLoop" condition for broken, leaving any third, "pending"
// resource in resourceType untouched: the one-healthy/one-broken/
// one-pending shape TestHandleListApps_Status and
// TestHandleListDatabases_Status both need to prove their list endpoint's
// batched status field categorizes each row independently.
func seedTriStateConditions(t *testing.T, db *store.DB, resourceType, healthy, broken string) {
	t.Helper()
	ctx := context.Background()
	if err := db.UpsertConditions(ctx, resourceType+"/"+healthy, []reconcile.Condition{
		{Type: "Ready", Status: reconcile.ConditionTrue, Reason: "Running"},
	}); err != nil {
		t.Fatalf("upsert %s conditions: %v", healthy, err)
	}
	if err := db.UpsertConditions(ctx, resourceType+"/"+broken, []reconcile.Condition{
		{Type: "Ready", Status: reconcile.ConditionFalse, Reason: "CrashLoop"},
	}); err != nil {
		t.Fatalf("upsert %s conditions: %v", broken, err)
	}
}

// seedWebAppForTest seeds a "web" application via SaveDesiredService, the
// fixture apps_storage_test.go, apps_database_test.go, and
// apps_log_drain_test.go all need before exercising a handler that
// operates on an existing app.
func seedWebAppForTest(t *testing.T, db *store.DB) {
	t.Helper()
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "img:v1", Port: 8080}); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

// seedRedisDatabaseForTest seeds a "main" redis database via
// SaveDesiredDatabase, the fixture database_public_access_test.go and
// backups_test.go both repeat before exercising a handler that operates
// on an existing database.
func seedRedisDatabaseForTest(t *testing.T, db *store.DB) {
	t.Helper()
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EngineRedis, Version: "7"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
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

package api

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

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

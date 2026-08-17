package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestBootstrapAdmin_CreatesWhenNoneExists(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := BootstrapAdmin(ctx, db, "admin", "s3cret-password"); err != nil {
		t.Fatalf("BootstrapAdmin() error = %v", err)
	}

	got, err := db.GetUserByEmail(ctx, "admin")
	if err != nil {
		t.Fatalf("GetUserByEmail() error = %v", err)
	}
	if got.Email != "admin" {
		t.Errorf("Email = %q, want %q", got.Email, "admin")
	}
	if got.PasswordHash == nil || *got.PasswordHash == "" || *got.PasswordHash == "s3cret-password" {
		t.Errorf("PasswordHash = %v, want a bcrypt hash, not empty or the raw password", got.PasswordHash)
	}
	if !got.IsFirstUser {
		t.Error("IsFirstUser = false, want true for the bootstrap user")
	}
	if len(got.Abilities) != 1 || got.Abilities[0] != AbilityRoot {
		t.Errorf("Abilities = %v, want [%q] for the bootstrap admin", got.Abilities, AbilityRoot)
	}
}

func TestBootstrapAdmin_NoOpWhenAlreadyExists(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := BootstrapAdmin(ctx, db, "admin", "first-password"); err != nil {
		t.Fatalf("first BootstrapAdmin() error = %v", err)
	}
	first, err := db.GetUserByEmail(ctx, "admin")
	if err != nil {
		t.Fatalf("GetUserByEmail() error = %v", err)
	}

	// A second call with different credentials, e.g. env vars still set
	// after a restart, must not overwrite an admin account an operator
	// may have since changed through some other path.
	if err := BootstrapAdmin(ctx, db, "someone-else", "second-password"); err != nil {
		t.Fatalf("second BootstrapAdmin() error = %v", err)
	}
	second, err := db.GetUserByEmail(ctx, "admin")
	if err != nil {
		t.Fatalf("GetUserByEmail() error = %v", err)
	}

	if second.Email != first.Email || *second.PasswordHash != *first.PasswordHash {
		t.Errorf("second bootstrap changed the admin account: before = %+v, after = %+v", first, second)
	}
	if _, err := db.GetUserByEmail(ctx, "someone-else"); !errors.Is(err, store.ErrUserNotFound) {
		t.Error("second bootstrap must not have created a second user")
	}
}

func TestBootstrapAdmin_MissingCredentialsErrors(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	tests := []struct {
		name     string
		username string
		password string
	}{
		{name: "empty username", username: "", password: "something"},
		{name: "empty password", username: "admin", password: ""},
		{name: "both empty", username: "", password: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := BootstrapAdmin(ctx, db, tt.username, tt.password); err == nil {
				t.Fatal("BootstrapAdmin() error = nil, want an error for missing credentials")
			}
			if n, err := db.CountUsers(ctx); err != nil || n != 0 {
				t.Errorf("CountUsers() = (%d, %v), want (0, nil) (no partial bootstrap)", n, err)
			}
		})
	}
}

func TestHandleLogin_Success(t *testing.T) {
	rt, db := newTestRouter(t)
	bootstrapTestAdmin(t, db)

	body := `{"username":"` + testAdminUsername + `","password":"` + testAdminPassword + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var found bool
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookieName && c.Value != "" {
			found = true
		}
	}
	if !found {
		t.Error("expected a non-empty session cookie to be set on successful login")
	}
}

func TestHandleLogin_Failures(t *testing.T) {
	tests := []struct {
		name       string
		bootstrap  bool
		body       string
		wantStatus int
	}{
		{
			name:       "wrong password",
			bootstrap:  true,
			body:       `{"username":"` + testAdminUsername + `","password":"wrong-password"}`,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "unknown username",
			bootstrap:  true,
			body:       `{"username":"nobody","password":"` + testAdminPassword + `"}`,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "no admin bootstrapped yet",
			bootstrap:  false,
			body:       `{"username":"admin","password":"anything"}`,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "missing password field",
			bootstrap:  true,
			body:       `{"username":"` + testAdminUsername + `"}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "malformed json",
			bootstrap:  true,
			body:       `{not valid json`,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt, db := newTestRouter(t)
			if tt.bootstrap {
				bootstrapTestAdmin(t, db)
			}

			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(tt.body))
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			for _, c := range rec.Result().Cookies() {
				if c.Name == sessionCookieName && c.Value != "" {
					t.Error("expected no session cookie to be set on a failed login")
				}
			}
		})
	}
}

func TestHandleLogout_RevokesSession(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	logoutReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	logoutReq.AddCookie(cookie)
	logoutRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(logoutRec, logoutReq)

	if logoutRec.Code != http.StatusNoContent {
		t.Fatalf("logout status = %d, want %d", logoutRec.Code, http.StatusNoContent)
	}

	// The same session token must no longer authenticate anything.
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/apps", nil)
	listReq.AddCookie(cookie)
	listRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(listRec, listReq)

	if listRec.Code != http.StatusUnauthorized {
		t.Fatalf("status after logout = %d, want %d (session should be revoked)", listRec.Code, http.StatusUnauthorized)
	}
}

func TestRequireAuth_RejectsMissingOrInvalidSession(t *testing.T) {
	rt, _ := newTestRouter(t)

	tests := []struct {
		name      string
		addCookie bool
		cookieVal string
	}{
		{name: "no cookie at all", addCookie: false},
		{name: "garbage token", addCookie: true, cookieVal: "not-a-real-token"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/apps", nil)
			if tt.addCookie {
				// This builds an outgoing request's Cookie header, not a
				// response Set-Cookie: Secure/HttpOnly/SameSite are
				// response-only semantics, so gosec's G124 doesn't apply
				// here even though it shares the http.Cookie type.
				req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: tt.cookieVal}) //nolint:gosec // request cookie, not a response Set-Cookie
			}
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusUnauthorized, rec.Body.String())
			}
		})
	}
}

func TestHandleLogin_RateLimited_AfterRepeatedFailures(t *testing.T) {
	rt, db := newTestRouter(t)
	bootstrapTestAdmin(t, db)

	// loginGraceFailures+1 failed attempts: the (loginGraceFailures+1)th
	// itself still gets processed normally (it's what tips the counter
	// past the threshold and sets the lock for whatever comes next), so
	// the lockout is only observable on a subsequent request, not that
	// one.
	wrongBody := `{"username":"` + testAdminUsername + `","password":"wrong"}`
	for i := 0; i < loginGraceFailures+1; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(wrongBody))
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: status = %d, want %d (still processed normally)", i, rec.Code, http.StatusUnauthorized)
		}
	}

	// Even the correct password must be rejected while locked out: the
	// limiter gates before any credential check runs.
	correctBody := `{"username":"` + testAdminUsername + `","password":"` + testAdminPassword + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(correctBody))
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("status with correct credentials while locked out = %d, want %d", rec.Code, http.StatusTooManyRequests)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("Retry-After header not set on a rate-limited response")
	}
}

func TestHandleLogin_SuccessResetsRateLimit(t *testing.T) {
	rt, db := newTestRouter(t)
	bootstrapTestAdmin(t, db)

	wrongBody := `{"username":"` + testAdminUsername + `","password":"wrong"}`
	for i := 0; i < loginGraceFailures; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(wrongBody))
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, req)
	}

	correctBody := `{"username":"` + testAdminUsername + `","password":"` + testAdminPassword + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(correctBody))
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: a correct login within the grace window must still succeed", rec.Code, http.StatusOK)
	}
}

func TestHandleLogin_UnknownUsername_CountsAgainstRateLimit(t *testing.T) {
	// Probing for a valid username must cost the same as guessing its
	// password, not be a free, unlimited oracle.
	rt, db := newTestRouter(t)
	bootstrapTestAdmin(t, db)

	body := `{"username":"nobody","password":"anything"}`
	for i := 0; i < loginGraceFailures+1; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, req)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("status after repeated unknown-username attempts = %d, want %d", rec.Code, http.StatusTooManyRequests)
	}
}

func TestWithSessionTTL(t *testing.T) {
	db := openTestDB(t)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	rt := NewRouter(logger, testBrand(), db, WithSessionTTL(1*time.Hour))

	if rt.sessions.ttl != 1*time.Hour {
		t.Errorf("sessions.ttl = %v, want 1h", rt.sessions.ttl)
	}
}

func TestWithSessionTTL_DefaultsWhenUnset(t *testing.T) {
	db := openTestDB(t)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	rt := NewRouter(logger, testBrand(), db)

	if rt.sessions.ttl != defaultSessionTTL {
		t.Errorf("sessions.ttl = %v, want defaultSessionTTL (%v)", rt.sessions.ttl, defaultSessionTTL)
	}
}

func TestHandleRegister_Success(t *testing.T) {
	rt, db := newTestRouter(t)

	body := `{"username":"admin","password":"a-real-password"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(body))
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var found bool
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookieName && c.Value != "" {
			found = true
		}
	}
	if !found {
		t.Error("expected a non-empty session cookie to be set on successful registration")
	}

	admin, err := db.GetUserByEmail(context.Background(), "admin")
	if err != nil {
		t.Fatalf("GetUserByEmail() error = %v", err)
	}
	if admin.Email != "admin" {
		t.Errorf("admin.Email = %q, want %q", admin.Email, "admin")
	}
	if !admin.IsFirstUser {
		t.Error("IsFirstUser = false, want true for the very first registration")
	}
}

// TestHandleRegister_AlreadyBootstrapped_Conflict proves /auth/register
// only ever creates the first user: once any user exists, it always 409s.
func TestHandleRegister_AlreadyBootstrapped_Conflict(t *testing.T) {
	rt, db := newTestRouter(t)
	bootstrapTestAdmin(t, db)

	body := `{"username":"someone-else","password":"a-real-password"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(body))
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusConflict)
	}

	// The original admin must survive untouched, and no second user must
	// have been created under the new request's email.
	admin, err := db.GetUserByEmail(context.Background(), testAdminUsername)
	if err != nil {
		t.Fatalf("GetUserByEmail() error = %v", err)
	}
	if admin.Email != testAdminUsername {
		t.Errorf("admin.Email = %q, want the original %q, want it unchanged", admin.Email, testAdminUsername)
	}
	if _, err := db.GetUserByEmail(context.Background(), "someone-else"); !errors.Is(err, store.ErrUserNotFound) {
		t.Error("a rejected registration must not have created a second user")
	}
}

func TestHandleRegister_ShortPasswordRejected(t *testing.T) {
	rt, db := newTestRouter(t)

	body := `{"username":"admin","password":"short"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(body))
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if n, err := db.CountUsers(context.Background()); err != nil || n != 0 {
		t.Errorf("CountUsers() = (%d, %v), want (0, nil): a rejected registration must not have created a user", n, err)
	}
}

func TestHandleRegister_MissingFields(t *testing.T) {
	rt, _ := newTestRouter(t)

	tests := []string{
		`{"username":"","password":"a-real-password"}`,
		`{"username":"admin","password":""}`,
		`{not json`,
	}
	for _, body := range tests {
		t.Run(body, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(body))
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
			}
		})
	}
}

func TestHandleRegister_ConcurrentRequests_OnlyOneWins(t *testing.T) {
	// Guards the race a route-only "does an admin exist" check would
	// leave open: the mutation itself must be the thing that decides,
	// not a check performed earlier and trusted.
	rt, db := newTestRouter(t)

	const n = 10
	results := make(chan int, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			body := fmt.Sprintf(`{"username":"admin-%d","password":"a-real-password"}`, i)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(body))
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, req)
			results <- rec.Code
		}(i)
	}
	wg.Wait()
	close(results)

	created := 0
	for code := range results {
		if code == http.StatusCreated {
			created++
		}
	}
	if created != 1 {
		t.Errorf("created = %d successful registrations out of %d concurrent attempts, want exactly 1", created, n)
	}

	// Exactly one user row must exist, not zero, not many:
	// ux_users_single_first_user (migrations/0035_users.sql) enforces
	// this at the schema level too, this proves the API layer converges
	// to the same invariant.
	if n, err := db.CountUsers(context.Background()); err != nil || n != 1 {
		t.Errorf("CountUsers() = (%d, %v), want (1, nil) after the race", n, err)
	}
}

package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestBootstrapAdmin_CreatesWhenNoneExists(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := BootstrapAdmin(ctx, db, "admin", "s3cret-password"); err != nil {
		t.Fatalf("BootstrapAdmin() error = %v", err)
	}

	got, err := db.GetAdminUser(ctx)
	if err != nil {
		t.Fatalf("GetAdminUser() error = %v", err)
	}
	if got.Username != "admin" {
		t.Errorf("Username = %q, want %q", got.Username, "admin")
	}
	if got.PasswordHash == "" || got.PasswordHash == "s3cret-password" {
		t.Errorf("PasswordHash = %q, want a bcrypt hash, not empty or the raw password", got.PasswordHash)
	}
}

func TestBootstrapAdmin_NoOpWhenAlreadyExists(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := BootstrapAdmin(ctx, db, "admin", "first-password"); err != nil {
		t.Fatalf("first BootstrapAdmin() error = %v", err)
	}
	first, err := db.GetAdminUser(ctx)
	if err != nil {
		t.Fatalf("GetAdminUser() error = %v", err)
	}

	// A second call with different credentials, e.g. env vars still set
	// after a restart, must not overwrite an admin account an operator
	// may have since changed through some other path.
	if err := BootstrapAdmin(ctx, db, "someone-else", "second-password"); err != nil {
		t.Fatalf("second BootstrapAdmin() error = %v", err)
	}
	second, err := db.GetAdminUser(ctx)
	if err != nil {
		t.Fatalf("GetAdminUser() error = %v", err)
	}

	if second.Username != first.Username || second.PasswordHash != first.PasswordHash {
		t.Errorf("second bootstrap changed the admin account: before = %+v, after = %+v", first, second)
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
			if _, err := db.GetAdminUser(ctx); !errors.Is(err, store.ErrAdminNotFound) {
				t.Errorf("GetAdminUser() error = %v, want ErrAdminNotFound (no partial bootstrap)", err)
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

	admin, err := db.GetAdminUser(context.Background())
	if err != nil {
		t.Fatalf("GetAdminUser() error = %v", err)
	}
	if admin.Username != "admin" {
		t.Errorf("admin.Username = %q, want %q", admin.Username, "admin")
	}
}

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

	// The original admin must survive untouched: a second registration
	// attempt must never overwrite the existing account.
	admin, err := db.GetAdminUser(context.Background())
	if err != nil {
		t.Fatalf("GetAdminUser() error = %v", err)
	}
	if admin.Username != testAdminUsername {
		t.Errorf("admin.Username = %q, want the original %q, want it unchanged", admin.Username, testAdminUsername)
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
	if _, err := db.GetAdminUser(context.Background()); !errors.Is(err, store.ErrAdminNotFound) {
		t.Error("a rejected registration must not have created an admin row")
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

	// Exactly one admin row must exist, not zero, not many: admin_user's
	// own CHECK (id = 1) constraint enforces this at the schema level
	// too, this proves the API layer converges to the same invariant.
	if _, err := db.GetAdminUser(context.Background()); err != nil {
		t.Errorf("GetAdminUser() error = %v, want exactly one admin to exist after the race", err)
	}
}

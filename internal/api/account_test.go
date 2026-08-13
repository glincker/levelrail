package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func TestHandleChangePassword_Success(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	body := `{"current_password":"` + testAdminPassword + `","new_password":"a-new-strong-password"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/auth/password", body))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}

	admin, err := db.GetAdminUser(context.Background())
	if err != nil {
		t.Fatalf("GetAdminUser() error = %v", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte("a-new-strong-password")); err != nil {
		t.Errorf("stored hash does not verify against the new password: %v", err)
	}

	// The old password must no longer work.
	loginRec := httptest.NewRecorder()
	loginBody := `{"username":"` + testAdminUsername + `","password":"` + testAdminPassword + `"}`
	rt.Handler().ServeHTTP(loginRec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(loginBody)))
	if loginRec.Code != http.StatusUnauthorized {
		t.Errorf("login with old password: status = %d, want %d", loginRec.Code, http.StatusUnauthorized)
	}
}

func TestHandleChangePassword_WrongCurrentPassword(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	body := `{"current_password":"totally-wrong","new_password":"a-new-strong-password"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/auth/password", body))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}

	admin, err := db.GetAdminUser(context.Background())
	if err != nil {
		t.Fatalf("GetAdminUser() error = %v", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(testAdminPassword)); err != nil {
		t.Errorf("password was changed despite a wrong current_password: %v", err)
	}
}

func TestHandleChangePassword_NewPasswordTooShort(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	body := `{"current_password":"` + testAdminPassword + `","new_password":"short"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/auth/password", body))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleChangePassword_RequiresSession(t *testing.T) {
	rt, _ := newTestRouter(t)

	body := `{"current_password":"x","new_password":"a-new-strong-password"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/api/v1/auth/password", strings.NewReader(body)))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandleChangePassword_RevokesOtherSessionsButNotCurrent(t *testing.T) {
	rt, db := newTestRouter(t)
	// Two independent logins, simulating two browsers signed into the
	// same single-admin account.
	cookieA := loginTestSession(t, rt, db)
	loginRec := httptest.NewRecorder()
	loginBody := `{"username":"` + testAdminUsername + `","password":"` + testAdminPassword + `"}`
	rt.Handler().ServeHTTP(loginRec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(loginBody)))
	if loginRec.Code != http.StatusOK {
		t.Fatalf("second login: status = %d, want %d", loginRec.Code, http.StatusOK)
	}
	var cookieB *http.Cookie
	for _, c := range loginRec.Result().Cookies() {
		if c.Name == sessionCookieName {
			cookieB = c
		}
	}
	if cookieB == nil {
		t.Fatal("second login: no session cookie returned")
	}

	changeBody := `{"current_password":"` + testAdminPassword + `","new_password":"a-new-strong-password"}`
	changeRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(changeRec, authedRequest(t, cookieA, http.MethodPut, "/api/v1/auth/password", changeBody))
	if changeRec.Code != http.StatusNoContent {
		t.Fatalf("change password: status = %d, want %d, body = %s", changeRec.Code, http.StatusNoContent, changeRec.Body.String())
	}

	// cookieA (the session that made the change) must still work.
	recA := httptest.NewRecorder()
	rt.Handler().ServeHTTP(recA, authedRequest(t, cookieA, http.MethodGet, "/api/v1/apps", ""))
	if recA.Code != http.StatusOK {
		t.Errorf("session that changed the password: status = %d, want %d (must stay logged in)", recA.Code, http.StatusOK)
	}

	// cookieB (the other, uninvolved session) must be revoked.
	recB := httptest.NewRecorder()
	rt.Handler().ServeHTTP(recB, authedRequest(t, cookieB, http.MethodGet, "/api/v1/apps", ""))
	if recB.Code != http.StatusUnauthorized {
		t.Errorf("other session after password change: status = %d, want %d (must be revoked)", recB.Code, http.StatusUnauthorized)
	}
}

func TestHandleGetSession_Success(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/auth/session", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got sessionInfoResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Username != testAdminUsername {
		t.Errorf("Username = %q, want %q", got.Username, testAdminUsername)
	}
	expiresAt, err := time.Parse(time.RFC3339, got.ExpiresAt)
	if err != nil {
		t.Fatalf("ExpiresAt = %q, not a valid RFC3339 timestamp: %v", got.ExpiresAt, err)
	}
	if !expiresAt.After(time.Now()) {
		t.Errorf("ExpiresAt = %v, want a time in the future", expiresAt)
	}
}

func TestHandleGetSession_RequiresSession(t *testing.T) {
	rt, _ := newTestRouter(t)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandleRevokeOtherSessions_RevokesOthersNotCaller(t *testing.T) {
	rt, db := newTestRouter(t)
	cookieA := loginTestSession(t, rt, db)
	loginRec := httptest.NewRecorder()
	loginBody := `{"username":"` + testAdminUsername + `","password":"` + testAdminPassword + `"}`
	rt.Handler().ServeHTTP(loginRec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(loginBody)))
	var cookieB *http.Cookie
	for _, c := range loginRec.Result().Cookies() {
		if c.Name == sessionCookieName {
			cookieB = c
		}
	}
	if cookieB == nil {
		t.Fatal("second login: no session cookie returned")
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookieA, http.MethodPost, "/api/v1/auth/sessions/revoke-others", ""))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}

	recA := httptest.NewRecorder()
	rt.Handler().ServeHTTP(recA, authedRequest(t, cookieA, http.MethodGet, "/api/v1/apps", ""))
	if recA.Code != http.StatusOK {
		t.Errorf("caller's own session: status = %d, want %d (must stay logged in)", recA.Code, http.StatusOK)
	}

	recB := httptest.NewRecorder()
	rt.Handler().ServeHTTP(recB, authedRequest(t, cookieB, http.MethodGet, "/api/v1/apps", ""))
	if recB.Code != http.StatusUnauthorized {
		t.Errorf("other session: status = %d, want %d (must be revoked)", recB.Code, http.StatusUnauthorized)
	}
}

func TestHandleRevokeOtherSessions_RequiresSession(t *testing.T) {
	rt, _ := newTestRouter(t)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/sessions/revoke-others", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

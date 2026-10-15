package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func lifecycleLogin(rt *Router, email string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"`+email+`","password":"correct horse battery"}`))
	rt.Handler().ServeHTTP(rec, req)
	return rec
}

func lifecycleUser(t *testing.T, rt *Router, admin *http.Cookie, email string) userResource {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, admin, http.MethodPost, "/api/v1/auth/users",
		`{"email":"`+email+`","password":"correct horse battery","role":"viewer"}`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create user: %d %s", rec.Code, rec.Body)
	}
	var u userResource
	_ = json.Unmarshal(rec.Body.Bytes(), &u)
	return u
}

func TestPasswordChangeRequiredBlocksSignInUntilCleared(t *testing.T) {
	db := openTestDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db)
	admin := loginTestSession(t, rt, db)
	u := lifecycleUser(t, rt, admin, "pat@example.com")
	login := lifecycleLogin(rt, u.Email)
	if login.Code != http.StatusOK {
		t.Fatalf("login before: %d %s", login.Code, login.Body)
	}
	var sess *http.Cookie
	for _, c := range login.Result().Cookies() {
		if c.Name == sessionCookieName {
			sess = c
		}
	}
	set := func(required bool) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		body := `{"required":false}`
		if required {
			body = `{"required":true}`
		}
		rt.Handler().ServeHTTP(rec, authedRequest(t, admin, http.MethodPut, "/api/v1/users/"+u.ID+"/password-change-required", body))
		return rec
	}
	if rec := set(true); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"must_change_password":true`) {
		t.Fatalf("require: %d %s", rec.Code, rec.Body)
	}
	check := httptest.NewRecorder()
	rt.Handler().ServeHTTP(check, authedRequest(t, sess, http.MethodGet, "/api/v1/auth/session", ""))
	if check.Code != http.StatusUnauthorized {
		t.Fatalf("existing session survived: %d", check.Code)
	}
	if rec := lifecycleLogin(rt, u.Email); rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "password change is required") {
		t.Fatalf("login while flagged: %d %s", rec.Code, rec.Body)
	}
	list := httptest.NewRecorder()
	rt.Handler().ServeHTTP(list, authedRequest(t, admin, http.MethodGet, "/api/v1/users", ""))
	if !strings.Contains(list.Body.String(), `"must_change_password":true`) {
		t.Fatalf("list does not show the flag: %s", list.Body)
	}
	if rec := set(false); rec.Code != http.StatusOK {
		t.Fatalf("clear: %d %s", rec.Code, rec.Body)
	}
	if rec := lifecycleLogin(rt, u.Email); rec.Code != http.StatusOK {
		t.Fatalf("login after clearing: %d %s", rec.Code, rec.Body)
	}
}

func TestPasswordChangeRequiredRefusals(t *testing.T) {
	db := openTestDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db)
	admin := loginTestSession(t, rt, db)
	sso := storeUserForTest(t, db, "sso-only@example.com")
	root, err := db.GetUserByEmail(context.Background(), testAdminUsername)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		id, body string
		want     int
	}{
		{sso.ID, `{"required":true}`, http.StatusConflict},
		{root.ID, `{"required":true}`, http.StatusBadRequest},
		{"user_missing", `{"required":true}`, http.StatusNotFound},
		{sso.ID, `{}`, http.StatusBadRequest},
	} {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, admin, http.MethodPut, "/api/v1/users/"+tc.id+"/password-change-required", tc.body))
		if rec.Code != tc.want {
			t.Errorf("%s %s = %d, want %d (%s)", tc.id, tc.body, rec.Code, tc.want, rec.Body)
		}
	}
}

func TestDeleteUserRemovesTheLibraryAccount(t *testing.T) {
	db := openTestDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db)
	admin := loginTestSession(t, rt, db)
	u := lifecycleUser(t, rt, admin, "gone@example.com")
	if rec := lifecycleLogin(rt, u.Email); rec.Code != http.StatusOK {
		t.Fatalf("login: %d", rec.Code)
	}
	ctx := context.Background()
	engine, err := rt.libSessions.EngineIDFor(ctx, u.ID)
	if err != nil || engine == "" {
		t.Fatalf("no library user: %v", err)
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, admin, http.MethodDelete, "/api/v1/users/"+u.ID, ""))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body)
	}
	var n int
	if err := db.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM theauth_users WHERE id = ?) + (SELECT COUNT(*) FROM theauth_sessions WHERE user_id = ?)
		+ (SELECT COUNT(*) FROM theauth_user_passwords WHERE user_id = ?)`, engine, engine, engine).Scan(&n); err != nil || n != 0 {
		t.Fatalf("library rows left after delete: %d %v", n, err)
	}
}

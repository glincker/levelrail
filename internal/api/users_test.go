package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestHandleListUsers_RequiresAuth(t *testing.T) {
	rt, _ := newTestRouter(t)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/users", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandleListUsers_ReturnsProvidersAndNoPasswordHash(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	oauthUser := storeUserForTest(t, db, "oauth@example.com")
	identity := store.OAuthIdentity{ID: "oid_test", UserID: oauthUser.ID, Provider: store.OAuthProviderGoogle, ProviderUserID: "g1", CreatedAt: time.Now()}
	if err := db.SaveOAuthIdentity(context.Background(), identity); err != nil {
		t.Fatalf("SaveOAuthIdentity() error = %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/users", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "password_hash") {
		t.Error("response leaked a password_hash field")
	}

	var got []userResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	var found bool
	for _, u := range got {
		if u.Email == "oauth@example.com" {
			found = true
			if u.HasPassword {
				t.Error("HasPassword = true, want false for an OAuth-only user")
			}
			if len(u.Providers) != 1 || u.Providers[0] != "google" {
				t.Errorf("Providers = %v, want [google]", u.Providers)
			}
		}
	}
	if !found {
		t.Error("oauth@example.com not present in list")
	}
}

func TestHandleCreateUser_RequiresAuth(t *testing.T) {
	rt, _ := newTestRouter(t)
	body := `{"email":"new@example.com","password":"a-real-password"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/users", strings.NewReader(body)))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandleCreateUser_AnyAuthenticatedUserCanCreateAnother(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	body := `{"email":"second@example.com","display_name":"Second","password":"a-real-password"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/auth/users", body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	got, err := db.GetUserByEmail(context.Background(), "second@example.com")
	if err != nil {
		t.Fatalf("GetUserByEmail() error = %v", err)
	}
	if got.IsFirstUser {
		t.Error("IsFirstUser = true, want false for a user created through this route")
	}
	if got.PasswordHash == nil {
		t.Error("PasswordHash = nil, want a bcrypt hash")
	}
}

func TestHandleCreateUser_DuplicateEmail_Conflict(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	body := `{"email":"` + testAdminUsername + `","password":"a-real-password"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/auth/users", body))
	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusConflict)
	}
}

func TestHandleDeleteUser_RequiresRoot(t *testing.T) {
	rt, db := newTestRouter(t)
	other := storeUserForTest(t, db, "victim@example.com")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/v1/users/"+other.ID, nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandleDeleteUser_RemovesUserAndRevokesSessions(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	other := storeUserForTest(t, db, "victim@example.com")
	otherCookie := sessionCookieForTest(t, rt, other.ID)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/users/"+other.ID, ""))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}

	check := httptest.NewRecorder()
	rt.Handler().ServeHTTP(check, authedRequest(t, otherCookie, http.MethodGet, "/api/v1/apps", ""))
	if check.Code != http.StatusUnauthorized {
		t.Errorf("removed user's session still authenticates: status = %d, want %d", check.Code, http.StatusUnauthorized)
	}
}

func TestHandleDeleteUser_CannotDeleteSelf(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	storeUserForTest(t, db, "second@example.com") // so the last-remaining-user guard doesn't also fire

	admin, err := db.GetUserByEmail(context.Background(), testAdminUsername)
	if err != nil {
		t.Fatalf("GetUserByEmail() error = %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/users/"+admin.ID, ""))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

// TestHandleDeleteUser_CannotDeleteLastRemainingUser authenticates with
// a root-scoped API token (no "self") to isolate this guard from the
// self-delete guard.
func TestHandleDeleteUser_CannotDeleteLastRemainingUser(t *testing.T) {
	rt, db := newTestRouter(t)
	admin, err := db.GetUserByEmail(context.Background(), testAdminUsername)
	if err == nil {
		t.Fatalf("expected no admin bootstrapped yet, got %+v", admin)
	}
	bootstrapTestAdmin(t, db)
	admin, err = db.GetUserByEmail(context.Background(), testAdminUsername)
	if err != nil {
		t.Fatalf("GetUserByEmail() error = %v", err)
	}

	plaintext := "test-root-token-value"
	if err := db.SaveAPIToken(context.Background(), store.APIToken{
		ID: "tok_test", Name: "test", TokenHash: hashToken(plaintext), Abilities: []string{AbilityRoot}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("SaveAPIToken() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/users/"+admin.ID, nil)
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

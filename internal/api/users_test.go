package api

import (
	"context"
	"encoding/json"
	"errors"
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
	body := `{"email":"new@example.com","password":"a-real-password","abilities":["read"]}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/users", strings.NewReader(body)))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

// TestHandleCreateUser_RequiresRoot replaces
// TestHandleCreateUser_AnyAuthenticatedUserCanCreateAnother: creating a
// user now hands the caller the power to mint access at any tier
// (including root), so only a root caller may reach this route at all.
// A non-root session gets 403, matching the same "your account lacks the
// required ability" contract every other AbilityRoot route uses.
func TestHandleCreateUser_RequiresRoot(t *testing.T) {
	rt, db := newTestRouter(t)
	bootstrapTestAdmin(t, db)
	reader := storeUserWithAbilitiesForTest(t, db, "reader@example.com", []string{AbilityRead})
	cookie := sessionCookieForTest(t, rt, reader.ID)

	body := `{"email":"second@example.com","display_name":"Second","password":"a-real-password","abilities":["read"]}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/auth/users", body))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusForbidden, rec.Body.String())
	}

	if _, err := db.GetUserByEmail(context.Background(), "second@example.com"); !errors.Is(err, store.ErrUserNotFound) {
		t.Error("a rejected create must not have created a second user")
	}
}

// TestHandleCreateUser_RootCanCreateAnotherWithAbilities is the positive
// case: a root session creates a new user with an explicit, non-root
// Abilities set, and that set is what gets saved and returned.
func TestHandleCreateUser_RootCanCreateAnotherWithAbilities(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	body := `{"email":"second@example.com","display_name":"Second","password":"a-real-password","abilities":["read","deploy"]}`
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
	if len(got.Abilities) != 2 || got.Abilities[0] != "read" || got.Abilities[1] != "deploy" {
		t.Errorf("Abilities = %v, want [read deploy]", got.Abilities)
	}
}

// TestHandleCreateUser_MissingAbilitiesRejected proves Abilities has no
// default: a root caller must explicitly choose what the new account can
// do, the same "no empty list" rule validateAbilities already enforces
// for API tokens.
func TestHandleCreateUser_MissingAbilitiesRejected(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	body := `{"email":"second@example.com","password":"a-real-password"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/auth/users", body))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleCreateUser_DuplicateEmail_Conflict(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	body := `{"email":"` + testAdminUsername + `","password":"a-real-password","abilities":["read"]}`
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

// TestHandleUpdateUserAbilities_CannotEditSelf is the self-lockout
// guard, the single most important safety rule in this feature: a root
// user attempting to change their own abilities is rejected with 400
// before any write happens, so a root user can never lock themselves out
// of their own control plane, accidentally or via a bug elsewhere.
func TestHandleUpdateUserAbilities_CannotEditSelf(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	admin, err := db.GetUserByEmail(context.Background(), testAdminUsername)
	if err != nil {
		t.Fatalf("GetUserByEmail() error = %v", err)
	}

	body := `{"abilities":["read"]}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/users/"+admin.ID+"/abilities", body))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}

	unchanged, err := db.GetUserByID(context.Background(), admin.ID)
	if err != nil {
		t.Fatalf("GetUserByID() error = %v", err)
	}
	if len(unchanged.Abilities) != 1 || unchanged.Abilities[0] != AbilityRoot {
		t.Errorf("Abilities = %v, want unchanged [root] after a rejected self-edit", unchanged.Abilities)
	}
}

// TestHandleUpdateUserAbilities_RootCanEditAnotherUser is the positive
// case the self-lockout guard above must still allow: a root user
// changing a *different* user's abilities succeeds.
func TestHandleUpdateUserAbilities_RootCanEditAnotherUser(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	other := storeUserWithAbilitiesForTest(t, db, "other@example.com", []string{AbilityRead})

	body := `{"abilities":["read","write","deploy"]}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/users/"+other.ID+"/abilities", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got userResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Abilities) != 3 {
		t.Errorf("response Abilities = %v, want 3 entries", got.Abilities)
	}

	updated, err := db.GetUserByID(context.Background(), other.ID)
	if err != nil {
		t.Fatalf("GetUserByID() error = %v", err)
	}
	if len(updated.Abilities) != 3 || updated.Abilities[0] != "read" || updated.Abilities[1] != "write" || updated.Abilities[2] != "deploy" {
		t.Errorf("Abilities = %v, want [read write deploy]", updated.Abilities)
	}
}

// TestHandleUpdateUserAbilities_RequiresRoot proves a non-root session
// cannot reach this route at all, regardless of whose abilities it
// targets.
func TestHandleUpdateUserAbilities_RequiresRoot(t *testing.T) {
	rt, db := newTestRouter(t)
	bootstrapTestAdmin(t, db)
	reader := storeUserWithAbilitiesForTest(t, db, "reader@example.com", []string{AbilityRead})
	other := storeUserForTest(t, db, "other@example.com")
	cookie := sessionCookieForTest(t, rt, reader.ID)

	body := `{"abilities":["root"]}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/users/"+other.ID+"/abilities", body))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
}

func TestHandleUpdateUserAbilities_InvalidAbilitiesRejected(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	other := storeUserForTest(t, db, "other@example.com")

	tests := []string{
		`{"abilities":[]}`,
		`{"abilities":["not-a-real-ability"]}`,
		`{"abilities":["root","read"]}`,
	}
	for _, body := range tests {
		t.Run(body, func(t *testing.T) {
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/users/"+other.ID+"/abilities", body))
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
			}
		})
	}
}

func TestHandleUpdateUserAbilities_NotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	body := `{"abilities":["read"]}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/users/does-not-exist/abilities", body))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

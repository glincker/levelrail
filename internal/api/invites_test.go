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

func createInviteBody(email, role string) string {
	return `{"email":"` + email + `","role":"` + role + `"}`
}

func TestHandleCreateInvite_ReturnsLinkEvenWithoutEmailConfigured(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/invites", createInviteBody("new@example.com", RoleOperator)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var resp createInviteResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Link == "" || !strings.Contains(resp.Link, "token=") {
		t.Errorf("Link = %q, want a token= link even with no SMTP configured", resp.Link)
	}
	if resp.Email != "new@example.com" || resp.Role != RoleOperator {
		t.Errorf("resp = %+v, want email/role echoed back", resp)
	}
}

func TestHandleCreateInvite_SendsEmailWhenConfigured(t *testing.T) {
	sender := newFakeEmailSender()
	rt, db := newTestRouterWithEmailSender(t, sender)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/invites", createInviteBody("new@example.com", RoleViewer)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	call := waitForSend(t, sender.calls)
	if call.to != "new@example.com" {
		t.Errorf("sent to %q, want %q", call.to, "new@example.com")
	}
	if !strings.Contains(call.body, "token=") {
		t.Errorf("body has no invite link: %s", call.body)
	}
}

func TestHandleCreateInvite_RequiresRootAbility(t *testing.T) {
	rt, db := newTestRouter(t)
	bootstrapTestAdmin(t, db)
	nonRoot := storeUserWithAbilitiesForTest(t, db, "operator@example.com", []string{AbilityRead, AbilityWrite})
	cookie := sessionCookieForTest(t, rt, nonRoot.ID)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/invites", createInviteBody("new@example.com", RoleViewer)))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d (non-root caller must be refused)", rec.Code, http.StatusForbidden)
	}
}

func TestHandleCreateInvite_DuplicatePendingEmailConflicts(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	first := httptest.NewRecorder()
	rt.Handler().ServeHTTP(first, authedRequest(t, cookie, http.MethodPost, "/api/v1/invites", createInviteBody("dup@example.com", RoleViewer)))
	if first.Code != http.StatusCreated {
		t.Fatalf("first create: status = %d, want %d", first.Code, http.StatusCreated)
	}

	second := httptest.NewRecorder()
	rt.Handler().ServeHTTP(second, authedRequest(t, cookie, http.MethodPost, "/api/v1/invites", createInviteBody("dup@example.com", RoleViewer)))
	if second.Code != http.StatusConflict {
		t.Fatalf("second create: status = %d, want %d, body = %s", second.Code, http.StatusConflict, second.Body.String())
	}
}

func TestHandleCreateInvite_MissingEmail(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/invites", `{"role":"viewer"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleListInvites_ExcludesRevoked(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	createRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(createRec, authedRequest(t, cookie, http.MethodPost, "/api/v1/invites", createInviteBody("pending@example.com", RoleViewer)))
	var created createInviteResponse
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	listRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(listRec, authedRequest(t, cookie, http.MethodGet, "/api/v1/invites", ""))
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d", listRec.Code, http.StatusOK)
	}
	var list []inviteResource
	if err := json.Unmarshal(listRec.Body.Bytes(), &list); err != nil {
		t.Fatalf("unmarshal list: %v", err)
	}
	if len(list) != 1 || list[0].ID != created.ID {
		t.Fatalf("list = %+v, want exactly [%s]", list, created.ID)
	}

	revokeRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(revokeRec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/invites/"+created.ID, ""))
	if revokeRec.Code != http.StatusNoContent {
		t.Fatalf("revoke status = %d, want %d, body = %s", revokeRec.Code, http.StatusNoContent, revokeRec.Body.String())
	}

	listAfter := httptest.NewRecorder()
	rt.Handler().ServeHTTP(listAfter, authedRequest(t, cookie, http.MethodGet, "/api/v1/invites", ""))
	var listAfterBody []inviteResource
	if err := json.Unmarshal(listAfter.Body.Bytes(), &listAfterBody); err != nil {
		t.Fatalf("unmarshal list after revoke: %v", err)
	}
	if len(listAfterBody) != 0 {
		t.Errorf("list after revoke = %+v, want empty", listAfterBody)
	}
}

func TestHandleRevokeInvite_NotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/invites/inv_nope", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func acceptInviteBody(token, password string) string {
	return `{"token":"` + token + `","password":"` + password + `"}`
}

func TestHandleAcceptInvite_FullRoundTrip(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	createRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(createRec, authedRequest(t, cookie, http.MethodPost, "/api/v1/invites", createInviteBody("invitee@example.com", RoleOperator)))
	var created createInviteResponse
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	token := tokenFromResetEmail(t, created.Link)

	const password = "a-new-strong-password" //nolint:gosec // fake fixture, not a real credential
	acceptRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(acceptRec, httptest.NewRequest(http.MethodPost, "/api/v1/invites/accept", strings.NewReader(acceptInviteBody(token, password))))
	if acceptRec.Code != http.StatusCreated {
		t.Fatalf("accept status = %d, want %d, body = %s", acceptRec.Code, http.StatusCreated, acceptRec.Body.String())
	}

	// The accept response signs the new user in: a session cookie must
	// come back, usable against an authenticated route.
	var acceptedSession *http.Cookie
	for _, c := range acceptRec.Result().Cookies() {
		if c.Name == sessionCookieName {
			acceptedSession = c
		}
	}
	if acceptedSession == nil {
		t.Fatal("accept invite: no session cookie returned")
	}

	user, err := db.GetUserByEmail(context.Background(), "invitee@example.com")
	if err != nil {
		t.Fatalf("GetUserByEmail() error = %v", err)
	}
	role, ok := roleForAbilities(user.Abilities)
	if !ok || role != RoleOperator {
		t.Errorf("created user abilities = %v, want the %q role's abilities", user.Abilities, RoleOperator)
	}

	// The account can log in with the chosen password, a real, usable
	// account, not just a row.
	loginRec := httptest.NewRecorder()
	loginBody := `{"username":"invitee@example.com","password":"` + password + `"}`
	rt.Handler().ServeHTTP(loginRec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(loginBody)))
	if loginRec.Code != http.StatusOK {
		t.Errorf("login as invited user: status = %d, want %d, body = %s", loginRec.Code, http.StatusOK, loginRec.Body.String())
	}

	// The invite no longer appears in the pending list.
	listRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(listRec, authedRequest(t, cookie, http.MethodGet, "/api/v1/invites", ""))
	var list []inviteResource
	if err := json.Unmarshal(listRec.Body.Bytes(), &list); err != nil {
		t.Fatalf("unmarshal list: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("pending list after accept = %+v, want empty", list)
	}

	// The token is single-use: replaying it must fail, and must never
	// create a second account.
	replayRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(replayRec, httptest.NewRequest(http.MethodPost, "/api/v1/invites/accept", strings.NewReader(acceptInviteBody(token, "another-strong-password"))))
	if replayRec.Code != http.StatusBadRequest {
		t.Errorf("replayed invite token: status = %d, want %d", replayRec.Code, http.StatusBadRequest)
	}
}

func TestHandleAcceptInvite_UnknownToken(t *testing.T) {
	rt, _ := newTestRouter(t)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/invites/accept", strings.NewReader(acceptInviteBody("totally-made-up", "a-new-strong-password"))))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), errInvalidOrExpiredInvite.Error()) {
		t.Errorf("body = %s, want the generic invalid/expired message", rec.Body.String())
	}
}

func TestHandleAcceptInvite_ExpiredTokenRejected(t *testing.T) {
	rt, db := newTestRouter(t)
	admin := storeUserForTest(t, db, "admin2@example.com")

	const plaintext = "expired-invite-token" //nolint:gosec // fake fixture, not a real credential
	now := time.Now().UTC()
	if err := db.SaveInvite(context.Background(), store.Invite{
		ID: "inv_expired", Email: "late@example.com", Role: RoleViewer, Abilities: []string{AbilityRead},
		TokenHash: hashToken(plaintext), CreatedBy: admin.ID, CreatedAt: now.Add(-8 * 24 * time.Hour), ExpiresAt: now.Add(-time.Hour),
	}); err != nil {
		t.Fatalf("seed expired invite: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/invites/accept", strings.NewReader(acceptInviteBody(plaintext, "a-new-strong-password"))))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), errInvalidOrExpiredInvite.Error()) {
		t.Errorf("body = %s, want the generic invalid/expired message", rec.Body.String())
	}

	if _, err := db.GetUserByEmail(context.Background(), "late@example.com"); err == nil {
		t.Error("an expired invite must never create a user")
	}
}

func TestHandleAcceptInvite_RevokedThenAcceptRejected(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	createRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(createRec, authedRequest(t, cookie, http.MethodPost, "/api/v1/invites", createInviteBody("revokeme@example.com", RoleViewer)))
	var created createInviteResponse
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	token := tokenFromResetEmail(t, created.Link)

	revokeRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(revokeRec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/invites/"+created.ID, ""))
	if revokeRec.Code != http.StatusNoContent {
		t.Fatalf("revoke status = %d, want %d, body = %s", revokeRec.Code, http.StatusNoContent, revokeRec.Body.String())
	}

	acceptRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(acceptRec, httptest.NewRequest(http.MethodPost, "/api/v1/invites/accept", strings.NewReader(acceptInviteBody(token, "a-new-strong-password"))))
	if acceptRec.Code != http.StatusBadRequest {
		t.Fatalf("accept-after-revoke status = %d, want %d, body = %s", acceptRec.Code, http.StatusBadRequest, acceptRec.Body.String())
	}
	if _, err := db.GetUserByEmail(context.Background(), "revokeme@example.com"); err == nil {
		t.Error("a revoked invite must never create a user")
	}
}

func TestHandleAcceptInvite_PasswordTooShort(t *testing.T) {
	rt, _ := newTestRouter(t)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/invites/accept", strings.NewReader(acceptInviteBody("whatever", "short"))))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleAcceptInvite_NotGatedBehindAuth(t *testing.T) {
	rt, _ := newTestRouter(t)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/invites/accept", strings.NewReader(acceptInviteBody("whatever", "a-new-strong-password"))))
	if rec.Code == http.StatusUnauthorized {
		t.Error("status = 401: accept-invite must not require authentication")
	}
}

func TestHandleAcceptInvite_DuplicateEmailAgainstExistingUser(t *testing.T) {
	rt, db := newTestRouter(t)
	admin := storeUserForTest(t, db, "admin3@example.com")
	storeUserForTest(t, db, "taken@example.com")

	const plaintext = "already-taken-email-token" //nolint:gosec // fake fixture, not a real credential
	now := time.Now().UTC()
	if err := db.SaveInvite(context.Background(), store.Invite{
		ID: "inv_taken", Email: "taken@example.com", Role: RoleViewer, Abilities: []string{AbilityRead},
		TokenHash: hashToken(plaintext), CreatedBy: admin.ID, CreatedAt: now, ExpiresAt: now.Add(time.Hour),
	}); err != nil {
		t.Fatalf("seed invite: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/invites/accept", strings.NewReader(acceptInviteBody(plaintext, "a-new-strong-password"))))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusConflict, rec.Body.String())
	}
}

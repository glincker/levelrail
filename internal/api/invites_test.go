package api

import (
	"context"
	"encoding/json"
	"fmt"
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

func createInviteBodyWithAbilities(email string, abilities []string) string {
	b, err := json.Marshal(createInviteRequest{Email: email, Abilities: abilities})
	if err != nil {
		panic(err)
	}
	return string(b)
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

func TestHandleCreateInvite_RequiresWriteAbility(t *testing.T) {
	rt, db := newTestRouter(t)
	bootstrapTestAdmin(t, db)
	readOnly := storeUserWithAbilitiesForTest(t, db, "readonly@example.com", []string{AbilityRead})
	cookie := sessionCookieForTest(t, rt, readOnly.ID)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/invites", createInviteBody("new@example.com", RoleViewer)))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d (read-only caller must be refused)", rec.Code, http.StatusForbidden)
	}
}

// TestHandleCreateInvite_PrivilegeCap proves handleCreateInvite's own
// privilege cap (invites.go): now that create-invite is AbilityWrite, not
// AbilityRoot (routes.go), the load-bearing security property is that a
// caller can never invite someone with abilities they don't hold
// themselves, whether those abilities come from a role preset or a
// hand-picked abilities list. An operator (read, read:sensitive, write,
// deploy) can invite another operator or a viewer, never a root user; a
// root caller is unrestricted, unchanged from before.
func TestHandleCreateInvite_PrivilegeCap(t *testing.T) {
	operatorAbilities := []string{AbilityRead, AbilityReadSensitive, AbilityWrite, AbilityDeploy}

	tests := []struct {
		name            string
		callerAbilities []string
		body            string
		wantStatus      int
	}{
		{
			name:            "operator inviting operator via role succeeds",
			callerAbilities: operatorAbilities,
			body:            createInviteBody("op-invitee@example.com", RoleOperator),
			wantStatus:      http.StatusCreated,
		},
		{
			name:            "operator inviting viewer via role succeeds",
			callerAbilities: operatorAbilities,
			body:            createInviteBody("viewer-invitee@example.com", RoleViewer),
			wantStatus:      http.StatusCreated,
		},
		{
			name:            "operator inviting root via role fails",
			callerAbilities: operatorAbilities,
			body:            createInviteBody("root-invitee@example.com", RoleAdmin),
			wantStatus:      http.StatusForbidden,
		},
		{
			name:            "root inviting anyone via role succeeds",
			callerAbilities: []string{AbilityRoot},
			body:            createInviteBody("root-invited-root@example.com", RoleAdmin),
			wantStatus:      http.StatusCreated,
		},
		{
			name:            "operator inviting a hand-picked ability they hold succeeds",
			callerAbilities: operatorAbilities,
			body:            createInviteBodyWithAbilities("hand-picked-ok@example.com", []string{AbilityRead, AbilityDeploy}),
			wantStatus:      http.StatusCreated,
		},
		{
			name:            "operator inviting a hand-picked ability they lack fails",
			callerAbilities: operatorAbilities,
			body:            createInviteBodyWithAbilities("hand-picked-fail@example.com", []string{AbilityWriteSensitive}),
			wantStatus:      http.StatusForbidden,
		},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt, db := newTestRouter(t)
			bootstrapTestAdmin(t, db)
			caller := storeUserWithAbilitiesForTest(t, db, fmt.Sprintf("caller-%d@example.com", i), tt.callerAbilities)
			cookie := sessionCookieForTest(t, rt, caller.ID)

			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/invites", tt.body))
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d, body = %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tt.wantStatus == http.StatusForbidden &&
				!strings.Contains(rec.Body.String(), "cannot invite a user with abilities you don't hold yourself") {
				t.Errorf("body = %s, want the privilege-cap message", rec.Body.String())
			}
		})
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

// TestHandleRevokeInvite_ByCreatorSucceeds proves the UX gap closed by
// relaxing revoke: an operator who made a typo in an invite they just
// created can fix it themselves, without needing a root caller.
func TestHandleRevokeInvite_ByCreatorSucceeds(t *testing.T) {
	rt, db := newTestRouter(t)
	bootstrapTestAdmin(t, db)
	operator := storeUserWithAbilitiesForTest(t, db, "creator-op@example.com", []string{AbilityRead, AbilityWrite})
	cookie := sessionCookieForTest(t, rt, operator.ID)

	createRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(createRec, authedRequest(t, cookie, http.MethodPost, "/api/v1/invites", createInviteBody("revoke-by-creator@example.com", RoleViewer)))
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create: status = %d, want %d, body = %s", createRec.Code, http.StatusCreated, createRec.Body.String())
	}
	var created createInviteResponse
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	revokeRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(revokeRec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/invites/"+created.ID, ""))
	if revokeRec.Code != http.StatusNoContent {
		t.Fatalf("revoke by creator: status = %d, want %d, body = %s", revokeRec.Code, http.StatusNoContent, revokeRec.Body.String())
	}
}

// TestHandleRevokeInvite_ByNonCreatorNonRootFails proves the boundary
// this doesn't widen: a non-root caller can fix their own mistakes, but
// still can't touch an invite someone else created.
func TestHandleRevokeInvite_ByNonCreatorNonRootFails(t *testing.T) {
	rt, db := newTestRouter(t)
	bootstrapTestAdmin(t, db)
	creator := storeUserWithAbilitiesForTest(t, db, "invite-creator@example.com", []string{AbilityRead, AbilityWrite})
	other := storeUserWithAbilitiesForTest(t, db, "not-the-creator@example.com", []string{AbilityRead, AbilityWrite})
	creatorCookie := sessionCookieForTest(t, rt, creator.ID)
	otherCookie := sessionCookieForTest(t, rt, other.ID)

	createRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(createRec, authedRequest(t, creatorCookie, http.MethodPost, "/api/v1/invites", createInviteBody("not-yours-to-revoke@example.com", RoleViewer)))
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create: status = %d, want %d, body = %s", createRec.Code, http.StatusCreated, createRec.Body.String())
	}
	var created createInviteResponse
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	revokeRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(revokeRec, authedRequest(t, otherCookie, http.MethodDelete, "/api/v1/invites/"+created.ID, ""))
	if revokeRec.Code != http.StatusForbidden {
		t.Fatalf("revoke by non-creator non-root: status = %d, want %d, body = %s", revokeRec.Code, http.StatusForbidden, revokeRec.Body.String())
	}
}

// TestHandleRevokeInvite_ByRootAlwaysSucceeds proves root's revoke
// authority is unchanged: root may revoke an invite it didn't create.
func TestHandleRevokeInvite_ByRootAlwaysSucceeds(t *testing.T) {
	rt, db := newTestRouter(t)
	rootCookie := loginTestSession(t, rt, db)
	operator := storeUserWithAbilitiesForTest(t, db, "another-operator@example.com", []string{AbilityRead, AbilityWrite})
	operatorCookie := sessionCookieForTest(t, rt, operator.ID)

	createRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(createRec, authedRequest(t, operatorCookie, http.MethodPost, "/api/v1/invites", createInviteBody("root-can-revoke@example.com", RoleViewer)))
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create: status = %d, want %d, body = %s", createRec.Code, http.StatusCreated, createRec.Body.String())
	}
	var created createInviteResponse
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	revokeRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(revokeRec, authedRequest(t, rootCookie, http.MethodDelete, "/api/v1/invites/"+created.ID, ""))
	if revokeRec.Code != http.StatusNoContent {
		t.Fatalf("revoke by root: status = %d, want %d, body = %s", revokeRec.Code, http.StatusNoContent, revokeRec.Body.String())
	}
}

// TestHandleListInvites_ScopesNonRootToOwnInvites proves handleListInvites'
// scoping decision: a non-root caller sees only invites they created,
// consistent with what they're actually allowed to revoke; root sees
// every pending invite, unchanged from before this change.
func TestHandleListInvites_ScopesNonRootToOwnInvites(t *testing.T) {
	rt, db := newTestRouter(t)
	rootCookie := loginTestSession(t, rt, db)
	operatorA := storeUserWithAbilitiesForTest(t, db, "operator-a@example.com", []string{AbilityRead, AbilityWrite})
	operatorB := storeUserWithAbilitiesForTest(t, db, "operator-b@example.com", []string{AbilityRead, AbilityWrite})
	cookieA := sessionCookieForTest(t, rt, operatorA.ID)
	cookieB := sessionCookieForTest(t, rt, operatorB.ID)

	for _, seed := range []struct {
		cookie *http.Cookie
		email  string
	}{
		{cookieA, "from-a@example.com"},
		{cookieB, "from-b@example.com"},
		{rootCookie, "from-root@example.com"},
	} {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, seed.cookie, http.MethodPost, "/api/v1/invites", createInviteBody(seed.email, RoleViewer)))
		if rec.Code != http.StatusCreated {
			t.Fatalf("seed create %q: status = %d, body = %s", seed.email, rec.Code, rec.Body.String())
		}
	}

	listA := httptest.NewRecorder()
	rt.Handler().ServeHTTP(listA, authedRequest(t, cookieA, http.MethodGet, "/api/v1/invites", ""))
	var invitesA []inviteResource
	if err := json.Unmarshal(listA.Body.Bytes(), &invitesA); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(invitesA) != 1 || invitesA[0].Email != "from-a@example.com" {
		t.Fatalf("operatorA list = %+v, want exactly [from-a@example.com]", invitesA)
	}

	listRoot := httptest.NewRecorder()
	rt.Handler().ServeHTTP(listRoot, authedRequest(t, rootCookie, http.MethodGet, "/api/v1/invites", ""))
	var invitesRoot []inviteResource
	if err := json.Unmarshal(listRoot.Body.Bytes(), &invitesRoot); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(invitesRoot) != 3 {
		t.Fatalf("root list = %+v, want 3 invites", invitesRoot)
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

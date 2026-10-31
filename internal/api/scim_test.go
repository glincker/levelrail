package api

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	testSCIMUserSchema = "urn:ietf:params:scim:schemas:core:2.0:User"
	testSCIMPatchOp    = "urn:ietf:params:scim:api:messages:2.0:PatchOp"
)

type scimHarness struct {
	t     *testing.T
	rt    *Router
	db    *store.DB
	admin *http.Cookie
	token string
	tokID string
}

func newSCIMHarness(t *testing.T) *scimHarness {
	t.Helper()
	db := openTestDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db)
	h := &scimHarness{t: t, rt: rt, db: db, admin: loginTestSession(t, rt, db)}
	h.token, h.tokID = h.mint("okta")
	return h
}

func (h *scimHarness) mint(name string) (string, string) {
	h.t.Helper()
	rec := httptest.NewRecorder()
	h.rt.Handler().ServeHTTP(rec, authedRequest(h.t, h.admin, http.MethodPost, "/api/v1/sso/scim/tokens", `{"name":"`+name+`"}`))
	if rec.Code != http.StatusCreated {
		h.t.Fatalf("mint: %d %s", rec.Code, rec.Body)
	}
	var out scimTokenCreated
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if len(out.Token) < 40 || out.Prefix != out.Token[:scimTokenShownLen] {
		h.t.Fatalf("token %+v", out)
	}
	return out.Token, out.ID
}

func (h *scimHarness) do(method, path, token, body string) *httptest.ResponseRecorder {
	h.t.Helper()
	req := httptest.NewRequest(method, scimBasePath+path, strings.NewReader(body))
	req.TLS = &tls.ConnectionState{}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Content-Type", scimContentType)
	rec := httptest.NewRecorder()
	h.rt.Handler().ServeHTTP(rec, req)
	return rec
}

func (h *scimHarness) createUser(email string) string {
	h.t.Helper()
	rec := h.do(http.MethodPost, "/Users", h.token, `{"schemas":["`+testSCIMUserSchema+`"],"userName":"`+email+`","externalId":"ext-`+email+`","name":{"givenName":"Ada","familyName":"L"},"emails":[{"value":"`+email+`","primary":true}],"active":true}`)
	if rec.Code != http.StatusCreated {
		h.t.Fatalf("create user: %d %s", rec.Code, rec.Body)
	}
	id, err := scimResponseID(rec.Body.Bytes())
	if err != nil {
		h.t.Fatal(err)
	}
	return id
}

func (h *scimHarness) patch(path, ops string) *httptest.ResponseRecorder {
	h.t.Helper()
	return h.do(http.MethodPatch, path, h.token, `{"schemas":["`+testSCIMPatchOp+`"],"Operations":`+ops+`}`)
}

func (h *scimHarness) platformUser(engineID string) store.User {
	h.t.Helper()
	legacy, err := h.rt.authEngine.LegacyUserFor(context.Background(), engineID)
	if err != nil || legacy == "" {
		h.t.Fatalf("no platform user for %s: %v", engineID, err)
	}
	u, err := h.db.GetUserByID(context.Background(), legacy)
	if err != nil {
		h.t.Fatal(err)
	}
	return *u
}

func TestSCIMAuth(t *testing.T) {
	h := newSCIMHarness(t)
	if rec := h.do(http.MethodGet, "/Users", "", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("no token: %d", rec.Code)
	}
	if rec := h.do(http.MethodGet, "/Users", "forged-token", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("forged token: %d", rec.Code)
	}
	plain := httptest.NewRequest(http.MethodGet, scimBasePath+"/Users", nil)
	plain.Header.Set("Authorization", "Bearer "+h.token)
	rec := httptest.NewRecorder()
	h.rt.Handler().ServeHTTP(rec, plain)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("plain http: %d", rec.Code)
	}
	if rec := h.do(http.MethodGet, "/ServiceProviderConfig", h.token, ""); rec.Code != http.StatusOK {
		t.Fatalf("service provider config: %d %s", rec.Code, rec.Body)
	}
	rot := httptest.NewRecorder()
	h.rt.Handler().ServeHTTP(rot, authedRequest(t, h.admin, http.MethodPost, "/api/v1/sso/scim/tokens/"+h.tokID+"/rotate", ""))
	if rot.Code != http.StatusCreated {
		t.Fatalf("rotate: %d %s", rot.Code, rot.Body)
	}
	var next scimTokenCreated
	_ = json.Unmarshal(rot.Body.Bytes(), &next)
	if rec := h.do(http.MethodGet, "/Users", h.token, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("rotated-out token still works: %d", rec.Code)
	}
	if rec := h.do(http.MethodGet, "/Users", next.Token, ""); rec.Code != http.StatusOK {
		t.Fatalf("rotated token: %d %s", rec.Code, rec.Body)
	}
	var stored int
	if err := h.db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM theauth_scim_tokens WHERE token_hash = ?`, next.Token).Scan(&stored); err != nil || stored != 0 {
		t.Fatalf("plaintext token stored: %d %v", stored, err)
	}
}

func TestSCIMUserLifecycle(t *testing.T) {
	h := newSCIMHarness(t)
	ctx := context.Background()
	id := h.createUser("ada@example.com")
	user := h.platformUser(id)
	if user.Email != "ada@example.com" {
		t.Fatalf("platform email = %q", user.Email)
	}
	if role, _ := h.db.UserRoleID(ctx, user.ID); role != "role_viewer" {
		t.Fatalf("default role = %q", role)
	}
	if rec := h.do(http.MethodGet, "/Users/"+id, h.token, ""); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "ada@example.com") {
		t.Fatalf("get: %d %s", rec.Code, rec.Body)
	}
	if rec := h.patch("/Users/"+id, `[{"op":"replace","path":"displayName","value":"Ada Lovelace"}]`); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Ada Lovelace") {
		t.Fatalf("update: %d %s", rec.Code, rec.Body)
	}
	if h.rt.accountDisabled(ctx, user.ID) {
		t.Fatal("an update disabled the account")
	}
	if rec := h.patch("/Users/"+id, `[{"op":"replace","path":"active","value":false}]`); rec.Code != http.StatusOK {
		t.Fatalf("deactivate: %d %s", rec.Code, rec.Body)
	}
	if !h.rt.accountDisabled(ctx, user.ID) {
		t.Fatal("deactivation left the account enabled")
	}
	if rec := h.do(http.MethodPost, "/Users", h.token, `{"schemas":["`+testSCIMUserSchema+`"],"userName":"ada@example.com","externalId":"ext-ada@example.com","emails":[{"value":"ada@example.com","primary":true}]}`); rec.Code != http.StatusOK {
		t.Fatalf("re-provision: %d %s", rec.Code, rec.Body)
	}
	if h.rt.accountDisabled(ctx, user.ID) {
		t.Fatal("re-provisioning left the account disabled")
	}
	if rec := h.do(http.MethodDelete, "/Users/"+id, h.token, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body)
	}
	if !h.rt.accountDisabled(ctx, user.ID) {
		t.Fatal("delete left the account enabled")
	}
	if rec := h.do(http.MethodGet, "/Users/"+id, h.token, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("deleted user still visible: %d", rec.Code)
	}
}

// Deactivation must end what the user already holds, not only block new sign-ins.
func TestSCIMDeactivationRevokesSessionsAndTokens(t *testing.T) {
	h := newSCIMHarness(t)
	ctx := context.Background()
	id := h.createUser("cy@example.com")
	user := h.platformUser(id)
	token, err := h.rt.libSessions.IssueSession(ctx, toLibUser(user), "test", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	sess := &http.Cookie{Name: sessionCookieName, Value: token} //nolint:gosec // request cookie, attributes only apply to Set-Cookie
	get := func(req *http.Request) int {
		rec := httptest.NewRecorder()
		h.rt.Handler().ServeHTTP(rec, req)
		return rec.Code
	}
	if code := get(authedRequest(t, sess, http.MethodGet, "/api/v1/auth/session", "")); code != http.StatusOK {
		t.Fatalf("session before: %d", code)
	}
	mint := httptest.NewRecorder()
	h.rt.Handler().ServeHTTP(mint, authedRequest(t, sess, http.MethodPost, "/api/v1/auth/tokens", `{"name":"ci","abilities":["read"]}`))
	if mint.Code != http.StatusCreated {
		t.Fatalf("mint api token: %d %s", mint.Code, mint.Body)
	}
	var created createTokenResponse
	_ = json.Unmarshal(mint.Body.Bytes(), &created)
	withBearer := func() *http.Request {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/apps", nil)
		req.Header.Set("Authorization", "Bearer "+created.Token)
		return req
	}
	if code := get(withBearer()); code != http.StatusOK {
		t.Fatalf("api token before: %d", code)
	}
	var before int
	if err := h.db.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM theauth_sessions WHERE user_id = ? AND revoked_at IS NULL)
		+ (SELECT COUNT(*) FROM theauth_api_tokens WHERE owner_id = ? AND revoked_at IS NULL)`, id, id).Scan(&before); err != nil || before < 2 {
		t.Fatalf("positive control: live library sessions and tokens = %d %v", before, err)
	}

	if rec := h.patch("/Users/"+id, `[{"op":"replace","path":"active","value":false}]`); rec.Code != http.StatusOK {
		t.Fatalf("deactivate: %d %s", rec.Code, rec.Body)
	}
	if code := get(authedRequest(t, sess, http.MethodGet, "/api/v1/auth/session", "")); code != http.StatusUnauthorized {
		t.Fatalf("session after deactivation: %d", code)
	}
	if code := get(withBearer()); code != http.StatusUnauthorized {
		t.Fatalf("api token after deactivation: %d", code)
	}
	var live int
	if err := h.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM theauth_sessions WHERE user_id = ? AND revoked_at IS NULL`, id).Scan(&live); err != nil || live != 0 {
		t.Fatalf("library sessions still live: %d %v", live, err)
	}
	if err := h.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM theauth_api_tokens WHERE owner_id = ? AND revoked_at IS NULL`, id).Scan(&live); err != nil || live != 0 {
		t.Fatalf("library api tokens still live: %d %v", live, err)
	}
}

func TestSCIMAcceptsEntraActiveStrings(t *testing.T) {
	h := newSCIMHarness(t)
	id := h.createUser("entra@example.com")
	if rec := h.patch("/Users/"+id, `[{"op":"Replace","path":"active","value":"False"}]`); rec.Code != http.StatusOK {
		t.Fatalf("deactivate with a string: %d %s", rec.Code, rec.Body)
	}
	if !h.rt.accountDisabled(context.Background(), h.platformUser(id).ID) {
		t.Fatal("string False did not deactivate")
	}
}

func TestSCIMAdoptsExistingAccountAndCreatesInactive(t *testing.T) {
	h := newSCIMHarness(t)
	ctx := context.Background()
	existing := storeUserForTest(t, h.db, "known@example.com")
	rec := h.do(http.MethodPost, "/Users", h.token, `{"schemas":["`+testSCIMUserSchema+`"],"userName":"known@example.com","externalId":"ext-known","emails":[{"value":"known@example.com","primary":true}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("adopt: %d %s", rec.Code, rec.Body)
	}
	id, _ := scimResponseID(rec.Body.Bytes())
	if got := h.platformUser(id); got.ID != existing.ID {
		t.Fatalf("adopted %q, want %q", got.ID, existing.ID)
	}
	rec = h.do(http.MethodPost, "/Users", h.token, `{"schemas":["`+testSCIMUserSchema+`"],"userName":"off@example.com","externalId":"ext-off","emails":[{"value":"off@example.com","primary":true}],"active":false}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create inactive: %d %s", rec.Code, rec.Body)
	}
	off, _ := scimResponseID(rec.Body.Bytes())
	if !h.rt.accountDisabled(ctx, h.platformUser(off).ID) {
		t.Fatal("a user created inactive can sign in")
	}
}

func TestSCIMGroupsDriveRoles(t *testing.T) {
	h := newSCIMHarness(t)
	ctx := context.Background()
	put := httptest.NewRecorder()
	h.rt.Handler().ServeHTTP(put, authedRequest(t, h.admin, http.MethodPut, "/api/v1/sso/group-mappings", `{"mappings":[{"group":"Engineering","role_id":"role_operator","priority":1}]}`))
	if put.Code != http.StatusOK {
		t.Fatalf("mappings: %d %s", put.Code, put.Body)
	}
	id := h.createUser("bea@example.com")
	user := h.platformUser(id)
	rec := h.do(http.MethodPost, "/Groups", h.token, `{"schemas":["urn:ietf:params:scim:schemas:core:2.0:Group"],"displayName":"Engineering","members":[{"value":"`+id+`"}]}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("group: %d %s", rec.Code, rec.Body)
	}
	gid, _ := scimResponseID(rec.Body.Bytes())
	if role, _ := h.db.UserRoleID(ctx, user.ID); role != "role_operator" {
		t.Fatalf("group join role = %q", role)
	}
	if rec := h.patch("/Groups/"+gid, `[{"op":"remove","path":"members","value":[{"value":"`+id+`"}]}]`); rec.Code != http.StatusOK && rec.Code != http.StatusNoContent {
		t.Fatalf("leave group: %d %s", rec.Code, rec.Body)
	}
	if role, _ := h.db.UserRoleID(ctx, user.ID); role != "role_viewer" {
		t.Fatalf("group leave role = %q", role)
	}
}

func TestSCIMKeepsRootOutOfProvisioning(t *testing.T) {
	h := newSCIMHarness(t)
	ctx := context.Background()
	root, err := h.db.GetUserByEmail(ctx, testAdminUsername)
	if err != nil {
		t.Fatal(err)
	}
	t.Run("adopting a root account is refused", func(t *testing.T) {
		rec := h.do(http.MethodPost, "/Users", h.token, `{"schemas":["`+testSCIMUserSchema+`"],"userName":"`+root.Email+`","externalId":"ext-root","emails":[{"value":"`+root.Email+`","primary":true}]}`)
		if rec.Code != http.StatusConflict {
			t.Fatalf("adopt root: %d %s", rec.Code, rec.Body)
		}
	})
	t.Run("deprovisioning a non-last root is refused", func(t *testing.T) {
		id := h.createUser("second@example.com")
		second := h.platformUser(id)
		if err := h.db.UpdateUserAbilities(ctx, second.ID, []string{AbilityRoot}); err != nil {
			t.Fatal(err)
		}
		if rec := h.patch("/Users/"+id, `[{"op":"replace","path":"active","value":false}]`); rec.Code != http.StatusConflict {
			t.Fatalf("deactivate: %d %s", rec.Code, rec.Body)
		}
		if rec := h.do(http.MethodDelete, "/Users/"+id, h.token, ""); rec.Code != http.StatusConflict {
			t.Fatalf("delete: %d %s", rec.Code, rec.Body)
		}
		if h.rt.accountDisabled(ctx, second.ID) {
			t.Fatal("a root member was disabled")
		}
	})
}

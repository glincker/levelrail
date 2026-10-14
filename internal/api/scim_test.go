package api

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/scim"
	"github.com/GLINCKER/levelrail/internal/store"
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
	if !strings.HasPrefix(out.Token, scimTokenPrefix) || out.Prefix != out.Token[:scimTokenShownLen] {
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
	req.Header.Set("Content-Type", scim.ContentType)
	rec := httptest.NewRecorder()
	h.rt.Handler().ServeHTTP(rec, req)
	return rec
}

func (h *scimHarness) createUser(email string) string {
	h.t.Helper()
	rec := h.do(http.MethodPost, "/Users", h.token, `{"schemas":["`+scim.SchemaUser+`"],"userName":"`+email+`","externalId":"ext-`+email+`","name":{"givenName":"Ada","familyName":"L"},"emails":[{"value":"`+email+`","primary":true}],"active":true}`)
	if rec.Code != http.StatusCreated {
		h.t.Fatalf("create user: %d %s", rec.Code, rec.Body)
	}
	var u scim.User
	_ = json.Unmarshal(rec.Body.Bytes(), &u)
	return u.ID
}

func TestSCIMAuth(t *testing.T) {
	h := newSCIMHarness(t)
	if rec := h.do(http.MethodGet, "/Users", "", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("no token: %d", rec.Code)
	}
	if rec := h.do(http.MethodGet, "/Users", scimTokenPrefix+"forged", ""); rec.Code != http.StatusUnauthorized {
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
		t.Fatalf("valid token: %d", rec.Code)
	}

	rot := httptest.NewRecorder()
	h.rt.Handler().ServeHTTP(rot, authedRequest(t, h.admin, http.MethodPost, "/api/v1/sso/scim/tokens/"+h.tokID+"/rotate", ""))
	var next scimTokenCreated
	_ = json.Unmarshal(rot.Body.Bytes(), &next)
	if rot.Code != http.StatusCreated || next.Token == "" || next.Name != "okta" {
		t.Fatalf("rotate: %d %s", rot.Code, rot.Body)
	}
	if rec := h.do(http.MethodGet, "/Users", h.token, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("rotated-out token still works: %d", rec.Code)
	}
	if rec := h.do(http.MethodGet, "/Users", next.Token, ""); rec.Code != http.StatusOK {
		t.Fatalf("new token: %d", rec.Code)
	}
	rev := httptest.NewRecorder()
	h.rt.Handler().ServeHTTP(rev, authedRequest(t, h.admin, http.MethodDelete, "/api/v1/sso/scim/tokens/"+next.ID, ""))
	if rev.Code != http.StatusNoContent {
		t.Fatalf("revoke: %d", rev.Code)
	}
	if rec := h.do(http.MethodGet, "/Users", next.Token, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("revoked token still works: %d", rec.Code)
	}
	var stored string
	_ = h.db.QueryRowContext(context.Background(), `SELECT token_hash FROM scim_tokens WHERE id = ?`, next.ID).Scan(&stored)
	if stored == next.Token || stored != hashToken(next.Token) {
		t.Fatal("token is not stored as its hash")
	}
}

func TestSCIMDeprovisionEndsSessionsAndIsAudited(t *testing.T) {
	h := newSCIMHarness(t)
	ctx := context.Background()
	id := h.createUser("ada@example.com")

	list := h.do(http.MethodGet, `/Users?filter=userName%20eq%20%22ADA@example.com%22`, h.token, "")
	var found scim.UserList
	_ = json.Unmarshal(list.Body.Bytes(), &found)
	if found.TotalResults != 1 || found.Resources[0].ID != id || found.Resources[0].ExternalID != "ext-ada@example.com" {
		t.Fatalf("filter: %s", list.Body)
	}
	if rec := h.do(http.MethodGet, `/Users?filter=password%20eq%20%22x%22`, h.token, ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad filter: %d", rec.Code)
	}

	user, err := h.db.GetUserByID(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	tok, err := h.rt.libSessions.IssueSession(ctx, toLibUser(*user), "test", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := h.rt.sessions.lookup(tok); !ok {
		t.Fatal("session not live before deprovisioning")
	}
	rec := h.do(http.MethodPatch, "/Users/"+id, h.token, `{"schemas":["`+scim.SchemaPatchOp+`"],"Operations":[{"op":"Replace","path":"active","value":"False"}]}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"active":false`) {
		t.Fatalf("deactivate: %d %s", rec.Code, rec.Body)
	}
	if _, ok := h.rt.sessions.lookup(tok); ok {
		t.Fatal("session survived deprovisioning")
	}
	if !h.rt.accountDisabled(ctx, id) {
		t.Fatal("account not disabled")
	}
	entries, err := h.db.ListAuditEntries(ctx, 50, nil, store.AuditEntryFilter{})
	if err != nil {
		t.Fatal(err)
	}
	audited := false
	for _, e := range entries {
		if e.Action == auditActionSCIMUserDeactivated && e.ActorID == h.tokID {
			audited = true
		}
	}
	if !audited {
		t.Fatal("deprovisioning was not audited")
	}

	rec = h.do(http.MethodPatch, "/Users/"+id, h.token, `{"Operations":[{"op":"replace","value":{"active":true}}]}`)
	if rec.Code != http.StatusOK || h.rt.accountDisabled(ctx, id) {
		t.Fatalf("reactivate: %d %s", rec.Code, rec.Body)
	}

	if rec := h.do(http.MethodDelete, "/Users/"+id, h.token, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d", rec.Code)
	}
	if rec := h.do(http.MethodGet, "/Users/"+id, h.token, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("get after delete: %d", rec.Code)
	}
	if !h.rt.accountDisabled(ctx, id) {
		t.Fatal("deleted account is not disabled")
	}
	if again := h.createUser("ada@example.com"); again != id || h.rt.accountDisabled(ctx, id) {
		t.Fatalf("re-create did not restore the account: %s", again)
	}
}

func TestSCIMRefusesLastRoot(t *testing.T) {
	h := newSCIMHarness(t)
	root, err := h.db.GetUserByEmail(context.Background(), testAdminUsername)
	if err != nil {
		t.Fatal(err)
	}
	rec := h.do(http.MethodPatch, "/Users/"+root.ID, h.token, `{"Operations":[{"op":"replace","path":"active","value":false}]}`)
	if rec.Code != http.StatusConflict || h.rt.accountDisabled(context.Background(), root.ID) {
		t.Fatalf("last root deactivated: %d %s", rec.Code, rec.Body)
	}
	if rec := h.do(http.MethodDelete, "/Users/"+root.ID, h.token, ""); rec.Code != http.StatusConflict {
		t.Fatalf("last root deleted: %d", rec.Code)
	}
}

func TestSCIMGroupsDriveRoles(t *testing.T) {
	h := newSCIMHarness(t)
	ctx := context.Background()
	if err := h.db.ReplaceSSOGroupMappings(ctx, []store.SSOGroupMapping{{Group: "Engineering", RoleID: "role_operator", Priority: 1}}, time.Now()); err != nil {
		t.Fatal(err)
	}
	id := h.createUser("grace@example.com")
	rec := h.do(http.MethodPost, "/Groups", h.token, `{"schemas":["`+scim.SchemaGroup+`"],"displayName":"engineering","members":[{"value":"`+id+`"}]}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create group: %d %s", rec.Code, rec.Body)
	}
	var g scim.Group
	_ = json.Unmarshal(rec.Body.Bytes(), &g)
	if role, _ := h.db.UserRoleID(ctx, id); role != "role_operator" {
		t.Fatalf("role after join = %q", role)
	}
	if rec := h.do(http.MethodPost, "/Groups", h.token, `{"displayName":"x","members":[{"value":"user_missing"}]}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown member: %d", rec.Code)
	}
	rec = h.do(http.MethodPatch, "/Groups/"+g.ID, h.token, `{"Operations":[{"op":"remove","path":"members[value eq \"`+id+`\"]"}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("remove member: %d %s", rec.Code, rec.Body)
	}
	if role, _ := h.db.UserRoleID(ctx, id); role != "role_viewer" {
		t.Fatalf("role after leave = %q", role)
	}
	if rec := h.do(http.MethodGet, `/Groups?filter=displayName%20eq%20%22Engineering%22`, h.token, ""); !strings.Contains(rec.Body.String(), g.ID) {
		t.Fatalf("group filter: %s", rec.Body)
	}
	if rec := h.do(http.MethodDelete, "/Groups/"+g.ID, h.token, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete group: %d", rec.Code)
	}
}

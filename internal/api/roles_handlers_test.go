package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/experimental"
)

func enableAccessRoles(t *testing.T) {
	t.Helper()
	experimental.Set(experimental.AccessRoles)
	t.Cleanup(experimental.Reset)
}

func roleReq(t *testing.T, rt *Router, cookie *http.Cookie, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, target, body))
	return rec
}

func createCustomRole(t *testing.T, rt *Router, cookie *http.Cookie, body string) Role {
	t.Helper()
	rec := roleReq(t, rt, cookie, http.MethodPost, "/api/v1/roles", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create role: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got Role
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode role: %v", err)
	}
	return got
}

func TestRoleWriteRoutes_RequireAuth(t *testing.T) {
	rt, _ := newTestRouter(t)
	assertRoutesRequireAuth(t, rt, []routeCase{
		{http.MethodPost, "/api/v1/roles"},
		{http.MethodPut, "/api/v1/roles/role_x"},
		{http.MethodDelete, "/api/v1/roles/role_x"},
		{http.MethodPut, "/api/v1/users/u/role"},
		{http.MethodGet, "/api/v1/users/u/environment-grants"},
		{http.MethodPut, "/api/v1/users/u/environment-grants"},
	})
}

func TestRoleWriteRoutes_RequireRoot(t *testing.T) {
	enableAccessRoles(t)
	rt, db := newTestRouter(t)
	op := storeUserWithAbilitiesForTest(t, db, "op@example.com", []string{AbilityRead, AbilityWrite, AbilityDeploy})
	cookie := sessionCookieForTest(t, rt, op.ID)
	for _, c := range []struct{ method, target, body string }{
		{http.MethodPost, "/api/v1/roles", `{"name":"x","abilities":["read"]}`},
		{http.MethodPut, "/api/v1/roles/role_x", `{"name":"x","abilities":["read"]}`},
		{http.MethodDelete, "/api/v1/roles/role_x", ""},
		{http.MethodPut, "/api/v1/users/" + op.ID + "/role", `{"role_id":"role_viewer"}`},
		{http.MethodGet, "/api/v1/users/" + op.ID + "/environment-grants", ""},
		{http.MethodPut, "/api/v1/users/" + op.ID + "/environment-grants", `{"environment_ids":[]}`},
	} {
		if rec := roleReq(t, rt, cookie, c.method, c.target, c.body); rec.Code != http.StatusForbidden {
			t.Errorf("%s %s: status = %d, want 403", c.method, c.target, rec.Code)
		}
	}
}

func TestRoleWriteRoutes_GatedByFlag(t *testing.T) {
	experimental.Set()
	t.Cleanup(experimental.Reset)
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := roleReq(t, rt, cookie, http.MethodPost, "/api/v1/roles", `{"name":"qa","abilities":["read"]}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body = %s", rec.Code, rec.Body.String())
	}
	var e experimentalError
	if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil || e.Code != ExperimentalDisabledCode || e.Feature != "access-roles" {
		t.Errorf("error body = %+v, %v, want the experimental-disabled shape for access-roles", e, err)
	}
	if rec := roleReq(t, rt, cookie, http.MethodGet, "/api/v1/roles", ""); rec.Code != http.StatusOK {
		t.Errorf("GET /roles with flag off: status = %d, want 200 (ungated)", rec.Code)
	}
}

func TestCreateRole_ValidationAndRoundTrip(t *testing.T) {
	enableAccessRoles(t)
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	for name, body := range map[string]string{
		"no name":             `{"name":" ","abilities":["read"]}`,
		"no abilities":        `{"name":"a","abilities":[]}`,
		"unknown ability":     `{"name":"a","abilities":["fly"]}`,
		"root not exclusive":  `{"name":"a","abilities":["root","read"]}`,
		"bad visibility":      `{"name":"a","abilities":["read"],"visibility":"some"}`,
		"granted with write":  `{"name":"a","abilities":["read","write"],"visibility":"granted"}`,
		"granted with deploy": `{"name":"a","abilities":["deploy"],"visibility":"granted"}`,
		"name too long":       `{"name":"` + strings.Repeat("a", 65) + `","abilities":["read"]}`,
	} {
		if rec := roleReq(t, rt, cookie, http.MethodPost, "/api/v1/roles", body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400, body = %s", name, rec.Code, rec.Body.String())
		}
	}

	created := createCustomRole(t, rt, cookie, `{"name":"auditor","description":"d","abilities":["read","read:sensitive"]}`)
	if created.ID == "" || created.Builtin || created.Visibility != "all" || created.UserCount != 0 {
		t.Errorf("created = %+v", created)
	}
	granted := createCustomRole(t, rt, cookie, `{"name":"tourist","abilities":["read"],"visibility":"granted"}`)
	if granted.Visibility != "granted" {
		t.Errorf("visibility = %q, want granted", granted.Visibility)
	}
	if rec := roleReq(t, rt, cookie, http.MethodPost, "/api/v1/roles", `{"name":"auditor","abilities":["read"]}`); rec.Code != http.StatusConflict {
		t.Errorf("duplicate name: status = %d, want 409", rec.Code)
	}

	rec := roleReq(t, rt, cookie, http.MethodGet, "/api/v1/roles", "")
	var list []Role
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 6 || !list[0].Builtin {
		t.Errorf("list = %+v, want 4 built-ins then 2 custom", list)
	}
}

func TestBuiltinRolesAreImmutable(t *testing.T) {
	enableAccessRoles(t)
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	if rec := roleReq(t, rt, cookie, http.MethodPut, "/api/v1/roles/role_viewer", `{"name":"viewer","abilities":["read","write"]}`); rec.Code != http.StatusForbidden {
		t.Errorf("update built-in: status = %d, want 403", rec.Code)
	}
	if rec := roleReq(t, rt, cookie, http.MethodDelete, "/api/v1/roles/role_guest", ""); rec.Code != http.StatusForbidden {
		t.Errorf("delete built-in: status = %d, want 403", rec.Code)
	}
	if rec := roleReq(t, rt, cookie, http.MethodDelete, "/api/v1/roles/role_nope", ""); rec.Code != http.StatusNotFound {
		t.Errorf("delete missing: status = %d, want 404", rec.Code)
	}
}

func TestDeleteRole_InUseIs409(t *testing.T) {
	enableAccessRoles(t)
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	role := createCustomRole(t, rt, cookie, `{"name":"qa","abilities":["read"]}`)
	u := storeUserForTest(t, db, "qa@example.com")
	if rec := roleReq(t, rt, cookie, http.MethodPut, "/api/v1/users/"+u.ID+"/role", `{"role_id":"`+role.ID+`"}`); rec.Code != http.StatusOK {
		t.Fatalf("assign: status = %d, body = %s", rec.Code, rec.Body.String())
	}

	if rec := roleReq(t, rt, cookie, http.MethodDelete, "/api/v1/roles/"+role.ID, ""); rec.Code != http.StatusConflict {
		t.Fatalf("delete in use: status = %d, want 409", rec.Code)
	}
	if rec := roleReq(t, rt, cookie, http.MethodPut, "/api/v1/users/"+u.ID+"/role", `{"role_id":"role_viewer"}`); rec.Code != http.StatusOK {
		t.Fatalf("reassign: status = %d", rec.Code)
	}
	if rec := roleReq(t, rt, cookie, http.MethodDelete, "/api/v1/roles/"+role.ID, ""); rec.Code != http.StatusNoContent {
		t.Errorf("delete unused: status = %d, want 204", rec.Code)
	}
}

func TestUpdateRole_ResyncsUserAbilities(t *testing.T) {
	enableAccessRoles(t)
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	role := createCustomRole(t, rt, cookie, `{"name":"qa","abilities":["read"]}`)
	u := storeUserForTest(t, db, "qa@example.com")
	roleReq(t, rt, cookie, http.MethodPut, "/api/v1/users/"+u.ID+"/role", `{"role_id":"`+role.ID+`"}`)

	rec := roleReq(t, rt, cookie, http.MethodPut, "/api/v1/roles/"+role.ID, `{"name":"qa","abilities":["read","deploy"]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("update: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	got, err := db.GetUserByID(context.Background(), u.ID)
	if err != nil || !slices.Equal(got.Abilities, []string{"read", "deploy"}) {
		t.Errorf("user abilities = %v, %v, want the role's new set", got.Abilities, err)
	}
	var resp Role
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.UserCount != 1 {
		t.Errorf("user_count = %d, want 1", resp.UserCount)
	}
}

func TestSetUserRole_Rules(t *testing.T) {
	enableAccessRoles(t)
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	admin, err := db.GetUserByEmail(context.Background(), testAdminUsername)
	if err != nil {
		t.Fatalf("admin lookup: %v", err)
	}
	u := storeUserForTest(t, db, "u@example.com")

	if rec := roleReq(t, rt, cookie, http.MethodPut, "/api/v1/users/"+admin.ID+"/role", `{"role_id":"role_viewer"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("self change: status = %d, want 400", rec.Code)
	}
	if rec := roleReq(t, rt, cookie, http.MethodPut, "/api/v1/users/missing/role", `{"role_id":"role_viewer"}`); rec.Code != http.StatusNotFound {
		t.Errorf("missing user: status = %d, want 404", rec.Code)
	}
	if rec := roleReq(t, rt, cookie, http.MethodPut, "/api/v1/users/"+u.ID+"/role", `{"role_id":"role_nope"}`); rec.Code != http.StatusNotFound {
		t.Errorf("missing role: status = %d, want 404", rec.Code)
	}
	if rec := roleReq(t, rt, cookie, http.MethodPut, "/api/v1/users/"+u.ID+"/role", `{}`); rec.Code != http.StatusBadRequest {
		t.Errorf("empty body: status = %d, want 400", rec.Code)
	}
	rec := roleReq(t, rt, cookie, http.MethodPut, "/api/v1/users/"+u.ID+"/role", `{"role_id":"role_operator"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("assign: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var res userResource
	_ = json.Unmarshal(rec.Body.Bytes(), &res)
	if res.RoleID != "role_operator" || res.Role != "operator" || !slices.Contains(res.Abilities, AbilityDeploy) {
		t.Errorf("user resource = %+v", res)
	}
}

// A root bearer token is the one caller that is not itself a root user, so it is the only way to reach the last-root guards.
func rootTokenRequest(rt *Router, plain, method, target, body string, t *testing.T) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+plain)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	return rec
}

func TestLastRootGuard_RoleChange(t *testing.T) {
	enableAccessRoles(t)
	rt, db := newTestRouter(t)
	ctx := context.Background()
	bootstrapTestAdmin(t, db)
	only, _ := db.GetUserByEmail(ctx, testAdminUsername)
	plain := seedMatrixToken(t, db, "tok_root_guard", []string{AbilityRoot})

	rec := rootTokenRequest(rt, plain, http.MethodPut, "/api/v1/users/"+only.ID+"/role", `{"role_id":"role_viewer"}`, t)
	if rec.Code != http.StatusConflict {
		t.Fatalf("demote last root: status = %d, want 409, body = %s", rec.Code, rec.Body.String())
	}

	second := storeUserWithAbilitiesForTest(t, db, "second@example.com", []string{AbilityRoot})
	rec = rootTokenRequest(rt, plain, http.MethodPut, "/api/v1/users/"+only.ID+"/role", `{"role_id":"role_viewer"}`, t)
	if rec.Code != http.StatusOK {
		t.Errorf("demote one of two roots: status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	rec = rootTokenRequest(rt, plain, http.MethodPut, "/api/v1/users/"+second.ID+"/role", `{"role_id":"role_viewer"}`, t)
	if rec.Code != http.StatusConflict {
		t.Errorf("demote the remaining root: status = %d, want 409", rec.Code)
	}
}

func TestLastRootGuard_DeleteUser(t *testing.T) {
	rt, db := newTestRouter(t)
	ctx := context.Background()
	bootstrapTestAdmin(t, db)
	only, _ := db.GetUserByEmail(ctx, testAdminUsername)
	plain := seedMatrixToken(t, db, "tok_root_guard", []string{AbilityRoot})
	storeUserWithAbilitiesForTest(t, db, "viewer@example.com", []string{AbilityRead})

	rec := rootTokenRequest(rt, plain, http.MethodDelete, "/api/v1/users/"+only.ID, "", t)
	if rec.Code != http.StatusConflict {
		t.Fatalf("delete last root: status = %d, want 409, body = %s", rec.Code, rec.Body.String())
	}
}

func TestLastRootGuard_RoleEdit(t *testing.T) {
	enableAccessRoles(t)
	rt, db := newTestRouter(t)
	ctx := context.Background()
	cookie := loginTestSession(t, rt, db)
	role := createCustomRole(t, rt, cookie, `{"name":"owners","abilities":["root"]}`)
	admin, _ := db.GetUserByEmail(ctx, testAdminUsername)
	if err := db.SetUserRole(ctx, admin.ID, role.ID); err != nil {
		t.Fatal(err)
	}

	rec := roleReq(t, rt, cookie, http.MethodPut, "/api/v1/roles/"+role.ID, `{"name":"owners","abilities":["read"]}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("strip root from the only root role: status = %d, want 409, body = %s", rec.Code, rec.Body.String())
	}
	storeUserWithAbilitiesForTest(t, db, "other@example.com", []string{AbilityRoot})
	rec = roleReq(t, rt, cookie, http.MethodPut, "/api/v1/roles/"+role.ID, `{"name":"owners","abilities":["read"]}`)
	if rec.Code != http.StatusOK {
		t.Errorf("strip root while another root exists: status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
}

func TestRoleCapGrantedAbilityRestriction_OnUpdate(t *testing.T) {
	enableAccessRoles(t)
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	role := createCustomRole(t, rt, cookie, `{"name":"qa","abilities":["read","write"]}`)
	if rec := roleReq(t, rt, cookie, http.MethodPut, "/api/v1/roles/"+role.ID, `{"name":"qa","abilities":["read","write"],"visibility":"granted"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for granted visibility with write", rec.Code)
	}
}

func TestEnvironmentGrants(t *testing.T) {
	enableAccessRoles(t)
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	u := storeUserForTest(t, db, "guest@example.com")
	path := "/api/v1/users/" + u.ID + "/environment-grants"

	if rec := roleReq(t, rt, cookie, http.MethodPut, path, `{"environment_ids":["env_dev"]}`); rec.Code != http.StatusBadRequest {
		t.Errorf("user without granted role: status = %d, want 400", rec.Code)
	}
	if rec := roleReq(t, rt, cookie, http.MethodPut, "/api/v1/users/"+u.ID+"/role", `{"role_id":"role_guest"}`); rec.Code != http.StatusOK {
		t.Fatalf("assign guest: status = %d", rec.Code)
	}
	if rec := roleReq(t, rt, cookie, http.MethodPut, path, `{"environment_ids":["env_nope"]}`); rec.Code != http.StatusBadRequest {
		t.Errorf("unknown environment: status = %d, want 400", rec.Code)
	}
	rec := roleReq(t, rt, cookie, http.MethodPut, path, `{"environment_ids":["env_dev","env_test","env_dev"]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("put: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	rec = roleReq(t, rt, cookie, http.MethodGet, path, "")
	var got environmentGrantsBody
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if !slices.Equal(got.EnvironmentIDs, []string{"env_dev", "env_test"}) {
		t.Errorf("grants = %v, want [env_dev env_test]", got.EnvironmentIDs)
	}
	vis, err := db.UserVisibility(context.Background(), u.ID)
	if err != nil || !vis.Allows("env_dev") || vis.Allows("env_uat") {
		t.Errorf("visibility = %+v, %v", vis, err)
	}
	if rec := roleReq(t, rt, cookie, http.MethodGet, "/api/v1/users/missing/environment-grants", ""); rec.Code != http.StatusNotFound {
		t.Errorf("missing user: status = %d, want 404", rec.Code)
	}
}

func TestInviteAndCreateUserWithStoredRoles(t *testing.T) {
	enableAccessRoles(t)
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	role := createCustomRole(t, rt, cookie, `{"name":"qa","abilities":["read","deploy"]}`)

	rec := roleReq(t, rt, cookie, http.MethodPost, "/api/v1/invites", `{"email":"qa@example.com","role_id":"`+role.ID+`"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("invite: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var inv createInviteResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &inv)
	if inv.Role != "qa" || !slices.Equal(inv.Abilities, []string{"read", "deploy"}) {
		t.Errorf("invite = %+v", inv.inviteResource)
	}
	token := inv.Link[strings.Index(inv.Link, "token=")+len("token="):]
	acc := httptest.NewRecorder()
	rt.Handler().ServeHTTP(acc, httptest.NewRequest(http.MethodPost, "/api/v1/invites/accept", strings.NewReader(`{"token":"`+token+`","password":"password-123"}`)))
	if acc.Code != http.StatusCreated {
		t.Fatalf("accept: status = %d, body = %s", acc.Code, acc.Body.String())
	}
	u, err := db.GetUserByEmail(context.Background(), "qa@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if id, _ := db.UserRoleID(context.Background(), u.ID); id != role.ID {
		t.Errorf("accepted user role_id = %q, want %q", id, role.ID)
	}

	if rec := roleReq(t, rt, cookie, http.MethodPost, "/api/v1/invites", `{"email":"x@example.com","role":"guest"}`); rec.Code != http.StatusCreated {
		t.Errorf("invite by role name guest: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if rec := roleReq(t, rt, cookie, http.MethodPost, "/api/v1/invites", `{"email":"y@example.com","role_id":"role_nope"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("unknown role_id: status = %d, want 400", rec.Code)
	}

	cu := roleReq(t, rt, cookie, http.MethodPost, "/api/v1/auth/users", `{"email":"c@example.com","password":"password-123","role_id":"`+role.ID+`"}`)
	if cu.Code != http.StatusCreated {
		t.Fatalf("create user: status = %d, body = %s", cu.Code, cu.Body.String())
	}
	var res userResource
	_ = json.Unmarshal(cu.Body.Bytes(), &res)
	if res.RoleID != role.ID || res.Role != "qa" {
		t.Errorf("created user = %+v", res)
	}
}

func TestRoleIDRequiresFlag(t *testing.T) {
	experimental.Set()
	t.Cleanup(experimental.Reset)
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rec := roleReq(t, rt, cookie, http.MethodPost, "/api/v1/invites", `{"email":"x@example.com","role_id":"role_viewer"}`)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 with the flag off", rec.Code)
	}
	if rec := roleReq(t, rt, cookie, http.MethodPost, "/api/v1/invites", `{"email":"x@example.com","role":"viewer"}`); rec.Code != http.StatusCreated {
		t.Errorf("legacy role name: status = %d, want 201", rec.Code)
	}
}

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func bindingsDo(t *testing.T, rt *Router, method, path, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	return rec
}

func seedBindingFixture(t *testing.T) (*Router, *store.DB) {
	t.Helper()
	rt, db, _ := newPipelineRouter(t)
	bootstrapTestAdmin(t, db)
	seedApp(t, db, "web")
	seedApp(t, db, "other")
	if err := db.SaveRegistryCredential(context.Background(), store.RegistryCredential{ID: "rc1", Name: "hub", RegistryHost: "registry.example.com", Username: "u", CreatedAt: "2026-01-01T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	return rt, db
}

func TestBindingsAuthzMatrix(t *testing.T) {
	t.Parallel()
	rt, db := seedBindingFixture(t)
	writeOnly := seedMatrixToken(t, db, "b-write", []string{AbilityRead})
	attachTestPolicy(t, db, "b-write-web", "Allow", AbilityWrite, "app:web", store.PrincipalTypeToken, "b-write")
	readOnly := seedMatrixToken(t, db, "b-read", []string{AbilityRead, AbilityReadSensitive})
	neither := seedMatrixToken(t, db, "b-none", []string{AbilityRead})
	both := seedMatrixToken(t, db, "b-both", []string{AbilityRead, AbilityReadSensitive})
	attachTestPolicy(t, db, "b-both-web", "Allow", AbilityWrite, "app:web", store.PrincipalTypeToken, "b-both")

	body := `{"ref":"registry:rc1","role":"registry_pull","rules":{"host":"registry.example.com"}}`
	for name, tc := range map[string]struct {
		token string
		want  int
	}{
		"write on app only":       {writeOnly, http.StatusNotFound},
		"read on connection only": {readOnly, http.StatusForbidden},
		"neither":                 {neither, http.StatusForbidden},
	} {
		if rec := bindingsDo(t, rt, http.MethodPost, "/api/v1/apps/web/bindings", tc.token, body); rec.Code != tc.want {
			t.Errorf("%s: POST = %d %s, want %d", name, rec.Code, rec.Body.String(), tc.want)
		}
	}
	rec := bindingsDo(t, rt, http.MethodPost, "/api/v1/apps/web/bindings", both, body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("both: POST = %d %s", rec.Code, rec.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &created)

	// No widening: the binding grants the app writer nothing on the connection.
	if rec := bindingsDo(t, rt, http.MethodGet, "/api/v1/connections/registry:rc1", writeOnly, ""); rec.Code != http.StatusForbidden {
		t.Errorf("app writer reads connection after bind = %d, want 403", rec.Code)
	}
	if rec := bindingsDo(t, rt, http.MethodPut, "/api/v1/connections/registry:rc1/meta", writeOnly, `{"scope_type":"instance"}`); rec.Code != http.StatusForbidden {
		t.Errorf("app writer sets meta after bind = %d, want 403", rec.Code)
	}
	// An id from another app is not found there.
	if rec := bindingsDo(t, rt, http.MethodDelete, "/api/v1/apps/other/bindings/"+created.ID, testAdminTokenFor(t, db), ""); rec.Code != http.StatusNotFound {
		t.Errorf("cross-app unbind = %d, want 404", rec.Code)
	}
	if rec := bindingsDo(t, rt, http.MethodDelete, "/api/v1/apps/web/bindings/"+created.ID, readOnly, ""); rec.Code != http.StatusForbidden {
		t.Errorf("unbind without app write = %d, want 403", rec.Code)
	}
	if rec := bindingsDo(t, rt, http.MethodDelete, "/api/v1/apps/web/bindings/"+created.ID, writeOnly, ""); rec.Code != http.StatusNoContent {
		t.Errorf("unbind = %d %s, want 204", rec.Code, rec.Body.String())
	}
	entries, err := db.ListAuditEntries(context.Background(), 100, nil, store.AuditEntryFilter{})
	if err != nil {
		t.Fatal(err)
	}
	var posts, deletes int
	for _, e := range entries {
		if strings.HasPrefix(e.Path, "/api/v1/apps/web/bindings") {
			if e.Method == http.MethodPost {
				posts++
			}
			if e.Method == http.MethodDelete {
				deletes++
			}
		}
	}
	if posts == 0 || deletes == 0 {
		t.Errorf("audit rows: posts=%d deletes=%d, want both", posts, deletes)
	}
}

func testAdminTokenFor(t *testing.T, db *store.DB) string {
	t.Helper()
	return seedMatrixToken(t, db, "b-root-"+strings.ReplaceAll(t.Name(), "/", "-"), []string{AbilityRoot, AbilityRead, AbilityReadSensitive, AbilityWrite, AbilityWriteSensitive})
}

func TestBindingsScopeAndRules(t *testing.T) {
	t.Parallel()
	rt, db := seedBindingFixture(t)
	root := testAdminTokenFor(t, db)
	for name, tc := range map[string]struct {
		body string
		want int
	}{
		"unknown role":  {`{"ref":"registry:rc1","role":"bogus"}`, http.StatusBadRequest},
		"role mismatch": {`{"ref":"registry:rc1","role":"source"}`, http.StatusBadRequest},
		"unknown rule":  {`{"ref":"registry:rc1","role":"registry_pull","rules":{"nope":1}}`, http.StatusBadRequest},
		"oversized":     {`{"ref":"registry:rc1","role":"registry_pull","rules":{"host":"` + strings.Repeat("a", 5000) + `"}}`, http.StatusBadRequest},
		"unknown ref":   {`{"ref":"registry:missing","role":"registry_pull"}`, http.StatusNotFound},
	} {
		if rec := bindingsDo(t, rt, http.MethodPost, "/api/v1/apps/web/bindings", root, tc.body); rec.Code != tc.want {
			t.Errorf("%s: POST = %d %s, want %d", name, rec.Code, rec.Body.String(), tc.want)
		}
	}
	// Unknown scope ids are refused.
	if rec := bindingsDo(t, rt, http.MethodPut, "/api/v1/connections/registry:rc1/meta", root, `{"scope_type":"org","scope_id":"org_missing"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("unknown org scope = %d, want 400", rec.Code)
	}
	if rec := bindingsDo(t, rt, http.MethodPut, "/api/v1/connections/registry:rc1/meta", root, `{"scope_type":"app","scope_id":"ghost"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("unknown app scope = %d, want 400", rec.Code)
	}
	// App scope covers only that app.
	if rec := bindingsDo(t, rt, http.MethodPut, "/api/v1/connections/registry:rc1/meta", root, `{"scope_type":"app","scope_id":"web"}`); rec.Code != http.StatusOK {
		t.Fatalf("scope to web = %d %s", rec.Code, rec.Body.String())
	}
	if rec := bindingsDo(t, rt, http.MethodPost, "/api/v1/apps/other/bindings", root, `{"ref":"registry:rc1","role":"registry_pull"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("bind outside app scope = %d, want 400", rec.Code)
	}
	if rec := bindingsDo(t, rt, http.MethodPost, "/api/v1/apps/web/bindings", root, `{"ref":"registry:rc1","role":"registry_pull"}`); rec.Code != http.StatusCreated {
		t.Fatalf("bind inside app scope = %d %s", rec.Code, rec.Body.String())
	}
	// Moving the scope away while a binding depends on it is refused.
	if rec := bindingsDo(t, rt, http.MethodPut, "/api/v1/connections/registry:rc1/meta", root, `{"scope_type":"app","scope_id":"other"}`); rec.Code != http.StatusConflict {
		t.Errorf("orphaning scope move = %d, want 409", rec.Code)
	}
	// Moving the scope off an app needs write:sensitive on that app.
	mover := seedMatrixToken(t, db, "b-mover", []string{AbilityRead, AbilityReadSensitive, AbilityWrite, AbilityWriteSensitive})
	attachTestPolicy(t, db, "b-mover-deny", "Deny", AbilityWriteSensitive, "app:web", store.PrincipalTypeToken, "b-mover")
	if rec := bindingsDo(t, rt, http.MethodPut, "/api/v1/connections/registry:rc1/meta", mover, `{"scope_type":"instance"}`); rec.Code != http.StatusForbidden {
		t.Errorf("scope move off a denied app = %d, want 403", rec.Code)
	}
}

func TestBindingsListsNeverNull(t *testing.T) {
	t.Parallel()
	rt, db := seedBindingFixture(t)
	root := testAdminTokenFor(t, db)
	rec := bindingsDo(t, rt, http.MethodGet, "/api/v1/apps/web/bindings", root, "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"bindings":[]`) || !strings.Contains(rec.Body.String(), `"resolved":[]`) {
		t.Errorf("app bindings = %d %s", rec.Code, rec.Body.String())
	}
	rec = bindingsDo(t, rt, http.MethodGet, "/api/v1/organizations/"+store.DefaultTeamID+"/bindings", root, "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"bindings":[]`) {
		t.Errorf("org bindings = %d %s", rec.Code, rec.Body.String())
	}
}

// An org-scoped token manages org defaults only with an admin role there; a developer role is refused.
func TestBindingsOrgTokenAdminAndRead(t *testing.T) {
	f := newTeamFixture(t)
	ctx := context.Background()
	if err := f.db.SaveRegistryCredential(ctx, store.RegistryCredential{ID: "rc1", Name: "hub", RegistryHost: "registry.example.com", Username: "u", CreatedAt: "2026-01-01T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	root := testAdminTokenFor(t, f.db)
	if rec := bindingsDo(t, f.rt, http.MethodPut, "/api/v1/connections/registry:rc1/meta", root, `{"scope_type":"org","scope_id":"`+teamA+`"}`); rec.Code != http.StatusOK {
		t.Fatalf("scope to org = %d %s", rec.Code, rec.Body.String())
	}
	orgToken := func(id, role string) string {
		plain := "org-bind-" + id
		abilities := []string{AbilityRead, AbilityReadSensitive, AbilityWriteSensitive}
		if err := f.db.SaveAPIToken(ctx, store.APIToken{ID: id, Name: id, TokenHash: hashToken(plain), Abilities: abilities, OwnerUserID: f.members[role].ID}); err != nil {
			t.Fatal(err)
		}
		if err := f.db.SetAPITokenScope(ctx, store.APITokenScope{TokenID: id, TeamID: teamA, Role: role}); err != nil {
			t.Fatal(err)
		}
		return plain
	}
	admin, dev := orgToken("tok-bind-admin", store.TeamRoleAdmin), orgToken("tok-bind-dev", store.TeamRoleDeveloper)
	path := "/api/v1/organizations/" + teamA + "/bindings"
	body := `{"ref":"registry:rc1","role":"registry_pull"}`

	if rec := bindingsDo(t, f.rt, http.MethodPost, path, dev, body); rec.Code != http.StatusForbidden {
		t.Errorf("non-admin POST = %d %s, want 403", rec.Code, rec.Body.String())
	}
	rec := bindingsDo(t, f.rt, http.MethodPost, path, admin, body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("admin POST = %d %s", rec.Code, rec.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	if rec := bindingsDo(t, f.rt, http.MethodGet, path, admin, ""); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), created.ID) {
		t.Errorf("admin GET = %d %s", rec.Code, rec.Body.String())
	}
	if rec := bindingsDo(t, f.rt, http.MethodDelete, path+"/"+created.ID, dev, ""); rec.Code != http.StatusForbidden {
		t.Errorf("non-admin DELETE = %d, want 403", rec.Code)
	}
	if rec := bindingsDo(t, f.rt, http.MethodGet, "/api/v1/organizations/"+teamB+"/bindings", admin, ""); rec.Code == http.StatusOK {
		t.Errorf("token for another org read = %d, want refusal", rec.Code)
	}
	if rec := bindingsDo(t, f.rt, http.MethodDelete, path+"/"+created.ID, admin, ""); rec.Code != http.StatusNoContent {
		t.Errorf("admin DELETE = %d %s, want 204", rec.Code, rec.Body.String())
	}
}

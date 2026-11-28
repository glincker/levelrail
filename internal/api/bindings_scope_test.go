package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/glincker/levelrail/internal/store"
)

func bindingsMust(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func seedRegistryConn(t *testing.T, db *store.DB, id string) {
	t.Helper()
	bindingsMust(t, db.SaveRegistryCredential(context.Background(), store.RegistryCredential{ID: id, Name: id, RegistryHost: "registry.example.com", Username: "u", CreatedAt: "2026-01-01T00:00:00Z"}))
}

func connectionScopeID(t *testing.T, rt *Router, token, ref string) (string, string) {
	t.Helper()
	rec := bindingsDo(t, rt, http.MethodGet, "/api/v1/connections/"+ref, token, "")
	var c struct {
		ScopeType string `json:"scope_type"`
		ScopeID   string `json:"scope_id"`
	}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &c) != nil {
		t.Fatalf("get %s = %d %s", ref, rec.Code, rec.Body.String())
	}
	return c.ScopeType, c.ScopeID
}

// A deleted app, org or connection takes its bindings and scope with it, so a
// later app with the same name in another org starts with nothing.
func TestBindingsDoNotOutliveTheirAppOrgOrConnection(t *testing.T) {
	t.Parallel()
	f := newTeamFixture(t)
	rt, db, ctx := f.rt, f.db, context.Background()
	root := testAdminTokenFor(t, db)
	for _, id := range []string{"rc1", "rc2", "rc3"} {
		seedRegistryConn(t, db, id)
	}
	put := func(ref, body string) {
		t.Helper()
		if rec := bindingsDo(t, rt, http.MethodPut, "/api/v1/connections/"+ref+"/meta", root, body); rec.Code != http.StatusOK {
			t.Fatalf("meta %s = %d %s", ref, rec.Code, rec.Body.String())
		}
	}
	post := func(path, ref string, want int) {
		t.Helper()
		if rec := bindingsDo(t, rt, http.MethodPost, path, root, `{"ref":"`+ref+`","role":"registry_pull"}`); rec.Code != want {
			t.Fatalf("bind %s on %s = %d %s, want %d", ref, path, rec.Code, rec.Body.String(), want)
		}
	}
	put("registry:rc2", `{"scope_type":"app","scope_id":"`+appA+`"}`)
	put("registry:rc3", `{"scope_type":"org","scope_id":"`+teamA+`"}`)
	post("/api/v1/apps/"+appA+"/bindings", "registry:rc1", http.StatusCreated)
	post("/api/v1/apps/"+appA+"/bindings", "registry:rc2", http.StatusCreated)
	post("/api/v1/organizations/"+teamA+"/bindings", "registry:rc3", http.StatusCreated)

	bindingsMust(t, db.DeleteDesiredService(ctx, appA))
	bindingsMust(t, db.SaveDesiredService(ctx, store.DesiredService{Name: appA, Image: "nginx:1", Port: 80}))
	bindingsMust(t, db.UpdateServiceProject(ctx, appA, projB))
	rec := bindingsDo(t, rt, http.MethodGet, "/api/v1/apps/"+appA+"/bindings", root, "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"bindings":[]`) || !strings.Contains(rec.Body.String(), `"resolved":[]`) {
		t.Errorf("recreated app bindings = %d %s, want none", rec.Code, rec.Body.String())
	}
	if typ, id := connectionScopeID(t, rt, root, "registry:rc2"); typ != store.ConnectionScopeApp || id != "" {
		t.Errorf("rc2 scope after app delete = %s/%q, want app with no id", typ, id)
	}
	post("/api/v1/apps/"+appA+"/bindings", "registry:rc2", http.StatusBadRequest)

	bindingsMust(t, db.DeleteOrganization(ctx, teamA))
	bindingsMust(t, db.SaveOrganization(ctx, store.Organization{ID: teamA, Name: teamA, CreatedAt: "2026-10-02T00:00:00Z"}))
	if rows, _ := db.ListConnectionBindingsByTarget(ctx, store.BindingTargetOrg, teamA); len(rows) != 0 {
		t.Errorf("recreated org bindings = %d, want 0", len(rows))
	}
	if _, id := connectionScopeID(t, rt, root, "registry:rc3"); id != "" {
		t.Errorf("rc3 scope id after org delete = %q, want none", id)
	}
	post("/api/v1/organizations/"+teamA+"/bindings", "registry:rc3", http.StatusBadRequest)

	// A detached scope is moved again only by an instance admin.
	put("registry:rc3", `{"scope_type":"org","scope_id":"`+teamB+`"}`)

	post("/api/v1/apps/"+appB+"/bindings", "registry:rc1", http.StatusCreated)
	bindingsMust(t, db.DeleteRegistryCredential(ctx, "rc1"))
	seedRegistryConn(t, db, "rc1")
	if rows, _ := db.ListConnectionBindingsByRef(ctx, "registry:rc1"); len(rows) != 0 {
		t.Errorf("bindings of a recreated connection = %d, want 0", len(rows))
	}
}

// Instance connections stay under instance admins: a tenant org admin cannot
// pull one into the org, edit it, or make it the org's default.
func TestBindingsInstanceConnectionNeedsInstanceAdmin(t *testing.T) {
	t.Parallel()
	f := newTeamFixture(t)
	rt, db, ctx := f.rt, f.db, context.Background()
	root := testAdminTokenFor(t, db)
	seedRegistryConn(t, db, "rc1")
	seedRegistryConn(t, db, "rc2")
	u, err := rt.createLocalUserInTeam(ctx, "dev@instance.test", "", teamMatrixPassword,
		[]string{AbilityRead, AbilityReadSensitive, AbilityWrite, AbilityWriteSensitive}, store.DefaultTeamID, store.TeamRoleDeveloper)
	bindingsMust(t, err)
	bindingsMust(t, db.AddTeamMember(ctx, teamB, u.ID, store.TeamRoleAdmin))
	plain := "scope-secret-dev"
	bindingsMust(t, db.SaveAPIToken(ctx, store.APIToken{ID: "tok-dev", Name: "tok-dev", TokenHash: hashToken(plain),
		Abilities: []string{AbilityRead, AbilityReadSensitive, AbilityWrite, AbilityWriteSensitive}, OwnerUserID: u.ID}))

	for name, body := range map[string]string{
		"move into the org": `{"scope_type":"org","scope_id":"` + teamB + `"}`,
		"edit expiry":       `{"expires_at":"2099-01-01T00:00:00Z"}`,
		"edit rotation":     `{"rotate_after":"2099-01-01T00:00:00Z"}`,
	} {
		if rec := bindingsDo(t, rt, http.MethodPut, "/api/v1/connections/registry:rc1/meta", plain, body); rec.Code != http.StatusForbidden {
			t.Errorf("%s = %d %s, want 403", name, rec.Code, rec.Body.String())
		}
	}
	if rec := bindingsDo(t, rt, http.MethodPost, "/api/v1/organizations/"+teamB+"/bindings", plain, `{"ref":"registry:rc1","role":"registry_pull"}`); rec.Code != http.StatusForbidden {
		t.Errorf("instance connection as org default = %d %s, want 403", rec.Code, rec.Body.String())
	}
	// The same admin still manages connections that belong to the org.
	if rec := bindingsDo(t, rt, http.MethodPut, "/api/v1/connections/registry:rc2/meta", root, `{"scope_type":"org","scope_id":"`+teamB+`"}`); rec.Code != http.StatusOK {
		t.Fatalf("root scopes rc2 = %d %s", rec.Code, rec.Body.String())
	}
	if rec := bindingsDo(t, rt, http.MethodPost, "/api/v1/organizations/"+teamB+"/bindings", plain, `{"ref":"registry:rc2","role":"registry_pull"}`); rec.Code != http.StatusCreated {
		t.Errorf("org connection as org default = %d %s, want 201", rec.Code, rec.Body.String())
	}
	if rec := bindingsDo(t, rt, http.MethodPut, "/api/v1/connections/registry:rc2/meta", plain, `{"expires_at":"2099-01-01T00:00:00Z"}`); rec.Code != http.StatusOK {
		t.Errorf("org admin edits org connection expiry = %d %s, want 200", rec.Code, rec.Body.String())
	}
	if rec := bindingsDo(t, rt, http.MethodPost, "/api/v1/organizations/"+teamB+"/bindings", root, `{"ref":"registry:rc1","role":"registry_pull"}`); rec.Code != http.StatusCreated {
		t.Errorf("root sets instance connection as org default = %d %s, want 201", rec.Code, rec.Body.String())
	}
}

// Expiry and rotation edits need the same authority as a scope move.
func TestBindingsMetaWritesCheckCurrentScope(t *testing.T) {
	t.Parallel()
	rt, db := seedBindingFixture(t)
	root := testAdminTokenFor(t, db)
	if rec := bindingsDo(t, rt, http.MethodPut, "/api/v1/connections/registry:rc1/meta", root, `{"scope_type":"app","scope_id":"web"}`); rec.Code != http.StatusOK {
		t.Fatalf("scope to web = %d %s", rec.Code, rec.Body.String())
	}
	denied := seedMatrixToken(t, db, "b-meta-denied", []string{AbilityRead, AbilityReadSensitive, AbilityWrite, AbilityWriteSensitive})
	attachTestPolicy(t, db, "b-meta-deny", "Deny", AbilityWriteSensitive, "app:web", store.PrincipalTypeToken, "b-meta-denied")
	for _, body := range []string{`{"expires_at":"2099-01-01T00:00:00Z"}`, `{"rotate_after":""}`, `{}`} {
		if rec := bindingsDo(t, rt, http.MethodPut, "/api/v1/connections/registry:rc1/meta", denied, body); rec.Code != http.StatusForbidden {
			t.Errorf("meta %s on a denied app = %d %s, want 403", body, rec.Code, rec.Body.String())
		}
	}
	if rec := bindingsDo(t, rt, http.MethodPut, "/api/v1/connections/registry:rc1/meta", root, `{"expires_at":"2099-01-01T00:00:00Z"}`); rec.Code != http.StatusOK {
		t.Errorf("root edits expiry = %d %s", rec.Code, rec.Body.String())
	}
}

// raceBindingStore changes the connection's scope right after the insert, the
// order a concurrent scope change can land in.
type raceBindingStore struct {
	*store.DB
	afterCreate func()
}

func (s raceBindingStore) CreateConnectionBinding(ctx context.Context, b store.ConnectionBinding) (store.ConnectionBinding, error) {
	out, err := s.DB.CreateConnectionBinding(ctx, b)
	if err == nil {
		s.afterCreate()
	}
	return out, err
}

// A binding outside its connection's scope is never listed, and one created
// while the scope moved is removed again.
func TestBindingsOutsideScopeAreHiddenAndRemoved(t *testing.T) {
	t.Parallel()
	rt, db := seedBindingFixture(t)
	ctx := context.Background()
	root := testAdminTokenFor(t, db)
	_, err := db.CreateConnectionBinding(ctx, store.ConnectionBinding{Ref: "registry:rc1", TargetType: store.BindingTargetApp, TargetID: "web", Role: "registry_pull"})
	bindingsMust(t, err)
	bindingsMust(t, db.UpsertConnectionMeta(ctx, store.ConnectionMeta{Ref: "registry:rc1", ScopeType: store.ConnectionScopeApp, ScopeID: "other"}))
	for _, path := range []string{"/api/v1/apps/web/bindings", "/api/v1/apps/web/timeline"} {
		rec := bindingsDo(t, rt, http.MethodGet, path, root, "")
		if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "registry:rc1") {
			t.Errorf("GET %s = %d, shows a binding outside scope: %s", path, rec.Code, rec.Body.String())
		}
	}
	bindingsMust(t, db.DeleteConnectionMeta(ctx, "registry:rc1"))
	if rec := bindingsDo(t, rt, http.MethodGet, "/api/v1/apps/web/timeline", root, ""); !strings.Contains(rec.Body.String(), "registry:rc1") {
		t.Errorf("timeline hides a covered binding: %s", rec.Body.String())
	}

	seedRegistryConn(t, db, "rc9")
	rt.apps = raceBindingStore{DB: db, afterCreate: func() {
		bindingsMust(t, db.UpsertConnectionMeta(ctx, store.ConnectionMeta{Ref: "registry:rc9", ScopeType: store.ConnectionScopeApp, ScopeID: "other"}))
	}}
	if rec := bindingsDo(t, rt, http.MethodPost, "/api/v1/apps/web/bindings", root, `{"ref":"registry:rc9","role":"registry_pull"}`); rec.Code != http.StatusConflict {
		t.Errorf("bind during a scope move = %d %s, want 409", rec.Code, rec.Body.String())
	}
	if rows, _ := db.ListConnectionBindingsByRef(ctx, "registry:rc9"); len(rows) != 0 {
		t.Errorf("bindings left after the scope moved = %d, want 0", len(rows))
	}
}

func TestBindingsRefuseMultiLineRules(t *testing.T) {
	t.Parallel()
	rt, db := seedBindingFixture(t)
	root := testAdminTokenFor(t, db)
	for name, body := range map[string]string{
		"newline host": `{"ref":"registry:rc1","role":"registry_pull","rules":{"host":"a\nb"}}`,
		"long host":    `{"ref":"registry:rc1","role":"registry_pull","rules":{"host":"` + strings.Repeat("h", 600) + `"}}`,
	} {
		if rec := bindingsDo(t, rt, http.MethodPost, "/api/v1/apps/web/bindings", root, body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s = %d %s, want 400", name, rec.Code, rec.Body.String())
		}
	}
}

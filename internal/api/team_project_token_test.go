package api

import (
	"context"
	"net/http"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

// TestProjectScopedTokenCannotReachOrganizationEnv checks that org wide env, which
// flows into every project of the team, is closed to a token limited to one project.
func TestProjectScopedTokenCannotReachOrganizationEnv(t *testing.T) {
	f := newTeamFixture(t)
	ctx := context.Background()
	mk := func(id, team, project string) string {
		plain := "org-env-" + id
		abilities := []string{AbilityRead, AbilityWrite}
		if err := f.db.SaveAPIToken(ctx, store.APIToken{ID: id, Name: id, TokenHash: hashToken(plain), Abilities: abilities, OwnerUserID: f.members[store.TeamRoleOwner].ID}); err != nil {
			t.Fatal(err)
		}
		if err := f.db.SetAPITokenScope(ctx, store.APITokenScope{TokenID: id, TeamID: team, ProjectID: project, Role: store.TeamRoleOwner}); err != nil {
			t.Fatal(err)
		}
		return plain
	}
	projTok := mk("tok-org-env-proj", teamA, projA)
	teamTok := mk("tok-org-env-team", teamA, "")
	otherTok := mk("tok-org-env-other", teamB, "")

	base := "/api/v1/organizations/" + teamA
	calls := []struct{ method, path, body string }{
		{http.MethodGet, base + "/env", ""},
		{http.MethodPut, base + "/env", `{"A":"1"}`},
		{http.MethodGet, base + "/env/secrets", ""},
		{http.MethodPut, base + "/env/secrets/K", `{"value":"v"}`},
		{http.MethodDelete, base + "/env/secrets/K", ""},
	}
	for _, c := range calls {
		if rec := bindingsDo(t, f.rt, c.method, c.path, projTok, c.body); rec.Code != http.StatusNotFound {
			t.Errorf("project token %s %s = %d, want 404", c.method, c.path, rec.Code)
		}
		if rec := bindingsDo(t, f.rt, c.method, c.path, otherTok, c.body); rec.Code != http.StatusNotFound {
			t.Errorf("other team token %s %s = %d, want 404", c.method, c.path, rec.Code)
		}
	}
	if rec := bindingsDo(t, f.rt, http.MethodGet, base+"/env", teamTok, ""); rec.Code != http.StatusOK {
		t.Errorf("team token GET env = %d %s, want 200", rec.Code, rec.Body.String())
	}
	if rec := bindingsDo(t, f.rt, http.MethodPut, base+"/env", teamTok, `{"A":"1"}`); rec.Code != http.StatusOK {
		t.Errorf("team token PUT env = %d %s, want 200", rec.Code, rec.Body.String())
	}
	if rec := bindingsDo(t, f.rt, http.MethodGet, "/api/v1/projects/"+projA+"/env", projTok, ""); rec.Code == http.StatusNotFound || rec.Code == http.StatusForbidden {
		t.Errorf("project token own project env = %d, want allowed", rec.Code)
	}
}

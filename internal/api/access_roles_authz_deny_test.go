package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/experimental"
)

func accessRolesDenyFeature() authzDenyFeature {
	return authzDenyFeature{
		name: "access-roles",
		match: func(_, p string) bool {
			return p == "/api/v1/roles" || strings.HasPrefix(p, "/api/v1/roles/") ||
				(strings.HasPrefix(p, "/api/v1/users/{id}/") && (strings.HasSuffix(p, "/role") || strings.HasSuffix(p, "/environment-grants")))
		},
		routes: []authzDenyRoute{
			{http.MethodGet, "/api/v1/roles", "", AbilityRead, false},
			{http.MethodPost, "/api/v1/roles", `{"name":"deny-qa","abilities":["read"]}`, AbilityRoot, false},
			{http.MethodPut, "/api/v1/roles/{id}", `{"name":"deny-qa","abilities":["read"]}`, AbilityRoot, false},
			{http.MethodDelete, "/api/v1/roles/{id}", "", AbilityRoot, false},
			{http.MethodPut, "/api/v1/users/{id}/role", `{"role_id":"role_viewer"}`, AbilityRoot, false},
			{http.MethodGet, "/api/v1/users/{id}/environment-grants", "", AbilityRoot, false},
			{http.MethodPut, "/api/v1/users/{id}/environment-grants", `{"environment_ids":[]}`, AbilityRoot, false},
		},
	}
}

func TestAuthzDeny_AccessRoles(t *testing.T) {
	runAuthzDeny(t, accessRolesDenyFeature())
}

// A user holding a custom role gets exactly that role's abilities, so a role without root is refused on every root route.
func TestAuthzDeny_AccessRolesCustomRoleWithoutAbility(t *testing.T) {
	a := newAuthzDenyActors(t)
	role := createCustomRole(t, a.rt, a.admin, `{"name":"deny-ops","abilities":["read","write","deploy"]}`)
	target := storeUserWithAbilitiesForTest(t, a.db, "deny-custom@example.com", []string{AbilityRead})
	rec := a.do(http.MethodPut, "/api/v1/users/"+target.ID+"/role", `{"role_id":"`+role.ID+`"}`, a.admin, "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("assign custom role = %d %s", rec.Code, rec.Body.String())
	}
	holder := sessionCookieForTest(t, a.rt, target.ID)
	a.secrets = append(a.secrets, holder.Value)
	for _, r := range accessRolesDenyFeature().routes {
		got := a.do(r.method, concretePathFor(r.pattern, "{id}", target.ID), r.body, holder, "", "")
		want := http.StatusForbidden
		if r.ability == AbilityRead {
			want = http.StatusOK
		}
		if got.Code != want {
			t.Errorf("%s as custom-role holder = %d, want %d, body = %.200s", r.key(), got.Code, want, got.Body.String())
		}
	}
	// The holder cannot grant itself root through its own user row either.
	if got := a.do(http.MethodPut, "/api/v1/users/"+target.ID+"/abilities", `{"abilities":["root"]}`, holder, "", ""); got.Code != http.StatusForbidden {
		t.Errorf("custom-role holder self-escalating abilities = %d, want 403", got.Code)
	}
	a.assertNoSecrets(t)
}

// With the flag off, root callers get the documented 404; callers without root still get 403 so the flag state is not disclosed to them.
func TestAuthzDeny_AccessRolesGateOff(t *testing.T) {
	a := newAuthzDenyActors(t)
	experimental.Set()
	for _, r := range accessRolesDenyFeature().routes {
		rec := a.do(r.method, r.path(), r.body, a.admin, "", "")
		if r.method == http.MethodGet && r.pattern == "/api/v1/roles" {
			if rec.Code != http.StatusOK {
				t.Errorf("%s flag off = %d, want 200 (ungated read)", r.key(), rec.Code)
			}
			continue
		}
		if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), ExperimentalDisabledCode) {
			t.Errorf("%s flag off as admin = %d %s, want 404 %s", r.key(), rec.Code, rec.Body.String(), ExperimentalDisabledCode)
		}
		if r.ability == AbilityRoot {
			if rec := a.do(r.method, r.path(), r.body, a.dev, "", ""); rec.Code != http.StatusForbidden {
				t.Errorf("%s flag off as developer = %d, want 403", r.key(), rec.Code)
			}
		}
	}
	a.assertNoSecrets(t)
}

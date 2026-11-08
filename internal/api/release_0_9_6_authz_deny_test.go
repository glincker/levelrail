package api

import (
	"net/http"
	"testing"
)

// Deny rows for custom template import, app maintenance, listening ports and
// resource usage: every caller below the route's ability is refused, and every
// unauthenticated one gets 401 before any data is read.
func TestAuthzDeny_TemplateMaintenancePortsUsage(t *testing.T) {
	a := newAuthzDenyActors(t)
	routes := []authzDenyRoute{
		{http.MethodGet, "/api/v1/templates/custom", "", AbilityRead, false},
		{http.MethodPost, "/api/v1/templates/custom", `{}`, AbilityWrite, false},
		{http.MethodDelete, "/api/v1/templates/custom/{id}", "", AbilityWrite, false},
		{http.MethodGet, "/api/v1/apps/{name}/maintenance", "", AbilityRead, false},
		{http.MethodPut, "/api/v1/apps/{name}/maintenance", `{}`, AbilityDeploy, false},
		{http.MethodDelete, "/api/v1/apps/{name}/maintenance", "", AbilityDeploy, false},
		{http.MethodGet, "/api/v1/apps/{name}/listening-ports", "", AbilityRead, false},
		{http.MethodGet, "/api/v1/apps/resource-usage", "", AbilityRead, false},
	}
	registered := map[string]bool{}
	for _, r := range loadMatrixRoutes(t) {
		registered[r.key()] = true
	}
	type caller struct {
		cookie *http.Cookie
		token  string
	}
	anon := map[string]caller{"anonymous": {}, "expired": {nil, a.expiredTok}, "revoked": {nil, a.revokedTok}, "garbage": {nil, "not-a-token"}}
	below := map[string]map[string]caller{
		AbilityRead:   {"no-ability session": {a.nobody, ""}},
		AbilityWrite:  {"no-ability session": {a.nobody, ""}, "read session": {a.reader, ""}, "read token": {nil, a.readTok}, "deploy-only token": {nil, a.deployOnlyTok}},
		AbilityDeploy: {"no-ability session": {a.nobody, ""}, "read session": {a.reader, ""}, "read token": {nil, a.readTok}},
	}
	for _, r := range routes {
		if !registered[r.key()] {
			t.Fatalf("route %q is not registered", r.key())
		}
		p := r.path()
		for who, c := range anon {
			if rec := a.do(r.method, p, r.body, c.cookie, c.token, ""); rec.Code != http.StatusUnauthorized {
				t.Errorf("%s as %s = %d, want 401", r.key(), who, rec.Code)
			}
		}
		for who, c := range below[r.ability] {
			if rec := a.do(r.method, p, r.body, c.cookie, c.token, ""); rec.Code != http.StatusForbidden {
				t.Errorf("%s as %s = %d, want 403, body = %.200s", r.key(), who, rec.Code, rec.Body.String())
			}
		}
	}
	a.assertNoSecrets(t)
}

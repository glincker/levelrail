package api

import (
	"net/http"
	"testing"
)

// Deny rows for the sensitive write routes added in 0.9.5. Each must refuse
// anonymous, stale, read-only and deploy-only callers before touching data.
func TestAuthzDeny_SensitiveWriteRoutes(t *testing.T) {
	a := newAuthzDenyActors(t)
	routes := []authzDenyRoute{
		{http.MethodPost, "/api/v1/databases/{name}/import", "", AbilityRoot, false},
		{http.MethodPost, "/api/v1/apps/{name}/compose-db-services/{service}/backups/{run}/restore", `{}`, AbilityRoot, false},
		{http.MethodPut, "/api/v1/apps/{name}/abuse", `{"enabled":false}`, AbilityWriteSensitive, false},
		{http.MethodPut, "/api/v1/apps/{name}/preview-policy", `{}`, AbilityWriteSensitive, false},
		{http.MethodPut, "/api/v1/settings/runtime/{key}", `{"value":"10"}`, AbilityRoot, false},
		{http.MethodDelete, "/api/v1/settings/runtime/{key}", "", AbilityRoot, false},
	}
	registered := map[string]bool{}
	for _, r := range loadMatrixRoutes(t) {
		registered[r.key()] = true
	}
	for _, r := range routes {
		if !registered[r.key()] {
			t.Fatalf("route %q is not registered", r.key())
		}
		p := r.path()
		if r.pattern == "/api/v1/settings/runtime/{key}" {
			p = "/api/v1/settings/runtime/abuse.default_rps"
		}
		anon := map[string]struct {
			cookie *http.Cookie
			token  string
		}{"anonymous": {nil, ""}, "expired": {nil, a.expiredTok}, "revoked": {nil, a.revokedTok}, "garbage": {nil, "not-a-token"}}
		for who, c := range anon {
			if rec := a.do(r.method, p, r.body, c.cookie, c.token, ""); rec.Code != http.StatusUnauthorized {
				t.Errorf("%s as %s = %d, want 401", r.key(), who, rec.Code)
			}
		}
		denied := map[string]struct {
			cookie *http.Cookie
			token  string
		}{
			"no-ability session": {a.nobody, ""},
			"read session":       {a.reader, ""},
			"developer session":  {a.dev, ""},
			"read token":         {nil, a.readTok},
			"deploy-only token":  {nil, a.deployOnlyTok},
			"developer token":    {nil, a.devTok},
		}
		for who, c := range denied {
			if rec := a.do(r.method, p, r.body, c.cookie, c.token, ""); rec.Code != http.StatusForbidden {
				t.Errorf("%s as %s = %d, want 403, body = %.200s", r.key(), who, rec.Code, rec.Body.String())
			}
		}
	}
	a.assertNoSecrets(t)
}

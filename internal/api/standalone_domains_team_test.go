package api

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Standalone domains are instance wide: a member of only a non-default
// team can neither list, create nor configure one, at any role.
func TestStandaloneDomainsAPI_ClosedToTenants(t *testing.T) {
	f := newTeamFixture(t)
	if err := f.db.CreateStandaloneDomain(context.Background(), store.StandaloneDomain{Domain: "tools.example.com", Target: "127.0.0.1:3000", TargetKind: "port"}); err != nil {
		t.Fatal(err)
	}
	for _, role := range allTeamRoles {
		for _, c := range []struct{ method, path, body string }{
			{http.MethodGet, "/api/v1/domains/standalone", ""},
			{http.MethodPost, "/api/v1/domains/standalone", `{"domain":"x.example.com","target":"https://example.org"}`},
			{http.MethodDelete, "/api/v1/domains/standalone/tools.example.com", ""},
			{http.MethodGet, "/api/v1/system/server-address", ""},
			{http.MethodGet, "/api/v1/apps/_standalone/domains/tools.example.com/maintenance", ""},
			{http.MethodPut, "/api/v1/apps/_standalone/domains/tools.example.com/maintenance", ""},
		} {
			rec := sdReq(t, f.rt, f.cookies[role], c.method, c.path, c.body)
			if rec.Code != http.StatusForbidden && rec.Code != http.StatusNotFound {
				t.Errorf("%s %s %s = %d, want 403 or 404", role, c.method, c.path, rec.Code)
			}
		}
		rec := sdReq(t, f.rt, f.cookies[role], http.MethodGet, "/api/v1/domains", "")
		if strings.Contains(rec.Body.String(), "tools.example.com") {
			t.Errorf("%s sees the standalone domain in GET /domains", role)
		}
	}
}

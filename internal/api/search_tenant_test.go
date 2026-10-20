package api

import (
	"encoding/json"
	"github.com/GLINCKER/levelrail/internal/store"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A member of team A may search, but only ever sees team A's records and no instance-wide hits.
func TestSearch_TeamTenantSeesOnlyItsOwnTeam(t *testing.T) {
	f := newTeamFixture(t)
	for _, query := range []string{"web", "db", "prod", "a", "b"} {
		for _, role := range allTeamRoles {
			rec := httptest.NewRecorder()
			f.rt.Handler().ServeHTTP(rec, authedRequest(t, f.cookies[role], http.MethodGet, "/api/v1/search?q="+query+"&limit=100", ""))
			if rec.Code != http.StatusOK {
				t.Fatalf("%s search %q = %d %s", role, query, rec.Code, rec.Body.String())
			}
			var out searchResp
			if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
				t.Fatal(err)
			}
			for _, h := range out.Hits {
				ref := strings.ToLower(h.Ref)
				for _, other := range []string{appB, dbB, projB, envB, teamB} {
					if strings.Contains(ref, other) {
						t.Errorf("%s searching %q saw team B record %q", role, query, h.Ref)
					}
				}
				if h.Kind == "tag" || h.Kind == "setting" {
					t.Errorf("%s searching %q saw instance-wide hit %q (%s)", role, query, h.Ref, h.Kind)
				}
			}
		}
	}
	rec := httptest.NewRecorder()
	f.rt.Handler().ServeHTTP(rec, authedRequest(t, f.cookies[store.TeamRoleViewer], http.MethodGet, "/api/v1/search?q="+appA, ""))
	var own searchResp
	if err := json.Unmarshal(rec.Body.Bytes(), &own); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, ref := range own.refs() {
		found = found || strings.Contains(ref, appA)
	}
	if !found {
		t.Fatalf("a team member must find its own app %q, got %v", appA, own.refs())
	}
}

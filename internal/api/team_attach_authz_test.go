package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/glincker/levelrail/internal/store"
)

// TestDatabaseAttachRefusesOtherTeamDatabase checks that a team A user cannot
// attach team B's database, which would inject its credentials into their app.
func TestDatabaseAttachRefusesOtherTeamDatabase(t *testing.T) {
	f := newTeamFixture(t)
	cookie := f.cookies[store.TeamRoleDeveloper]
	calls := []struct{ method, path, body string }{
		{http.MethodPut, "/api/v1/apps/" + appA + "/database", `{"database_name":"` + dbB + `"}`},
		{http.MethodPost, "/api/v1/apps/" + appA + "/connections", `{"database":"` + dbB + `"}`},
	}
	for _, c := range calls {
		rec := httptest.NewRecorder()
		f.rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, c.method, c.path, c.body))
		if rec.Code != http.StatusBadRequest && rec.Code != http.StatusNotFound && rec.Code != http.StatusForbidden {
			t.Errorf("%s %s = %d %s, want refusal", c.method, c.path, rec.Code, rec.Body.String())
		}
	}
	svc, err := f.db.GetDesiredService(context.Background(), appA)
	if err != nil {
		t.Fatal(err)
	}
	if svc.DatabaseAttachment != nil || len(svc.DatabaseEnv) != 0 {
		t.Errorf("app A got team B database attached: %+v %+v", svc.DatabaseAttachment, svc.DatabaseEnv)
	}
}

// TestMoveProjectNeedsRoleInDestinationTeam checks a team A admin cannot file a
// project under a team they do not belong to.
func TestMoveProjectNeedsRoleInDestinationTeam(t *testing.T) {
	f := newTeamFixture(t)
	rec := httptest.NewRecorder()
	body := `{"org_id":"` + teamB + `"}`
	f.rt.Handler().ServeHTTP(rec, authedRequest(t, f.cookies[store.TeamRoleAdmin], http.MethodPut, "/api/v1/projects/"+projA+"/organization", body))
	if rec.Code < 400 {
		t.Fatalf("move into foreign team = %d %s", rec.Code, rec.Body.String())
	}
	team, err := f.db.TeamOfProject(context.Background(), projA)
	if err != nil || !strings.EqualFold(team, teamA) {
		t.Errorf("project team = %q %v, want %s", team, err, teamA)
	}
}

package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/glincker/levelrail/internal/store"
)

func TestDatabaseLogsDownload_DeniesOtherTeam(t *testing.T) {
	f := newTeamFixture(t)
	cookie := f.cookies[store.TeamRoleOwner]
	code, body := teamDo(t, f.rt, http.MethodGet, "/api/v1/databases/"+dbB+"/logs/download", "", withCookie(cookie))
	if code != http.StatusForbidden && code != http.StatusNotFound {
		t.Errorf("other team database log download = %d %s, want 403 or 404", code, body)
	}
}

func TestProjectResources_ListsOwnProjectAndDeniesOtherTeam(t *testing.T) {
	f := newTeamFixture(t)
	cookie := f.cookies[store.TeamRoleOwner]

	for _, kind := range []string{"apps", "databases"} {
		code, body := teamDo(t, f.rt, http.MethodGet, "/api/v1/projects/"+projA+"/"+kind, "", withCookie(cookie))
		if code != http.StatusOK {
			t.Fatalf("own project %s = %d %s", kind, code, body)
		}
		if strings.Contains(body, appB) || strings.Contains(body, dbB) {
			t.Errorf("own project %s leaks team B: %s", kind, body)
		}
		code, _ = teamDo(t, f.rt, http.MethodGet, "/api/v1/projects/"+projB+"/"+kind, "", withCookie(cookie))
		if code != http.StatusForbidden && code != http.StatusNotFound {
			t.Errorf("other team project %s = %d, want 403 or 404", kind, code)
		}
	}
	_, body := teamDo(t, f.rt, http.MethodGet, "/api/v1/projects/"+projA+"/apps", "", withCookie(cookie))
	if !strings.Contains(body, appA) {
		t.Errorf("own project apps missing %s: %s", appA, body)
	}
}

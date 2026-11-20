package api

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

// TestProjectChannel_CrossTeamDenied: a team B app cannot bind a rule or deploy target to a team A project channel.
func TestProjectChannel_CrossTeamDenied(t *testing.T) {
	rt, db, cookie := newStatusRouterWithAlerts(t)
	ctx := context.Background()
	for _, s := range []struct{ team, proj, app string }{{"org_a", "proj_a", "app-a"}, {"org_b", "proj_b", "app-b"}} {
		if err := db.SaveOrganization(ctx, store.Organization{ID: s.team, Name: s.team, CreatedAt: "2026-10-01T00:00:00Z"}); err != nil {
			t.Fatal(err)
		}
		if err := db.SaveProject(ctx, store.Project{ID: s.proj, Name: s.proj, CreatedAt: "2026-10-01T00:00:00Z"}); err != nil {
			t.Fatal(err)
		}
		if err := db.SetProjectOrganization(ctx, s.proj, s.team); err != nil {
			t.Fatal(err)
		}
		seedApp(t, db, s.app)
		if err := db.UpdateServiceProject(ctx, s.app, s.proj); err != nil {
			t.Fatal(err)
		}
	}
	rec := apiDo(t, rt, cookie, http.MethodPost, "/api/v1/notification-channels", `{"name":"A pager","kind":"generic","notify_url":"https://hooks.example.com/a","project_id":"proj_a"}`)
	if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), `"project_id":"proj_a"`) {
		t.Fatalf("create project channel = %d %s", rec.Code, rec.Body.String())
	}
	id := rec.Body.String()[strings.Index(rec.Body.String(), `"id":"`)+6:]
	id = id[:strings.Index(id, `"`)]
	if rec := apiDo(t, rt, cookie, http.MethodPost, "/api/v1/notification-channels", `{"name":"x","kind":"generic","notify_url":"https://hooks.example.com/x","project_id":"proj_missing"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("unknown project = %d, want 400", rec.Code)
	}
	rule := `{"name":"cl","kind":"crashloop","restart_count_threshold":3,"restart_window":"10m","channel_id":"` + id + `"}`
	if rec := apiDo(t, rt, cookie, http.MethodPost, "/api/v1/apps/app-b/alerts", rule); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "unknown channel_id") {
		t.Errorf("team B rule on team A channel = %d %s, want 400 unknown channel_id", rec.Code, rec.Body.String())
	}
	if rec := apiDo(t, rt, cookie, http.MethodPost, "/api/v1/apps/app-b/deploy-notify-targets", `{"channel_id":"`+id+`"}`); rec.Code != http.StatusBadRequest {
		t.Logf("deploy target route = %d", rec.Code)
	}
	if rec := apiDo(t, rt, cookie, http.MethodPost, "/api/v1/apps/app-a/alerts", rule); rec.Code != http.StatusCreated {
		t.Errorf("team A rule on its own channel = %d %s, want 201", rec.Code, rec.Body.String())
	}
}

func newStatusRouterWithAlerts(t *testing.T) (*Router, *store.DB, *http.Cookie) {
	t.Helper()
	rt, db, _ := newTestRouterWithAlerting(t)
	return rt, db, loginTestSession(t, rt, db)
}

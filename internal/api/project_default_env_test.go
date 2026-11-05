package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestDefaultEnv_CreateProjectSettingOnAndOff(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/projects", `{"name":"off"}`))
	var off projectResource
	_ = json.Unmarshal(rec.Body.Bytes(), &off)
	if rec.Code != http.StatusCreated || off.DefaultEnvironmentID != "" {
		t.Fatalf("setting off: code %d, default %q; want 201 and none", rec.Code, off.DefaultEnvironmentID)
	}
	if envs, _ := db.ListEnvironmentsByProject(context.Background(), off.ID); len(envs) != 0 {
		t.Errorf("setting off created %d environments, want 0", len(envs))
	}

	t.Setenv("APP_PROJECTS_DEFAULT_ENVIRONMENT", "true")
	t.Setenv("APP_PROJECTS_DEFAULT_ENVIRONMENT_NAME", "live")
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/projects", `{"name":"on"}`))
	var on projectResource
	_ = json.Unmarshal(rec.Body.Bytes(), &on)
	if rec.Code != http.StatusCreated || on.DefaultEnvironmentID == "" {
		t.Fatalf("setting on: code %d, default %q; want 201 and an environment", rec.Code, on.DefaultEnvironmentID)
	}
	env, err := db.GetEnvironment(context.Background(), on.DefaultEnvironmentID)
	if err != nil || env.Name != "live" || env.Protected || env.ProjectID != on.ID {
		t.Errorf("default environment = %+v, %v; want unprotected 'live' in the project", env, err)
	}
}

func TestDefaultEnv_ResolutionOnAppCreate(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	if err := db.SaveProject(ctx, store.Project{ID: "proj_1", Name: "p", CreatedAt: "2026-08-14T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveDefaultEnvironment(ctx, store.Environment{ID: "env_d", ProjectID: "proj_1", Name: "production", CreatedAt: "2026-08-14T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps", `{"name":"web","image":"levelrail/web:1","port":3000,"project_id":"proj_1"}`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d; body = %s", rec.Code, rec.Body.String())
	}
	svc, _ := db.GetDesiredService(ctx, "web")
	if svc.EnvironmentID != "env_d" {
		t.Errorf("app environment = %q, want env_d", svc.EnvironmentID)
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps", `{"name":"solo","image":"levelrail/web:1","port":3000}`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("no-project create status = %d; body = %s", rec.Code, rec.Body.String())
	}
	if solo, _ := db.GetDesiredService(ctx, "solo"); solo.EnvironmentID != "" {
		t.Errorf("project-less app environment = %q, want none", solo.EnvironmentID)
	}
}

func TestDefaultEnv_ResolutionRefusesOtherTeamsProject(t *testing.T) {
	f := newTeamFixture(t)
	if err := f.db.SetProjectDefaultEnvironment(context.Background(), projB, envB); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	f.rt.Handler().ServeHTTP(rec, authedRequest(t, f.cookies[store.TeamRoleOwner], http.MethodPost, "/api/v1/apps", `{"name":"sneak","image":"nginx:1","port":80,"project_id":"`+projB+`"}`))
	if rec.Code != http.StatusBadRequest && rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 400 or 404; body = %s", rec.Code, rec.Body.String())
	}
	if _, err := f.db.GetDesiredService(context.Background(), "sneak"); err == nil {
		t.Error("the app was created in another team's project")
	}
}

func TestDefaultEnv_SetDefaultWrongProjectAndDeleteRefused(t *testing.T) {
	f := newTeamFixture(t)
	owner := f.cookies[store.TeamRoleOwner]
	ctx := context.Background()
	if err := f.db.SaveEnvironment(ctx, store.Environment{ID: "env_a2", ProjectID: projA, Name: "staging", CreatedAt: "2026-10-01T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	put := func(cookie *http.Cookie, project, env string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		f.rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/projects/"+project+"/default-environment", `{"environment_id":"`+env+`"}`))
		return rec
	}
	if rec := put(owner, projA, envA); rec.Code != http.StatusOK {
		t.Fatalf("set first default = %d; %s", rec.Code, rec.Body.String())
	}
	if rec := put(owner, projA, "env_a2"); rec.Code != http.StatusOK {
		t.Fatalf("set second default = %d; %s", rec.Code, rec.Body.String())
	}
	if got, _ := f.db.DefaultEnvironmentID(ctx, projA); got != "env_a2" {
		t.Errorf("default = %q, want env_a2 (previous cleared)", got)
	}
	if rec := put(owner, projA, envB); rec.Code != http.StatusNotFound && rec.Code != http.StatusBadRequest {
		t.Errorf("other team's environment = %d, want 404 or 400", rec.Code)
	}
	if rec := put(owner, projA2, envA); rec.Code != http.StatusNotFound {
		t.Errorf("environment of a different project = %d, want 404", rec.Code)
	}
	if rec := put(f.cookies[store.TeamRoleDeveloper], projA, envA); rec.Code != http.StatusForbidden {
		t.Errorf("developer set default = %d, want 403", rec.Code)
	}
	if rec := put(owner, projB, envB); rec.Code != http.StatusNotFound && rec.Code != http.StatusForbidden {
		t.Errorf("other team's project = %d, want 404 or 403", rec.Code)
	}

	rec := httptest.NewRecorder()
	f.rt.Handler().ServeHTTP(rec, authedRequest(t, owner, http.MethodDelete, "/api/v1/environments/env_a2", ""))
	if rec.Code != http.StatusConflict {
		t.Errorf("delete default = %d, want 409; %s", rec.Code, rec.Body.String())
	}
}

func TestDefaultEnv_BackfillDryRunThenApply(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	for _, id := range []string{"proj_1", "proj_2"} {
		if err := db.SaveProject(ctx, store.Project{ID: id, Name: id, CreatedAt: "2026-08-14T00:00:00Z"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.SaveEnvironment(ctx, store.Environment{ID: "env_prod", ProjectID: "proj_2", Name: "Production", CreatedAt: "2026-08-14T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	call := func(body string) backfillDefaultEnvResponse {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/projects/default-environments/backfill", body))
		if rec.Code != http.StatusOK {
			t.Fatalf("backfill = %d; %s", rec.Code, rec.Body.String())
		}
		var out backfillDefaultEnvResponse
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return out
	}
	dry := call(`{}`)
	if !dry.DryRun || len(dry.Projects) != 2 || dry.Applied != 0 {
		t.Fatalf("dry run = %+v", dry)
	}
	if ids, _ := db.DefaultEnvironmentIDs(ctx); len(ids) != 0 {
		t.Fatalf("dry run changed %d projects", len(ids))
	}
	applied := call(`{"dry_run":false}`)
	if applied.DryRun || applied.Applied != 2 {
		t.Fatalf("apply = %+v", applied)
	}
	if got, _ := db.DefaultEnvironmentID(ctx, "proj_2"); got != "env_prod" {
		t.Errorf("proj_2 default = %q, want the existing production environment promoted", got)
	}
	if again := call(`{"dry_run":false}`); len(again.Projects) != 0 {
		t.Errorf("second run still lists %d projects", len(again.Projects))
	}
}

func TestDefaultEnv_BackfillClosedToTeams(t *testing.T) {
	f := newTeamFixture(t)
	rec := httptest.NewRecorder()
	f.rt.Handler().ServeHTTP(rec, authedRequest(t, f.cookies[store.TeamRoleOwner], http.MethodPost, "/api/v1/projects/default-environments/backfill", `{"dry_run":false}`))
	if rec.Code != http.StatusForbidden {
		t.Errorf("team owner backfill = %d, want 403", rec.Code)
	}
}

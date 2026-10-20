package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/experimental"
	"github.com/GLINCKER/levelrail/internal/store"
)

func envReq(rt *Router, cookie *http.Cookie, method, target, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, target, nil)
	} else {
		req = httptest.NewRequest(method, target, strings.NewReader(body))
	}
	req.AddCookie(cookie)
	rt.Handler().ServeHTTP(rec, req)
	return rec
}

func seedAppAndDB(t *testing.T, db *store.DB) {
	t.Helper()
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "nginx:1", Port: 80}); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: "pg", Engine: "postgres", Version: "16"}); err != nil {
		t.Fatal(err)
	}
}

func TestListAllEnvironments(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedAppAndDB(t, db)
	if err := db.SetServiceEnvironment(context.Background(), "web", "env_production"); err != nil {
		t.Fatal(err)
	}
	rec := envReq(rt, cookie, http.MethodGet, "/api/v1/environments", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var got []environmentListResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) < 4 || got[0].ID != "env_dev" || got[3].ID != "env_production" || got[3].AppCount != 1 || got[3].Kind != "production" || got[3].Scope != "global" || !got[3].Protected {
		t.Fatalf("list = %+v", got)
	}
}

func TestCreateGlobalEnvironment(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	tests := []struct {
		name, body string
		want       int
	}{
		{"custom", `{"name":"Sandbox","kind":"custom"}`, http.StatusCreated},
		{"default kind", `{"name":"Sandbox 2"}`, http.StatusCreated},
		{"preview reserved", `{"name":"P","kind":"preview"}`, http.StatusBadRequest},
		{"bogus kind", `{"name":"P","kind":"qa"}`, http.StatusBadRequest},
		{"empty name", `{"name":" ","kind":"dev"}`, http.StatusBadRequest},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := envReq(rt, cookie, http.MethodPost, "/api/v1/environments", tc.body)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tc.want, rec.Body.String())
			}
			if tc.want == http.StatusCreated {
				var e environmentResource
				_ = json.Unmarshal(rec.Body.Bytes(), &e)
				if e.Scope != "global" || e.ProjectID != store.GlobalProjectID {
					t.Errorf("env = %+v", e)
				}
			}
		})
	}
}

func TestEnvironmentsDisabledWhenFlagOff(t *testing.T) {
	defer experimental.Set(experimental.All()...)
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	experimental.Set()
	if rec := envReq(rt, cookie, http.MethodGet, "/api/v1/environments", ""); rec.Code != http.StatusNotFound {
		t.Errorf("list with flag off = %d", rec.Code)
	}
	if rec := envReq(rt, cookie, http.MethodPatch, "/api/v1/environments/env_dev", `{"name":"X"}`); rec.Code != http.StatusNotFound {
		t.Errorf("rename with flag off = %d", rec.Code)
	}
	if rec := envReq(rt, cookie, http.MethodPatch, "/api/v1/environments/env_dev", `{"protected":true}`); rec.Code != http.StatusOK {
		t.Errorf("protected toggle must stay ungated, got %d", rec.Code)
	}
}

func TestEnvironmentList_ReadableWithOnlyAccessRoles(t *testing.T) {
	defer experimental.Set(experimental.All()...)
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	experimental.Set(experimental.AccessRoles)
	if rec := envReq(rt, cookie, http.MethodGet, "/api/v1/environments", ""); rec.Code != http.StatusOK {
		t.Errorf("list with only access-roles = %d, want 200 so the grants editor works", rec.Code)
	}
	if rec := envReq(rt, cookie, http.MethodPost, "/api/v1/environments", `{"name":"qa","kind":"custom"}`); rec.Code != http.StatusNotFound {
		t.Errorf("create with only access-roles = %d, want 404 (still gated)", rec.Code)
	}
}

func TestPatchEnvironment(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rec := envReq(rt, cookie, http.MethodPost, "/api/v1/environments", `{"name":"Sandbox","kind":"dev"}`)
	var created environmentResource
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	target := "/api/v1/environments/" + created.ID

	rec = envReq(rt, cookie, http.MethodPatch, target, `{"name":"QA","kind":"test","sort_order":7,"protected":true}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch = %d: %s", rec.Code, rec.Body.String())
	}
	var got environmentResource
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got.Name != "QA" || got.Kind != "test" || got.SortOrder != 7 || !got.Protected {
		t.Errorf("patched = %+v", got)
	}
	if rec := envReq(rt, cookie, http.MethodPatch, target, `{"kind":"preview"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("preview kind = %d", rec.Code)
	}
	if rec := envReq(rt, cookie, http.MethodPatch, "/api/v1/environments/env_production", `{"kind":"dev"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("reserved retype = %d", rec.Code)
	}
	if rec := envReq(rt, cookie, http.MethodPatch, "/api/v1/environments/env_production", `{"name":"Prod","kind":"production"}`); rec.Code != http.StatusOK {
		t.Errorf("reserved patch with unchanged kind = %d: %s", rec.Code, rec.Body.String())
	}
	if rec := envReq(rt, cookie, http.MethodPatch, target, `{"name":""}`); rec.Code != http.StatusBadRequest {
		t.Errorf("empty name = %d", rec.Code)
	}
	if rec := envReq(rt, cookie, http.MethodPatch, "/api/v1/environments/env_missing", `{"protected":true}`); rec.Code != http.StatusNotFound {
		t.Errorf("missing = %d", rec.Code)
	}
}

func TestDeleteEnvironment_GlobalRules(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedAppAndDB(t, db)
	ctx := context.Background()

	for _, id := range []string{"env_dev", "env_test", "env_uat", "env_production"} {
		if rec := envReq(rt, cookie, http.MethodDelete, "/api/v1/environments/"+id, ""); rec.Code != http.StatusForbidden {
			t.Errorf("delete reserved %s = %d", id, rec.Code)
		}
	}

	mk := func(body string) string {
		rec := envReq(rt, cookie, http.MethodPost, "/api/v1/environments", body)
		var e environmentResource
		_ = json.Unmarshal(rec.Body.Bytes(), &e)
		return e.ID
	}
	sandbox := mk(`{"name":"Sandbox","kind":"dev"}`)
	locked := mk(`{"name":"Locked","kind":"uat","protected":true}`)
	_ = db.SetServiceEnvironment(ctx, "web", sandbox)
	_ = db.SetDatabaseEnvironment(ctx, "pg", sandbox)

	if rec := envReq(rt, cookie, http.MethodDelete, "/api/v1/environments/"+sandbox, ""); rec.Code != http.StatusConflict {
		t.Fatalf("delete tagged = %d: %s", rec.Code, rec.Body.String())
	}
	if rec := envReq(rt, cookie, http.MethodDelete, "/api/v1/environments/"+sandbox+"?move_to="+locked, ""); rec.Code != http.StatusConflict {
		t.Fatalf("bulk move into protected = %d", rec.Code)
	}
	if rec := envReq(rt, cookie, http.MethodDelete, "/api/v1/environments/"+sandbox+"?move_to=env_nope", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown move_to = %d", rec.Code)
	}
	if rec := envReq(rt, cookie, http.MethodDelete, "/api/v1/environments/"+sandbox+"?move_to=env_test", ""); rec.Code >= 300 {
		t.Fatalf("delete with move_to = %d: %s", rec.Code, rec.Body.String())
	}
	apps, dbs, _ := db.EnvironmentMemberCounts(ctx, "env_test")
	if apps != 1 || dbs != 1 {
		t.Errorf("members after move = %d apps, %d dbs", apps, dbs)
	}
	if _, err := db.GetEnvironment(ctx, sandbox); err == nil {
		t.Error("environment still exists")
	}
	if rec := envReq(rt, cookie, http.MethodDelete, "/api/v1/environments/"+locked, ""); rec.Code >= 300 {
		t.Errorf("delete empty = %d", rec.Code)
	}
}

func TestPreviewEnvironmentUsesPreviewKind(t *testing.T) {
	rt, _ := newTestRouter(t)
	projectID, envID, err := rt.ensurePreviewEnvironmentTier(context.Background(), "web")
	if err != nil || projectID != "preview-web" {
		t.Fatalf("project = %q, err = %v", projectID, err)
	}
	e, _ := rt.environments.GetEnvironment(context.Background(), envID)
	if e.Kind != store.EnvironmentKindPreview {
		t.Errorf("kind = %q", e.Kind)
	}
}

func TestMoveAppEnvironment_UnprotectedAppliesImmediately(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedAppAndDB(t, db)
	rec := envReq(rt, cookie, http.MethodPut, "/api/v1/apps/web/environment", `{"environment_id":"env_dev"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("move = %d: %s", rec.Code, rec.Body.String())
	}
	ref, _ := db.EnvironmentOfApp(context.Background(), "web")
	if ref == nil || ref.ID != "env_dev" {
		t.Errorf("ref = %+v", ref)
	}
	if rec := envReq(rt, cookie, http.MethodPut, "/api/v1/apps/web/environment", `{"environment_id":"env_nope"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("unknown target = %d", rec.Code)
	}
	if rec := envReq(rt, cookie, http.MethodPut, "/api/v1/apps/ghost/environment", `{"environment_id":"env_dev"}`); rec.Code != http.StatusNotFound {
		t.Errorf("unknown app = %d", rec.Code)
	}
}

func TestMoveAppEnvironment_ProtectedTargetNeedsConfirmThenApproval(t *testing.T) {
	rt, db := newTestRouter(t)
	requester := loginTestSession(t, rt, db)
	seedAppAndDB(t, db)
	ctx := context.Background()

	if rec := envReq(rt, requester, http.MethodPut, "/api/v1/apps/web/environment", `{"environment_id":"env_production"}`); rec.Code != http.StatusConflict {
		t.Fatalf("without confirm = %d: %s", rec.Code, rec.Body.String())
	}
	rec := envReq(rt, requester, http.MethodPut, "/api/v1/apps/web/environment", `{"environment_id":"env_production","confirm":true}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("with confirm = %d: %s", rec.Code, rec.Body.String())
	}
	var res deployTriggerResult
	_ = json.Unmarshal(rec.Body.Bytes(), &res)
	if res.PendingApproval == nil || res.PendingApproval.Action != store.DeployApprovalActionMoveApp || res.PendingApproval.EnvironmentID != "env_production" {
		t.Fatalf("result = %s", rec.Body.String())
	}
	if ref, _ := db.EnvironmentOfApp(ctx, "web"); ref != nil {
		t.Fatalf("app moved before approval: %+v", ref)
	}

	approver := storeUserWithAbilitiesForTest(t, db, "approver@example.com", []string{AbilityRead, AbilityDeploy})
	approverCookie := sessionCookieForTest(t, rt, approver.ID)
	rec = envReq(rt, approverCookie, http.MethodPost, "/api/v1/deploy-approvals/"+res.PendingApproval.ID+"/approve", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("approve = %d: %s", rec.Code, rec.Body.String())
	}
	if ref, _ := db.EnvironmentOfApp(ctx, "web"); ref == nil || ref.ID != "env_production" {
		t.Fatalf("app not moved after approval: %+v", ref)
	}
}

func TestMoveAppEnvironment_ProtectedSourceNeedsConfirm(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedAppAndDB(t, db)
	_ = db.SetServiceEnvironment(context.Background(), "web", "env_production")
	if rec := envReq(rt, cookie, http.MethodPut, "/api/v1/apps/web/environment", `{"environment_id":"env_dev"}`); rec.Code != http.StatusConflict {
		t.Fatalf("leaving protected without confirm = %d", rec.Code)
	}
	if rec := envReq(rt, cookie, http.MethodPut, "/api/v1/apps/web/environment", `{"environment_id":""}`); rec.Code != http.StatusConflict {
		t.Fatalf("clearing protected without confirm = %d", rec.Code)
	}
	if ref, _ := db.EnvironmentOfApp(context.Background(), "web"); ref == nil || ref.ID != "env_production" {
		t.Fatalf("app moved: %+v", ref)
	}
}

func TestMoveEnvironment_FreezeBlocks(t *testing.T) {
	rt, db, cookie := newSafetyRouter(t, nil)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "pg", Engine: "postgres", Version: "16"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ReplaceDeployFreezeWindows(context.Background(), store.DeployFreezeScopeGlobal, []store.DeployFreezeWindow{{Cron: "* * * * *", Duration: 2 * time.Hour}}); err != nil {
		t.Fatal(err)
	}
	if rec := envReq(rt, cookie, http.MethodPut, "/api/v1/apps/web/environment", `{"environment_id":"env_dev"}`); rec.Code != http.StatusLocked {
		t.Fatalf("app move in freeze = %d: %s", rec.Code, rec.Body.String())
	}
	if rec := envReq(rt, cookie, http.MethodPut, "/api/v1/databases/pg/environment", `{"environment_id":"env_dev"}`); rec.Code != http.StatusLocked {
		t.Fatalf("db move in freeze = %d: %s", rec.Code, rec.Body.String())
	}
	if ref, _ := db.EnvironmentOfApp(context.Background(), "web"); ref != nil {
		t.Fatalf("app moved during freeze: %+v", ref)
	}
}

func TestMoveDatabaseEnvironment(t *testing.T) {
	rt, db := newTestRouter(t)
	requester := loginTestSession(t, rt, db)
	seedAppAndDB(t, db)
	ctx := context.Background()
	if rec := envReq(rt, requester, http.MethodPut, "/api/v1/databases/pg/environment", `{"environment_id":"env_dev"}`); rec.Code != http.StatusOK {
		t.Fatalf("move = %d: %s", rec.Code, rec.Body.String())
	}
	if rec := envReq(rt, requester, http.MethodPut, "/api/v1/databases/pg/environment", `{"environment_id":"env_production"}`); rec.Code != http.StatusConflict {
		t.Fatalf("protected without confirm = %d", rec.Code)
	}
	rec := envReq(rt, requester, http.MethodPut, "/api/v1/databases/pg/environment", `{"environment_id":"env_production","confirm":true}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("protected with confirm = %d: %s", rec.Code, rec.Body.String())
	}
	var res deployTriggerResult
	_ = json.Unmarshal(rec.Body.Bytes(), &res)
	approver := storeUserWithAbilitiesForTest(t, db, "approver2@example.com", []string{AbilityRead, AbilityDeploy})
	cookie := sessionCookieForTest(t, rt, approver.ID)
	if rec := envReq(rt, cookie, http.MethodPost, "/api/v1/deploy-approvals/"+res.PendingApproval.ID+"/approve", ""); rec.Code != http.StatusOK {
		t.Fatalf("approve = %d: %s", rec.Code, rec.Body.String())
	}
	if ref, _ := db.EnvironmentOfDatabase(ctx, "pg"); ref == nil || ref.ID != "env_production" {
		t.Fatalf("db env = %+v", ref)
	}
	if rec := envReq(rt, requester, http.MethodPut, "/api/v1/databases/ghost/environment", `{"environment_id":"env_dev"}`); rec.Code != http.StatusNotFound {
		t.Errorf("unknown db = %d", rec.Code)
	}
}

func TestListFiltersByEnvironment(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedAppAndDB(t, db)
	ctx := context.Background()
	_ = db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: "other", Engine: "postgres", Version: "16"})
	_ = db.SetDatabaseEnvironment(ctx, "pg", "env_dev")
	_ = db.SetServiceEnvironment(ctx, "web", "env_dev")

	var dbs []databaseListResource
	rec := envReq(rt, cookie, http.MethodGet, "/api/v1/databases?environment=env_dev", "")
	_ = json.Unmarshal(rec.Body.Bytes(), &dbs)
	if len(dbs) != 1 || dbs[0].Name != "pg" {
		t.Fatalf("databases = %s", rec.Body.String())
	}
	rec = envReq(rt, cookie, http.MethodGet, "/api/v1/databases", "")
	_ = json.Unmarshal(rec.Body.Bytes(), &dbs)
	if len(dbs) != 2 {
		t.Fatalf("unfiltered databases = %d", len(dbs))
	}

	var apps []appListResource
	rec = envReq(rt, cookie, http.MethodGet, "/api/v1/apps?environment=env_dev", "")
	_ = json.Unmarshal(rec.Body.Bytes(), &apps)
	if len(apps) != 1 || apps[0].Name != "web" {
		t.Fatalf("apps = %s", rec.Body.String())
	}

	rec = envReq(rt, cookie, http.MethodPut, "/api/v1/apps/web/environment", `{"environment_id":"env_production","confirm":true}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("seed approval = %d", rec.Code)
	}
	var out deployApprovalListResponse
	rec = envReq(rt, cookie, http.MethodGet, "/api/v1/deploy-approvals?environment=env_production", "")
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if len(out.Approvals) != 1 {
		t.Fatalf("approvals = %s", rec.Body.String())
	}
	rec = envReq(rt, cookie, http.MethodGet, "/api/v1/deploy-approvals?environment=env_dev", "")
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if len(out.Approvals) != 0 {
		t.Fatalf("approvals for other env = %s", rec.Body.String())
	}
}

func TestGlobalEnvironmentSupportsEnvVars(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rec := envReq(rt, cookie, http.MethodPut, "/api/v1/environments/env_dev/env", `{"LOG_LEVEL":"debug"}`)
	if rec.Code >= 300 {
		t.Fatalf("set env = %d: %s", rec.Code, rec.Body.String())
	}
	rec = envReq(rt, cookie, http.MethodGet, "/api/v1/environments/env_dev/env", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "LOG_LEVEL") {
		t.Fatalf("get env = %d: %s", rec.Code, rec.Body.String())
	}
}

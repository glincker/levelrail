package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func rawCall(t *testing.T, rt *Router, req *http.Request) (int, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

func TestFirstApplyCreatesDeclaredProject(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	var plan stackPlanResponse
	if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/stacks/qa1/plan", stackBody(t, iacTestDocs, nil)), &plan); code != http.StatusOK || len(plan.Warnings) == 0 {
		t.Fatalf("plan = %d %+v", code, plan)
	}
	var out stackApplyResponse
	if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/stacks/qa1/apply", stackBody(t, iacTestDocs, nil)), &out); code != http.StatusOK || out.Revision == nil {
		t.Fatalf("apply = %d %+v", code, out)
	}
	if _, err := db.GetStackByName(t.Context(), "qa1"); err != nil {
		t.Fatalf("stack missing: %v", err)
	}
	if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/stacks/qa1/apply", stackBody(t, iacTestDocs, nil)), &out); code != http.StatusOK || !out.Noop {
		t.Fatalf("second apply = %d %+v", code, out)
	}
}

func TestFirstApplyMissingProjectIsExplicit422(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	noProject := strings.Replace(iacTestDocs, "kind: Project\nmetadata: {name: shop}\nspec:\n  env: {REGION: eu}\n---\n", "", 1)
	for _, verb := range []string{"plan", "apply"} {
		code, body := rawCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/stacks/qa1/"+verb, stackBody(t, noProject, map[string]any{"project": "shop"})))
		if code != http.StatusUnprocessableEntity || !strings.Contains(body, `project \"shop\" does not exist`) || !strings.Contains(body, "projects create") {
			t.Fatalf("%s = %d %s", verb, code, body)
		}
	}
	if _, err := db.GetStackByName(t.Context(), "qa1"); err == nil {
		t.Fatal("a refused apply created the stack")
	}
}

func TestFirstApplyFromCITokenNeverCreatesProject(t *testing.T) {
	rt, db := newTestRouter(t)
	tok := seedMatrixToken(t, db, "ci:runner", []string{AbilityRead, AbilityWrite})
	code, body := rawCall(t, rt, bearerRequest(http.MethodPost, "/api/v1/stacks/qa1/apply", stackBody(t, iacTestDocs, nil), tok))
	if code != http.StatusUnprocessableEntity || !strings.Contains(body, "cannot create it") {
		t.Fatalf("apply = %d %s", code, body)
	}
	if projects, _ := db.ListProjects(t.Context()); len(projects) != 0 {
		t.Fatalf("project created: %+v", projects)
	}
}

func TestFirstApplyRefusesEmptyPlan(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveProject(t.Context(), store.Project{ID: "p-other", Name: "other", CreatedAt: "2026-10-25T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	code, body := rawCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/stacks/qa1/apply", stackBody(t, iacTestDocs, map[string]any{"project": "other"})))
	if code != http.StatusUnprocessableEntity || !strings.Contains(body, "project filter") || !strings.Contains(body, "App/web") {
		t.Fatalf("apply = %d %s", code, body)
	}
	if _, err := db.GetStackByName(t.Context(), "qa1"); err == nil {
		t.Fatal("an empty apply created the stack")
	}
}

const stackDBDocs = `version: 1
kind: Project
metadata: {name: shop}
---
version: 1
kind: Database
metadata: {name: ordersdb}
spec: {engine: postgres, version: "16", project: shop}
`

func TestStackDestroyKeepsDatabasesUnlessDeleteData(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	apply := func(name string, extra map[string]any) {
		t.Helper()
		code, body := rawCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/stacks/"+name+"/apply", stackBody(t, stackDBDocs, extra)))
		if code != http.StatusOK || strings.Contains(body, `"status":"failed"`) {
			t.Fatalf("apply = %d %s", code, body)
		}
	}
	apply("dbstack", nil)

	if code, _ := rawCall(t, rt, authedRequest(t, cookie, http.MethodDelete, "/api/v1/stacks/dbstack?mode=forget&delete_data=true", "")); code != http.StatusBadRequest {
		t.Fatalf("delete_data with forget = %d, want 400", code)
	}
	var out stackDeleteResponse
	if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodDelete, "/api/v1/stacks/dbstack?mode=destroy", ""), &out); code != http.StatusOK || len(out.DataKept) != 1 {
		t.Fatalf("destroy = %d %+v", code, out)
	}
	if _, err := db.GetDesiredDatabase(t.Context(), "ordersdb"); err != nil {
		t.Fatalf("destroy without delete_data removed the database: %v", err)
	}

	apply("dbstack2", map[string]any{"adopt": true})
	out = stackDeleteResponse{}
	if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodDelete, "/api/v1/stacks/dbstack2?mode=destroy&delete_data=true", ""), &out); code != http.StatusOK || len(out.DataKept) != 0 {
		t.Fatalf("destroy with delete_data = %d %+v", code, out)
	}
	if _, err := db.GetDesiredDatabase(t.Context(), "ordersdb"); err == nil {
		t.Fatal("delete_data left the database behind")
	}
}

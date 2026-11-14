package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/iac"
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

func TestStackBootstrapNoSideEffectOnStalePlan(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	var plan stackPlanResponse
	if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/stacks/qa1/plan", stackBody(t, iacTestDocs, nil)), &plan); code != http.StatusOK || plan.Plan == nil {
		t.Fatalf("plan = %d", code)
	}
	var stale iacErrorBody
	code := iacCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/stacks/qa1/apply", stackBody(t, iacTestDocs, map[string]any{"expected_plan_hash": "v2:wrong"})), &stale)
	if code != http.StatusConflict || stale.Code != "plan_stale" {
		t.Fatalf("stale apply = %d %+v", code, stale)
	}
	if _, err := db.GetStackByName(t.Context(), "qa1"); err == nil {
		t.Fatal("a stale first apply created the stack")
	}
	if _, found := rt.projectByName(t.Context(), "shop"); found {
		t.Fatal("a stale first apply created the project")
	}
	var out stackApplyResponse
	code = iacCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/stacks/qa1/apply", stackBody(t, iacTestDocs, map[string]any{"expected_plan_hash": plan.Plan.Hash})), &out)
	if code != http.StatusOK || out.Revision == nil {
		t.Fatalf("apply with the planned hash = %d %+v", code, out)
	}
}

func TestStackBootstrapCloudRefusedLeavesNothing(t *testing.T) {
	rt, db, cookie, _ := approvalFixture(t)
	iac.SetCloudPlanner(&fakeCloud{action: "create"})
	t.Cleanup(func() { iac.SetCloudPlanner(nil) })
	var out iacErrorBody
	code := iacCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/stacks/qa-cloud/apply", stackBody(t, cloudStackDocs, map[string]any{"project": "shop"})), &out)
	if code != http.StatusForbidden || out.Code != iac.CodeApprovalRequired || !strings.Contains(out.Error, "without applying") {
		t.Fatalf("cloud first apply = %d %+v", code, out)
	}
	if _, err := db.GetStackByName(t.Context(), "qa-cloud"); err == nil {
		t.Fatal("a refused cloud first apply created the stack")
	}
	var ok stackApplyResponse
	if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/stacks/qa-plain/apply", stackBody(t, iacTestDocs, nil)), &ok); code != http.StatusOK || ok.Revision == nil {
		t.Fatalf("non-cloud first apply = %d %+v", code, ok)
	}
}

func TestStackReviewedHashFirstApply(t *testing.T) {
	docs := iacTestDocs + "      DB_PASSWORD: ${{ secrets.DB_PASSWORD }}\n---\nversion: 1\nkind: Environment\nmetadata: {name: staging}\nspec: {project: shop}\n"
	variants := map[string]map[string]any{
		"plain":     nil,
		"secrets":   {"secrets": map[string]string{"DB_PASSWORD": "s3cret-value-9876"}},
		"no_deploy": {"no_deploy": true},
		"both":      {"no_deploy": true, "secrets": map[string]string{"DB_PASSWORD": "s3cret-value-9876"}},
		"project":   {"project": "shop"},
		"vars":      {"vars": map[string]string{"X": "1"}},
	}
	for name, extra := range variants {
		t.Run(name, func(t *testing.T) {
			rt, db := newTestRouter(t)
			cookie := loginTestSession(t, rt, db)
			var plan stackPlanResponse
			if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/stacks/qa1/plan", stackBody(t, docs, extra)), &plan); code != http.StatusOK || plan.Plan == nil {
				t.Fatalf("plan = %d", code)
			}
			applyExtra := map[string]any{"expected_plan_hash": plan.Plan.Hash}
			for k, v := range extra {
				applyExtra[k] = v
			}
			var out stackApplyResponse
			code := iacCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/stacks/qa1/apply", stackBody(t, docs, applyExtra)), &out)
			if code != http.StatusOK || out.Revision == nil {
				t.Fatalf("apply with the reviewed hash = %d %+v", code, out)
			}
		})
	}
}

func TestStackReviewedHashFirstApplyBesideAnotherStack(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	var first stackApplyResponse
	if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/stacks/s1/apply", stackBody(t, iacTestDocs, nil)), &first); code != http.StatusOK {
		t.Fatalf("first stack = %d", code)
	}
	docs := strings.NewReplacer("shop", "shop2", "frontend", "frontend2", "web", "web2", "app.example.com", "app2.example.com").Replace(iacTestDocs)
	extra := map[string]any{"no_deploy": true, "secrets": map[string]string{"DB_PASSWORD": "s3cret-value-9876"}}
	var plan stackPlanResponse
	if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/stacks/s2/plan", stackBody(t, docs, extra)), &plan); code != http.StatusOK || plan.Plan == nil {
		t.Fatalf("plan = %d", code)
	}
	extra["expected_plan_hash"] = plan.Plan.Hash
	var out stackApplyResponse
	code := iacCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/stacks/s2/apply", stackBody(t, docs, extra)), &out)
	if code != http.StatusOK || out.Revision == nil {
		t.Fatalf("apply with the reviewed hash = %d %+v", code, out)
	}
}

func TestStackReviewedHashFirstApplyWithTokens(t *testing.T) {
	for name, abilities := range map[string][]string{"root": {AbilityRoot}, "write": {AbilityRead, AbilityWrite}} {
		t.Run(name, func(t *testing.T) {
			rt, db := newTestRouter(t)
			tok := seedApproverToken(t, db, "tok-"+name, abilities)
			var plan stackPlanResponse
			if code := tokenPost(t, rt, tok, "/api/v1/stacks/qa1/plan", stackBody(t, iacTestDocs, map[string]any{"no_deploy": true}), &plan); code != http.StatusOK || plan.Plan == nil {
				t.Fatalf("plan = %d", code)
			}
			var out stackApplyResponse
			body := stackBody(t, iacTestDocs, map[string]any{"no_deploy": true, "expected_plan_hash": plan.Plan.Hash})
			if code := tokenPost(t, rt, tok, "/api/v1/stacks/qa1/apply", body, &out); code != http.StatusOK || out.Revision == nil {
				t.Fatalf("apply = %d %+v", code, out)
			}
		})
	}
}

func TestStackReviewedHashFirstApplyNewStackNewProject(t *testing.T) {
	proj := "version: 1\nkind: Project\nmetadata: {name: pr2}\n---\n"
	app := "version: 1\nkind: App\nmetadata: {name: web2}\nspec:\n  service:\n    build: {type: image, image: \"ghcr.io/acme/web:1.2.3\"}\n    port: 3000\n    env:\n      LOG_LEVEL: info\n"
	appInProject := strings.Replace(app, "spec:\n", "spec:\n  project: pr2\n", 1)
	cases := map[string]struct {
		docs  string
		extra map[string]any
	}{
		"doc only":          {proj + app, nil},
		"doc and field":     {proj + app, map[string]any{"project": "pr2"}},
		"doc with app ref":  {proj + appInProject, map[string]any{"project": "pr2"}},
		"secrets no_deploy": {proj + appInProject, map[string]any{"project": "pr2", "no_deploy": true, "secrets": map[string]string{"DB_PASSWORD": "late-secret-value-1234"}}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			rt, db := newTestRouter(t)
			cookie := loginTestSession(t, rt, db)
			var plan stackPlanResponse
			if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/stacks/s2/plan", stackBody(t, c.docs, c.extra)), &plan); code != http.StatusOK || plan.Plan == nil {
				t.Fatalf("plan = %d", code)
			}
			extra := map[string]any{"expected_plan_hash": plan.Plan.Hash}
			for k, v := range c.extra {
				extra[k] = v
			}
			var out stackApplyResponse
			if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/stacks/s2/apply", stackBody(t, c.docs, extra)), &out); code != http.StatusOK || out.Revision == nil {
				t.Fatalf("apply = %d %+v", code, out)
			}
		})
	}
}

func TestStackBootstrapLateFailureLeavesNothing(t *testing.T) {
	// The document must embed the value or the plaintext guard has nothing to match.
	leak := map[string]any{"secrets": map[string]string{"DB_PASSWORD": "late-secret-value-1234"}}
	leakyDocs := strings.Replace(iacTestDocs, "LOG_LEVEL: info", "LOG_LEVEL: late-secret-value-1234", 1)

	t.Run("new stack and new project", func(t *testing.T) {
		rt, db := newTestRouter(t)
		cookie := loginTestSession(t, rt, db)
		code, body := rawCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/stacks/late1/apply", stackBody(t, leakyDocs, leak)))
		if code != http.StatusUnprocessableEntity || !strings.Contains(body, "secret value") {
			t.Fatalf("apply = %d %s", code, body)
		}
		if _, err := db.GetStackByName(t.Context(), "late1"); err == nil {
			t.Fatal("a late refusal left the stack")
		}
		if projects, _ := db.ListProjects(t.Context()); len(projects) != 0 {
			t.Fatalf("a late refusal left the project: %+v", projects)
		}
	})

	t.Run("existing project is untouched", func(t *testing.T) {
		rt, db := newTestRouter(t)
		cookie := loginTestSession(t, rt, db)
		if err := db.SaveProject(t.Context(), store.Project{ID: "p-shop", Name: "shop", CreatedAt: "2026-10-25T00:00:00Z"}); err != nil {
			t.Fatal(err)
		}
		code, _ := rawCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/stacks/late2/apply", stackBody(t, leakyDocs, leak)))
		if code != http.StatusUnprocessableEntity {
			t.Fatalf("apply = %d", code)
		}
		if _, err := db.GetStackByName(t.Context(), "late2"); err == nil {
			t.Fatal("a late refusal left the stack")
		}
		if projects, _ := db.ListProjects(t.Context()); len(projects) != 1 || projects[0].ID != "p-shop" {
			t.Fatalf("existing project changed: %+v", projects)
		}
	})

	t.Run("success path keeps the rows", func(t *testing.T) {
		rt, db := newTestRouter(t)
		cookie := loginTestSession(t, rt, db)
		var out stackApplyResponse
		if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/stacks/late3/apply", stackBody(t, iacTestDocs, nil)), &out); code != http.StatusOK {
			t.Fatalf("apply = %d", code)
		}
		if _, err := db.GetStackByName(t.Context(), "late3"); err != nil {
			t.Fatalf("stack missing: %v", err)
		}
		if projects, _ := db.ListProjects(t.Context()); len(projects) != 1 {
			t.Fatalf("projects = %+v", projects)
		}
	})
}

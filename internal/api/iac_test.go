package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/iac"
	"github.com/GLINCKER/levelrail/internal/store"
)

const iacTestDocs = `version: 1
kind: Project
metadata: {name: shop}
spec:
  env: {REGION: eu}
---
version: 1
kind: Tag
metadata: {name: frontend}
---
version: 1
kind: App
metadata: {name: web}
spec:
  project: shop
  tags: [frontend]
  service:
    build: {type: image, image: "ghcr.io/acme/web:1.2.3"}
    port: 3000
    domains: [app.example.com]
    replicas: 2
    resources: {memory: 512Mi, cpu: 0.5}
    health:
      readiness: {path: /healthz, interval: 5s, timeout: 2s}
    env:
      LOG_LEVEL: info
`

func iacRequestBody(t *testing.T, docs string, extra map[string]any) string {
	t.Helper()
	body := map[string]any{"files": []iacFile{{Name: "infra/app.yaml", Content: docs}}}
	for k, v := range extra {
		body[k] = v
	}
	return jsonBody(t, body)
}

func iacCall(t *testing.T, rt *Router, req *http.Request, out any) int {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if out != nil && rec.Body.Len() > 0 {
		if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
			t.Fatalf("decode %q: %v", rec.Body.String(), err)
		}
	}
	return rec.Code
}

func TestIaCPlanApplyExportRoundTrip(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	body := iacRequestBody(t, iacTestDocs, map[string]any{"source": "ci"})

	var plan iacPlanResponse
	if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/apply/plan", body), &plan); code != http.StatusOK {
		t.Fatalf("plan status = %d", code)
	}
	if plan.Plan.Summary.Create != 3 || plan.Plan.Summary.Error != 0 {
		t.Fatalf("plan = %+v", plan.Plan)
	}
	if apps, _ := db.ListDesiredServices(t.Context()); len(apps) != 0 {
		t.Fatal("planning created an app")
	}

	var applied iacApplyResponse
	applyBody := iacRequestBody(t, iacTestDocs, map[string]any{"source": "ci", "expected_plan_hash": plan.Plan.Hash})
	if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/apply", applyBody), &applied); code != http.StatusOK {
		t.Fatalf("apply status = %d", code)
	}
	if !applied.Result.OK() || applied.Result.Applied != 3 {
		t.Fatalf("apply = %+v", applied.Result)
	}
	svc, err := db.GetDesiredService(t.Context(), "web")
	if err != nil {
		t.Fatal(err)
	}
	if svc.Image != "ghcr.io/acme/web:1.2.3" || svc.Replicas != 2 || svc.ProjectID == "" || len(svc.Domains) != 1 {
		t.Fatalf("stored app = %+v", svc)
	}

	var again iacPlanResponse
	code2 := iacCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/apply/plan", body), &again)
	if again.Plan == nil {
		t.Fatalf("second plan: status %d issues %+v", code2, again.Issues)
	}
	if again.Plan.Pending() {
		t.Fatalf("second plan is not empty: %+v", again.Plan.Changes)
	}

	var exported iac.ExportResult
	if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/export?project=shop", ""), &exported); code != http.StatusOK {
		t.Fatalf("export status = %d", code)
	}
	joined := exported.Join()
	if !strings.Contains(joined, "kind: App") || !strings.Contains(joined, "kind: Project") || strings.Contains(joined, "managed-by") {
		t.Fatalf("export:\n%s", joined)
	}
	var replan iacPlanResponse
	code := iacCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/apply/plan", iacRequestBody(t, joined, nil)), &replan)
	if code != http.StatusOK || replan.Plan == nil || replan.Plan.Pending() {
		t.Fatalf("exported files are not a no-op: %d %+v %+v\n%s", code, replan.Plan, replan.Issues, joined)
	}
}

func TestIaCPlanReportsLineNumberedIssues(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	bad := "version: 1\nkind: App\nmetadata: {name: web}\nspec:\n  service:\n    build: {type: image, image: x}\n    port: 80\n    env:\n      DB_PASSWORD: hunter2\n"
	var out iacPlanResponse
	code := iacCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/apply/plan", iacRequestBody(t, bad, nil)), &out)
	if code != http.StatusUnprocessableEntity || len(out.Issues) != 1 || out.Issues[0].Line != 9 || out.Issues[0].File != "infra/app.yaml" {
		t.Fatalf("code = %d issues = %+v", code, out.Issues)
	}
	if strings.Contains(jsonBody(t, out), "hunter2") {
		t.Fatal("the response echoes the refused secret")
	}
}

func TestIaCPlanNeverResolvesPlaceholdersFromServerEnv(t *testing.T) {
	t.Setenv("IAC_API_TEST_SECRET", "server-side-value")
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	docs := "version: 1\nkind: App\nmetadata: {name: web}\nspec:\n  service:\n    build: {type: image, image: x}\n    port: 80\n    env:\n      X: ${{ env.IAC_API_TEST_SECRET }}\n"
	var out iacPlanResponse
	code := iacCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/apply/plan", iacRequestBody(t, docs, nil)), &out)
	if code != http.StatusUnprocessableEntity || len(out.Issues) != 1 || !strings.Contains(out.Issues[0].Message, "IAC_API_TEST_SECRET is not provided") {
		t.Fatalf("code = %d issues = %+v", code, out.Issues)
	}
	if strings.Contains(jsonBody(t, out), "server-side-value") {
		t.Fatal("the response carries the server environment value")
	}
}

func TestIaCApplyRefusesPruneWithoutSource(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	code := iacCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/apply", iacRequestBody(t, iacTestDocs, map[string]any{"prune": true})), nil)
	if code != http.StatusBadRequest {
		t.Fatalf("status = %d", code)
	}
}

func TestIaCPerResourceDenyReportedOnItem(t *testing.T) {
	rt, db := newTestRouter(t)
	bootstrapTestAdmin(t, db)
	if err := db.SaveDesiredService(t.Context(), store.DesiredService{Name: "victim", Image: "x:1", Port: 80}); err != nil {
		t.Fatal(err)
	}
	tok := seedMatrixToken(t, db, "iac-tok", []string{AbilityRoot})
	attachTestPolicy(t, db, "deny-victim", "Deny", "*", "app:victim", store.PrincipalTypeToken, "iac-tok")

	docs := strings.Replace(iacTestDocs, "name: web", "name: victim", 1)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apply/plan", strings.NewReader(iacRequestBody(t, docs, nil)))
	req.Header.Set("Authorization", "Bearer "+tok)
	var plan iacPlanResponse
	if code := iacCall(t, rt, req, &plan); code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	var denied bool
	for _, c := range plan.Plan.Changes {
		denied = denied || (c.Kind == iac.KindApp && c.Denied)
	}
	if !denied {
		t.Fatalf("app item not reported as denied: %+v", plan.Plan.Changes)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/apply", strings.NewReader(iacRequestBody(t, docs, nil)))
	req.Header.Set("Authorization", "Bearer "+tok)
	var applied iacApplyResponse
	iacCall(t, rt, req, &applied)
	if applied.Result.Applied != 0 {
		t.Fatalf("apply went ahead despite a denied item: %+v", applied.Result)
	}
	svc, _ := db.GetDesiredService(t.Context(), "victim")
	if svc.Image != "x:1" {
		t.Fatalf("denied app was changed: %+v", svc)
	}
}

func TestIaCReadOnlyTokenCanPlanButNotApply(t *testing.T) {
	rt, db := newTestRouter(t)
	bootstrapTestAdmin(t, db)
	tok := seedMatrixToken(t, db, "ro-tok", []string{AbilityRead})
	post := func(path string) int {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(iacRequestBody(t, iacTestDocs, nil)))
		req.Header.Set("Authorization", "Bearer "+tok)
		return iacCall(t, rt, req, nil)
	}
	if code := post("/api/v1/apply/plan"); code != http.StatusOK {
		t.Fatalf("plan status = %d", code)
	}
	if code := post("/api/v1/apply"); code != http.StatusForbidden {
		t.Fatalf("apply status = %d", code)
	}
}

func TestUpdateAppKeepsFieldsThePutBodyCannotExpress(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(t.Context(), store.DesiredService{
		Name: "web", Image: "x:1", Port: 80,
		SecretEnv:  []store.SecretEnvRef{{Name: "API_KEY", Required: true}},
		VaultEnv:   map[string]store.VaultEnvRef{"V": {Path: "a", Key: "b"}},
		Volumes:    []store.ServiceVolume{{Name: "app-web-data", ContainerPath: "/data"}},
		PullPolicy: store.PullPolicyAlways,
	}); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web", `{"image":"x:2","port":80}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	got, err := db.GetDesiredService(t.Context(), "web")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.SecretEnv) != 1 || !got.SecretEnv[0].Required || len(got.VaultEnv) != 1 || len(got.Volumes) != 1 || got.PullPolicy != store.PullPolicyAlways {
		t.Fatalf("PUT dropped stored settings: %+v", got)
	}
}

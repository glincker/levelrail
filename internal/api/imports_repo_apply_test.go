package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/quota"
	"github.com/GLINCKER/levelrail/internal/repoplan"
	"github.com/GLINCKER/levelrail/internal/store"
)

const applyRepo = "https://github.com/acme/mono"

func applyBody(t *testing.T, mutate func(m map[string]any)) string {
	t.Helper()
	m := map[string]any{
		"repo_url": applyRepo, "branch": "main", "token": "ghp_supersecret",
		"packages": []map[string]any{
			{"package": "web", "name": "mono-web", "directory": "apps/web", "port": 3000},
			{"package": "api", "name": "mono-api", "directory": "apps/api", "port": 3000},
			{"package": "ui", "name": "mono-ui", "directory": "packages/ui", "port": 8080},
		},
	}
	if mutate != nil {
		mutate(m)
	}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func doApply(t *testing.T, rt *Router, cookie *http.Cookie, body string) (int, importApplyResponse, string) {
	t.Helper()
	rec := layoutReq(t, rt, cookie, http.MethodPost, "/api/v1/imports/repo/apply", body)
	var out importApplyResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out, rec.Body.String()
}

func statusByName(out importApplyResponse) map[string]importApplyResult {
	m := map[string]importApplyResult{}
	for _, r := range out.Results {
		m[r.Name] = r
	}
	return m
}

func newApplyRouter(t *testing.T) (*Router, *store.DB, *http.Cookie) {
	t.Helper()
	rt, db := newTestRouterWithGitSourceSecrets(t, newFakeGitSourceSecrets())
	useFakeLayout(rt, &fakeLayoutSource{files: planMonoFixture})
	return rt, db, loginTestSession(t, rt, db)
}

func TestImportRepoApply_CreatesAppsWithGitSources(t *testing.T) {
	rt, db, cookie := newApplyRouter(t)
	code, out, body := doApply(t, rt, cookie, applyBody(t, nil))
	if code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", code, body)
	}
	got := statusByName(out)
	for _, name := range []string{"mono-web", "mono-api", "mono-ui"} {
		if got[name].Status != importStatusCreated {
			t.Fatalf("%s = %+v, body = %s", name, got[name], body)
		}
	}
	gs, err := db.GetGitSource(context.Background(), "mono-api")
	if err != nil || gs.BaseDirectory != "apps/api" || gs.RepoURL != applyRepo || gs.BuildType != "dockerfile" {
		t.Fatalf("git source = %+v, err = %v", gs, err)
	}
	if gs, _ := db.GetGitSource(context.Background(), "mono-web"); gs == nil || gs.BuildType != "railpack" {
		t.Fatalf("web git source = %+v", gs)
	}
	if strings.Contains(body, "ghp_supersecret") {
		t.Fatalf("response leaks the token: %s", body)
	}
}

func TestImportRepoApply_FailureSkipsDependentsOnly(t *testing.T) {
	rt, db, cookie := newApplyRouter(t)
	seedApp(t, db, "mono-ui")
	code, out, body := doApply(t, rt, cookie, applyBody(t, nil))
	if code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", code, body)
	}
	got := statusByName(out)
	if got["mono-ui"].Status != importStatusFailed || got["mono-ui"].Code != importCodeNameTaken {
		t.Fatalf("ui = %+v", got["mono-ui"])
	}
	if got["mono-web"].Status != importStatusSkippedDependency || got["mono-web"].Code != importCodeDependencyFailed {
		t.Fatalf("web = %+v, want skipped_dependency", got["mono-web"])
	}
	if got["mono-api"].Status != importStatusCreated {
		t.Fatalf("api = %+v, want created", got["mono-api"])
	}
	if _, err := db.GetDesiredService(context.Background(), "mono-web"); err == nil {
		t.Fatal("skipped app was created")
	}
}

func TestImportRepoApply_IdempotentReapply(t *testing.T) {
	rt, _, cookie := newApplyRouter(t)
	if code, _, body := doApply(t, rt, cookie, applyBody(t, nil)); code != http.StatusOK {
		t.Fatalf("first apply = %d %s", code, body)
	}
	_, out, body := doApply(t, rt, cookie, applyBody(t, nil))
	for _, r := range out.Results {
		if r.Status != importStatusAlreadyExists {
			t.Fatalf("second apply row = %+v, body = %s", r, body)
		}
	}
}

func TestImportRepoApply_DryRunCreatesNothingAndReportsEveryError(t *testing.T) {
	rt, db, cookie := newApplyRouter(t)
	body := applyBody(t, func(m map[string]any) {
		m["dry_run"] = true
		m["packages"] = []map[string]any{
			{"name": "mono-web", "directory": "apps/web", "port": 3000},
			{"name": "Bad Name", "directory": "apps/api", "port": 3000},
			{"name": "mono-ui", "directory": "packages/ui", "port": 70000},
			{"name": "ghost", "directory": "apps/ghost", "port": 1},
			{"name": "mono-web", "directory": "apps/web", "port": 3000},
		}
	})
	code, out, raw := doApply(t, rt, cookie, body)
	if code != http.StatusOK || !out.DryRun {
		t.Fatalf("status = %d, body = %s", code, raw)
	}
	wantCodes := []string{"", importCodeInvalidName, importCodeInvalidPort, importCodeUnknownPackage, importCodeDuplicate}
	for i, want := range wantCodes {
		if out.Results[i].Code != want {
			t.Errorf("row %d code = %q, want %q (%+v)", i, out.Results[i].Code, want, out.Results[i])
		}
	}
	if out.Results[0].Status != importStatusWouldCreate {
		t.Errorf("row 0 = %+v", out.Results[0])
	}
	svcs, _ := db.ListDesiredServices(context.Background())
	if len(svcs) != 0 {
		t.Fatalf("dry run created %d apps", len(svcs))
	}
}

func TestImportRepoApply_ConfirmThreshold(t *testing.T) {
	t.Setenv(EnvImportConfirmAbove, "2")
	rt, db, cookie := newApplyRouter(t)
	code, _, body := doApply(t, rt, cookie, applyBody(t, nil))
	if code != http.StatusConflict || !strings.Contains(body, importCodeConfirmation) {
		t.Fatalf("status = %d, body = %s, want 409 confirmation_required", code, body)
	}
	if svcs, _ := db.ListDesiredServices(context.Background()); len(svcs) != 0 {
		t.Fatalf("refused import created %d apps", len(svcs))
	}
	_, dry, _ := doApply(t, rt, cookie, applyBody(t, func(m map[string]any) { m["dry_run"] = true }))
	if !dry.NeedsConfirmation || dry.ConfirmAbove != 2 {
		t.Fatalf("dry run = %+v, want needs_confirmation", dry)
	}
	if code, _, body = doApply(t, rt, cookie, applyBody(t, func(m map[string]any) { m["confirm"] = true })); code != http.StatusOK {
		t.Fatalf("confirmed status = %d %s", code, body)
	}
}

func TestImportRepoApply_QuotaExceeded(t *testing.T) {
	rt, db, cookie := newApplyRouter(t)
	ctx := context.Background()
	if err := db.SaveProject(ctx, store.Project{ID: "proj_q", Name: "shop", CreatedAt: "2026-10-01T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetQuota(ctx, quota.ScopeProject, "proj_q", quota.Amounts{Apps: 1}); err != nil {
		t.Fatal(err)
	}
	mutate := func(m map[string]any) {
		m["packages"] = []map[string]any{
			{"name": "mono-api", "directory": "apps/api", "port": 3000, "project_id": "proj_q"},
			{"name": "mono-web", "directory": "apps/web", "port": 3000, "project_id": "proj_q"},
		}
	}
	for _, dry := range []bool{true, false} {
		_, out, body := doApply(t, rt, cookie, applyBody(t, func(m map[string]any) { mutate(m); m["dry_run"] = dry }))
		got := statusByName(out)
		if got["mono-web"].Status != importStatusFailed || got["mono-web"].Code != quotaExceededCode {
			t.Fatalf("dry=%v web = %+v, body = %s", dry, got["mono-web"], body)
		}
		if got["mono-api"].Status == importStatusFailed {
			t.Fatalf("dry=%v api = %+v", dry, got["mono-api"])
		}
	}
}

func TestImportRepoApply_SecretsStoredNeverEchoed(t *testing.T) {
	setter := &fakeSecretSetter{}
	db := openTestDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db, WithGitSourceSecrets(newFakeGitSourceSecrets()), WithSecretSetter(setter))
	useFakeLayout(rt, &fakeLayoutSource{files: planMonoFixture})
	cookie := loginTestSession(t, rt, db)
	body := applyBody(t, func(m map[string]any) {
		m["packages"] = []map[string]any{{
			"name": "mono-api", "directory": "apps/api", "port": 3000,
			"env": map[string]string{"LOG_LEVEL": "debug", "DB_PASSWORD": "hunter2-value"}, "secret_env_keys": []string{"DB_PASSWORD"},
		}}
	})
	code, out, raw := doApply(t, rt, cookie, body)
	if code != http.StatusOK || out.Results[0].Status != importStatusCreated {
		t.Fatalf("status = %d, body = %s", code, raw)
	}
	if strings.Contains(raw, "hunter2-value") || strings.Contains(raw, "ghp_supersecret") {
		t.Fatalf("response echoes a secret: %s", raw)
	}
	if setter.calls != 1 || setter.lastKey != "DB_PASSWORD" || setter.lastValue != "hunter2-value" {
		t.Fatalf("secret setter = %+v", setter)
	}
	svc, _ := db.GetDesiredService(context.Background(), "mono-api")
	if svc.Env["LOG_LEVEL"] != "debug" || svc.Env["DB_PASSWORD"] != "" {
		t.Fatalf("env = %+v, secret must not be a plain env value", svc.Env)
	}
}

func TestImportRepoApply_Authorization(t *testing.T) {
	rt, db, _ := newApplyRouter(t)
	do := func(tok string) (int, importApplyResponse) {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/imports/repo/apply", strings.NewReader(applyBody(t, nil)))
		req.Header.Set("Authorization", "Bearer "+tok)
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, req)
		var out importApplyResponse
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out
	}
	if code, _ := do(seedMatrixToken(t, db, "ro", []string{AbilityRead})); code != http.StatusForbidden {
		t.Fatalf("read-only token = %d, want 403", code)
	}
	code, out := do(seedMatrixToken(t, db, "wr", []string{AbilityRead, AbilityWrite}))
	if code != http.StatusOK {
		t.Fatalf("write token = %d", code)
	}
	for _, r := range out.Results {
		if r.Status != importStatusFailed || r.Code != importCodeForbidden {
			t.Fatalf("write-only token row = %+v, want forbidden (git source needs the sensitive ability)", r)
		}
	}
	if svcs, _ := db.ListDesiredServices(context.Background()); len(svcs) != 0 {
		t.Fatalf("forbidden import created %d apps", len(svcs))
	}
	if code, out = do(seedMatrixToken(t, db, "ws", []string{AbilityRead, AbilityWrite, AbilityWriteSensitive})); code != http.StatusOK || out.Results[0].Status != importStatusCreated {
		t.Fatalf("sensitive token = %d %+v", code, out.Results)
	}
}

func TestImportOrder_DependenciesFirstAndCyclesBreak(t *testing.T) {
	results := []importApplyResult{{Directory: "apps/web"}, {Directory: "apps/api"}, {Directory: "packages/ui"}, {Directory: "packages/a"}, {Directory: "packages/b"}}
	rows := map[string]repoplan.Row{
		"apps/web":    {DependsOn: []string{"packages/ui"}},
		"packages/ui": {DependsOn: []string{"packages/a"}},
		"packages/a":  {DependsOn: []string{"packages/b"}},
		"packages/b":  {DependsOn: []string{"packages/a"}},
	}
	order := importOrder(results, rows)
	pos := map[string]int{}
	for i, idx := range order {
		pos[results[idx].Directory] = i
	}
	if len(order) != len(results) || pos["packages/ui"] > pos["apps/web"] || pos["packages/a"] > pos["packages/ui"] {
		t.Fatalf("order = %v", pos)
	}
}

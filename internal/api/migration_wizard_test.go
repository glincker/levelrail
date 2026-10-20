package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/migrationplan"
	"github.com/GLINCKER/levelrail/internal/platformimport"
)

func TestMigrationWizardPlanImportAndChecklist(t *testing.T) {
	t.Setenv(platformimport.AllowLoopbackEnv, "true")
	src := newImportSourceServer(t)
	db := openTestDB(t)
	rt := NewRouter(nil, testBrand(), db, WithSecretSetter(&fakeSecretSetter{}))
	cookie := loginTestSession(t, rt, db)
	do := func(method, path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, body))
		return rec
	}
	base := strings.TrimSuffix(importBody(src.URL), "}")

	rec := do(http.MethodPost, "/api/v1/migration/plan", base+"}")
	if rec.Code != http.StatusOK {
		t.Fatalf("plan status %d: %s", rec.Code, rec.Body.String())
	}
	var rep migrationplan.Report
	if err := json.Unmarshal(rec.Body.Bytes(), &rep); err != nil {
		t.Fatal(err)
	}
	if len(rep.Items) != 2 || strings.Contains(rec.Body.String(), "abc123") {
		t.Fatalf("report: %s", rec.Body.String())
	}
	if _, err := db.GetDesiredService(t.Context(), "storefront"); err == nil {
		t.Fatal("plan must not create anything")
	}

	rec = do(http.MethodPost, "/api/v1/migration/import", base+`,"dry_run":true,"only":["a1"],"overrides":{"a1":{"name":"shop-web"}}}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "shop-web") {
		t.Fatalf("dry run: %d %s", rec.Code, rec.Body.String())
	}
	if _, err := db.GetDesiredService(t.Context(), "shop-web"); err == nil {
		t.Fatal("dry run must not create anything")
	}

	if rec = do(http.MethodPost, "/api/v1/migration/import", base+"}"); rec.Code != http.StatusBadRequest {
		t.Fatalf("import without a selection: %d", rec.Code)
	}
	rec = do(http.MethodPost, "/api/v1/migration/import", base+`,"only":["a1"],"overrides":{"a1":{"name":"shop-web"}}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("import: %d %s", rec.Code, rec.Body.String())
	}
	if _, err := db.GetDesiredService(t.Context(), "shop-web"); err != nil {
		t.Fatalf("selected app not created: %v", err)
	}
	if _, err := db.GetDesiredDatabase(t.Context(), "shop-db"); err == nil {
		t.Fatal("unselected database must not be created")
	}

	rec = do(http.MethodGet, "/api/v1/migration/checklists/shop-web", "")
	var cl checklistView
	if err := json.Unmarshal(rec.Body.Bytes(), &cl); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("checklist: %d %v %s", rec.Code, err, rec.Body.String())
	}
	if cl.Current != migrationplan.StepDataVerified || cl.Progress != 1 || !cl.Tracked || len(cl.Volumes) != 1 {
		t.Fatalf("checklist view: %+v", cl)
	}

	step := func(id, body string) *httptest.ResponseRecorder {
		return do(http.MethodPost, "/api/v1/migration/checklists/shop-web/steps/"+id, body)
	}
	if rec = step(migrationplan.StepOldStopped, `{"action":"complete","confirm":true}`); rec.Code != http.StatusConflict {
		t.Fatalf("skipping ahead: %d", rec.Code)
	}
	if rec = step(migrationplan.StepDataVerified, `{"action":"complete"}`); rec.Code != http.StatusConflict ||
		!strings.Contains(rec.Body.String(), "app-shop-web-uploads") {
		t.Fatalf("data verify without a volume comparison: %d %s", rec.Code, rec.Body.String())
	}
	bad := `{"action":"complete","volumes":[{"name":"app-shop-web-uploads","source_files":3,"source_bytes":90,"target_files":2,"target_bytes":90}]}`
	if rec = step(migrationplan.StepDataVerified, bad); rec.Code != http.StatusConflict {
		t.Fatalf("mismatched volume must block: %d", rec.Code)
	}
	good := `{"action":"complete","volumes":[{"name":"app-shop-web-uploads","source_files":3,"source_bytes":90,"target_files":3,"target_bytes":90}]}`
	if rec = step(migrationplan.StepDataVerified, good); rec.Code != http.StatusOK {
		t.Fatalf("verified volume: %d %s", rec.Code, rec.Body.String())
	}
	if rec = step(migrationplan.StepShadowRunning, `{"action":"complete"}`); rec.Code != http.StatusConflict {
		t.Fatalf("shadow without a healthy app: %d", rec.Code)
	}
	if rec = step(migrationplan.StepImported, `{"action":"reset"}`); rec.Code != http.StatusOK {
		t.Fatalf("reset: %d", rec.Code)
	}
	rec = do(http.MethodGet, "/api/v1/migration/checklists", "")
	if err := json.Unmarshal(rec.Body.Bytes(), &struct {
		Checklists []checklistView `json:"checklists"`
	}{}); err != nil || !strings.Contains(rec.Body.String(), `"progress":0`) {
		t.Fatalf("list after reset: %s", rec.Body.String())
	}
}

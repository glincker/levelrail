package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/coolifyexit"
	"github.com/GLINCKER/levelrail/internal/coolifyexit/exportfixture"
	"github.com/GLINCKER/levelrail/internal/platformimport"
)

func coolifyExitBody(url string) string {
	return `{"platform":"coolify","url":"` + url + `","token":"` + exportfixture.Token + `","allow_loopback":true,"only":["storefront"]}`
}

func TestCoolifyExitReportAndCronImport(t *testing.T) {
	t.Setenv(platformimport.AllowLoopbackEnv, "true")
	src := exportfixture.New(t)
	db := openTestDB(t)
	rt := NewRouter(nil, testBrand(), db, WithSecretSetter(&fakeSecretSetter{}))
	cookie := loginTestSession(t, rt, db)

	post := func(path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, path, body))
		return rec
	}

	rec := post("/api/v1/migration/coolify/exit-report", coolifyExitBody(src.URL))
	if rec.Code != http.StatusOK {
		t.Fatalf("report status %d: %s", rec.Code, rec.Body.String())
	}
	for _, leak := range []string{exportfixture.Token, "fake-db-password", "fake-session-secret", "fake-webhook-secret"} {
		if strings.Contains(rec.Body.String(), leak) {
			t.Fatalf("report leaked %q", leak)
		}
	}
	var before coolifyexit.Report
	if err := json.Unmarshal(rec.Body.Bytes(), &before); err != nil || before.Mode != coolifyexit.ModeDryRun {
		t.Fatalf("dry run: %v mode=%q", err, before.Mode)
	}
	if _, err := db.GetDesiredService(t.Context(), "storefront"); err == nil {
		t.Fatal("the report must not create anything")
	}

	if rec = post("/api/v1/imports/platform/apply", coolifyExitBody(src.URL)); rec.Code != http.StatusOK {
		t.Fatalf("apply status %d: %s", rec.Code, rec.Body.String())
	}
	jobs, err := db.ListScheduledTasksForService(t.Context(), "storefront")
	if err != nil || len(jobs) != 2 {
		t.Fatalf("jobs = %d, err = %v", len(jobs), err)
	}
	for _, j := range jobs {
		if !j.Enabled || len(j.Command) != 3 || j.Command[0] != "sh" {
			t.Errorf("job: %+v", j)
		}
	}

	rec = post("/api/v1/migration/coolify/exit-report", coolifyExitBody(src.URL))
	var after coolifyexit.Report
	if err := json.Unmarshal(rec.Body.Bytes(), &after); err != nil || after.Mode != coolifyexit.ModeVerify {
		t.Fatalf("verify: %v mode=%q body=%s", err, after.Mode, rec.Body.String())
	}
	if v := src.Violations(); len(v) != 0 {
		t.Fatalf("source was written to: %v", v)
	}

	if rec = post("/api/v1/migration/coolify/exit-report", `{"platform":"docker","snapshot":"[]"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("a pasted-file platform status %d", rec.Code)
	}
}

func TestPlatformImportSkipsJobsForNonRoot(t *testing.T) {
	t.Setenv(platformimport.AllowLoopbackEnv, "true")
	src := exportfixture.New(t)
	db := openTestDB(t)
	rt := NewRouter(nil, testBrand(), db, WithSecretSetter(&fakeSecretSetter{}))
	tok := seedMatrixToken(t, db, "importer", []string{AbilityRead, AbilityReadSensitive, AbilityWrite, AbilityWriteSensitive, AbilityDeploy})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/imports/platform/apply", strings.NewReader(coolifyExitBody(src.URL)))
	req.Header.Set("Content-Type", "application/json")
	bearer(tok)(req)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("apply status %d: %s", rec.Code, rec.Body.String())
	}
	if _, err := db.GetDesiredService(t.Context(), "storefront"); err != nil {
		t.Fatalf("app must still be imported: %v", err)
	}
	jobs, err := db.ListScheduledTasksForService(t.Context(), "storefront")
	if err != nil || len(jobs) != 0 {
		t.Fatalf("non-root import created %d job(s), err = %v", len(jobs), err)
	}
	if !strings.Contains(rec.Body.String(), "scheduled job(s) were skipped") {
		t.Errorf("result must say jobs were skipped: %s", rec.Body.String())
	}
}

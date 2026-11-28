package api

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/glincker/levelrail/internal/projectarchive"
	"github.com/glincker/levelrail/internal/store"
)

func seedTwoAppProject(t *testing.T, db *store.DB) {
	t.Helper()
	ctx := context.Background()
	if err := db.SaveProject(ctx, store.Project{ID: "proj_x", Name: "shop", CreatedAt: "2026-08-16T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	for _, a := range []store.DesiredService{
		{Name: "web", Image: "ghcr.io/acme/web:1", Port: 3000, Domains: []string{"shop.example.com"}, Env: map[string]string{"MODE": "prod", "API_TOKEN": projectExportCanary}},
		{Name: "worker", Image: "ghcr.io/acme/worker:1", Port: 8080, Env: map[string]string{"MODE": "prod"}},
	} {
		if err := db.SaveDesiredService(ctx, a); err != nil {
			t.Fatal(err)
		}
		if err := db.UpdateServiceProject(ctx, a.Name, "proj_x"); err != nil {
			t.Fatal(err)
		}
	}
}

func exportArchive(t *testing.T, rt *Router, cookie *http.Cookie, body map[string]any) (int, []byte) {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/projects/proj_x/export", jsonBody(t, body)))
	return rec.Code, rec.Body.Bytes()
}

func importCall(t *testing.T, rt *Router, cookie *http.Cookie, path string, req projectImportRequest) (int, projectImportResponse) {
	t.Helper()
	b, _ := json.Marshal(req)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, path, string(b)))
	var out projectImportResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if rec.Code == http.StatusUnprocessableEntity {
		t.Logf("422 body: %s", rec.Body.String())
	}
	return rec.Code, out
}

func TestProjectArchive_WithoutSecretsFlagHoldsNoValues(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedTwoAppProject(t, db)
	code, raw := exportArchive(t, rt, cookie, map[string]any{})
	if code != http.StatusOK {
		t.Fatalf("status %d: %s", code, raw)
	}
	arc, err := projectarchive.Read(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if arc.Secrets != nil || arc.Manifest.SecretsIncluded {
		t.Fatal("secrets present without the flag")
	}
	var all strings.Builder
	for _, b := range arc.Spec {
		all.Write(b)
	}
	if strings.Contains(all.String(), projectExportCanary) || !strings.Contains(all.String(), "API_TOKEN") {
		t.Fatalf("spec must name the variable and never hold its value:\n%s", all.String())
	}
}

func TestProjectArchive_RoundTripAcrossInstances(t *testing.T) {
	t.Setenv(envSecretReveal, "allow")
	src, srcDB := newTestRouter(t)
	srcCookie := loginTestSession(t, src, srcDB)
	seedTwoAppProject(t, srcDB)
	pass := "correct horse battery"
	code, raw := exportArchive(t, src, srcCookie, map[string]any{"include_secrets": true, "passphrase": pass})
	if code != http.StatusOK {
		t.Fatalf("status %d: %s", code, raw)
	}
	if bytes.Contains(raw, []byte(projectExportCanary)) {
		t.Fatal("plaintext secret in the archive bytes")
	}

	dst, dstDB := newTestRouter(t)
	dstCookie := loginTestSession(t, dst, dstDB)
	if c, _ := importCall(t, dst, dstCookie, "/api/v1/projects/import/plan", projectImportRequest{Archive: raw, Passphrase: "wrong passphrase!!"}); c != http.StatusUnprocessableEntity {
		t.Fatalf("wrong passphrase status = %d", c)
	}
	c, plan := importCall(t, dst, dstCookie, "/api/v1/projects/import/plan", projectImportRequest{Archive: raw, Passphrase: pass})
	if c != http.StatusOK || len(plan.Plan.Conflicts) != 0 || !plan.Plan.SecretsApplied || len(plan.Plan.Creates) < 3 {
		t.Fatalf("plan = %d %+v", c, plan.Plan)
	}
	if len(plan.Plan.Manual) == 0 || !strings.Contains(strings.Join(plan.Plan.Manual, " "), "shop.example.com") {
		t.Fatalf("manual steps = %v", plan.Plan.Manual)
	}
	c, done := importCall(t, dst, dstCookie, "/api/v1/projects/import", projectImportRequest{Archive: raw, Passphrase: pass, ExpectedPlanHash: plan.Plan.Hash})
	if c != http.StatusOK || done.Result == nil || !done.Result.OK() {
		t.Fatalf("import = %d %+v", c, done)
	}
	for _, n := range []string{"web", "worker"} {
		if _, err := dstDB.GetDesiredService(t.Context(), n); err != nil {
			t.Fatalf("app %s missing: %v", n, err)
		}
	}
}

func TestProjectArchive_PlanReportsNameConflicts(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedTwoAppProject(t, db)
	_, raw := exportArchive(t, rt, cookie, map[string]any{})
	c, plan := importCall(t, rt, cookie, "/api/v1/projects/import/plan", projectImportRequest{Archive: raw})
	if c != http.StatusOK || len(plan.Plan.Conflicts) == 0 {
		t.Fatalf("expected conflicts: %d %+v", c, plan.Plan)
	}
	kinds := map[string]bool{}
	for _, cf := range plan.Plan.Conflicts {
		kinds[cf.Kind] = true
	}
	if !kinds["Project"] || !kinds["App"] {
		t.Fatalf("conflict kinds = %v", kinds)
	}
	if c, _ := importCall(t, rt, cookie, "/api/v1/projects/import", projectImportRequest{Archive: raw}); c != http.StatusConflict {
		t.Fatalf("import over conflicts = %d", c)
	}
}

func TestProjectArchive_TamperedArchiveRejected(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedTwoAppProject(t, db)
	_, raw := exportArchive(t, rt, cookie, map[string]any{})
	bad := append([]byte{}, raw...)
	bad[len(bad)/2] ^= 0xff
	if c, _ := importCall(t, rt, cookie, "/api/v1/projects/import/plan", projectImportRequest{Archive: bad}); c != http.StatusUnprocessableEntity {
		t.Fatalf("tampered archive status = %d", c)
	}
}

func TestProjectArchive_ImportNeedsPlanHash(t *testing.T) {
	src, srcDB := newTestRouter(t)
	srcCookie := loginTestSession(t, src, srcDB)
	seedTwoAppProject(t, srcDB)
	_, raw := exportArchive(t, src, srcCookie, map[string]any{})
	dst, dstDB := newTestRouter(t)
	dstCookie := loginTestSession(t, dst, dstDB)
	if c, _ := importCall(t, dst, dstCookie, "/api/v1/projects/import", projectImportRequest{Archive: raw}); c != http.StatusUnprocessableEntity {
		t.Fatalf("no hash status = %d", c)
	}
	if c, _ := importCall(t, dst, dstCookie, "/api/v1/projects/import", projectImportRequest{Archive: raw, ExpectedPlanHash: "v2:deadbeef"}); c != http.StatusConflict {
		t.Fatalf("stale hash status = %d", c)
	}
	if _, err := dstDB.GetDesiredService(t.Context(), "web"); err == nil {
		t.Fatal("a refused import created an app")
	}
}

func TestProjectArchive_ImportOnlyCreatesProjectDocuments(t *testing.T) {
	src, srcDB := newTestRouter(t)
	srcCookie := loginTestSession(t, src, srcDB)
	seedTwoAppProject(t, srcDB)
	_, raw := exportArchive(t, src, srcCookie, map[string]any{})
	arc, err := projectarchive.Read(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	w := projectarchive.NewWriter(&out)
	for n, b := range arc.Spec {
		extra := "\n---\nkind: Setting\nmetadata: {name: evil}\nspec: {value: x}\n---\nkind: App\nmetadata: {name: intruder}\nspec: {project: elsewhere, service: {build: {image: ghcr.io/x/y:1}}}\n"
		if err := w.AddBytes(projectarchive.SpecPrefix+n, append(b, []byte(extra)...)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(projectarchive.Manifest{Project: "shop"}); err != nil {
		t.Fatal(err)
	}
	dst, dstDB := newTestRouter(t)
	dstCookie := loginTestSession(t, dst, dstDB)
	c, plan := importCall(t, dst, dstCookie, "/api/v1/projects/import/plan", projectImportRequest{Archive: out.Bytes()})
	if c != http.StatusOK {
		t.Fatalf("plan = %d", c)
	}
	for _, it := range plan.Plan.Creates {
		if it.Kind == "Setting" || it.Name == "intruder" || it.Name == "evil" {
			t.Fatalf("foreign document planned: %+v", it)
		}
	}
	if !strings.Contains(strings.Join(plan.Plan.Warnings, " "), "do not belong to this project") {
		t.Fatalf("warnings = %v", plan.Plan.Warnings)
	}
	if c, _ := importCall(t, dst, dstCookie, "/api/v1/projects/import", projectImportRequest{Archive: out.Bytes(), ExpectedPlanHash: plan.Plan.Hash}); c != http.StatusOK {
		t.Fatalf("import = %d", c)
	}
	if _, err := dstDB.GetDesiredService(t.Context(), "intruder"); err == nil {
		t.Fatal("foreign app was created")
	}
}

func TestProjectArchive_ImportAuditsAttemptAndOutcome(t *testing.T) {
	src, srcDB := newTestRouter(t)
	srcCookie := loginTestSession(t, src, srcDB)
	seedTwoAppProject(t, srcDB)
	_, raw := exportArchive(t, src, srcCookie, map[string]any{})
	dst, dstDB := newTestRouter(t)
	dstCookie := loginTestSession(t, dst, dstDB)
	_, plan := importCall(t, dst, dstCookie, "/api/v1/projects/import/plan", projectImportRequest{Archive: raw})
	if c, _ := importCall(t, dst, dstCookie, "/api/v1/projects/import", projectImportRequest{Archive: raw, ExpectedPlanHash: plan.Plan.Hash}); c != http.StatusOK {
		t.Fatalf("import = %d", c)
	}
	entries, err := dstDB.ListAuditEntries(t.Context(), 50, nil, store.AuditEntryFilter{})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, e := range entries {
		seen[e.Action] = true
	}
	if !seen[AuditActionProjectArchiveImportAttempt] || !seen[AuditActionProjectArchiveImport] {
		t.Fatalf("audit actions = %v", seen)
	}
}

func TestProjectArchive_IncludeDataNeedsReadSensitive(t *testing.T) {
	db := openTestDB(t)
	rt := NewRouter(slog.New(slog.NewTextHandler(discardWriter{}, nil)), testBrand(), db, WithBackupDownloader(&fakeBackupDownloader{content: "x"}))
	cookie := loginTestSession(t, rt, db)
	seedTwoAppProject(t, db)
	tok := mintTokenBody(t, rt, cookie, `{"name":"limited","abilities":["read","write:sensitive"]}`)
	post := func(body string) int {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/proj_x/export", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+tok.Token)
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, req)
		return rec.Code
	}
	if c := post(`{"include_data":true}`); c != http.StatusForbidden {
		t.Fatalf("include_data without read:sensitive = %d", c)
	}
	if c := post(`{}`); c != http.StatusOK {
		t.Fatalf("plain export = %d", c)
	}
}

func TestProjectArchive_ImportConcurrencyLimit(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	var releases []func()
	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		rel, ok := rt.acquireImportSlot(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/projects/import", ""))
		if !ok {
			t.Fatalf("slot %d refused", i)
		}
		releases = append(releases, rel)
	}
	rec := httptest.NewRecorder()
	if _, ok := rt.acquireImportSlot(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/projects/import", "")); ok || rec.Code != http.StatusTooManyRequests {
		t.Fatalf("third slot = %v %d", ok, rec.Code)
	}
	releases[0]()
	rec = httptest.NewRecorder()
	rel, ok := rt.acquireImportSlot(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/projects/import", ""))
	if !ok {
		t.Fatal("a freed slot was not reusable")
	}
	rel()
	releases[1]()
}

package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestDatabaseImport_ViewerGets403(t *testing.T) {
	rt, db := newTestRouter(t)
	if err := db.SaveDesiredDatabase(t.Context(), store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	viewer := seedMatrixToken(t, db, "viewer1", []string{AbilityRead})

	if rec := incTokenPost(rt, viewer, "/api/v1/databases/main/import", "dump"); rec.Code != http.StatusForbidden {
		t.Errorf("viewer import = %d, want 403", rec.Code)
	}
	if rec := incTokenGet(rt, viewer, "/api/v1/databases/main/import"); rec.Code != http.StatusForbidden {
		t.Errorf("viewer check = %d, want 403", rec.Code)
	}
}

type cutOffUpload struct{ sent bool }

func (f *cutOffUpload) Read(p []byte) (int, error) {
	if f.sent {
		return 0, errors.New("connection reset")
	}
	f.sent = true
	return copy(p, "partial"), nil
}

func TestSpoolImportBody_OverCapAndCutOffNeverYieldASpool(t *testing.T) {
	rec := httptest.NewRecorder()
	over := http.MaxBytesReader(rec, io.NopCloser(strings.NewReader(strings.Repeat("x", 100))), 10)
	_, _, err := spoolImportBody(over)
	var tooBig *http.MaxBytesError
	if !errors.As(err, &tooBig) {
		t.Fatalf("over cap err = %v, want MaxBytesError", err)
	}
	if _, _, err := spoolImportBody(&cutOffUpload{}); err == nil {
		t.Fatal("cut-off upload must fail, got nil")
	}
	sp, n, err := spoolImportBody(strings.NewReader("select 1;"))
	if err != nil || n != 9 {
		t.Fatalf("spool = %d, %v", n, err)
	}
	defer sp.discard()
	if b, _ := io.ReadAll(sp.f); string(b) != "select 1;" {
		t.Errorf("spool not rewound, got %q", b)
	}
}

// importFakeRuntime answers the emptiness probe and records the restore stream.
type importFakeRuntime struct {
	*fakeExecAppRuntime
	probe     string
	restores  int
	restoredN int
}

func (f *importFakeRuntime) Exec(context.Context, string, []string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader(f.probe)), nil
}

func (f *importFakeRuntime) ExecWithInput(_ context.Context, _ string, _ []string, in io.Reader) (io.ReadCloser, error) {
	f.restores++
	n, _ := io.Copy(io.Discard, in)
	f.restoredN = int(n)
	return io.NopCloser(strings.NewReader("")), nil
}

func newImportFixture(t *testing.T, probe string) (*Router, *store.DB, *importFakeRuntime, string) {
	t.Helper()
	rt, db := newTestRouter(t)
	if err := db.SaveDesiredDatabase(t.Context(), store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	fake := &importFakeRuntime{fakeExecAppRuntime: &fakeExecAppRuntime{}, probe: probe}
	rt.execRuntime = func(string) (docker.Runtime, error) { return fake, nil }
	return rt, db, fake, seedMatrixToken(t, db, "root1", []string{AbilityRoot})
}

func importAudit(t *testing.T, db *store.DB) []store.AuditEntry {
	t.Helper()
	entries, err := db.ListAuditEntries(t.Context(), 50, nil, store.AuditEntryFilter{Action: AuditActionDatabaseImport})
	if err != nil {
		t.Fatal(err)
	}
	return entries
}

func TestDatabaseImport_NonEmptyNeedsForceAndAudits(t *testing.T) {
	const canary = "CANARY-DUMP-CONTENT"
	rt, db, fake, tok := newImportFixture(t, "users|3\n")
	const path = "/api/v1/databases/main/import"

	if rec := incTokenPost(rt, tok, path, canary); rec.Code != http.StatusConflict || fake.restores != 0 {
		t.Fatalf("no force = %d restores=%d, want 409 and no restore", rec.Code, fake.restores)
	}
	if rec := incTokenPost(rt, tok, path+"?force=true", canary); rec.Code != http.StatusBadRequest || fake.restores != 0 {
		t.Fatalf("force without confirm = %d restores=%d, want 400 and no restore", rec.Code, fake.restores)
	}
	rec := incTokenPost(rt, tok, path+"?force=true&confirm=main", canary)
	if rec.Code != http.StatusOK || fake.restores != 1 || fake.restoredN != len(canary) {
		t.Fatalf("force = %d restores=%d bytes=%d body=%s", rec.Code, fake.restores, fake.restoredN, rec.Body.String())
	}
	var res databaseImportResource
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil || !res.Replaced || res.Bytes != int64(len(canary)) {
		t.Fatalf("resource = %+v, err = %v", res, err)
	}

	entries := importAudit(t, db)
	if len(entries) != 3 {
		t.Fatalf("audit entries = %d, want 3 (409, 400, 200)", len(entries))
	}
	got := map[int]string{}
	for _, e := range entries {
		got[e.StatusCode] = e.Detail
		if strings.Contains(e.Detail+e.Path, canary) {
			t.Fatal("audit entry carries dump content")
		}
	}
	want := `{"database":"main","bytes":19,"replaced":true}`
	if got[http.StatusOK] != want {
		t.Errorf("200 detail = %s, want %s", got[http.StatusOK], want)
	}
}

func TestDatabaseImport_EmptyTargetNeedsNoForce(t *testing.T) {
	rt, _, fake, tok := newImportFixture(t, "")
	rec := incTokenPost(rt, tok, "/api/v1/databases/main/import", "select 1;")
	if rec.Code != http.StatusOK || fake.restores != 1 {
		t.Fatalf("empty import = %d restores=%d", rec.Code, fake.restores)
	}
}

func TestDatabaseImport_OverCapIs413WithNoSpoolLeft(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	rt, db, fake, tok := newImportFixture(t, "")
	limit := 1 << 20
	if _, err := rt.runtimeSettings.Set(t.Context(), runtimesettings.KeyDatabaseImportMaxBytes, json.RawMessage("1048576"), "test"); err != nil {
		t.Fatal(err)
	}
	over := strings.Repeat("x", limit+1)

	// Declared length is refused before reading; an unknown length is cut by the reader cap.
	for _, declared := range []bool{true, false} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/databases/main/import", io.NopCloser(strings.NewReader(over)))
		req.Header.Set("Authorization", "Bearer "+tok)
		req.ContentLength = -1
		if declared {
			req.ContentLength = int64(len(over))
		}
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusRequestEntityTooLarge {
			t.Fatalf("declared=%v status = %d, want 413", declared, rec.Code)
		}
	}
	if fake.restores != 0 {
		t.Fatal("an oversized dump reached the restore")
	}
	if left, _ := os.ReadDir(os.TempDir()); len(left) != 0 {
		t.Fatalf("spool files left behind: %d", len(left))
	}
	if n := len(importAudit(t, db)); n != 2 {
		t.Errorf("audit entries = %d, want 2", n)
	}
}

func TestDatabaseImport_NonRootAndAnonymousDenied(t *testing.T) {
	rt, db, fake, _ := newImportFixture(t, "")
	writer := seedMatrixToken(t, db, "writer1", []string{AbilityRead, AbilityWriteSensitive})
	const path = "/api/v1/databases/main/import"

	if rec := incTokenGet(rt, writer, path); rec.Code != http.StatusForbidden {
		t.Errorf("non-root GET = %d, want 403", rec.Code)
	}
	if rec := incTokenPost(rt, writer, path, "dump"); rec.Code != http.StatusForbidden {
		t.Errorf("non-root POST = %d, want 403", rec.Code)
	}
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader("dump")))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("anonymous %s = %d, want 401", method, rec.Code)
		}
	}
	if fake.restores != 0 || len(importAudit(t, db)) != 0 {
		t.Fatal("a denied request reached the handler")
	}
}

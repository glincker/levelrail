package api

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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

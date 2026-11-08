package api

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/platformimport"
)

func TestImportBoundsK8sManifestOverCapIs413(t *testing.T) {
	platformimport.SetLimits(0, 0)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/import/k8s", strings.NewReader(strings.Repeat("a", 4096)))
	if _, ok := readK8sManifest(rec, req, 10); ok {
		t.Fatal("oversize manifest must be refused")
	}
	if rec.Code != 413 {
		t.Fatalf("status = %d, want 413", rec.Code)
	}
}

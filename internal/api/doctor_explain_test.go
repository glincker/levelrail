package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/fixit"
)

// A new check or fix must not ship without a plain-language entry.
func TestDoctorExplainCoversRegistry(t *testing.T) {
	t.Parallel()
	rt := &Router{}
	for _, d := range rt.doctorDefs() {
		if _, ok := fixit.Explain(d.ID); !ok {
			t.Errorf("doctor check %q has no explanation in internal/fixit/explain.go", d.ID)
		}
	}
	fixIDs := map[string]bool{"tokens.legacy.migrate": true}
	for _, f := range rt.fixitFixes() {
		fixIDs[f.ID] = true
		e, ok := fixit.Explain(f.CheckID)
		if !ok {
			t.Errorf("fix check %q has no explanation", f.CheckID)
			continue
		}
		if e.FixID != f.ID {
			t.Errorf("explanation for %q names fix %q, registered fix is %q", f.CheckID, e.FixID, f.ID)
		}
	}
	for _, id := range fixit.ExplainIDs() {
		if e, _ := fixit.Explain(id); e.FixID != "" && !fixIDs[e.FixID] {
			t.Errorf("explanation for %q names unregistered fix %q", id, e.FixID)
		}
	}
}

func TestDoctorExplainHandler(t *testing.T) {
	t.Parallel()
	rt := &Router{}
	rec := httptest.NewRecorder()
	rt.handleDoctorExplain(rec, httptest.NewRequest(http.MethodGet, "/api/v1/doctor/explain?id=disk_space", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "disk.reclaim") {
		t.Fatalf("known id: %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	rt.handleDoctorExplain(rec, httptest.NewRequest(http.MethodGet, "/api/v1/doctor/explain?id=nope", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown id: %d", rec.Code)
	}
}

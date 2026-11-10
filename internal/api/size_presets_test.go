package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

func TestSizePresetListAndParse(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rec := serve(rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/size-presets", ""))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"name":"small"`) {
		t.Fatalf("list = %d %s", rec.Code, rec.Body.String())
	}
	if _, err := runtimesettings.ParseSizePresets("a=1:512Mi,a=2:1Gi"); err == nil {
		t.Error("duplicate preset accepted")
	}
	if _, err := runtimesettings.ParseSizePresets("a=0:512Mi"); err == nil {
		t.Error("zero cpu accepted")
	}
	got, err := runtimesettings.ParseSizePresets("big=2:2Gi")
	if err != nil || got[0].MemoryBytes != 2<<30 {
		t.Errorf("parse = %+v %v", got, err)
	}
}

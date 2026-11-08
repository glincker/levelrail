package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSizePresetsHandlerServesDefaults(t *testing.T) {
	rec := httptest.NewRecorder()
	(&Router{}).handleSizePresets(rec, httptest.NewRequest(http.MethodGet, "/api/v1/size-presets", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"name":"small"`) {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}

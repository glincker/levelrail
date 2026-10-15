package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/ingress"
)

type fakeAccess struct{ got ingress.AccessFilter }

func (f *fakeAccess) AppAccess(_ string, flt ingress.AccessFilter) []ingress.AccessRecord {
	f.got = flt
	return nil
}

func TestHandleAppAccess(t *testing.T) {
	fa := &fakeAccess{}
	rt := &Router{access: fa}
	req := httptest.NewRequest(http.MethodGet, "/x?min_ms=250&min_status=500&limit=20&path=/api", nil)
	req.SetPathValue("name", "web")
	w := httptest.NewRecorder()
	rt.handleAppAccess(w, req)
	if w.Code != http.StatusOK || w.Body.String() != "[]\n" {
		t.Fatalf("status %d body %q", w.Code, w.Body.String())
	}
	if fa.got.MinMillis != 250 || fa.got.MinStatus != 500 || fa.got.Limit != 20 || fa.got.Path != "/api" {
		t.Fatalf("filter not parsed: %+v", fa.got)
	}

	req = httptest.NewRequest(http.MethodGet, "/x?min_ms=abc", nil)
	w = httptest.NewRecorder()
	rt.handleAppAccess(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bad min_ms status %d", w.Code)
	}

	w = httptest.NewRecorder()
	(&Router{}).handleAppAccess(w, req)
	if w.Code != http.StatusNotImplemented {
		t.Fatalf("unconfigured status %d", w.Code)
	}
}

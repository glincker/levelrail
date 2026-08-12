//go:build !embedweb

package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestHandler_UsesPackageDistFS_Stub pins Handler()'s wiring to
// handlerFromFS(DistFS) for the default (non -tags embedweb) build,
// where DistFS is the stub package-level embed.FS's zero value (see
// embed_stub.go), so this must behave exactly like the no-dist case.
func TestHandler_UsesPackageDistFS_Stub(t *testing.T) {
	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want %d (stub DistFS, no -tags embedweb in this build)", rec.Code, http.StatusNotImplemented)
	}
}

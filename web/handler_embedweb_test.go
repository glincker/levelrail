//go:build embedweb

package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestHandler_UsesPackageDistFS_Embedded pins Handler()'s wiring to
// handlerFromFS(DistFS) for an -tags embedweb build, where embed_real.go
// embeds the real web/dist output (see that file's doc comment): unlike
// the stub build, this must serve real content, not a 501, proving
// DistFS actually holds the built frontend and not an accidentally
// empty embed.
func TestHandler_UsesPackageDistFS_Embedded(t *testing.T) {
	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: this build tag means web/dist must actually be embedded (run `cd web && npm run build` first)", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, "<!doctype html") && !strings.Contains(body, "<!DOCTYPE html") {
		t.Errorf("body = %q, want the real built index.html shell", body)
	}
}

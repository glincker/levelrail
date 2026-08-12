package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func TestHandlerFromFS_NoDist_Returns501(t *testing.T) {
	h := handlerFromFS(fstest.MapFS{}) // no "dist" entry at all: the stub-build shape

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}
}

func TestHandlerFromFS_ServesRealAsset(t *testing.T) {
	h := handlerFromFS(fstest.MapFS{
		"dist/index.html":         &fstest.MapFile{Data: []byte("<html>shell</html>")},
		"dist/assets/app-123.js":  &fstest.MapFile{Data: []byte("console.log(1)")},
		"dist/assets/app-123.css": &fstest.MapFile{Data: []byte("body{}")},
	})

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/assets/app-123.js", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Body.String(); got != "console.log(1)" {
		t.Errorf("body = %q, want the real asset content, not the SPA fallback", got)
	}
}

func TestHandlerFromFS_UnknownPath_FallsBackToIndex(t *testing.T) {
	h := handlerFromFS(fstest.MapFS{
		"dist/index.html": &fstest.MapFile{Data: []byte("<html>shell</html>")},
	})

	tests := []string{"/", "/apps", "/apps/myapp/env", "/apps/myapp/deploys/42/logs"}
	for _, path := range tests {
		t.Run(path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 (SPA fallback), for path %q", rec.Code, path)
			}
			if got := rec.Body.String(); got != "<html>shell</html>" {
				t.Errorf("body = %q, want index.html's content for unmatched client-side route %q", got, path)
			}
		})
	}
}

func TestHandler_UsesPackageDistFS(t *testing.T) {
	// Handler() itself just delegates to handlerFromFS(DistFS); this
	// pins that wiring so a future refactor can't silently drop it.
	// DistFS is the stub (empty) in a default (non -tags embedweb)
	// test run, so this should behave exactly like the no-dist case.
	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want %d (stub DistFS, no -tags embedweb in this test run)", rec.Code, http.StatusNotImplemented)
	}
}

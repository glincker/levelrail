package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestHandlerFromFS_HTMLRoutesNeverFrameable(t *testing.T) {
	h := handlerFromFS(fstest.MapFS{
		"dist/index.html": &fstest.MapFile{Data: []byte("<html>shell</html>")},
	}, nil)
	for _, path := range []string{"/", "/apps", "/settings/security", "/nodes/a/b"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		csp := rec.Header().Get("Content-Security-Policy")
		if rec.Header().Get("X-Frame-Options") != "DENY" {
			t.Errorf("%s: X-Frame-Options = %q", path, rec.Header().Get("X-Frame-Options"))
		}
		if !strings.Contains(csp, "frame-ancestors 'none'") {
			t.Errorf("%s: CSP lacks frame-ancestors 'none'", path)
		}
		if strings.Contains(csp, "unsafe-eval") {
			t.Errorf("%s: CSP allows unsafe-eval", path)
		}
	}
}

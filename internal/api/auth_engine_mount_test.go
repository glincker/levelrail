package api

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func serveOnce(t *testing.T, h http.Handler, method, target, body string) (int, string) {
	t.Helper()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	out, _ := io.ReadAll(rec.Result().Body)
	return rec.Code, string(out)
}

func TestAuthEngineMountLeavesLegacyRoutesByteIdentical(t *testing.T) {
	const prefix = "/api/v1/auth-lib"
	db := openTestDB(t)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))

	off := NewRouter(logger, testBrand(), db)
	engine := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	on := NewRouter(logger, testBrand(), db, WithAuthEngine(prefix, engine))
	ignored := NewRouter(logger, testBrand(), db, WithAuthEngine(prefix, nil))

	legacy := []struct{ method, target, body string }{
		{http.MethodGet, "/api/v1/auth/setup-status", ""},
		{http.MethodPost, "/api/v1/auth/login", `{"username":"nobody","password":"wrong"}`},
		{http.MethodGet, "/api/v1/auth/me", ""},
		{http.MethodGet, "/api/v1/tokens", ""},
	}
	for _, c := range legacy {
		wantCode, wantBody := serveOnce(t, off.Handler(), c.method, c.target, c.body)
		for name, rt := range map[string]*Router{"engine mounted": on, "nil handler": ignored} {
			code, body := serveOnce(t, rt.Handler(), c.method, c.target, c.body)
			if code != wantCode || body != wantBody {
				t.Errorf("%s %s with %s: got %d %q, want %d %q", c.method, c.target, name, code, body, wantCode, wantBody)
			}
		}
	}

	if code, _ := serveOnce(t, on.Handler(), http.MethodGet, prefix+"/anything", ""); code != http.StatusTeapot {
		t.Errorf("mounted prefix status = %d, want %d", code, http.StatusTeapot)
	}
	if code, _ := serveOnce(t, off.Handler(), http.MethodGet, prefix+"/anything", ""); code == http.StatusTeapot {
		t.Error("flag off must not serve the library prefix")
	}
}

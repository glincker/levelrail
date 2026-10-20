package api

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func serveOnce(t *testing.T, h http.Handler, method, target, body string) int {
	t.Helper()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code
}

func TestRouterMountsTheLibraryEngine(t *testing.T) {
	db := openTestDB(t)
	rt := NewRouter(slog.New(slog.NewTextHandler(discardWriter{}, nil)), testBrand(), db)
	prefix := rt.authEngine.Prefix()
	if code := serveOnce(t, rt.Handler(), http.MethodGet, prefix+"/anything", ""); code == http.StatusOK {
		t.Errorf("unknown library route status = %d, want a non-200", code)
	}
	if code := serveOnce(t, rt.Handler(), http.MethodPost, "/api/v1/auth/login", `{"username":"nobody","password":"wrong-pass-1"}`); code != http.StatusUnauthorized {
		t.Errorf("login with unknown user status = %d, want 401", code)
	}
}

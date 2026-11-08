package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func condServe(h http.HandlerFunc, path, inm string, enabled bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if inm != "" {
		req.Header.Set("If-None-Match", inm)
	}
	rec := httptest.NewRecorder()
	conditionalGetMiddleware(func(*http.Request) bool { return enabled }, h).ServeHTTP(rec, req)
	return rec
}

func TestConditionalGet(t *testing.T) {
	t.Parallel()
	jsonH := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"a":1}`)
	}
	first := condServe(jsonH, "/api/v1/apps", "", true)
	etag := first.Header().Get("Etag")
	if etag == "" || first.Code != 200 || first.Body.String() != `{"a":1}` {
		t.Fatalf("first = %d etag %q body %q", first.Code, etag, first.Body.String())
	}
	if got := condServe(jsonH, "/api/v1/apps", etag, true); got.Code != http.StatusNotModified || got.Body.Len() != 0 {
		t.Errorf("match = %d body %q", got.Code, got.Body.String())
	}
	if got := condServe(jsonH, "/api/v1/apps", `"other"`, true); got.Code != 200 {
		t.Errorf("mismatch = %d", got.Code)
	}
	if got := condServe(jsonH, "/api/v1/apps", etag, false); got.Code != 200 || got.Header().Get("Etag") != "" {
		t.Errorf("disabled = %d etag %q", got.Code, got.Header().Get("Etag"))
	}
}

func TestConditionalGet_NoETagOnSecretsOrAuth(t *testing.T) {
	t.Parallel()
	noStore := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = io.WriteString(w, `{"token":"x"}`)
	}
	if got := condServe(noStore, "/api/v1/apps/a/env", "", true); got.Header().Get("Etag") != "" {
		t.Errorf("no-store response got an etag")
	}
	plain := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{}`)
	}
	if got := condServe(plain, "/api/v1/auth/me", "", true); got.Header().Get("Etag") != "" {
		t.Errorf("auth route got an etag")
	}
}

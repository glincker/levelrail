package main

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func serve(h http.Handler, method, host, origin string) int {
	req := httptest.NewRequest(method, "/api/desktop/exchange", nil)
	req.Host = host
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w.Code
}

func TestDesktopModeOffLeavesHandlerUntouched(t *testing.T) {
	t.Setenv("APP_DESKTOP_MODE", "")
	next := http.NotFoundHandler()
	h := wrapDesktopMode(slog.Default(), nil, nil, next)
	if got := serve(h, "POST", "evil.example", "http://evil.example"); got != http.StatusNotFound {
		t.Fatalf("off mode: code %d, want 404 with no Host or Origin enforcement", got)
	}
}

func TestDesktopModeInvalidValueFailsClosed(t *testing.T) {
	t.Setenv("APP_DESKTOP_MODE", "maybe")
	h := wrapDesktopMode(slog.Default(), nil, nil, http.NotFoundHandler())
	if got := serve(h, "GET", "levelrail.localhost:8080", ""); got != http.StatusServiceUnavailable {
		t.Fatalf("code %d, want 503", got)
	}
}

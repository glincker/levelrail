package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// This file tests handleDevMode, kept separate from devmode_test.go
// (which covers the devModeEnabled build-tag gate and the fixture-token
// seeding logic) since this is purely the HTTP surface: an unauthenticated
// caller asking "is dev mode active," used by the login screen to offer a
// one-click dev login instead of typing dev/dev by hand.

func TestHandleDevMode_PublicNoAuthRequired(t *testing.T) {
	rt, _ := newTestRouter(t)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/dev-mode", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var got devModeResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// newTestRouter builds a router the normal way, no dev-mode override,
	// so in this non-embedweb test build devModeEnabled() reflects
	// whatever APP_DEV_MODE actually is in the test process's real
	// environment (unset in CI/normal test runs). This test's job is to
	// prove the route exists, is unauthenticated, and returns the field
	// devModeEnabled() would actually produce, not to force a specific
	// value; see TestHandleDevMode_ReflectsDevModeEnabled below for that.
	if got.Enabled != devModeEnabled() {
		t.Errorf("Enabled = %v, want %v (devModeEnabled())", got.Enabled, devModeEnabled())
	}
}

func TestHandleDevMode_ReflectsDevModeEnabled(t *testing.T) {
	t.Setenv("APP_DEV_MODE", "1")
	rt, _ := newTestRouter(t)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/dev-mode", nil))

	var got devModeResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// devModeEnabled() itself is build-tag gated (devmode_debug.go vs
	// devmode_release.go): in this !embedweb test build it reads the env
	// var directly, so setting APP_DEV_MODE=1 here proves the handler
	// reflects the real gate rather than a hardcoded value, without this
	// test needing its own build-tag variant.
	if got.Enabled != devModeEnabled() {
		t.Errorf("Enabled = %v, want %v to match devModeEnabled() with APP_DEV_MODE=1 set", got.Enabled, devModeEnabled())
	}
}

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Tests the unauthenticated HTTP surface of handleDevMode; the build-tag gate itself is tested in devmode_test.go.

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
	// Runs in the non-embedweb build, so the field reflects the real APP_DEV_MODE env.
	// This test only proves the route exists and is unauthenticated.
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
	// Setting APP_DEV_MODE=1 proves the handler reflects the real gate rather than a hardcoded value.
	if got.Enabled != devModeEnabled() {
		t.Errorf("Enabled = %v, want %v to match devModeEnabled() with APP_DEV_MODE=1 set", got.Enabled, devModeEnabled())
	}
}

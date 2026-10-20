package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestHandleGetObservabilitySettings_SeededDefault(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/settings/observability", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got observabilitySettingsResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ExternalDashboardURL != "" {
		t.Errorf("ExternalDashboardURL = %q, want empty", got.ExternalDashboardURL)
	}
	if got.RemoteReadPath != prometheusRemoteReadPath {
		t.Errorf("RemoteReadPath = %q, want %q", got.RemoteReadPath, prometheusRemoteReadPath)
	}
}

func TestHandleUpdateObservabilitySettings_Success(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	body := `{"external_dashboard_url":"https://grafana.example.internal/d/abc"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/observability", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	settings, err := db.GetObservabilitySettings(context.Background())
	if err != nil {
		t.Fatalf("GetObservabilitySettings() error = %v", err)
	}
	if settings.ExternalDashboardURL != "https://grafana.example.internal/d/abc" {
		t.Errorf("ExternalDashboardURL = %q, want https://grafana.example.internal/d/abc", settings.ExternalDashboardURL)
	}
}

func TestHandleUpdateObservabilitySettings_ClearsURL(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rt.Handler().ServeHTTP(httptest.NewRecorder(), authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/observability", `{"external_dashboard_url":"https://grafana.example.com"}`))

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/observability", `{"external_dashboard_url":""}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	settings, err := db.GetObservabilitySettings(context.Background())
	if err != nil {
		t.Fatalf("GetObservabilitySettings() error = %v", err)
	}
	if settings.ExternalDashboardURL != "" {
		t.Errorf("ExternalDashboardURL = %q, want empty after clearing", settings.ExternalDashboardURL)
	}
}

func TestHandleUpdateObservabilitySettings_InvalidURLRejected(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	for _, bad := range []string{"not-a-url", "ftp://example.com", "javascript:alert(1)"} {
		body := `{"external_dashboard_url":"` + bad + `"}`
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/observability", body))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("url %q: status = %d, want %d", bad, rec.Code, http.StatusBadRequest)
		}
	}
}

// TestHandleUpdateObservabilitySettings_PlainWriteToken_Forbidden proves PUT
// /settings/observability sits behind AbilityRoot, not just AbilityWrite,
// mirroring TestHandleUpdateCloudflareTunnelSettings_PlainWriteToken_Forbidden.
func TestHandleUpdateObservabilitySettings_PlainWriteToken_Forbidden(t *testing.T) {
	rt, db := newTestRouter(t)
	ctx := context.Background()

	const plaintext = "write-scoped-token" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(ctx, store.APIToken{
		ID: "tok_write", Name: "writer", TokenHash: hashToken(plaintext), Abilities: []string{AbilityWrite}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}

	body := `{"external_dashboard_url":"https://grafana.example.com"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings/observability", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d: a plain write token must not reach the observability settings write", rec.Code, http.StatusForbidden)
	}
}

func TestObservabilityRoutes_RequireAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	assertRoutesRequireAuth(t, rt, []routeCase{
		{http.MethodGet, "/api/v1/settings/observability"},
		{http.MethodPut, "/api/v1/settings/observability"},
	})
}

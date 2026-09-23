package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestValidateAppResource_Health(t *testing.T) {
	base := appResource{Name: "web", Image: "nginx", Port: 80}
	tests := []struct {
		name    string
		health  *store.ServiceHealth
		wantErr string
	}{
		{name: "none"},
		{name: "legacy http", health: &store.ServiceHealth{Readiness: &store.ServiceProbe{Path: "/healthz"}}},
		{name: "https skip verify", health: &store.ServiceHealth{Readiness: &store.ServiceProbe{Path: "/", Scheme: "https", TLSSkipVerify: true, ExpectedStatus: "200-399"}}},
		{name: "exec liveness", health: &store.ServiceHealth{Liveness: &store.ServiceProbe{Exec: []string{"/bin/sh", "-c", "redis-cli ping"}}}},
		{name: "empty probe", health: &store.ServiceHealth{Readiness: &store.ServiceProbe{}}, wantErr: "health.readiness: path (for an HTTP probe) or exec"},
		{name: "bad status", health: &store.ServiceHealth{Liveness: &store.ServiceProbe{Path: "/", ExpectedStatus: "ok"}}, wantErr: `health.liveness: expected_status "ok"`},
		{name: "skip verify on http", health: &store.ServiceHealth{Readiness: &store.ServiceProbe{Path: "/", TLSSkipVerify: true}}, wantErr: "tls_skip_verify only applies to scheme: https"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := base
			app.Health = tt.health
			err := validateAppResource(app)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validateAppResource() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("validateAppResource() error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestAppHealthRoutes_RequireAuth(t *testing.T) {
	rt, _ := newTestRouter(t)
	assertRoutesRequireAuth(t, rt, []routeCase{
		{http.MethodGet, "/api/v1/apps/web/health"},
		{http.MethodPut, "/api/v1/apps/web/health"},
		{http.MethodDelete, "/api/v1/apps/web/health"},
	})
}

func TestHandleSetAppHealth(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantErr    string
		check      func(t *testing.T, h *store.ServiceHealth)
	}{
		{
			name:       "https readiness and exec liveness",
			body:       `{"readiness":{"path":"/health","scheme":"https","tls_skip_verify":true,"follow_redirects":false,"expected_status":"200-399"},"liveness":{"path":"","exec":["pg_isready"],"timeout":5000000000}}`,
			wantStatus: http.StatusOK,
			check: func(t *testing.T, h *store.ServiceHealth) {
				if h == nil || h.Readiness.Scheme != "https" || !h.Readiness.TLSSkipVerify || h.Readiness.FollowRedirects == nil || *h.Readiness.FollowRedirects || h.Readiness.ExpectedStatus != "200-399" {
					t.Errorf("readiness = %+v", h.Readiness)
				}
				if h == nil || len(h.Liveness.Exec) != 1 || h.Liveness.Exec[0] != "pg_isready" {
					t.Errorf("liveness = %+v", h.Liveness)
				}
			},
		},
		{name: "invalid probe rejected", body: `{"readiness":{"path":"/","expected_status":"2xx"}}`, wantStatus: http.StatusBadRequest, wantErr: "health.readiness: expected_status"},
		{name: "empty body clears", body: `{}`, wantStatus: http.StatusOK, check: func(t *testing.T, h *store.ServiceHealth) {
			if h != nil {
				t.Errorf("health = %+v, want cleared", h)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt, db := newTestRouter(t)
			cookie := loginTestSession(t, rt, db)
			seedWebAppForTest(t, db)

			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/health", tt.body))
			if rec.Code != tt.wantStatus || !strings.Contains(rec.Body.String(), tt.wantErr) {
				t.Fatalf("status = %d body = %s, want %d containing %q", rec.Code, rec.Body.String(), tt.wantStatus, tt.wantErr)
			}
			if tt.check == nil {
				return
			}
			saved, err := db.GetDesiredService(context.Background(), "web")
			if err != nil {
				t.Fatalf("GetDesiredService() error = %v", err)
			}
			tt.check(t, saved.Health)

			rec = httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/health", ""))
			var got appHealthResource
			if err := json.NewDecoder(rec.Body).Decode(&got); err != nil || got.Name != "web" {
				t.Fatalf("GET response %s, decode error %v", rec.Body.String(), err)
			}
			tt.check(t, got.Health)
		})
	}
}

func TestHandleAppHealth_NotFoundAndClear(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/missing/health", `{"readiness":{"path":"/"}}`))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("PUT missing app status = %d, want 404", rec.Code)
	}

	seedWebAppForTest(t, db)
	if err := db.UpdateServiceHealth(context.Background(), "web", &store.ServiceHealth{Readiness: &store.ServiceProbe{Path: "/"}}); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/web/health", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("DELETE status = %d, body %s", rec.Code, rec.Body.String())
	}
	saved, err := db.GetDesiredService(context.Background(), "web")
	if err != nil || saved.Health != nil {
		t.Errorf("health after DELETE = %+v (err %v), want nil", saved.Health, err)
	}
}

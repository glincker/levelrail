package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestAppLogDrainRoutes_RequireAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	routes := []struct {
		method string
		target string
	}{
		{http.MethodGet, "/api/v1/apps/web/log-drain"},
		{http.MethodPut, "/api/v1/apps/web/log-drain"},
		{http.MethodDelete, "/api/v1/apps/web/log-drain"},
	}
	for _, r := range routes {
		t.Run(r.method+" "+r.target, func(t *testing.T) {
			req := httptest.NewRequest(r.method, r.target, nil)
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
			}
		})
	}
}

func TestHandleSetAppLogDrain(t *testing.T) {
	tests := []struct {
		name       string
		seedApp    bool
		body       string
		wantStatus int
	}{
		{
			name:       "app not found",
			seedApp:    false,
			body:       `{"type":"http","target":"https://collector.example.com","enabled":true}`,
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "invalid type",
			seedApp:    true,
			body:       `{"type":"carrier-pigeon","target":"https://collector.example.com","enabled":true}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "http drain requires target",
			seedApp:    true,
			body:       `{"type":"http","target":"","enabled":true}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid body",
			seedApp:    true,
			body:       `not json`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "http drain succeeds",
			seedApp:    true,
			body:       `{"type":"http","target":"https://collector.example.com/ingest","enabled":true}`,
			wantStatus: http.StatusOK,
		},
		{
			name:       "syslog drain succeeds with empty target",
			seedApp:    true,
			body:       `{"type":"syslog","target":"","enabled":true}`,
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt, db := newTestRouter(t)
			cookie := loginTestSession(t, rt, db)
			if tt.seedApp {
				if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "img:v1", Port: 8080}); err != nil {
					t.Fatalf("seed: %v", err)
				}
			}

			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/log-drain", tt.body))
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d, body = %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestHandleSetAppLogDrain_PersistsAndRoundTrips(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "img:v1", Port: 8080}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/log-drain", `{"type":"http","target":"https://collector.example.com/ingest","enabled":true}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var got logDrainResource
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode PUT response: %v", err)
	}
	if got.AppName != "web" || got.Type != store.LogDrainHTTP || got.Target != "https://collector.example.com/ingest" || !got.Enabled {
		t.Errorf("PUT response = %+v", got)
	}

	saved, err := db.GetDesiredService(context.Background(), "web")
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}
	if saved.LogDrain == nil || saved.LogDrain.Type != store.LogDrainHTTP || saved.LogDrain.Target != "https://collector.example.com/ingest" {
		t.Fatalf("persisted LogDrain = %+v", saved.LogDrain)
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/log-drain", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var gotGet logDrainResource
	if err := json.NewDecoder(rec.Body).Decode(&gotGet); err != nil {
		t.Fatalf("decode GET response: %v", err)
	}
	if gotGet != got {
		t.Errorf("GET response = %+v, want %+v", gotGet, got)
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET app status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var app appResource
	if err := json.NewDecoder(rec.Body).Decode(&app); err != nil {
		t.Fatalf("decode app response: %v", err)
	}
	if app.LogDrain == nil || app.LogDrain.Target != "https://collector.example.com/ingest" {
		t.Errorf("app.LogDrain = %+v, want it surfaced on the general app resource too", app.LogDrain)
	}
}

func TestHandleGetAppLogDrain_NoneConfigured(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "img:v1", Port: 8080}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/log-drain", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestHandleGetAppLogDrain_AppNotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/missing/log-drain", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleClearAppLogDrain(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "img:v1", Port: 8080}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/log-drain", `{"type":"http","target":"https://collector.example.com","enabled":true}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, body = %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/web/log-drain", ""))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE status = %d, want %d, body = %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}

	saved, err := db.GetDesiredService(context.Background(), "web")
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}
	if saved.LogDrain != nil {
		t.Errorf("LogDrain = %+v, want nil after clear", saved.LogDrain)
	}
}

func TestHandleClearAppLogDrain_AppNotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/missing/log-drain", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

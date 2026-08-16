package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/secrets"
)

const validComposeYAML = `
services:
  web:
    image: nginx:1.27
    ports: ["8080:80"]
  redis:
    image: redis:7
`

func TestHandleDeployCompose_RequiresAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/myapp/compose", nil)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandleDeployCompose_CreatesAppAndServices(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/myapp/compose", validComposeYAML))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got composeDeployResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.AppID != "myapp" {
		t.Errorf("AppID = %q, want myapp", got.AppID)
	}
	if len(got.Services) != 2 {
		t.Fatalf("got %d services, want 2", len(got.Services))
	}

	app, err := db.GetAppByName(context.Background(), "myapp")
	if err != nil {
		t.Fatalf("GetAppByName() error = %v", err)
	}
	services, err := db.ListServicesByApp(context.Background(), app.ID)
	if err != nil {
		t.Fatalf("ListServicesByApp() error = %v", err)
	}
	if len(services) != 2 {
		t.Fatalf("ListServicesByApp() returned %d services, want 2", len(services))
	}
}

func TestHandleDeployCompose_InvalidYAML(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/myapp/compose", "not: [valid"))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleDeployCompose_RejectsBuild(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/myapp/compose", "services:\n  web:\n    build: .\n"))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleDeployCompose_Redeploy_ReusesSameApp(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/myapp/compose", validComposeYAML))
		if rec.Code != http.StatusOK {
			t.Fatalf("attempt %d: status = %d, body = %s", i, rec.Code, rec.Body.String())
		}
	}

	apps, err := db.ListApps(context.Background())
	if err != nil {
		t.Fatalf("ListApps() error = %v", err)
	}
	if len(apps) != 1 {
		t.Fatalf("got %d apps after redeploy, want 1 (must reuse, not duplicate)", len(apps))
	}
}

const magicVarComposeYAML = `
services:
  app:
    image: myapp:latest
    environment:
      DB_PASSWORD: $SERVICE_PASSWORD_DB
  db:
    image: postgres:16
    environment:
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
`

func TestHandleDeployCompose_MagicVar_NoSecretsManagerConfigured(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/myapp/compose", magicVarComposeYAML))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleDeployCompose_MagicVar_GeneratedAndSharedAcrossServices(t *testing.T) {
	db := openTestDB(t)
	mk, err := secrets.GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() error = %v", err)
	}
	secretsManager := secrets.NewManager(db, mk)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	rt := NewRouter(logger, testBrand(), db, WithComposeSecrets(secretsManager))
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/myapp/compose", magicVarComposeYAML))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got composeDeployResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}

	appVal, err := secretsManager.Resolve(context.Background(), "myapp-app", "DB_PASSWORD")
	if err != nil {
		t.Fatalf("resolve myapp-app/DB_PASSWORD: %v", err)
	}
	dbVal, err := secretsManager.Resolve(context.Background(), "myapp-db", "POSTGRES_PASSWORD")
	if err != nil {
		t.Fatalf("resolve myapp-db/POSTGRES_PASSWORD: %v", err)
	}
	if appVal != dbVal {
		t.Errorf("app's DB_PASSWORD (%q) != db's POSTGRES_PASSWORD (%q), want the same shared generated value", appVal, dbVal)
	}
	if appVal == "" {
		t.Error("generated value is empty")
	}

	for _, svc := range got.Services {
		if _, ok := svc.Env["DB_PASSWORD"]; ok {
			t.Errorf("service %q: DB_PASSWORD leaked into literal Env, want it secret-only", svc.Name)
		}
	}
}

func TestHandleDeployCompose_MagicVar_RedeployReusesSameGeneratedValue(t *testing.T) {
	db := openTestDB(t)
	mk, err := secrets.GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() error = %v", err)
	}
	secretsManager := secrets.NewManager(db, mk)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	rt := NewRouter(logger, testBrand(), db, WithComposeSecrets(secretsManager))
	cookie := loginTestSession(t, rt, db)

	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/myapp/compose", magicVarComposeYAML))
		if rec.Code != http.StatusOK {
			t.Fatalf("attempt %d: status = %d, body = %s", i, rec.Code, rec.Body.String())
		}
	}

	val1, err := secretsManager.Resolve(context.Background(), "myapp-app", "DB_PASSWORD")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/myapp/compose", magicVarComposeYAML))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	val2, err := secretsManager.Resolve(context.Background(), "myapp-app", "DB_PASSWORD")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if val1 != val2 {
		t.Errorf("redeploy regenerated the secret (%q != %q), want the same value reused across redeploys", val1, val2)
	}
}

func TestHandleDeployCompose_UnresolvedVar(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	yaml := "services:\n  app:\n    image: myapp:latest\n    environment:\n      API_KEY: ${SERVICE_OPENAI_API_KEY}\n"
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/myapp/compose", yaml))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

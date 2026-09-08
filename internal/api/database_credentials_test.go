package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/reconcile/database"
	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

func newTestRouterWithComposeSecrets(t *testing.T) (*Router, *store.DB, *secrets.Manager) {
	t.Helper()
	db := openTestDB(t)
	mk, err := secrets.GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() error = %v", err)
	}
	mgr := secrets.NewManager(db, mk)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	rt := NewRouter(logger, testBrand(), db, WithComposeSecrets(mgr))
	return rt, db, mgr
}

func TestHandleGetDatabaseCredentials_Redis_NoPassword(t *testing.T) {
	rt, db, _ := newTestRouterWithComposeSecrets(t)
	cookie := loginTestSession(t, rt, db)

	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{
		Name: "cache", Engine: store.EngineRedis, Version: "7",
	}); err != nil {
		t.Fatalf("seed database: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/databases/cache/credentials", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got databaseCredentialsResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Host != "db-cache" {
		t.Errorf("Host = %q, want db-cache", got.Host)
	}
	if got.Port != 6379 {
		t.Errorf("Port = %d, want 6379", got.Port)
	}
	if got.Password != "" {
		t.Errorf("Password = %q, want empty: Redis is passwordless in this platform", got.Password)
	}
	if got.Username != "" || got.Database != "" {
		t.Errorf("Username/Database = %q/%q, want both empty for Redis", got.Username, got.Database)
	}
	if got.URL != "redis://db-cache:6379" {
		t.Errorf("URL = %q, want redis://db-cache:6379", got.URL)
	}
}

func TestHandleGetDatabaseCredentials_Postgres_WithPassword(t *testing.T) {
	rt, db, mgr := newTestRouterWithComposeSecrets(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{
		Name: "main", Engine: store.EnginePostgres, Version: "16",
	}); err != nil {
		t.Fatalf("seed database: %v", err)
	}
	secretKey, ok := database.PasswordSecretKey(store.EnginePostgres)
	if !ok {
		t.Fatal("PasswordSecretKey(postgres) reported not ok")
	}
	if err := mgr.SetValue(ctx, "main", secretKey, "s3cr3t"); err != nil {
		t.Fatalf("SetValue() error = %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/databases/main/credentials", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got databaseCredentialsResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Host != "db-main" || got.Port != 5432 {
		t.Errorf("Host/Port = %q/%d, want db-main/5432", got.Host, got.Port)
	}
	if got.Username != "main" || got.Database != "main" {
		t.Errorf("Username/Database = %q/%q, want both main", got.Username, got.Database)
	}
	if got.Password != "s3cr3t" {
		t.Errorf("Password = %q, want s3cr3t", got.Password)
	}
	if got.URL != "postgres://main:s3cr3t@db-main:5432/main" { //nolint:gosec // fake fixture URL for a test-only in-memory secret, not a real credential
		t.Errorf("URL = %q, want postgres://main:s3cr3t@db-main:5432/main", got.URL)
	}
}

func TestHandleGetDatabaseCredentials_NoSecretsManagerConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithComposeSecrets
	cookie := loginTestSession(t, rt, db)

	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{
		Name: "main", Engine: store.EnginePostgres, Version: "16",
	}); err != nil {
		t.Fatalf("seed database: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/databases/main/credentials", ""))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d, body = %s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
}

func TestHandleGetDatabaseCredentials_UnknownDatabase_NotFound(t *testing.T) {
	rt, db, _ := newTestRouterWithComposeSecrets(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/databases/nonexistent/credentials", ""))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// TestHandleGetDatabaseCredentials_PlainReadToken_Forbidden proves this
// route sits behind AbilityReadSensitive, not plain AbilityRead: it
// discloses a real secret's plaintext.
func TestHandleGetDatabaseCredentials_PlainReadToken_Forbidden(t *testing.T) {
	rt, db, _ := newTestRouterWithComposeSecrets(t)
	ctx := context.Background()

	if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{
		Name: "main", Engine: store.EnginePostgres, Version: "16",
	}); err != nil {
		t.Fatalf("seed database: %v", err)
	}
	const plaintext = "read-scoped-token-db-creds" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(ctx, store.APIToken{
		ID: "tok_read_db_creds", Name: "reader", TokenHash: hashToken(plaintext), Abilities: []string{AbilityRead}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/databases/main/credentials", nil)
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d: a plain read token must not be able to read a database's credentials", rec.Code, http.StatusForbidden)
	}
}

func TestDatabaseCredentialsRoute_RequiresAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/databases/main/credentials", nil)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

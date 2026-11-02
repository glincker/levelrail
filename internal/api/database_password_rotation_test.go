package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestDatabasePasswordRotationNotConfigured(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/databases/main/password-rotation", ""))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("GET = %d, want 501 when rotation is not wired", rec.Code)
	}
}

func TestDatabasePasswordRotationPreviewAndPolicy(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveDesiredService(ctx, store.DesiredService{
		Name: "web", Image: "img:v1", Port: 3000,
		DatabaseEnv: map[string]store.DatabaseEnvRef{"DATABASE_URL": {Database: "main", Field: "url"}},
	}); err != nil {
		t.Fatal(err)
	}
	rt.dbRotationStore = db
	rt.secrets = &fakeSecretSetter{}
	rt.execRuntime = func(string) (docker.Runtime, error) { return nil, nil }

	do := func(method, path, body string) (*httptest.ResponseRecorder, databasePasswordRotationResource) {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, body))
		var res databasePasswordRotationResource
		_ = json.Unmarshal(rec.Body.Bytes(), &res)
		return rec, res
	}

	rec, res := do(http.MethodPost, "/api/v1/databases/main/password-rotation", `{"dry_run":true}`)
	if rec.Code != http.StatusOK || !res.Supported || !res.RestartRequired || len(res.Apps) != 1 || res.Apps[0] != "web" {
		t.Fatalf("dry run = %d %+v", rec.Code, res)
	}
	if strings.Contains(rec.Body.String(), "password\":") {
		t.Errorf("preview body unexpectedly carries credential fields: %s", rec.Body.String())
	}

	rec, _ = do(http.MethodPost, "/api/v1/databases/main/password-rotation", `{}`)
	if rec.Code == http.StatusOK {
		t.Fatalf("a real rotation without step-up returned 200")
	}

	rec, res = do(http.MethodPut, "/api/v1/databases/main/password-rotation/policy", `{"interval_days":30}`)
	if rec.Code != http.StatusOK || res.IntervalDays != 30 || res.NextDue == "" {
		t.Fatalf("policy = %d %+v", rec.Code, res)
	}
	if rec, _ = do(http.MethodPut, "/api/v1/databases/main/password-rotation/policy", `{"interval_days":99999}`); rec.Code != http.StatusBadRequest {
		t.Errorf("out of range policy = %d, want 400", rec.Code)
	}

	if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: "cache", Engine: store.EngineRedis, Version: "7"}); err != nil {
		t.Fatal(err)
	}
	rec, res = do(http.MethodGet, "/api/v1/databases/cache/password-rotation", "")
	if rec.Code != http.StatusOK || res.Supported || res.Reason == "" {
		t.Errorf("redis GET = %d %+v, want unsupported with a reason", rec.Code, res)
	}
}

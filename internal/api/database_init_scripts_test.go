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

func seedPostgresDatabase(t *testing.T, db *store.DB, name string) {
	t.Helper()
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: name, Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("SaveDesiredDatabase(%q) error = %v", name, err)
	}
}

func TestHandleListDatabaseInitScripts_Empty(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedPostgresDatabase(t, db, "mydb")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/databases/mydb/init-scripts", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got []databaseInitScriptResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got = %+v, want empty", got)
	}
}

func TestHandleListDatabaseInitScripts_DatabaseNotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/databases/missing/init-scripts", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleCreateDatabaseInitScript_Success(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedPostgresDatabase(t, db, "mydb")

	rec := httptest.NewRecorder()
	body := `{"filename":"01-extensions.sql","content":"CREATE EXTENSION IF NOT EXISTS vector;"}`
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/mydb/init-scripts", body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var got databaseInitScriptResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Filename != "01-extensions.sql" || got.Content != "CREATE EXTENSION IF NOT EXISTS vector;" || got.DatabaseName != "mydb" {
		t.Errorf("got = %+v, want filename/content/database_name populated", got)
	}
	if got.ID == "" {
		t.Error("ID is empty, want a minted id")
	}

	scripts, err := db.ListDatabaseInitScripts(context.Background(), "mydb")
	if err != nil {
		t.Fatalf("ListDatabaseInitScripts() error = %v", err)
	}
	if len(scripts) != 1 {
		t.Fatalf("stored scripts = %d, want 1", len(scripts))
	}
}

func TestHandleCreateDatabaseInitScript_RejectsPathTraversal(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedPostgresDatabase(t, db, "mydb")

	for _, filename := range []string{"../escape.sql", "sub/dir.sql", "..sql", "/etc/passwd"} {
		t.Run(filename, func(t *testing.T) {
			rec := httptest.NewRecorder()
			body := `{"filename":"` + filename + `","content":"x"}`
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/mydb/init-scripts", body))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
			}
		})
	}
}

func TestHandleCreateDatabaseInitScript_RejectsWrongExtension(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedPostgresDatabase(t, db, "mydb")

	rec := httptest.NewRecorder()
	body := `{"filename":"init.txt","content":"x"}`
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/mydb/init-scripts", body))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), ".sql") {
		t.Errorf("body = %q, want it to mention the accepted extensions", rec.Body.String())
	}
}

func TestHandleCreateDatabaseInitScript_RejectsUnsupportedEngine(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "cache", Engine: store.EngineRedis, Version: "7"}); err != nil {
		t.Fatalf("SaveDesiredDatabase() error = %v", err)
	}

	rec := httptest.NewRecorder()
	body := `{"filename":"01-init.sh","content":"echo hi"}`
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/cache/init-scripts", body))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "does not support init scripts") {
		t.Errorf("body = %q, want a clear unsupported-engine message", rec.Body.String())
	}
}

func TestHandleCreateDatabaseInitScript_MissingContent(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedPostgresDatabase(t, db, "mydb")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/mydb/init-scripts", `{"filename":"01-init.sql"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleCreateDatabaseInitScript_DuplicateFilenameConflicts(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedPostgresDatabase(t, db, "mydb")

	body := `{"filename":"01-init.sql","content":"x"}`
	first := httptest.NewRecorder()
	rt.Handler().ServeHTTP(first, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/mydb/init-scripts", body))
	if first.Code != http.StatusCreated {
		t.Fatalf("first create status = %d, want %d, body = %s", first.Code, http.StatusCreated, first.Body.String())
	}

	second := httptest.NewRecorder()
	rt.Handler().ServeHTTP(second, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/mydb/init-scripts", body))
	if second.Code != http.StatusConflict {
		t.Fatalf("second create status = %d, want %d, body = %s", second.Code, http.StatusConflict, second.Body.String())
	}
}

func TestHandleCreateDatabaseInitScript_PlainWriteToken_Forbidden(t *testing.T) {
	rt, db := newTestRouter(t)
	seedPostgresDatabase(t, db, "mydb")
	ctx := context.Background()

	const plaintext = "write-scoped-token" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(ctx, store.APIToken{
		ID: "tok_write", Name: "writer", TokenHash: hashToken(plaintext), Abilities: []string{AbilityWrite}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/databases/mydb/init-scripts", strings.NewReader(`{"filename":"01-init.sql","content":"x"}`))
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d: a plain write token must not create code that executes inside a database container", rec.Code, http.StatusForbidden)
	}
}

func TestHandleUpdateDatabaseInitScript_Success(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedPostgresDatabase(t, db, "mydb")

	created := httptest.NewRecorder()
	rt.Handler().ServeHTTP(created, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/mydb/init-scripts", `{"filename":"01-init.sql","content":"x"}`))
	var script databaseInitScriptResource
	if err := json.Unmarshal(created.Body.Bytes(), &script); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	rec := httptest.NewRecorder()
	body := `{"filename":"01-init-v2.sql","content":"CREATE EXTENSION IF NOT EXISTS vector;"}`
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/databases/mydb/init-scripts/"+script.ID, body))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var updated databaseInitScriptResource
	if err := json.Unmarshal(rec.Body.Bytes(), &updated); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if updated.Filename != "01-init-v2.sql" || updated.Content != "CREATE EXTENSION IF NOT EXISTS vector;" {
		t.Errorf("updated = %+v, want the new filename/content", updated)
	}
}

func TestHandleUpdateDatabaseInitScript_WrongDatabase404s(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedPostgresDatabase(t, db, "mydb")
	seedPostgresDatabase(t, db, "other")

	created := httptest.NewRecorder()
	rt.Handler().ServeHTTP(created, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/mydb/init-scripts", `{"filename":"01-init.sql","content":"x"}`))
	var script databaseInitScriptResource
	if err := json.Unmarshal(created.Body.Bytes(), &script); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/databases/other/init-scripts/"+script.ID, `{"filename":"x.sql","content":"y"}`))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d: a script belonging to a different database must not be editable through this path", rec.Code, http.StatusNotFound)
	}
}

func TestHandleDeleteDatabaseInitScript_Success(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedPostgresDatabase(t, db, "mydb")

	created := httptest.NewRecorder()
	rt.Handler().ServeHTTP(created, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/mydb/init-scripts", `{"filename":"01-init.sql","content":"x"}`))
	var script databaseInitScriptResource
	if err := json.Unmarshal(created.Body.Bytes(), &script); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/databases/mydb/init-scripts/"+script.ID, ""))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}

	scripts, err := db.ListDatabaseInitScripts(context.Background(), "mydb")
	if err != nil {
		t.Fatalf("ListDatabaseInitScripts() error = %v", err)
	}
	if len(scripts) != 0 {
		t.Errorf("scripts after delete = %+v, want empty", scripts)
	}
}

func TestDatabaseInitScriptRoutes_RequireAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	routes := []struct{ method, target string }{
		{http.MethodGet, "/api/v1/databases/mydb/init-scripts"},
		{http.MethodPost, "/api/v1/databases/mydb/init-scripts"},
		{http.MethodPut, "/api/v1/databases/mydb/init-scripts/dis_1"},
		{http.MethodDelete, "/api/v1/databases/mydb/init-scripts/dis_1"},
	}
	for _, rt2 := range routes {
		t.Run(rt2.method, func(t *testing.T) {
			req := httptest.NewRequest(rt2.method, rt2.target, nil)
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
			}
		})
	}
}

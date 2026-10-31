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

func seedMemcached(t *testing.T, db *store.DB) {
	t.Helper()
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "cache", Engine: store.EngineMemcached, Version: "1.6"}); err != nil {
		t.Fatalf("seed memcached: %v", err)
	}
}

func TestMemcached_BackupsAreRefusedNotFaked(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedMemcached(t, db)

	rec := httptest.NewRecorder()
	body := `{"target_id":"bkt_1","schedule":"0 3 * * *","retain":7}`
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/databases/cache/backup-schedule", body))
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "not available") {
		t.Fatalf("set schedule = %d %s, want 400 not available", rec.Code, rec.Body.String())
	}
}

func TestMemcached_PublicBindIsRefused(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedMemcached(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/databases/cache/public-access", `{"bind_address":"public"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("public bind = %d %s, want 400", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/databases/cache/public-access", `{}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("loopback bind = %d %s, want 200", rec.Code, rec.Body.String())
	}
}

func TestListDatabaseEngines_ReportsBackupSupport(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/database-engines", ""))
	var got []databaseEngineResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, e := range got {
		seen[e.ID] = e.BackupsSupported
	}
	if seen["memcached"] || !seen["meilisearch"] || !seen["postgres"] {
		t.Fatalf("backups_supported = %v", seen)
	}
}

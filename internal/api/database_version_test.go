package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestCheckVersionChange(t *testing.T) {
	tests := []struct {
		name         string
		engine       string
		from, to     string
		wantErrMatch string
	}{
		{"postgres minor", store.EnginePostgres, "16", "16.4", ""},
		{"postgres same", store.EnginePostgres, "16.4", "16.4", ""},
		{"postgres major refused", store.EnginePostgres, "16", "17", "major version"},
		{"mysql major refused", store.EngineMySQL, "8.0", "9.0", "major version"},
		{"mariadb patch", store.EngineMariaDB, "11.4", "11.4.2", ""},
		{"redis upgrade allowed", store.EngineRedis, "7", "8", ""},
		{"redis downgrade refused", store.EngineRedis, "8", "7", "downgrading"},
		{"latest cannot be compared", store.EnginePostgres, "latest", "16", "cannot compare"},
		{"bad tag", store.EnginePostgres, "16", "16 4;rm", "image tag"},
		{"empty", store.EnginePostgres, "16", "", "image tag"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkVersionChange(tt.engine, tt.from, tt.to)
			if tt.wantErrMatch == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErrMatch) {
				t.Fatalf("error = %v, want it to contain %q", err, tt.wantErrMatch)
			}
		})
	}
}

func TestHandleSetDatabaseVersion(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	do := func(path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, path, body))
		return rec
	}

	if rec := do("/api/v1/databases/main/version", `{"version":"17"}`); rec.Code != http.StatusConflict {
		t.Fatalf("major change status = %d, want 409, body = %s", rec.Code, rec.Body.String())
	}
	if rec := do("/api/v1/databases/ghost/version", `{"version":"16.4"}`); rec.Code != http.StatusNotFound {
		t.Fatalf("missing database status = %d, want 404", rec.Code)
	}
	if rec := do("/api/v1/databases/main/version", `{"version":"16.4"}`); rec.Code != http.StatusOK {
		t.Fatalf("minor change status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	got, err := db.GetDesiredDatabase(context.Background(), "main")
	if err != nil || got.Version != "16.4" {
		t.Fatalf("stored version = %v (err %v), want 16.4", got, err)
	}
}

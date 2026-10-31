package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestDisablePoolerFlagsLinkedApps(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: "pdb", Engine: store.EnginePostgres, Version: "17"}); err != nil {
		t.Fatalf("seed db: %v", err)
	}
	linked := store.DesiredService{Name: "linked", Image: "levelrail/web:1", Port: 3000, DatabaseEnv: map[string]store.DatabaseEnvRef{"DATABASE_URL": {Database: "pdb", Field: "url"}}}
	other := store.DesiredService{Name: "other", Image: "levelrail/web:1", Port: 3000}
	for _, s := range []store.DesiredService{linked, other} {
		if err := db.SaveDesiredService(ctx, s); err != nil {
			t.Fatalf("seed app: %v", err)
		}
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/databases/pdb/pooler", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	got, _ := db.GetDesiredService(ctx, "linked")
	if !got.EnvDirty {
		t.Error("linked app not flagged env_dirty after pooler disable")
	}
	got, _ = db.GetDesiredService(ctx, "other")
	if got.EnvDirty {
		t.Error("unlinked app flagged")
	}
}

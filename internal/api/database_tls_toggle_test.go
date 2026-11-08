package api

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestDatabaseTLSToggle_AcceptsValkeyRejectsOthers(t *testing.T) {
	rt, db, _ := newAccessTestRouter(t)
	ctx := context.Background()
	for _, d := range []store.DesiredDatabase{
		{Name: "cache", Engine: store.EngineValkey, Version: "8"},
		{Name: "kv", Engine: store.EngineKeyDB, Version: "latest"},
	} {
		if err := db.SaveDesiredDatabase(ctx, d); err != nil {
			t.Fatalf("seed %s: %v", d.Name, err)
		}
	}
	cookie := loginTestSession(t, rt, db)

	rec := viewerDo(t, rt, cookie, http.MethodPut, "/api/v1/databases/cache/tls", `{"enabled":true}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"mode":"on"`) {
		t.Fatalf("valkey toggle = %d %s, want 200 mode on", rec.Code, rec.Body.String())
	}
	if mode, _ := db.GetDatabaseTLSMode(ctx, "cache"); mode != store.DatabaseTLSModeOn {
		t.Errorf("stored mode = %q, want on", mode)
	}

	rec = viewerDo(t, rt, cookie, http.MethodPut, "/api/v1/databases/kv/tls", `{"enabled":true}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("keydb toggle = %d, want 400", rec.Code)
	}
}

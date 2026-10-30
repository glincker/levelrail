package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestDeleteAppRemovesPerAppAccessRows(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	now := time.Now().UTC()

	seedApp(t, db, "site")
	if err := db.SavePolicy(ctx, store.Policy{ID: "pol_cleanup", Name: "ci-cleanup", Document: "{}", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("save policy: %v", err)
	}
	if err := db.SaveCIOIDCTrust(ctx, store.CIOIDCTrust{ID: "cit_cleanup", ServiceName: "site", Repository: "org/repo", RefPattern: "refs/heads/*", PolicyID: "pol_cleanup", CreatedAt: now}); err != nil {
		t.Fatalf("save trust: %v", err)
	}
	if err := db.CreateBuildHook(ctx, store.BuildHook{ID: "bh_cleanup", ServiceName: "site", Name: "main", TokenHash: "h", CreatedAt: now}); err != nil {
		t.Fatalf("create hook: %v", err)
	}
	if err := db.SetImageAutoUpdate(ctx, "site", true); err != nil {
		t.Fatalf("auto update: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/site", ""))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d", rec.Code)
	}
	seedApp(t, db, "site")

	if trusts, err := db.ListCIOIDCTrusts(ctx, "site"); err != nil || len(trusts) != 0 {
		t.Errorf("trusts = %v, err = %v", trusts, err)
	}
	if _, err := db.GetPolicy(ctx, "pol_cleanup"); err == nil {
		t.Error("policy still exists")
	}
	if hooks, err := db.ListBuildHooks(ctx, "site"); err != nil || len(hooks) != 0 {
		t.Errorf("hooks = %v, err = %v", hooks, err)
	}
	if au, err := db.GetImageAutoUpdate(ctx, "site"); err != nil || au.Enabled {
		t.Errorf("auto update = %+v, err = %v", au, err)
	}
}

package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploysafety"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestImageRetention(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	old := time.Now().Add(-72 * time.Hour)
	inv := &fakeImageInventory{records: []docker.ImageRecord{
		{ID: "sha256:v1", Tags: []string{"img:v1"}, SizeBytes: 100, CreatedAt: old},
		{ID: "sha256:v2", Tags: []string{"img:v2"}, SizeBytes: 90, CreatedAt: old},
		{ID: "sha256:v3", Tags: []string{"img:v3"}, SizeBytes: 80, CreatedAt: old},
		{ID: "sha256:v4", Tags: []string{"img:v4"}, SizeBytes: 70, CreatedAt: old},
		{ID: "sha256:pin", Tags: []string{"img:v5"}, SizeBytes: 60, CreatedAt: old},
		{ID: "sha256:shared", Tags: []string{"base:1"}, SizeBytes: 50, CreatedAt: old},
	}}
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	rt := NewRouter(logger, testBrand(), db, WithImageInventory(func(string) (docker.ImageInventory, error) { return inv, nil }),
		WithDeployGate(db, deploysafety.BuiltinDefaults()))
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	seedApp(t, db, "api")
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "api", Image: "api:1", Port: 3000}); err != nil {
		t.Fatal(err)
	}
	for i, tag := range []string{"img:v1", "img:v2", "img:v3", "img:v4", "img:v5", "base:1"} {
		a := store.DeployAttempt{ID: "att" + tag, ServiceName: "web", Image: tag, Status: store.DeployAttemptStatusSucceeded, StartedAt: time.Now().Add(-time.Duration(i) * time.Hour)}
		if err := db.SaveDeployAttempt(ctx, a); err != nil {
			t.Fatal(err)
		}
	}
	extra := []store.DeployAttempt{
		{ID: "attapi", ServiceName: "api", Image: "base:1", Status: store.DeployAttemptStatusFailed, StartedAt: old},
		// A redeploy of the current image must not use up the previous release's slot.
		{ID: "attdup", ServiceName: "web", Image: "img:v1", Status: store.DeployAttemptStatusSucceeded, StartedAt: time.Now().Add(-30 * time.Minute)},
		{ID: "attheld", ServiceName: "web", Image: "img:v4", Status: store.DeployAttemptStatusHeld, StartedAt: time.Now().Add(-10 * time.Minute)},
	}
	for _, a := range extra {
		if err := db.SaveDeployAttempt(ctx, a); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.SaveRollbackPin(ctx, store.RollbackPin{ServiceName: "web", DeployAttemptID: "attimg:v5", Image: "img:v5", ImageID: "sha256:pin", PinRef: "img:v5"}); err != nil {
		t.Fatal(err)
	}
	do := func(method, target, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, target, body))
		return rec
	}

	rec := do(http.MethodPost, "/api/v1/apps/web/image-retention/preview", `{"keep":2}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("preview = %d: %s", rec.Code, rec.Body)
	}
	var p imageRetentionPreview
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, c := range p.Remove {
		got[c.ID] = true
	}
	if !p.DryRun || len(inv.removed) != 0 {
		t.Fatalf("dry run removed %v", inv.removed)
	}
	if got["sha256:v1"] || got["sha256:v2"] {
		t.Errorf("current or previous release listed for removal: %v", got)
	}
	if got["sha256:pin"] {
		t.Errorf("rollback pin listed for removal")
	}
	if got["sha256:shared"] {
		t.Errorf("shared image listed for removal")
	}
	if got["sha256:v4"] {
		t.Errorf("image of a held deploy listed for removal")
	}
	if !got["sha256:v3"] || p.RemoveCount != 1 {
		t.Errorf("remove = %v count %d, want only v3", got, p.RemoveCount)
	}

	if rec = do(http.MethodPut, "/api/v1/apps/web/image-retention", `{"keep":1}`); rec.Code != http.StatusBadRequest {
		t.Errorf("keep 1 status = %d, want 400", rec.Code)
	}
	if rec = do(http.MethodPut, "/api/v1/apps/web/image-retention", `{"keep":2}`); rec.Code != http.StatusOK {
		t.Fatalf("set = %d: %s", rec.Code, rec.Body)
	}
	var res imageRetentionResource
	rec = do(http.MethodGet, "/api/v1/apps/web/image-retention", "")
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil || res.Keep != 2 || res.Source != imageKeepSourceApp {
		t.Fatalf("get = %+v err=%v", res, err)
	}
	if rec = do(http.MethodGet, "/api/v1/apps/nope/image-retention", ""); rec.Code != http.StatusNotFound {
		t.Errorf("missing app = %d", rec.Code)
	}

	// A larger explicit request count wins over the app's own smaller count.
	if rec = do(http.MethodPost, "/api/v1/images/gc", `{"dry_run":false,"keep_per_app":3}`); rec.Code != http.StatusOK || len(inv.removed) != 0 {
		t.Fatalf("gc keep 3 = %d removed %v, want nothing", rec.Code, inv.removed)
	}
	if rec = do(http.MethodPost, "/api/v1/images/gc", `{"dry_run":false}`); rec.Code != http.StatusOK {
		t.Fatalf("gc = %d", rec.Code)
	}
	removedV3 := false
	for _, id := range inv.removed {
		switch id {
		case "sha256:v3", "img:v3":
			removedV3 = true
		default:
			t.Errorf("gc removed protected image %s", id)
		}
	}
	if !removedV3 {
		t.Errorf("gc removed %v, want v3", inv.removed)
	}

	// The protected floor holds even when the count is written through the generic settings route.
	if err := db.SaveProject(ctx, store.Project{ID: "proj_1", Name: "p", CreatedAt: "2026-08-20T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveEnvironment(ctx, store.Environment{ID: "env_prod", ProjectID: "proj_1", Name: "production", Protected: true, CreatedAt: "2026-08-20T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetServiceEnvironment(ctx, "api", "env_prod"); err != nil {
		t.Fatal(err)
	}
	if rec = rsDo(t, rt, cookie, http.MethodPut, rsBase+"/"+runtimesettings.KeyAppImageKeepOverrides, `{"value":"{\"api\":2,\"web\":2}"}`); rec.Code != http.StatusOK {
		t.Fatalf("generic set = %d: %s", rec.Code, rec.Body)
	}
	overrides, err := rt.appImageKeepOverrides(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n, _, protected := rt.effectiveImageKeep(ctx, "api", overrides); n != 3 || !protected {
		t.Errorf("protected app keep = %d protected=%v, want the global 3", n, protected)
	}
	if rec = do(http.MethodPut, "/api/v1/apps/api/image-retention", `{"keep":2}`); rec.Code != http.StatusConflict {
		t.Errorf("protected PUT below global = %d, want 409", rec.Code)
	}
}

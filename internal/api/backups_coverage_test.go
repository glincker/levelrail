package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

func newCoverageRouter(t *testing.T, p *fakeBackupProtection, vols *fakeOrphanedVolumeManager) (*Router, *store.DB, *http.Cookie) {
	t.Helper()
	db := openTestDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db, WithBackupProtection(p, db), WithOrphanedVolumeManager(vols))
	return rt, db, loginTestSession(t, rt, db)
}

func seedDatabaseForCoverage(t *testing.T, db *store.DB, name string) {
	t.Helper()
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: name, Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatal(err)
	}
}

func TestBackupCoverageRoutes_RequireAuth(t *testing.T) {
	rt, _ := newTestRouter(t)
	assertRoutesRequireAuth(t, rt, []routeCase{
		{http.MethodGet, "/api/v1/backups/coverage"},
		{http.MethodPost, "/api/v1/backups/protect-all"},
		{http.MethodGet, "/api/v1/storage/orphans"},
		{http.MethodDelete, "/api/v1/storage/orphans/db-x-data"},
	})
}

func TestBackupCoverage_ListsEveryResource(t *testing.T) {
	rt, db, cookie := newCoverageRouter(t, &fakeBackupProtection{}, &fakeOrphanedVolumeManager{})
	seedDatabaseForCoverage(t, db, "pg")
	seedServiceWithVolume(t, db)

	rec := do(t, rt, cookie, http.MethodGet, "/api/v1/backups/coverage", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got backupCoverageResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Summary.Total != 2 || got.Summary.Unprotected != 2 || got.Summary.Protected != 0 {
		t.Fatalf("summary = %+v", got.Summary)
	}
	for _, r := range got.Resources {
		if r.State != "unprotected" || r.PrimaryAction != "protect" {
			t.Fatalf("row = %+v", r)
		}
	}
	if got.Defaults.Schedule == "" {
		t.Fatal("defaults missing")
	}
}

func TestProtectAll_PlanThenApply(t *testing.T) {
	rt, db, cookie := newCoverageRouter(t, &fakeBackupProtection{}, &fakeOrphanedVolumeManager{})
	seedDatabaseForCoverage(t, db, "pg")
	seedServiceWithVolume(t, db)
	target := seedBackupTargetForAPI(t, db)
	body := `{"target_id":"` + target.ID + `","schedule":"0 4 * * *","retain":3}`

	var plan protectAllResponse
	rec := do(t, rt, cookie, http.MethodPost, "/api/v1/backups/protect-all", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("plan status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &plan); err != nil {
		t.Fatal(err)
	}
	if plan.Applied || len(plan.Plan) != 2 {
		t.Fatalf("plan = %+v", plan)
	}
	d, err := db.GetDesiredDatabase(context.Background(), "pg")
	if err != nil || d.BackupSchedule != "" {
		t.Fatalf("plan mutated state: %+v, %v", d, err)
	}

	rec = do(t, rt, cookie, http.MethodPost, "/api/v1/backups/protect-all", `{"target_id":"`+target.ID+`","schedule":"0 4 * * *","retain":3,"apply":true}`)
	var applied protectAllResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &applied); err != nil {
		t.Fatal(err)
	}
	if !applied.Applied || len(applied.Done) != 2 || len(applied.Failed) != 0 {
		t.Fatalf("applied = %+v", applied)
	}
	d, err = db.GetDesiredDatabase(context.Background(), "pg")
	if err != nil || d.BackupSchedule != "0 4 * * *" || d.BackupTargetID != target.ID || d.BackupRetain != 3 {
		t.Fatalf("database not scheduled: %+v, %v", d, err)
	}
	cfg, err := db.GetServiceVolumeBackupSchedule(context.Background(), "web", "data")
	if err != nil || cfg.BackupSchedule != "0 4 * * *" || cfg.BackupTargetID != target.ID {
		t.Fatalf("volume not scheduled: %+v, %v", cfg, err)
	}
}

func TestProtectAll_OnlyProtectsTheNamedResource(t *testing.T) {
	rt, db, cookie := newCoverageRouter(t, &fakeBackupProtection{}, &fakeOrphanedVolumeManager{})
	seedDatabaseForCoverage(t, db, "pg")
	seedDatabaseForCoverage(t, db, "cache")
	target := seedBackupTargetForAPI(t, db)
	body := `{"target_id":"` + target.ID + `","apply":true,"only":[{"kind":"database","name":"cache"}]}`
	rec := do(t, rt, cookie, http.MethodPost, "/api/v1/backups/protect-all", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	cache, _ := db.GetDesiredDatabase(context.Background(), "cache")
	pg, _ := db.GetDesiredDatabase(context.Background(), "pg")
	if cache.BackupSchedule == "" || pg.BackupSchedule != "" {
		t.Fatalf("cache = %q, pg = %q", cache.BackupSchedule, pg.BackupSchedule)
	}
}

func TestBackupKeyAck_PersistsPerPublicKey(t *testing.T) {
	rt, db, cookie := newCoverageRouter(t, &fakeBackupProtection{}, &fakeOrphanedVolumeManager{})
	var cov backupCoverageResponse
	rec := do(t, rt, cookie, http.MethodGet, "/api/v1/backups/coverage", "")
	if err := json.Unmarshal(rec.Body.Bytes(), &cov); err != nil {
		t.Fatal(err)
	}
	if !cov.Encryption.Enabled || cov.Encryption.KeyStoredOffBox {
		t.Fatalf("encryption before ack = %+v", cov.Encryption)
	}
	if rec := do(t, rt, cookie, http.MethodPost, "/api/v1/backups/encryption-key/ack", ""); rec.Code != http.StatusOK {
		t.Fatalf("ack = %d, body = %s", rec.Code, rec.Body.String())
	}
	rec = do(t, rt, cookie, http.MethodGet, "/api/v1/backups/coverage", "")
	if err := json.Unmarshal(rec.Body.Bytes(), &cov); err != nil {
		t.Fatal(err)
	}
	if !cov.Encryption.KeyStoredOffBox || cov.Encryption.AcknowledgedAt == "" {
		t.Fatalf("encryption after ack = %+v", cov.Encryption)
	}
	if err := db.SetBackupKeyAck(context.Background(), "age1rotated", "test", time.Now()); err != nil {
		t.Fatal(err)
	}
	rec = do(t, rt, cookie, http.MethodGet, "/api/v1/backups/coverage", "")
	if err := json.Unmarshal(rec.Body.Bytes(), &cov); err != nil {
		t.Fatal(err)
	}
	if cov.Encryption.KeyStoredOffBox {
		t.Fatal("an acknowledgement for a different key must not count")
	}
}

func TestProtectAll_Validation(t *testing.T) {
	rt, db, cookie := newCoverageRouter(t, &fakeBackupProtection{}, &fakeOrphanedVolumeManager{})
	target := seedBackupTargetForAPI(t, db)
	for _, c := range []struct {
		name string
		body string
		want int
	}{
		{"missing target", `{}`, http.StatusBadRequest},
		{"bad cron", `{"target_id":"` + target.ID + `","schedule":"not cron"}`, http.StatusBadRequest},
		{"negative retain", `{"target_id":"` + target.ID + `","retain":-1}`, http.StatusBadRequest},
		{"unknown target", `{"target_id":"bkt_nope"}`, http.StatusNotFound},
	} {
		t.Run(c.name, func(t *testing.T) {
			if rec := do(t, rt, cookie, http.MethodPost, "/api/v1/backups/protect-all", c.body); rec.Code != c.want {
				t.Fatalf("status = %d, want %d, body = %s", rec.Code, c.want, rec.Body.String())
			}
		})
	}
}

func orphanFixtureVolumes() *fakeOrphanedVolumeManager {
	return &fakeOrphanedVolumeManager{volumes: []docker.NamedVolume{
		{Name: "db-gone-data", SizeBytes: 2 << 20, CreatedAt: "2026-09-01T00:00:00Z"},
		{Name: "db-pg-data", SizeBytes: 5 << 20, CreatedAt: "2026-09-01T00:00:00Z"},
		{Name: "app-web-data", SizeBytes: 1 << 20, CreatedAt: "2026-09-01T00:00:00Z"},
		{Name: "db-busy-data", SizeBytes: 1 << 20, CreatedAt: "2026-09-01T00:00:00Z", Mounted: true},
	}}
}

func TestStorageOrphans_ListAndReclaim(t *testing.T) {
	vols := orphanFixtureVolumes()
	rt, db, cookie := newCoverageRouter(t, &fakeBackupProtection{}, vols)
	seedDatabaseForCoverage(t, db, "pg")
	seedServiceWithVolume(t, db)
	target := seedBackupTargetForAPI(t, db)
	ctx := context.Background()
	if err := db.StartBackupHistory(ctx, store.BackupHistory{ID: "bk_gone", DatabaseName: "gone", TargetID: target.ID, ObjectKey: "k", StartedAt: "2026-09-02T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if err := db.FinishBackupHistory(ctx, "bk_gone", store.BackupStatusSucceeded, 10, "sum", "", "2026-09-02T00:01:00Z"); err != nil {
		t.Fatal(err)
	}

	rec := do(t, rt, cookie, http.MethodGet, "/api/v1/storage/orphans", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var list storageOrphansResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Orphans) != 1 || list.Orphans[0].ID != "db-gone-data" {
		t.Fatalf("orphans = %+v", list.Orphans)
	}
	o := list.Orphans[0]
	if o.BelongedTo.Kind != "database" || o.BelongedTo.Name != "gone" || o.BackupCount != 1 || o.LastBackup == nil || o.AgeDays == nil {
		t.Fatalf("orphan = %+v", o)
	}
	if list.TotalBytes != 2<<20 {
		t.Fatalf("total = %d", list.TotalBytes)
	}

	if rec := do(t, rt, cookie, http.MethodDelete, "/api/v1/storage/orphans/db-gone-data", `{"confirm":"wrong"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("wrong confirm = %d", rec.Code)
	}
	if rec := do(t, rt, cookie, http.MethodDelete, "/api/v1/storage/orphans/db-pg-data", `{"confirm":"db-pg-data"}`); rec.Code != http.StatusNotFound {
		t.Fatalf("desired volume = %d, want 404", rec.Code)
	}
	if rec := do(t, rt, cookie, http.MethodDelete, "/api/v1/storage/orphans/db-busy-data", `{"confirm":"db-busy-data"}`); rec.Code != http.StatusNotFound {
		t.Fatalf("mounted volume = %d, want 404", rec.Code)
	}
	if len(vols.removed) != 0 {
		t.Fatalf("removed too early: %v", vols.removed)
	}
	if rec := do(t, rt, cookie, http.MethodDelete, "/api/v1/storage/orphans/db-gone-data", `{"confirm":"db-gone-data"}`); rec.Code != http.StatusOK {
		t.Fatalf("reclaim = %d, body = %s", rec.Code, rec.Body.String())
	}
	if len(vols.removed) != 1 || vols.removed[0] != "db-gone-data" {
		t.Fatalf("removed = %v", vols.removed)
	}
}

func TestDeleteApp_DeleteDataNeedsTypedConfirm(t *testing.T) {
	vols := &fakeOrphanedVolumeManager{}
	rt, db, cookie := newCoverageRouter(t, &fakeBackupProtection{}, vols)
	seedServiceWithVolume(t, db)

	if rec := do(t, rt, cookie, http.MethodDelete, "/api/v1/apps/web?delete_data=true", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("no confirm = %d, body = %s", rec.Code, rec.Body.String())
	}
	if _, err := db.GetDesiredService(context.Background(), "web"); err != nil {
		t.Fatalf("app deleted without confirmation: %v", err)
	}
	if rec := do(t, rt, cookie, http.MethodDelete, "/api/v1/apps/web?delete_data=true&confirm=web", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("confirmed = %d, body = %s", rec.Code, rec.Body.String())
	}
	if len(vols.removed) != 1 || vols.removed[0] != "app-web-data" {
		t.Fatalf("removed = %v", vols.removed)
	}
}

func TestDeleteApp_WithoutDeleteDataKeepsVolumes(t *testing.T) {
	vols := &fakeOrphanedVolumeManager{}
	rt, db, cookie := newCoverageRouter(t, &fakeBackupProtection{}, vols)
	seedServiceWithVolume(t, db)
	if rec := do(t, rt, cookie, http.MethodDelete, "/api/v1/apps/web", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete = %d, body = %s", rec.Code, rec.Body.String())
	}
	if len(vols.removed) != 0 {
		t.Fatalf("volumes removed without delete_data: %v", vols.removed)
	}
}

func TestOrphanAttentionThreshold(t *testing.T) {
	t.Setenv(envOrphanAttentionBytes, "")
	if got := orphanAttentionThreshold(); got != defaultOrphanAttentionBytes {
		t.Fatalf("default = %d", got)
	}
	t.Setenv(envOrphanAttentionBytes, "2048")
	if got := orphanAttentionThreshold(); got != 2048 {
		t.Fatalf("override = %d", got)
	}
}

func TestAttentionFeed_BackupCoverageRecommendations(t *testing.T) {
	t.Setenv(envOrphanAttentionBytes, "1024")
	rt, db, cookie := newCoverageRouter(t, &fakeBackupProtection{}, orphanFixtureVolumes())
	seedDatabaseForCoverage(t, db, "pg")

	rec := do(t, rt, cookie, http.MethodGet, "/api/v1/attention/feed", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var feed attentionFeedResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &feed); err != nil {
		t.Fatal(err)
	}
	kinds := map[string]string{}
	for _, it := range feed.Items {
		kinds[it.Kind] = it.Class
	}
	if kinds[kindBackupUnprotected] != "recommendation" || kinds[kindOrphanedData] != "recommendation" {
		t.Fatalf("kinds = %v", kinds)
	}
}

func TestClassifyOrphanVolume(t *testing.T) {
	apps := map[string]orphanOwner{"app-my-app-uploads": {Kind: "volume", AppName: "my-app", Name: "uploads", Role: "volume"}}
	for _, c := range []struct {
		name, kind, owner, app, role string
	}{
		{"db-orders-data", "database", "orders", "", "data"},
		{"db-orders-wal-archive", "database", "orders", "", "wal archive"},
		{"db-orders-certs", "database", "orders", "", "certificates"},
		{"app-my-app-uploads", "volume", "uploads", "my-app", "volume"},
		{"app-unknown-vol", "app", "unknown-vol", "", "volume"},
	} {
		got := classifyOrphanVolume(c.name, apps)
		if got.Kind != c.kind || got.Name != c.owner || got.AppName != c.app || got.Role != c.role {
			t.Errorf("%s = %+v", c.name, got)
		}
	}
}

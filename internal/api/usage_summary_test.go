package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestHandleUsageSummary(t *testing.T) {
	db := openTestDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db, WithOrphanedVolumeManager(&fakeOrphanedVolumeManager{
		volumes: []docker.NamedVolume{
			{Name: "db-main-data", SizeBytes: 4096},
			{Name: "app-web-data", SizeBytes: -1},
		},
	}))
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("save database: %v", err)
	}
	if err := db.SaveDesiredService(ctx, store.DesiredService{
		Name: "web", Image: "nginx", Port: 80, Replicas: 1,
		Volumes: []store.ServiceVolume{{Name: "app-web-data", ContainerPath: "/data"}},
	}); err != nil {
		t.Fatalf("save service: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if err := db.SaveBackupTarget(ctx, store.BackupTarget{
		ID: "bkt_test1", Name: "primary", Provider: store.BackupProviderAWS, Bucket: "backups", CreatedAt: now,
	}); err != nil {
		t.Fatalf("save target: %v", err)
	}
	if err := db.StartBackupHistory(ctx, store.BackupHistory{ID: "b1", DatabaseName: "main", TargetID: "bkt_test1", StartedAt: now}); err != nil {
		t.Fatalf("start backup: %v", err)
	}
	if err := db.FinishBackupHistory(ctx, "b1", store.BackupStatusSucceeded, 2048, "sum", "", now); err != nil {
		t.Fatalf("finish backup: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/usage/summary", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got usageSummaryResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Databases) != 1 || got.Databases[0].Name != "main" {
		t.Errorf("databases = %+v", got.Databases)
	}
	if got.Volumes.TotalBytes != 4096 || got.Volumes.UnknownCount != 1 || len(got.Volumes.Items) != 2 {
		t.Errorf("volumes = %+v", got.Volumes)
	}
	if got.Volumes.Items[0].Owner != "main" || got.Volumes.Items[0].OwnerKind != usageOwnerDatabase {
		t.Errorf("largest volume owner = %+v", got.Volumes.Items[0])
	}
	if got.Backups.TotalBytes != 2048 || got.Backups.Count != 1 {
		t.Errorf("backups = %+v", got.Backups)
	}
}

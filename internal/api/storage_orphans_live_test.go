package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/dockertest"
	"github.com/GLINCKER/levelrail/internal/store"
)

func volumeExists(t *testing.T, c *docker.Client, name string) bool {
	t.Helper()
	vols, err := c.ListNamedVolumes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range vols {
		if v.Name == name {
			return true
		}
	}
	return false
}

// TestStorageOrphans_Live_InspectAndReclaim runs the orphan flow against a
// real Docker daemon: a volume whose database is gone is listed and
// reclaimed, while a volume of a live database and one mounted by a
// container are never offered.
func TestStorageOrphans_Live_InspectAndReclaim(t *testing.T) {
	dockertest.SkipIfShort(t)
	client, err := docker.NewClient()
	if err != nil {
		t.Skipf("no docker client available: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	ctx := context.Background()

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	orphan := "db-liveorphan" + suffix + "-data"
	kept := "db-livekeep" + suffix + "-data"
	mounted := "db-livemounted" + suffix + "-data"
	for _, v := range []string{orphan, kept, mounted} {
		if err := client.EnsureVolume(ctx, v); err != nil {
			t.Skipf("docker daemon not reachable: %v", err)
		}
		v := v
		t.Cleanup(func() { _ = client.RemoveVolume(context.Background(), v) })
	}
	holder := "levelrail-live-orphan-holder-" + suffix
	id, err := client.Create(ctx, docker.ContainerSpec{
		Name:    holder,
		Image:   "nginx:alpine",
		Volumes: []docker.VolumeMount{{Name: mounted, ContainerPath: "/data"}},
	})
	if err != nil {
		t.Skipf("could not create the holder container: %v", err)
	}
	t.Cleanup(func() { _ = client.Remove(context.Background(), id, true) })

	db := openTestDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db, WithOrphanedVolumeManager(client))
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: "livekeep" + suffix, Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatal(err)
	}

	rec := do(t, rt, cookie, http.MethodGet, "/api/v1/storage/orphans", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list = %d, body = %s", rec.Code, rec.Body.String())
	}
	var list storageOrphansResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	seen := map[string]storageOrphanResource{}
	for _, o := range list.Orphans {
		seen[o.ID] = o
	}
	got, ok := seen[orphan]
	if !ok {
		t.Fatalf("real orphan %s not listed: %+v", orphan, list.Orphans)
	}
	if got.BelongedTo.Kind != "database" || got.BelongedTo.Name != "liveorphan"+suffix || got.BackupCount != 0 || got.CreatedAt == "" {
		t.Fatalf("orphan description = %+v", got)
	}
	if _, bad := seen[kept]; bad {
		t.Fatal("volume of a live database was offered for reclaim")
	}
	if _, bad := seen[mounted]; bad {
		t.Fatal("mounted volume was offered for reclaim")
	}

	if rec := do(t, rt, cookie, http.MethodDelete, "/api/v1/storage/orphans/"+mounted, `{"confirm":"`+mounted+`"}`); rec.Code != http.StatusNotFound {
		t.Fatalf("reclaiming a mounted volume = %d", rec.Code)
	}
	if rec := do(t, rt, cookie, http.MethodDelete, "/api/v1/storage/orphans/"+orphan, `{"confirm":"nope"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("wrong confirmation = %d", rec.Code)
	}
	if !volumeExists(t, client, orphan) {
		t.Fatal("volume vanished without a valid confirmation")
	}
	if rec := do(t, rt, cookie, http.MethodDelete, "/api/v1/storage/orphans/"+orphan, `{"confirm":"`+orphan+`"}`); rec.Code != http.StatusOK {
		t.Fatalf("reclaim = %d, body = %s", rec.Code, rec.Body.String())
	}
	if volumeExists(t, client, orphan) {
		t.Fatal("reclaimed volume still exists on the daemon")
	}
	if !volumeExists(t, client, kept) || !volumeExists(t, client, mounted) {
		t.Fatal("reclaim removed a volume it must not touch")
	}
}

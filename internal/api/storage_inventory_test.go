package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeInventorySource struct {
	fakeOrphanedVolumeManager
	vols []docker.InventoryVolume
	ctrs []docker.InventoryContainer
}

func (f *fakeInventorySource) ListAllVolumes(context.Context) ([]docker.InventoryVolume, error) {
	return f.vols, nil
}

func (f *fakeInventorySource) ListStoppedContainers(context.Context) ([]docker.InventoryContainer, error) {
	return f.ctrs, nil
}

func TestStorageInventory_OwnersAndProtection(t *testing.T) {
	fake := &fakeInventorySource{
		vols: []docker.InventoryVolume{
			{Name: "app-web-data", SizeBytes: 100, InUse: true},
			{Name: "db-main-data", SizeBytes: 200},
			{Name: "scratch", SizeBytes: -1},
		},
		ctrs: []docker.InventoryContainer{{ID: "c1", Name: "web-0123abcd", SizeBytes: 7}},
	}
	rt, db := newTestRouterWithOrphanedVolumeManager(t, fake)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{
		Name: "web", Image: "levelrail/web:v1", Port: 3000,
		Volumes: []store.ServiceVolume{{Name: "app-web-data", ContainerPath: "/data"}},
	}); err != nil {
		t.Fatalf("seed service: %v", err)
	}
	if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: "main", Engine: "postgres", Version: "16"}); err != nil {
		t.Fatalf("seed database: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/system/storage/inventory?node=local", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got struct {
		Volumes []struct {
			Name       string `json:"name"`
			OwnerApp   string `json:"owner_app"`
			IsDatabase bool   `json:"is_database"`
			Orphaned   bool   `json:"orphaned"`
			Protected  bool   `json:"protected"`
		} `json:"volumes"`
		Containers []struct {
			Name     string `json:"name"`
			OwnerApp string `json:"owner_app"`
		} `json:"containers"`
		Unsupported bool `json:"unsupported"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Unsupported || len(got.Volumes) != 3 {
		t.Fatalf("unexpected response: %s", rec.Body.String())
	}
	by := map[string]int{}
	for i, v := range got.Volumes {
		by[v.Name] = i
	}
	if v := got.Volumes[by["app-web-data"]]; v.OwnerApp != "web" || v.Orphaned || v.Protected {
		t.Errorf("web volume: %+v", v)
	}
	if v := got.Volumes[by["db-main-data"]]; !v.IsDatabase || !v.Protected || v.Orphaned {
		t.Errorf("db volume: %+v", v)
	}
	if v := got.Volumes[by["scratch"]]; !v.Orphaned || v.Protected {
		t.Errorf("scratch volume: %+v", v)
	}
	if len(got.Containers) != 1 || got.Containers[0].OwnerApp != "web" {
		t.Errorf("containers: %+v", got.Containers)
	}
}

func TestStorageInventory_RemoteNodeUnsupported(t *testing.T) {
	rt, db := newTestRouterWithOrphanedVolumeManager(t, &fakeInventorySource{})
	cookie := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/system/storage/inventory?node=edge-1", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var got storageInventoryResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.Unsupported || got.Reason == "" || len(got.Volumes) != 0 {
		t.Errorf("response: %+v", got)
	}
}

func TestStorageInventory_RequiresAbility(t *testing.T) {
	rt, db := newTestRouterWithOrphanedVolumeManager(t, &fakeInventorySource{})
	const plaintext = "no-ability-token" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(context.Background(), store.APIToken{
		ID: "tok_none", Name: "none", TokenHash: hashToken(plaintext), Abilities: []string{AbilityDeploy}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/storage/inventory", strings.NewReader(""))
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}

func TestStorageInventory_NotConfigured(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/system/storage/inventory", ""))
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want 501", rec.Code)
	}
}

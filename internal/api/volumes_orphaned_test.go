package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeOrphanedVolumeManager is a hand-written fake for
// OrphanedVolumeManager, the same "not a mocking framework" convention
// fakeDockerPruner (system_prune_test.go) already establishes.
type fakeOrphanedVolumeManager struct {
	volumes   []docker.NamedVolume
	removed   []string
	removeErr map[string]error
}

func (f *fakeOrphanedVolumeManager) ListNamedVolumes(context.Context) ([]docker.NamedVolume, error) {
	return f.volumes, nil
}

func (f *fakeOrphanedVolumeManager) RemoveVolume(_ context.Context, name string) error {
	if err, ok := f.removeErr[name]; ok {
		return err
	}
	f.removed = append(f.removed, name)
	return nil
}

func newTestRouterWithOrphanedVolumeManager(t *testing.T, m OrphanedVolumeManager) (*Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	return NewRouter(discardLogger(), testBrand(), db, WithOrphanedVolumeManager(m)), db
}

func TestHandleListOrphanedVolumes_NotConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithOrphanedVolumeManager
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/system/volumes/orphaned", ""))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNotImplemented, rec.Body.String())
	}
}

func TestHandleCleanupOrphanedVolumes_NotConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithOrphanedVolumeManager
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	req := authedRequest(t, cookie, http.MethodPost, "/api/v1/system/volumes/orphaned/cleanup", `{"names":["app-old-data"]}`)
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNotImplemented, rec.Body.String())
	}
}

func TestOrphanedVolumesRoutes_RequireAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	assertRoutesRequireAuth(t, rt, []routeCase{
		{http.MethodGet, "/api/v1/system/volumes/orphaned"},
		{http.MethodPost, "/api/v1/system/volumes/orphaned/cleanup"},
	})
}

// TestHandleCleanupOrphanedVolumes_PlainWriteToken_Forbidden proves POST
// .../orphaned/cleanup sits behind AbilityRoot, not AbilityWrite, the
// same boundary TestHandleSystemPrune_PlainWriteToken_Forbidden already
// establishes for POST /system/prune: this deletes real Docker volumes
// fleet-wide with no undo.
func TestHandleCleanupOrphanedVolumes_PlainWriteToken_Forbidden(t *testing.T) {
	rt, db := newTestRouterWithOrphanedVolumeManager(t, &fakeOrphanedVolumeManager{})
	ctx := context.Background()

	const plaintext = "write-scoped-token" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(ctx, store.APIToken{
		ID: "tok_write", Name: "writer", TokenHash: hashToken(plaintext), Abilities: []string{AbilityWrite}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/system/volumes/orphaned/cleanup", strings.NewReader(`{"names":["app-old-data"]}`))
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d: a plain write token must not reach a fleet-wide destructive action", rec.Code, http.StatusForbidden)
	}
}

// TestHandleListOrphanedVolumes_FiltersDesiredAndMounted is this
// feature's core safety-relevant behavior: a volume still referenced by
// a live app service's ServiceVolume, still referenced by a live
// database's data volume formula, or still mounted into any container
// must never appear in the orphaned list, only a volume that is none of
// those three.
func TestHandleListOrphanedVolumes_FiltersDesiredAndMounted(t *testing.T) {
	fake := &fakeOrphanedVolumeManager{
		volumes: []docker.NamedVolume{
			{Name: "app-web-data", SizeBytes: 100, CreatedAt: "2026-01-01T00:00:00Z"},                    // desired: referenced by seeded service
			{Name: "db-main-data", SizeBytes: 200, CreatedAt: "2026-01-01T00:00:00Z"},                    // desired: derived from seeded database
			{Name: "app-mounted-data", SizeBytes: 300, CreatedAt: "2026-01-01T00:00:00Z", Mounted: true}, // not in desired state, but still mounted
			{Name: "app-gone-data", SizeBytes: 400, CreatedAt: "2026-01-02T00:00:00Z"},                   // genuinely orphaned
		},
	}
	rt, db := newTestRouterWithOrphanedVolumeManager(t, fake)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	svc := store.DesiredService{
		Name: "web", Image: "levelrail/web:v1", Port: 3000,
		Volumes: []store.ServiceVolume{{Name: "app-web-data", ContainerPath: "/data"}},
	}
	if err := db.SaveDesiredService(ctx, svc); err != nil {
		t.Fatalf("seed service: %v", err)
	}
	if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: "main", Engine: "postgres", Version: "16"}); err != nil {
		t.Fatalf("seed database: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/system/volumes/orphaned", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got []orphanedVolumeResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 || got[0].Name != "app-gone-data" {
		t.Fatalf("orphaned volumes = %+v, want exactly [app-gone-data]", got)
	}
	if got[0].SizeBytes == nil || *got[0].SizeBytes != 400 {
		t.Errorf("size_bytes = %v, want 400", got[0].SizeBytes)
	}
}

// TestHandleListOrphanedVolumes_UnknownSizeOmitted proves a volume whose
// driver didn't report a size (docker.NamedVolume.SizeBytes == -1) comes
// back with size_bytes omitted, never a fabricated 0.
func TestHandleListOrphanedVolumes_UnknownSizeOmitted(t *testing.T) {
	fake := &fakeOrphanedVolumeManager{
		volumes: []docker.NamedVolume{{Name: "app-unknown-size", SizeBytes: -1}},
	}
	rt, db := newTestRouterWithOrphanedVolumeManager(t, fake)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/system/volumes/orphaned", ""))
	var got []orphanedVolumeResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 || got[0].SizeBytes != nil {
		t.Fatalf("orphaned volumes = %+v, want one entry with size_bytes omitted", got)
	}
	if bytes.Contains(rec.Body.Bytes(), []byte("size_bytes")) {
		t.Errorf("body %s contains size_bytes, want it omitted entirely", rec.Body.String())
	}
}

func TestHandleCleanupOrphanedVolumes_RequiresNames(t *testing.T) {
	rt, db := newTestRouterWithOrphanedVolumeManager(t, &fakeOrphanedVolumeManager{})
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/system/volumes/orphaned/cleanup", `{"names":[]}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

// TestHandleCleanupOrphanedVolumes_Success removes exactly the requested,
// still-orphaned volumes and reports reclaimed bytes.
func TestHandleCleanupOrphanedVolumes_Success(t *testing.T) {
	fake := &fakeOrphanedVolumeManager{
		volumes: []docker.NamedVolume{
			{Name: "app-gone-a", SizeBytes: 100},
			{Name: "app-gone-b", SizeBytes: 250},
		},
	}
	rt, db := newTestRouterWithOrphanedVolumeManager(t, fake)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	req := authedRequest(t, cookie, http.MethodPost, "/api/v1/system/volumes/orphaned/cleanup", `{"names":["app-gone-a","app-gone-b"]}`)
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got cleanupOrphanedVolumesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	sort.Strings(got.Removed)
	if len(got.Removed) != 2 || got.Removed[0] != "app-gone-a" || got.Removed[1] != "app-gone-b" {
		t.Errorf("Removed = %v, want [app-gone-a app-gone-b]", got.Removed)
	}
	if got.ReclaimedBytes != 350 {
		t.Errorf("ReclaimedBytes = %d, want 350", got.ReclaimedBytes)
	}
	sort.Strings(fake.removed)
	if len(fake.removed) != 2 || fake.removed[0] != "app-gone-a" || fake.removed[1] != "app-gone-b" {
		t.Errorf("RemoveVolume calls = %v, want both names", fake.removed)
	}
}

// TestHandleCleanupOrphanedVolumes_SkipsNoLongerOrphaned is the TOCTOU
// guard: a requested name that is no longer orphaned (e.g. a live app
// started referencing it again, or it was never orphaned to begin with)
// is reported in Skipped and RemoveVolume is never called for it.
func TestHandleCleanupOrphanedVolumes_SkipsNoLongerOrphaned(t *testing.T) {
	fake := &fakeOrphanedVolumeManager{
		volumes: []docker.NamedVolume{{Name: "app-still-live", Mounted: true}},
	}
	rt, db := newTestRouterWithOrphanedVolumeManager(t, fake)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	req := authedRequest(t, cookie, http.MethodPost, "/api/v1/system/volumes/orphaned/cleanup", `{"names":["app-still-live"]}`)
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got cleanupOrphanedVolumesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Skipped) != 1 || got.Skipped[0] != "app-still-live" {
		t.Errorf("Skipped = %v, want [app-still-live]", got.Skipped)
	}
	if len(got.Removed) != 0 {
		t.Errorf("Removed = %v, want none", got.Removed)
	}
	if len(fake.removed) != 0 {
		t.Errorf("RemoveVolume called for a still-mounted volume: %v", fake.removed)
	}
}

// TestHandleCleanupOrphanedVolumes_SurfacesRemovalError proves one
// volume's own removal failure doesn't stop the rest, and is reported in
// Errors rather than a 5xx, the same "partial failure is a real,
// reportable outcome" shape TestHandleSystemPrune_SurfacesPartialStageErrors
// already establishes for POST /system/prune.
func TestHandleCleanupOrphanedVolumes_SurfacesRemovalError(t *testing.T) {
	fake := &fakeOrphanedVolumeManager{
		volumes: []docker.NamedVolume{
			{Name: "app-ok", SizeBytes: 10},
			{Name: "app-fails", SizeBytes: 20},
		},
		removeErr: map[string]error{"app-fails": errors.New("daemon busy")},
	}
	rt, db := newTestRouterWithOrphanedVolumeManager(t, fake)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	req := authedRequest(t, cookie, http.MethodPost, "/api/v1/system/volumes/orphaned/cleanup", `{"names":["app-ok","app-fails"]}`)
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got cleanupOrphanedVolumesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Removed) != 1 || got.Removed[0] != "app-ok" {
		t.Errorf("Removed = %v, want [app-ok]", got.Removed)
	}
	if len(got.Errors) != 1 {
		t.Errorf("Errors = %v, want one entry for app-fails", got.Errors)
	}
}

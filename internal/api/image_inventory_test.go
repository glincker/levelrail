package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
)

type fakeImageInventory struct {
	records []docker.ImageRecord
	removed []string
}

func (f *fakeImageInventory) ListAllImages(context.Context) ([]docker.ImageRecord, error) {
	return f.records, nil
}

func (f *fakeImageInventory) ImageLayerHistory(_ context.Context, ref string) ([]docker.ImageLayer, error) {
	if ref != "sha256:live" {
		return nil, nil
	}
	return []docker.ImageLayer{{ID: "l1", CreatedBy: "RUN build", SizeBytes: 70}, {ID: "l2", CreatedBy: "FROM base", SizeBytes: 30}}, nil
}

func (f *fakeImageInventory) RemoveImage(_ context.Context, id string) error {
	f.removed = append(f.removed, id)
	return nil
}

func TestImageRoutes(t *testing.T) {
	db := openTestDB(t)
	inv := &fakeImageInventory{records: []docker.ImageRecord{
		{ID: "sha256:live", Tags: []string{"img:v1"}, SizeBytes: 100, CreatedAt: time.Now().Add(-72 * time.Hour)},
		{ID: "sha256:stale", Tags: []string{"img:v0"}, SizeBytes: 50, CreatedAt: time.Now().Add(-72 * time.Hour)},
	}}
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	rt := NewRouter(logger, testBrand(), db, WithImageInventory(func(string) (docker.ImageInventory, error) { return inv, nil }))
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")

	do := func(method, target, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, target, body))
		return rec
	}

	rec := do(http.MethodGet, "/api/v1/images", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d: %s", rec.Code, rec.Body)
	}
	var list imageListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if list.Totals.Count != 2 || list.Totals.ProtectedCount != 2 {
		t.Errorf("totals = %+v, want 2 images both protected (live and unattributed)", list.Totals)
	}

	rec = do(http.MethodGet, "/api/v1/images/layers?id=sha256:live", "")
	var layers imageLayersResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &layers); err != nil || layers.SizeBytes != 100 {
		t.Errorf("layers = %+v err=%v", layers, err)
	}
	if rec = do(http.MethodGet, "/api/v1/images/layers?id=sha256:none", ""); rec.Code != http.StatusNotFound {
		t.Errorf("missing image status = %d", rec.Code)
	}

	rec = do(http.MethodPost, "/api/v1/images/gc", `{"dry_run":false}`)
	if rec.Code != http.StatusOK || len(inv.removed) != 0 {
		t.Errorf("gc status = %d removed = %v, want nothing removed", rec.Code, inv.removed)
	}

	rec = do(http.MethodPost, "/api/v1/images/verify", `{"ref":"x/y@sha256:abc"}`)
	var v imageVerifyResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil || v.Status != verifyStatusNotConfigured {
		t.Errorf("verify = %+v err=%v", v, err)
	}
}

func TestImageRoutes_NotConfigured(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/images", ""))
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want 501", rec.Code)
	}
}

func TestImageRemove(t *testing.T) {
	db := openTestDB(t)
	old := time.Now().Add(-72 * time.Hour)
	inv := &fakeImageInventory{records: []docker.ImageRecord{
		{ID: "sha256:live", Tags: []string{"img:v1"}, SizeBytes: 100, CreatedAt: old},
		{ID: "sha256:foreign", Tags: []string{"nginx:1", "nginx:latest"}, SizeBytes: 60, SharedBytes: 10, CreatedAt: old},
		{ID: "sha256:stopped", Tags: []string{"redis:7"}, SizeBytes: 40, Containers: 1, CreatedAt: old},
		{ID: "sha256:other", SizeBytes: 30, CreatedAt: old},
	}}
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	rt := NewRouter(logger, testBrand(), db, WithImageInventory(func(string) (docker.ImageInventory, error) { return inv, nil }))
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	do := func(body string) (imageRemoveResponse, int) {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/images/remove", body))
		var out imageRemoveResponse
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return out, rec.Code
	}

	out, code := do(`{"ids":["sha256:foreign"],"dry_run":true}`)
	if code != http.StatusOK || len(out.Candidates) != 1 || len(inv.removed) != 0 {
		t.Fatalf("dry run: code=%d out=%+v removed=%v", code, out, inv.removed)
	}
	if out.ReclaimedBytes != 50 {
		t.Errorf("dry run reclaimed = %d, want 50 (size minus shared)", out.ReclaimedBytes)
	}

	out, _ = do(`{"ids":["sha256:foreign","sha256:live","sha256:stopped","sha256:nope"]}`)
	if len(out.Removed) != 1 || len(out.Failed) != 3 || out.ReclaimedBytes != 50 {
		t.Fatalf("explicit: %+v", out)
	}
	if got := inv.removed; len(got) != 2 || got[0] != "nginx:1" || got[1] != "nginx:latest" {
		t.Errorf("multi-tag image must remove every tag, removed = %v", got)
	}

	inv.removed = nil
	out, _ = do(`{"include_unmanaged_unused":true}`)
	if len(out.Removed) != 2 || len(inv.removed) != 3 {
		t.Errorf("unmanaged unused: %+v removed=%v", out, inv.removed)
	}
	for _, id := range inv.removed {
		if id == "redis:7" || id == "img:v1" {
			t.Errorf("removed protected image %s", id)
		}
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/images", ""))
	var list imageListResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if list.Totals.UnmanagedUnusedCount != 2 || list.Totals.UnmanagedUnusedBytes != 80 {
		t.Errorf("totals = %+v", list.Totals)
	}

	if _, code := do(`{}`); code != http.StatusBadRequest {
		t.Errorf("empty request code = %d", code)
	}
	ids := make([]string, 201)
	body, _ := json.Marshal(map[string]any{"ids": ids})
	if _, code := do(string(body)); code != http.StatusBadRequest {
		t.Errorf("cap exceeded code = %d, want 400", code)
	}
}

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

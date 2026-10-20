package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/objectstore"
	"github.com/GLINCKER/levelrail/internal/store"
)

func newBuildCacheEnv(t *testing.T) (*storageEnv, storageDestinationResource) {
	t.Helper()
	e := newStorageEnv(t)
	d := e.createDestination(t, "")
	secrets := &memSecrets{}
	_ = secrets.SetValue(context.Background(), store.BackupTargetSecretsKey(d.ID), "access_key_id", "AKIDSECRET")
	_ = secrets.SetValue(context.Background(), store.BackupTargetSecretsKey(d.ID), "secret_access_key", "topsecretvalue")
	bc := &objectstore.BuildCache{
		Store: e.db, Resolver: &objectstore.Resolver{Store: e.db, Secrets: secrets, MaxAttempts: 1}, Options: objectstore.DefaultBuildCacheOptions(),
	}
	e.rt.SetStorage(StorageDeps{Options: e.db, BuildCache: bc, BuildCacheSettings: e.db})
	if err := e.db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "img:1", Port: 3000}); err != nil {
		t.Fatalf("SaveDesiredService: %v", err)
	}
	return e, d
}

func TestBuildCacheSettingLifecycle(t *testing.T) {
	e, d := newBuildCacheEnv(t)
	tests := []struct {
		name, body string
		want       int
	}{
		{"missing target", `{"app_name":"web"}`, http.StatusBadRequest},
		{"bad mode", `{"app_name":"web","target_id":"` + d.ID + `","mode":"all"}`, http.StatusBadRequest},
		{"unknown target", `{"app_name":"web","target_id":"bkt_none"}`, http.StatusNotFound},
		{"unknown app", `{"app_name":"ghost","target_id":"` + d.ID + `"}`, http.StatusNotFound},
		{"unsafe app name", `{"app_name":"../x","target_id":"` + d.ID + `"}`, http.StatusBadRequest},
		{"global default", `{"target_id":"` + d.ID + `","mode":"min"}`, http.StatusOK},
		{"app setting", `{"app_name":"web","target_id":"` + d.ID + `"}`, http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := e.do(t, http.MethodPut, "/api/v1/build-cache", tt.body)
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tt.want, rec.Body.String())
			}
		})
	}

	rec := e.do(t, http.MethodGet, "/api/v1/build-cache", "")
	var list []buildCacheResource
	if err := json.NewDecoder(rec.Body).Decode(&list); err != nil || len(list) != 2 {
		t.Fatalf("list = %d %v", len(list), err)
	}
	if list[1].AppName != "web" || list[1].Mode != "max" || list[1].KeyPrefix != "build-cache/web/" || !list[1].Enabled {
		t.Errorf("app row = %+v", list[1])
	}
	if strings.Contains(rec.Body.String(), "topsecret") || strings.Contains(rec.Body.String(), "AKIDSECRET") {
		t.Fatal("list leaked credentials")
	}

	if rec := e.do(t, http.MethodDelete, "/api/v1/build-cache?app=web", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete = %d", rec.Code)
	}
	if rec := e.do(t, http.MethodDelete, "/api/v1/build-cache?app=web", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("second delete = %d", rec.Code)
	}
}

func TestBuildCacheStatsAndClear(t *testing.T) {
	e, d := newBuildCacheEnv(t)
	if rec := e.do(t, http.MethodPost, "/api/v1/build-cache/clear", `{"app_name":"web"}`); rec.Code != http.StatusConflict {
		t.Fatalf("clear unconfigured = %d, want 409", rec.Code)
	}
	if rec := e.do(t, http.MethodPut, "/api/v1/build-cache", `{"app_name":"web","target_id":"`+d.ID+`"}`); rec.Code != http.StatusOK {
		t.Fatalf("set = %d", rec.Code)
	}
	now := time.Now().UTC()
	e.srv.Put("build-cache/web/blobs/a", []byte("12345"), now)
	e.srv.Put("build-cache/web/manifests/cache", []byte("6"), now)
	e.srv.Put("build-cache/api/blobs/keep", []byte("7"), now)

	rec := e.do(t, http.MethodGet, "/api/v1/build-cache/stats?app=web", "")
	var stats objectstore.CacheStats
	if err := json.NewDecoder(rec.Body).Decode(&stats); err != nil || rec.Code != http.StatusOK || stats.Objects != 2 || stats.Bytes != 6 {
		t.Fatalf("stats = %d %+v %v", rec.Code, stats, err)
	}
	if rec := e.do(t, http.MethodGet, "/api/v1/build-cache/stats", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("stats without app = %d", rec.Code)
	}

	rec = e.do(t, http.MethodPost, "/api/v1/build-cache/clear", `{"app_name":"web"}`)
	var res objectstore.ClearResult
	if err := json.NewDecoder(rec.Body).Decode(&res); err != nil || rec.Code != http.StatusOK || res.Deleted != 2 || res.More {
		t.Fatalf("clear = %d %+v %v", rec.Code, res, err)
	}
	if keys := e.srv.Keys(); len(keys) != 1 || keys[0] != "build-cache/api/blobs/keep" {
		t.Fatalf("remaining keys = %v", keys)
	}
	if rec := e.do(t, http.MethodPost, "/api/v1/build-cache/clear", `{}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("clear without app = %d", rec.Code)
	}
}

func TestBuildCacheRoutesRequireAuthAndConfig(t *testing.T) {
	rt, _ := newTestRouter(t)
	assertRoutesRequireAuth(t, rt, []routeCase{
		{http.MethodGet, "/api/v1/build-cache"},
		{http.MethodPut, "/api/v1/build-cache"},
		{http.MethodDelete, "/api/v1/build-cache"},
		{http.MethodGet, "/api/v1/build-cache/stats"},
		{http.MethodPost, "/api/v1/build-cache/clear"},
	})

	e := newStorageEnv(t)
	if rec := e.do(t, http.MethodGet, "/api/v1/build-cache", ""); rec.Code != http.StatusNotImplemented {
		t.Fatalf("storage without build cache = %d, want 501", rec.Code)
	}
}

func TestDeployAttemptCacheWarning(t *testing.T) {
	e, _ := newBuildCacheEnv(t)
	ctx := context.Background()
	if err := e.db.SaveDeployAttempt(ctx, store.DeployAttempt{ID: "att_1", ServiceName: "web", Image: "img:1", Status: store.DeployAttemptStatusRunning, StartedAt: time.Now()}); err != nil {
		t.Fatalf("SaveDeployAttempt: %v", err)
	}
	if err := e.db.SetDeployAttemptCacheWarning(ctx, "att_1", "build cache unavailable"); err != nil {
		t.Fatalf("SetDeployAttemptCacheWarning: %v", err)
	}
	rec := e.do(t, http.MethodGet, "/api/v1/apps/web/deploy-attempts", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"cache_warning":"build cache unavailable"`) {
		t.Fatalf("attempts = %d %s", rec.Code, rec.Body.String())
	}
}

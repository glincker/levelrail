package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/objectstore"
	"github.com/GLINCKER/levelrail/internal/store"
)

func newBucketEnv(t *testing.T) (*storageEnv, storageDestinationResource) {
	t.Helper()
	e := newStorageEnv(t)
	d := e.createDestination(t, "")
	secrets := &memSecrets{}
	_ = secrets.SetValue(context.Background(), store.BackupTargetSecretsKey(d.ID), "access_key_id", "AKIDSECRET")
	_ = secrets.SetValue(context.Background(), store.BackupTargetSecretsKey(d.ID), "secret_access_key", "topsecretvalue")
	resolver := &objectstore.Resolver{Store: e.db, Secrets: secrets, MaxAttempts: 1}
	e.rt.SetStorage(StorageDeps{
		Options: e.db, Clients: resolver, Buckets: e.db,
		BucketSweeper: &objectstore.BucketSweeper{Store: e.db, Resolver: resolver, Logger: discardLogger()},
	})
	if err := e.db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "img:1", Port: 3000}); err != nil {
		t.Fatalf("SaveDesiredService: %v", err)
	}
	return e, d
}

func TestStorageBucketCreateAndBind(t *testing.T) {
	e, d := newBucketEnv(t)
	tests := []struct {
		name, body string
		want       int
	}{
		{"bad name", `{"name":"Bad_Name","target_id":"` + d.ID + `"}`, http.StatusBadRequest},
		{"unknown target", `{"name":"uploads","target_id":"bkt_none"}`, http.StatusBadRequest},
		{"unknown app", `{"name":"uploads","target_id":"` + d.ID + `","app_name":"ghost"}`, http.StatusBadRequest},
		{"negative quota", `{"name":"uploads","target_id":"` + d.ID + `","quota_bytes":-1}`, http.StatusBadRequest},
		{"bad env prefix", `{"name":"uploads","target_id":"` + d.ID + `","app_name":"web","env_prefix":"lower"}`, http.StatusBadRequest},
		{"created and bound", `{"name":"uploads","target_id":"` + d.ID + `","app_name":"web"}`, http.StatusCreated},
		{"duplicate name", `{"name":"uploads","target_id":"` + d.ID + `"}`, http.StatusConflict},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := e.do(t, http.MethodPost, "/api/v1/storage/buckets", tt.body)
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tt.want, rec.Body.String())
			}
		})
	}

	rec := e.do(t, http.MethodGet, "/api/v1/apps/web/buckets", "")
	var list []storageBucketResource
	if err := json.NewDecoder(rec.Body).Decode(&list); err != nil || len(list) != 1 {
		t.Fatalf("app buckets = %d %v", len(list), err)
	}
	if list[0].Apps[0].EnvPrefix != "S3" || len(list[0].EnvKeys) != 6 || list[0].Public {
		t.Errorf("bucket = %+v", list[0])
	}
	if strings.Contains(rec.Body.String(), "topsecret") {
		t.Fatal("response leaked credentials")
	}

	other := e.do(t, http.MethodPost, "/api/v1/storage/buckets", `{"name":"media","target_id":"`+d.ID+`"}`)
	var media storageBucketResource
	_ = json.NewDecoder(other.Body).Decode(&media)
	if rec := e.do(t, http.MethodPut, "/api/v1/apps/web/buckets/"+media.ID, ""); rec.Code != http.StatusConflict {
		t.Fatalf("second bucket on the default prefix = %d, want 409", rec.Code)
	}
	if rec := e.do(t, http.MethodPut, "/api/v1/apps/web/buckets/"+media.ID, `{"env_prefix":"MEDIA"}`); rec.Code != http.StatusOK {
		t.Fatalf("bind with own prefix = %d", rec.Code)
	}
	if rec := e.do(t, http.MethodDelete, "/api/v1/apps/web/buckets/"+media.ID, ""); rec.Code != http.StatusOK {
		t.Fatalf("unbind = %d", rec.Code)
	}
}

func TestStorageBucketObjectsQuotaAndPublic(t *testing.T) {
	e, d := newBucketEnv(t)
	rec := e.do(t, http.MethodPost, "/api/v1/storage/buckets", `{"name":"files","target_id":"`+d.ID+`","quota_bytes":10,"lifecycle_days":7}`)
	var b storageBucketResource
	if err := json.NewDecoder(rec.Body).Decode(&b); err != nil || rec.Code != http.StatusCreated {
		t.Fatalf("create = %d %v", rec.Code, err)
	}
	objects := "/api/v1/storage/buckets/" + b.ID + "/objects"

	if rec := e.do(t, http.MethodPut, objects+"?key=a.txt", "12345678"); rec.Code != http.StatusCreated {
		t.Fatalf("upload = %d: %s", rec.Code, rec.Body.String())
	}
	if rec := e.do(t, http.MethodPut, objects+"?key=b.txt", "12345"); rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("upload over quota = %d, want 413", rec.Code)
	}
	if rec := e.do(t, http.MethodPut, objects+"?key=../escape", "x"); rec.Code != http.StatusBadRequest {
		t.Fatalf("upload with traversal key = %d, want 400", rec.Code)
	}
	if _, ok := e.srv.Data(b.Prefix + "a.txt"); !ok {
		t.Fatalf("object not stored under the bucket prefix: %v", e.srv.Keys())
	}

	var page bucketObjectsResource
	rec = e.do(t, http.MethodGet, objects, "")
	if err := json.NewDecoder(rec.Body).Decode(&page); err != nil || len(page.Objects) != 1 || page.Objects[0].Key != "a.txt" {
		t.Fatalf("list = %+v %v", page, err)
	}

	if rec := e.do(t, http.MethodGet, objects+"/download?key=a.txt", ""); rec.Code != http.StatusOK || rec.Body.String() != "12345678" ||
		!strings.HasPrefix(rec.Header().Get("Content-Disposition"), "attachment") {
		t.Fatalf("download = %d %q %v", rec.Code, rec.Body.String(), rec.Header())
	}

	public := publicBucketPathPrefix + b.ID + "/a.txt"
	pub := httptest.NewRecorder()
	e.rt.Handler().ServeHTTP(pub, httptest.NewRequest(http.MethodGet, public, nil))
	if pub.Code != http.StatusNotFound {
		t.Fatalf("private bucket on public route = %d, want 404", pub.Code)
	}
	if rec := e.do(t, http.MethodPatch, "/api/v1/storage/buckets/"+b.ID, `{"public":true}`); rec.Code != http.StatusOK {
		t.Fatalf("make public = %d", rec.Code)
	}
	pub = httptest.NewRecorder()
	e.rt.Handler().ServeHTTP(pub, httptest.NewRequest(http.MethodGet, public, nil))
	if pub.Code != http.StatusOK || pub.Body.String() != "12345678" || pub.Header().Get("Content-Security-Policy") == "" {
		t.Fatalf("public object = %d %q %v", pub.Code, pub.Body.String(), pub.Header())
	}

	rec = e.do(t, http.MethodPost, "/api/v1/storage/buckets/"+b.ID+"/presign", `{"key":"a.txt","method":"GET","expires_seconds":60}`)
	var ps presignResource
	if err := json.NewDecoder(rec.Body).Decode(&ps); err != nil || rec.Code != http.StatusOK || !strings.Contains(ps.URL, "X-Amz-Signature") {
		t.Fatalf("presign = %d %+v %v", rec.Code, ps, err)
	}
	if rec := e.do(t, http.MethodPost, "/api/v1/storage/buckets/"+b.ID+"/presign", `{"key":"a.txt","expires_seconds":99999999}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("presign too long = %d, want 400", rec.Code)
	}

	e.srv.Put(b.Prefix+"stale.txt", []byte("z"), time.Now().AddDate(0, 0, -30))
	if rec := e.do(t, http.MethodPost, "/api/v1/storage/buckets/"+b.ID+"/lifecycle/run", ""); rec.Code != http.StatusOK {
		t.Fatalf("lifecycle run = %d: %s", rec.Code, rec.Body.String())
	}
	if _, ok := e.srv.Data(b.Prefix + "stale.txt"); ok {
		t.Fatal("expired object survived the sweep")
	}

	if rec := e.do(t, http.MethodDelete, objects+"?key=a.txt", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete object = %d", rec.Code)
	}
	if rec := e.do(t, http.MethodDelete, "/api/v1/storage/buckets/"+b.ID+"?purge=true", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete bucket = %d", rec.Code)
	}
}

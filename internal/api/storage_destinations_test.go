package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/netguard"
	"github.com/GLINCKER/levelrail/internal/objectstore"
	"github.com/GLINCKER/levelrail/internal/objectstore/objectstoretest"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

type memSecrets struct {
	mu sync.Mutex
	m  map[string]string
}

func (s *memSecrets) SetValue(_ context.Context, svc, key, val string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.m == nil {
		s.m = map[string]string{}
	}
	s.m[svc+"/"+key] = val
	return nil
}

func (s *memSecrets) Resolve(_ context.Context, svc, key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.m[svc+"/"+key], nil
}

type storageEnv struct {
	rt     *Router
	cookie *http.Cookie
	srv    *objectstoretest.Server
	db     *store.DB
}

func newStorageEnv(t *testing.T) *storageEnv {
	t.Helper()
	t.Setenv(netguard.AllowPrivateEnv, "true")
	secrets := &memSecrets{}
	db := openTestDB(t)
	tel, err := telemetry.Open(context.Background(), filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tel.Close() })
	srv := objectstoretest.New("logs")
	t.Cleanup(srv.Close)

	resolver := &objectstore.Resolver{Store: db, Secrets: secrets, MaxAttempts: 1}
	opts := objectstore.DefaultOptions()
	arc := objectstore.NewArchiver(db, resolver, tel, discardLogger(), opts)
	t.Cleanup(arc.Wait)
	rt := NewRouter(discardLogger(), testBrand(), db, WithBackupSecrets(secrets),
		WithStorage(StorageDeps{Options: db, Archive: db, Clients: resolver, Archiver: arc, ArchiveRoot: opts.Prefix}))
	return &storageEnv{rt: rt, cookie: loginTestSession(t, rt, db), srv: srv, db: db}
}

func (e *storageEnv) do(t *testing.T, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	e.rt.Handler().ServeHTTP(rec, authedRequest(t, e.cookie, method, target, body))
	return rec
}

func (e *storageEnv) createDestination(t *testing.T, extra string) storageDestinationResource {
	t.Helper()
	body := `{"name":"logs","preset":"minio","endpoint":"` + e.srv.URL + `","bucket":"logs","access_key_id":"AKIDSECRET","secret_access_key":"topsecretvalue"` + extra + `}`
	rec := e.do(t, http.MethodPost, "/api/v1/storage/destinations", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "AKIDSECRET") || strings.Contains(rec.Body.String(), "topsecretvalue") {
		t.Fatal("create response leaked credentials")
	}
	var d storageDestinationResource
	if err := json.NewDecoder(rec.Body).Decode(&d); err != nil {
		t.Fatal(err)
	}
	return d
}

func TestStorageDestinationLifecycle(t *testing.T) {
	e := newStorageEnv(t)
	d := e.createDestination(t, `,"verify":true`)
	if d.Preset != "minio" || !d.PathStyle || d.Provider != "custom" {
		t.Fatalf("unexpected destination %+v", d)
	}

	rec := e.do(t, http.MethodPost, "/api/v1/storage/destinations/"+d.ID+"/test", "")
	var probe objectstore.ProbeResult
	_ = json.NewDecoder(rec.Body).Decode(&probe)
	if rec.Code != http.StatusOK || !probe.OK || len(probe.Steps) != 3 {
		t.Fatalf("test = %d %+v", rec.Code, probe)
	}

	e.srv.RejectAuth = true
	rec = e.do(t, http.MethodPost, "/api/v1/storage/destinations/"+d.ID+"/test", "")
	probe = objectstore.ProbeResult{}
	_ = json.NewDecoder(rec.Body).Decode(&probe)
	if rec.Code != http.StatusOK || probe.OK || probe.Reason != objectstore.ReasonInvalidCredentials {
		t.Fatalf("rejected test = %d %+v", rec.Code, probe)
	}
	e.srv.RejectAuth = false

	rec = e.do(t, http.MethodGet, "/api/v1/storage/destinations", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), d.ID) {
		t.Fatalf("list = %d %s", rec.Code, rec.Body.String())
	}
	if rec = e.do(t, http.MethodDelete, "/api/v1/storage/destinations/"+d.ID, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete = %d %s", rec.Code, rec.Body.String())
	}
}

func TestStorageDestinationCreateValidation(t *testing.T) {
	e := newStorageEnv(t)
	tests := []struct {
		name, body string
	}{
		{"missing bucket", `{"name":"x","preset":"aws","region":"us-east-1","access_key_id":"a","secret_access_key":"b"}`},
		{"r2 needs account id", `{"name":"x","preset":"r2","bucket":"b","access_key_id":"a","secret_access_key":"b"}`},
		{"custom needs endpoint", `{"name":"x","preset":"custom","bucket":"b","access_key_id":"a","secret_access_key":"b"}`},
		{"missing credentials", `{"name":"x","preset":"aws","region":"us-east-1","bucket":"b"}`},
		{"bad scheme", `{"name":"x","preset":"custom","endpoint":"file:///etc","bucket":"b","access_key_id":"a","secret_access_key":"b"}`},
		{"unknown preset", `{"name":"x","preset":"gcs","bucket":"b","access_key_id":"a","secret_access_key":"b"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if rec := e.do(t, http.MethodPost, "/api/v1/storage/destinations", tt.body); rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestStorageVerifyBlocksInternalEndpointByDefault(t *testing.T) {
	e := newStorageEnv(t)
	t.Setenv(netguard.AllowPrivateEnv, "false")
	body := `{"name":"x","preset":"minio","endpoint":"` + e.srv.URL + `","bucket":"logs","access_key_id":"a","secret_access_key":"b","verify":true}`
	rec := e.do(t, http.MethodPost, "/api/v1/storage/destinations", body)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), objectstore.ReasonEndpointBlocked) {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestLogArchivePolicyDumpAndObjects(t *testing.T) {
	e := newStorageEnv(t)
	d := e.createDestination(t, "")

	tests := []struct {
		name, body string
		want       int
	}{
		{"interval too short", `{"target_id":"` + d.ID + `","interval":"1m"}`, http.StatusBadRequest},
		{"unknown target", `{"target_id":"bkt_none"}`, http.StatusNotFound},
		{"unknown app", `{"target_id":"` + d.ID + `","app_name":"ghost"}`, http.StatusNotFound},
		{"global policy", `{"target_id":"` + d.ID + `","interval":"30m","retention_days":30}`, http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if rec := e.do(t, http.MethodPut, "/api/v1/log-archive/policy", tt.body); rec.Code != tt.want {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tt.want, rec.Body.String())
			}
		})
	}

	rec := e.do(t, http.MethodGet, "/api/v1/log-archive/policies", "")
	if !strings.Contains(rec.Body.String(), `"interval":"30m0s"`) {
		t.Fatalf("policies = %s", rec.Body.String())
	}
	if rec := e.do(t, http.MethodDelete, "/api/v1/storage/destinations/"+d.ID, ""); rec.Code != http.StatusConflict {
		t.Fatalf("delete in-use destination = %d, want 409", rec.Code)
	}

	rec = e.do(t, http.MethodPost, "/api/v1/log-archive/dump", `{"target_id":"`+d.ID+`","from":"2026-09-24T00:00:00Z","to":"2026-09-24T01:00:00Z"}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("dump = %d %s", rec.Code, rec.Body.String())
	}
	if rec = e.do(t, http.MethodPost, "/api/v1/log-archive/dump", `{"target_id":"`+d.ID+`","from":"nope","to":"x"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad dump = %d", rec.Code)
	}

	e.srv.Put("log-archive/service/web/2026/09/24/00/1.ndjson.gz", []byte("gzbytes"), time.Now().UTC())
	rec = e.do(t, http.MethodGet, "/api/v1/log-archive/objects?target_id="+d.ID+"&app=web", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "1.ndjson.gz") {
		t.Fatalf("objects = %d %s", rec.Code, rec.Body.String())
	}
	rec = e.do(t, http.MethodGet, "/api/v1/log-archive/objects/download?target_id="+d.ID+"&key=log-archive/service/web/2026/09/24/00/1.ndjson.gz", "")
	if rec.Code != http.StatusOK || rec.Body.String() != "gzbytes" || rec.Header().Get("Content-Type") != "application/gzip" {
		t.Fatalf("download = %d %q", rec.Code, rec.Body.String())
	}
	for _, key := range []string{"other/prefix/x", "log-archive/../secrets"} {
		if rec = e.do(t, http.MethodGet, "/api/v1/log-archive/objects/download?target_id="+d.ID+"&key="+key, ""); rec.Code != http.StatusBadRequest {
			t.Fatalf("download %q = %d, want 400", key, rec.Code)
		}
	}
}

func TestStorageRoutesRequireAuthAndConfig(t *testing.T) {
	rt, _ := newTestRouter(t)
	assertRoutesRequireAuth(t, rt, []routeCase{
		{http.MethodGet, "/api/v1/storage/providers"},
		{http.MethodGet, "/api/v1/storage/destinations"},
		{http.MethodPost, "/api/v1/storage/destinations"},
		{http.MethodPost, "/api/v1/storage/destinations/x/test"},
		{http.MethodGet, "/api/v1/log-archive/policies"},
		{http.MethodPut, "/api/v1/log-archive/policy"},
		{http.MethodPost, "/api/v1/log-archive/dump"},
		{http.MethodGet, "/api/v1/log-archive/objects"},
	})

	rt2, db := newTestRouter(t)
	cookie := loginTestSession(t, rt2, db)
	rec := httptest.NewRecorder()
	rt2.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/storage/destinations", ""))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("unconfigured status = %d, want 501", rec.Code)
	}
}

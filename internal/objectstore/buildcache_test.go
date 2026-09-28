package objectstore

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/netguard"
	"github.com/GLINCKER/levelrail/internal/objectstore/objectstoretest"
	"github.com/GLINCKER/levelrail/internal/store"
)

type cacheEnv struct {
	db  *store.DB
	srv *objectstoretest.Server
	bc  *BuildCache
}

func newCacheEnv(t *testing.T) *cacheEnv {
	t.Helper()
	t.Setenv(netguard.AllowPrivateEnv, "true")
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	srv := objectstoretest.New("cache")
	t.Cleanup(srv.Close)
	if err := db.SaveBackupTarget(ctx, store.BackupTarget{ID: "bkt_1", Name: "t", Provider: "custom", Endpoint: srv.URL, Region: "auto", Bucket: "cache", CreatedAt: "2026-01-01T00:00:00Z"}); err != nil {
		t.Fatalf("SaveBackupTarget: %v", err)
	}
	if err := db.SaveStorageOptions(ctx, store.StorageOptions{TargetID: "bkt_1", Preset: PresetCustom, PathStyle: true}); err != nil {
		t.Fatalf("SaveStorageOptions: %v", err)
	}
	return &cacheEnv{db: db, srv: srv, bc: &BuildCache{
		Store: db, Resolver: &Resolver{Store: db, Secrets: staticSecrets{}}, Options: DefaultBuildCacheOptions(),
	}}
}

func (e *cacheEnv) set(t *testing.T, app string, enabled bool, mode string) {
	t.Helper()
	if err := e.db.UpsertBuildCacheSetting(context.Background(), store.BuildCacheSetting{AppName: app, TargetID: "bkt_1", Enabled: enabled, Mode: mode, UpdatedAt: "2026-01-01T00:00:00Z"}); err != nil {
		t.Fatalf("UpsertBuildCacheSetting: %v", err)
	}
}

func TestBuildCacheEffective(t *testing.T) {
	tests := []struct {
		name       string
		global     *bool
		app        *bool
		wantOK     bool
		wantScope  string
		wantMode   string
		appMode    string
		globalMode string
	}{
		{name: "nothing configured"},
		{name: "global applies", global: ptr(true), globalMode: "min", wantOK: true, wantScope: "", wantMode: "min"},
		{name: "app overrides global", global: ptr(true), globalMode: "min", app: ptr(true), appMode: "max", wantOK: true, wantScope: "web", wantMode: "max"},
		{name: "app disable beats global", global: ptr(true), globalMode: "min", app: ptr(false), appMode: "max"},
		{name: "disabled global", global: ptr(false), globalMode: "min"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newCacheEnv(t)
			if tt.global != nil {
				e.set(t, "", *tt.global, tt.globalMode)
			}
			if tt.app != nil {
				e.set(t, "web", *tt.app, tt.appMode)
			}
			got, ok, err := e.bc.Effective(context.Background(), "web")
			if err != nil || ok != tt.wantOK {
				t.Fatalf("ok = %v err = %v, want ok %v", ok, err, tt.wantOK)
			}
			if ok && (got.AppName != tt.wantScope || got.Mode != tt.wantMode) {
				t.Errorf("got scope %q mode %q", got.AppName, got.Mode)
			}
		})
	}
}

func ptr[T any](v T) *T { return &v }

func TestBuildCacheS3Cache(t *testing.T) {
	e := newCacheEnv(t)
	ctx := context.Background()
	if c, err := e.bc.S3Cache(ctx, "web"); err != nil || c != nil {
		t.Fatalf("unconfigured: %v %v", c, err)
	}
	e.set(t, "", true, "min")
	c, err := e.bc.S3Cache(ctx, "web")
	if err != nil || c == nil {
		t.Fatalf("S3Cache: %v %v", c, err)
	}
	if c.Bucket != "cache" || c.Prefix != "build-cache/web/" || c.Mode != "min" || !c.PathStyle || c.AccessKeyID == "" || c.SecretAccessKey == "" {
		t.Errorf("unexpected config %+v", c)
	}
	if _, err := e.bc.S3Cache(ctx, "../evil"); !errors.Is(err, ErrInvalidBuildCacheApp) {
		t.Errorf("bad app name err = %v", err)
	}
}

func TestBuildCacheEndpointGuard(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		addrs    []string
		wantErr  bool
		allow    bool
	}{
		{name: "empty endpoint is provider default", endpoint: ""},
		{name: "literal loopback blocked", endpoint: "http://127.0.0.1:9000", wantErr: true},
		{name: "host resolving to private blocked", endpoint: "https://minio.internal", addrs: []string{"10.0.0.5"}, wantErr: true},
		{name: "public host ok", endpoint: "https://s3.example.com", addrs: []string{"93.184.216.34"}},
		{name: "private allowed by env", endpoint: "http://127.0.0.1:9000", allow: true},
		{name: "userinfo rejected", endpoint: "https://user@s3.example.com", addrs: []string{"93.184.216.34"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(netguard.AllowPrivateEnv, fmt.Sprint(tt.allow))
			bc := &BuildCache{ResolveHost: func(context.Context, string, string) ([]netip.Addr, error) {
				var out []netip.Addr
				for _, a := range tt.addrs {
					out = append(out, netip.MustParseAddr(a))
				}
				return out, nil
			}}
			err := bc.checkEndpoint(context.Background(), tt.endpoint)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestBuildCacheClearBoundedAndPrefixScoped(t *testing.T) {
	e := newCacheEnv(t)
	ctx := context.Background()
	e.set(t, "web", true, "max")
	e.bc.Options.ClearMaxObjects = 1500
	now := time.Now()
	for i := 0; i < 2300; i++ {
		e.srv.Put(fmt.Sprintf("build-cache/web/blobs/%05d", i), []byte("x"), now)
	}
	e.srv.Put("build-cache/api/blobs/keep", []byte("x"), now)
	e.srv.Put("log-archive/keep", []byte("x"), now)

	res, err := e.bc.Clear(ctx, "web")
	if err != nil || res.Deleted != 1500 || !res.More {
		t.Fatalf("first clear = %+v err %v", res, err)
	}
	res, err = e.bc.Clear(ctx, "web")
	if err != nil || res.Deleted != 800 || res.More {
		t.Fatalf("second clear = %+v err %v", res, err)
	}
	keys := e.srv.Keys()
	if len(keys) != 2 || !strings.HasPrefix(keys[0], "build-cache/api/") {
		t.Fatalf("other prefixes must survive, got %v", keys)
	}
	s, err := e.db.GetBuildCacheSetting(ctx, "web")
	if err != nil || s.LastClearedAt == "" {
		t.Fatalf("cleared not stamped: %+v %v", s, err)
	}
}

func TestBuildCacheClearNotConfigured(t *testing.T) {
	e := newCacheEnv(t)
	if _, err := e.bc.Clear(context.Background(), "web"); !errors.Is(err, ErrBuildCacheNotConfigured) {
		t.Fatalf("err = %v", err)
	}
}

func TestBuildCacheStats(t *testing.T) {
	e := newCacheEnv(t)
	e.set(t, "web", true, "max")
	newest := time.Date(2026, 9, 24, 10, 0, 0, 0, time.UTC)
	e.srv.Put("build-cache/web/manifests/cache", []byte("abc"), newest)
	e.srv.Put("build-cache/web/blobs/a", []byte("de"), newest.Add(-time.Hour))
	st, err := e.bc.Stats(context.Background(), "web")
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if st.Objects != 2 || st.Bytes != 5 || st.Truncated || !st.LastModified.Equal(newest) {
		t.Errorf("stats = %+v", st)
	}

	e.bc.Options.StatsMaxObjects = 1
	for i := 0; i < 1200; i++ {
		e.srv.Put(fmt.Sprintf("build-cache/web/blobs/x%05d", i), []byte("z"), newest)
	}
	st, err = e.bc.Stats(context.Background(), "web")
	if err != nil || !st.Truncated {
		t.Fatalf("expected truncated, got %+v err %v", st, err)
	}
}

func TestBuildCacheRecordBuild(t *testing.T) {
	e := newCacheEnv(t)
	ctx := context.Background()
	e.set(t, "", true, "max")
	e.bc.RecordBuild(ctx, "web", "")
	g, _ := e.db.GetBuildCacheSetting(ctx, "")
	if g.LastResult != "ok" || g.LastBuildAt == "" || g.LastWarning != "" {
		t.Fatalf("ok result = %+v", g)
	}
	e.bc.RecordBuild(ctx, "web", "s3 denied")
	g, _ = e.db.GetBuildCacheSetting(ctx, "")
	if g.LastResult != "fallback" || g.LastWarning != "s3 denied" {
		t.Fatalf("fallback result = %+v", g)
	}
}

func TestBuildCacheOptionsFromEnv(t *testing.T) {
	env := map[string]string{EnvBuildCachePrefix: "/bc/", EnvBuildCacheClearMaxObjects: "7", EnvBuildCacheStatsMaxObjects: "x"}
	o := BuildCacheOptionsFromEnv(func(k string) (string, bool) { v, ok := env[k]; return v, ok })
	if o.Prefix != "bc" || o.ClearMaxObjects != 7 || o.StatsMaxObjects != DefaultBuildCacheOptions().StatsMaxObjects {
		t.Fatalf("options = %+v", o)
	}
}

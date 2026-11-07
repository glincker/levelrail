package deploy

import (
	"context"
	"testing"

	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

type caDBs struct {
	engine string
	mode   string
}

func (c caDBs) GetDesiredDatabase(_ context.Context, name string) (*store.DesiredDatabase, error) {
	return &store.DesiredDatabase{Name: name, Engine: c.engine}, nil
}

func (c caDBs) GetDatabaseTLSMode(context.Context, string) (string, error) { return c.mode, nil }

func caService(cache string) store.DesiredService {
	url := store.DatabaseEnvRef{Database: "cache", Field: "url"}
	return store.DesiredService{NextCache: cache, DatabaseEnv: map[string]store.DatabaseEnvRef{
		"REDIS_URL": url, nextCacheURLEnvVar: url,
	}}
}

func TestNextCacheCALink(t *testing.T) {
	ctx := context.Background()
	want := store.DatabaseEnvRef{Database: "cache", Field: "tls_ca"}

	d := caService(spec.NextCacheRedis)
	EnsureNextCacheCALink(ctx, &d, caDBs{engine: store.EngineRedis, mode: store.DatabaseTLSModeDefault})
	if d.DatabaseEnv[nextCacheCAEnvVar] != want {
		t.Fatalf("TLS on must link the CA: %v", d.DatabaseEnv)
	}

	d = caService(spec.NextCacheRedis)
	EnsureNextCacheCALink(ctx, &d, caDBs{engine: store.EngineRedis, mode: store.DatabaseTLSModeOff})
	if _, ok := d.DatabaseEnv[nextCacheCAEnvVar]; ok {
		t.Fatalf("TLS off must not link the CA: %v", d.DatabaseEnv)
	}

	d = caService(spec.NextCacheRedis)
	EnsureNextCacheCALink(ctx, &d, caDBs{engine: "unknown-kind", mode: store.DatabaseTLSModeOn})
	if _, ok := d.DatabaseEnv[nextCacheCAEnvVar]; ok {
		t.Fatalf("unsupported kind must skip: %v", d.DatabaseEnv)
	}

	d = caService(spec.NextCacheRedis)
	d.DatabaseEnv[nextCacheCAEnvVar] = want
	EnsureNextCacheCALink(ctx, &d, caDBs{engine: store.EngineRedis, mode: store.DatabaseTLSModeOff})
	if _, ok := d.DatabaseEnv[nextCacheCAEnvVar]; ok {
		t.Fatalf("TLS turned off must drop a stale CA link: %v", d.DatabaseEnv)
	}
}

func TestNextCacheNoneRemovesBoth(t *testing.T) {
	d := caService(spec.NextCacheNone)
	d.DatabaseEnv[nextCacheCAEnvVar] = store.DatabaseEnvRef{Database: "cache", Field: "tls_ca"}
	EnsureNextCacheLink(&d)
	if _, ok := d.DatabaseEnv[nextCacheURLEnvVar]; ok {
		t.Fatalf("url link kept: %v", d.DatabaseEnv)
	}
	if _, ok := d.DatabaseEnv[nextCacheCAEnvVar]; ok {
		t.Fatalf("CA link kept: %v", d.DatabaseEnv)
	}
	if _, ok := d.DatabaseEnv["REDIS_URL"]; !ok {
		t.Fatalf("the user's own REDIS_URL link must stay: %v", d.DatabaseEnv)
	}
}

func TestApplyNextCacheCAOnDeploy(t *testing.T) {
	url := store.DatabaseEnvRef{Database: "cache", Field: "url"}
	prev := &store.DesiredService{DatabaseEnv: map[string]store.DatabaseEnvRef{"REDIS_URL": url}}
	p := &Pipeline{store: linkStore{svc: prev, dbs: map[string]string{"cache": "redis"}}}
	req := Request{ServiceName: "web", Service: spec.Service{Next: &spec.Next{Cache: "redis"}}}
	d := store.DesiredService{}
	applyNextCache(&d, req, prev)
	p.applyNextCacheCA(context.Background(), req, &d)
	if d.DatabaseEnv[nextCacheCAEnvVar].Field != "tls_ca" {
		t.Fatalf("deploy must link the CA: %v", d.DatabaseEnv)
	}
}

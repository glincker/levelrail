package deploy

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

func nextCacheSource(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"dependencies":{"next":"15.0.0"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestNextBuildID(t *testing.T) {
	next, plain := nextCacheSource(t), t.TempDir()
	redis := &spec.Next{Cache: "redis"}
	cases := []struct {
		name string
		dir  string
		svc  spec.Service
		want string
	}{
		{"redis next app", next, spec.Service{Next: redis}, "d1"},
		{"user value wins", next, spec.Service{Next: redis, Env: map[string]spec.EnvVar{"NEXT_BUILD_ID": {Value: "mine"}}}, ""},
		{"not next", plain, spec.Service{Next: redis}, ""},
		{"cache none", next, spec.Service{Next: &spec.Next{Cache: "none"}}, ""},
		{"no next block", next, spec.Service{}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := nextBuildID(tc.dir, "d1", tc.svc); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestApplyNextCache(t *testing.T) {
	ref := store.DatabaseEnvRef{Database: "cache", Field: "url"}
	prev := &store.DesiredService{DatabaseEnv: map[string]store.DatabaseEnvRef{"REDIS_URL": ref}}
	redis := &spec.Next{Cache: "redis"}

	d := store.DesiredService{}
	applyNextCache(&d, Request{Service: spec.Service{Next: redis}, nextBuildID: "d1"}, prev)
	if d.Env["NEXT_BUILD_ID"] != "d1" || d.DatabaseEnv["LEVELRAIL_NEXT_CACHE_URL"] != ref {
		t.Fatalf("not wired: %+v", d)
	}

	d = store.DesiredService{}
	applyNextCache(&d, Request{Service: spec.Service{Next: &spec.Next{Cache: "none"}}}, prev)
	if d.Env != nil || d.DatabaseEnv != nil {
		t.Fatalf("cache none must not touch env: %+v", d)
	}

	d = store.DesiredService{}
	own := map[string]spec.EnvVar{"LEVELRAIL_NEXT_CACHE_URL": {Value: "redis://x"}}
	applyNextCache(&d, Request{Service: spec.Service{Next: redis, Env: own}}, prev)
	if d.DatabaseEnv != nil {
		t.Fatalf("user value must win: %+v", d)
	}
}

func TestNextCacheLink(t *testing.T) {
	if _, ok := nextCacheLink(nil, nil); ok {
		t.Fatal("no link must not be ok")
	}
	if _, ok := nextCacheLink(map[string]spec.EnvVar{"REDIS_URL": {From: "redis.c.url"}}, nil); !ok {
		t.Fatal("env REDIS_URL should count")
	}
}

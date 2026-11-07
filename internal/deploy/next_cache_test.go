package deploy

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/build"
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

type linkStore struct {
	ServiceStore
	svc *store.DesiredService
	dbs map[string]string
}

func (l linkStore) GetDesiredService(context.Context, string) (*store.DesiredService, error) {
	return l.svc, nil
}

func (l linkStore) GetDesiredDatabase(_ context.Context, name string) (*store.DesiredDatabase, error) {
	if e, ok := l.dbs[name]; ok {
		return &store.DesiredDatabase{Name: name, Engine: e}, nil
	}
	return nil, store.ErrDatabaseNotFound
}

func TestNextCacheLinkSources(t *testing.T) {
	url := store.DatabaseEnvRef{Database: "cache", Field: "url"}
	redisDBs := map[string]string{"cache": "redis"}
	userEnv := map[string]spec.EnvVar{"REDIS_URL": {Value: "redis://x"}}
	cases := []struct {
		name    string
		svc     *store.DesiredService
		dbs     map[string]string
		env     map[string]spec.EnvVar
		wantErr bool
		wantRef bool
	}{
		{"current link only", &store.DesiredService{DatabaseEnv: map[string]store.DatabaseEnvRef{"REDIS_URL": url}}, redisDBs, nil, false, true},
		{"custom var name on redis db", &store.DesiredService{DatabaseEnv: map[string]store.DatabaseEnvRef{"CACHE_URL": url}}, redisDBs, nil, false, true},
		{"custom var on non redis db", &store.DesiredService{DatabaseEnv: map[string]store.DatabaseEnvRef{"DB_URL": {Database: "pg", Field: "url"}}}, map[string]string{"pg": "postgres"}, nil, true, false},
		{"app yaml only", &store.DesiredService{}, nil, userEnv, false, false},
		{"none", nil, nil, nil, true, false},
		{"user value wins", &store.DesiredService{DatabaseEnv: map[string]store.DatabaseEnvRef{"REDIS_URL": url}}, redisDBs, map[string]spec.EnvVar{"LEVELRAIL_NEXT_CACHE_URL": {Value: "redis://u"}}, false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := &Pipeline{store: linkStore{svc: c.svc, dbs: c.dbs}}
			req := Request{ServiceName: "web", Service: spec.Service{Next: &spec.Next{Cache: "redis"}, Env: c.env}}
			err := p.checkNextCache(context.Background(), nextCacheSource(t), req)
			if (err != nil) != c.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, c.wantErr)
			}
			if err != nil && !strings.Contains(err.Error(), "linked to web") {
				t.Fatalf("error must name the app: %v", err)
			}
			d := store.DesiredService{}
			applyNextCache(&d, req, p.nextCachePrev(context.Background(), "web"))
			if got := d.DatabaseEnv["LEVELRAIL_NEXT_CACHE_URL"] == url; got != c.wantRef {
				t.Fatalf("cache ref set = %v, want %v", got, c.wantRef)
			}
		})
	}
}

func TestAddNextDockerfileArgs(t *testing.T) {
	dir := nextCacheSource(t)
	write := func(body string) string {
		p := filepath.Join(dir, "Dockerfile")
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	req := Request{AttemptID: "att1", Service: spec.Service{Next: &spec.Next{Cache: "redis"}}}

	var logs []string
	progress := func(e build.ProgressEvent) { logs = append(logs, e.Log) }

	declared := write("FROM node\nARG NEXT_DEPLOYMENT_ID\nARG NEXT_BUILD_ID\n")
	r := req
	got := addNextDockerfileArgs(context.Background(), &r, dir, declared, map[string]string{"A": "b"}, progress)
	if got["NEXT_DEPLOYMENT_ID"] == "" || got["NEXT_BUILD_ID"] == "" || got["A"] != "b" || r.nextDeploymentID == "" {
		t.Fatalf("declared ARGs must be injected: %+v", got)
	}

	logs = nil
	missing := write("FROM node\n")
	r = req
	got = addNextDockerfileArgs(context.Background(), &r, dir, missing, nil, progress)
	if len(got) != 0 || r.nextDeploymentID != "" || len(logs) < 1 || !strings.Contains(logs[0], "ARG NEXT_DEPLOYMENT_ID") {
		t.Fatalf("undeclared ARG must log a hint and inject nothing: %+v %v", got, logs)
	}

	// Runtime cache env is shared with the Dockerfile path through finishDeploy.
	d := store.DesiredService{}
	ref := store.DatabaseEnvRef{Database: "cache", Field: "url"}
	applyNextCache(&d, req, &store.DesiredService{DatabaseEnv: map[string]store.DatabaseEnvRef{"REDIS_URL": ref}})
	if d.DatabaseEnv["LEVELRAIL_NEXT_CACHE_URL"] != ref {
		t.Fatalf("redis cache env not applied: %+v", d)
	}
}

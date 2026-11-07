package deploy

import (
	"context"
	"testing"

	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

func specEnvOf(env map[string]string) map[string]spec.EnvVar {
	out := map[string]spec.EnvVar{}
	for k, v := range env {
		out[k] = spec.EnvVar{Value: v}
	}
	return out
}

func TestGeneratedIDsRegenerateEachDeploy(t *testing.T) {
	t.Setenv(EnvNextDeploymentID, "true")
	src := nextSource(t, true)
	ctx := context.Background()

	d1 := store.DesiredService{Env: map[string]string{"A": "1"}}
	id1 := nextDeploymentID(ctx, src, "att1", nil)
	req1 := Request{nextDeploymentID: id1}
	d1.Env[nextDeploymentIDEnvVar] = id1
	markGeneratedEnv(&d1, req1)

	rebuilt := StripGeneratedEnv(d1.Env)
	if _, ok := rebuilt[nextDeploymentIDEnvVar]; ok || rebuilt["A"] != "1" {
		t.Fatalf("generated id should be stripped, user env kept: %v", rebuilt)
	}
	id2 := nextDeploymentID(ctx, src, "att2", specEnvOf(rebuilt))
	if id2 != "att2" || id2 == d1.Env[nextDeploymentIDEnvVar] {
		t.Fatalf("second deploy id = %q, want fresh att2", id2)
	}
	// The grace window reads the previous id from the stored release.
	if old := d1.Env[nextDeploymentIDEnvVar]; old != "att1" {
		t.Fatalf("previous release id = %q", old)
	}
}

func TestUserSetIDSurvivesStrip(t *testing.T) {
	d := store.DesiredService{Env: map[string]string{nextDeploymentIDEnvVar: "att1"}}
	markGeneratedEnv(&d, Request{nextDeploymentID: "att1"})
	d.Env[nextDeploymentIDEnvVar] = "mine"
	if got := StripGeneratedEnv(d.Env)[nextDeploymentIDEnvVar]; got != "mine" {
		t.Fatalf("user value lost: %q", got)
	}
}

func TestEnsureNextCacheLink(t *testing.T) {
	ref := store.DatabaseEnvRef{Database: "c", Field: "url"}
	d := store.DesiredService{NextCache: spec.NextCacheRedis, DatabaseEnv: map[string]store.DatabaseEnvRef{"REDIS_URL": ref}}
	EnsureNextCacheLink(&d)
	if d.DatabaseEnv[nextCacheURLEnvVar] != ref {
		t.Fatalf("link not set: %v", d.DatabaseEnv)
	}
	d.NextCache = spec.NextCacheNone
	EnsureNextCacheLink(&d)
	if _, ok := d.DatabaseEnv[nextCacheURLEnvVar]; ok {
		t.Fatalf("link not dropped")
	}
}

package deploy

import (
	"context"
	"testing"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

type externalAwareServiceStore struct {
	*fakeServiceStore
	external map[string]store.ExternalDatabase
}

func (f *externalAwareServiceStore) GetExternalDatabase(_ context.Context, name string) (*store.ExternalDatabase, error) {
	d, ok := f.external[name]
	if !ok {
		return nil, store.ErrExternalDatabaseNotFound
	}
	return &d, nil
}

func TestPipeline_Deploy_FromReference_ExternalDatabase(t *testing.T) {
	cases := []struct {
		name    string
		env     map[string]spec.EnvVar
		wantErr bool
	}{
		{"url resolves", map[string]spec.EnvVar{"DATABASE_URL": {From: "legacy.url"}}, false},
		{"password resolves for redis too", map[string]spec.EnvVar{"REDIS_PASSWORD": {From: "cache.password"}}, false},
		{"username on redis rejected", map[string]spec.EnvVar{"U": {From: "cache.username"}}, true},
		{"unknown database rejected", map[string]spec.EnvVar{"X": {From: "nope.url"}}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			builder := &fakeBuilder{result: &build.Result{Tag: "x:y"}}
			st := &externalAwareServiceStore{
				fakeServiceStore: &fakeServiceStore{},
				external: map[string]store.ExternalDatabase{
					"legacy": {Name: "legacy", Engine: store.EnginePostgres},
					"cache":  {Name: "cache", Engine: store.EngineRedis},
				},
			}
			p := New(builder, st)
			svc := dockerfileService()
			svc.Env = tc.env
			_, err := p.Deploy(context.Background(), Request{ServiceName: "web", Service: svc, SourceDir: "/repo", CommitSHA: "abc1234", ImageRepo: "levelrail/web"}, nil)
			if (err != nil) != tc.wantErr {
				t.Fatalf("Deploy() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

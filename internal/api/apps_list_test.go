package api

import (
	"context"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

// countingEnvStore wraps a fixed environment set and counts calls to
// GetEnvironment, the per-row lookup environmentNames used to make once
// per distinct environment before it switched to GetEnvironmentsByIDs.
type countingEnvStore struct {
	EnvironmentStore
	envs           map[string]store.Environment
	getEnvironment int
	getByIDs       int
}

func (f *countingEnvStore) GetEnvironment(_ context.Context, id string) (store.Environment, error) {
	f.getEnvironment++
	e, ok := f.envs[id]
	if !ok {
		return store.Environment{}, store.ErrEnvironmentNotFound
	}
	return e, nil
}

func (f *countingEnvStore) GetEnvironmentsByIDs(_ context.Context, ids []string) (map[string]store.Environment, error) {
	f.getByIDs++
	out := make(map[string]store.Environment, len(ids))
	for _, id := range ids {
		if e, ok := f.envs[id]; ok {
			out[id] = e
		}
	}
	return out, nil
}

// TestEnvironmentNames_BatchesInsteadOfPerRow proves environmentNames
// makes exactly one GetEnvironmentsByIDs call regardless of how many
// services share a handful of environments, and never calls the
// per-row GetEnvironment at all: the N+1 handleListApps used to make
// (one GetEnvironment call per distinct environment on the page).
func TestEnvironmentNames_BatchesInsteadOfPerRow(t *testing.T) {
	fake := &countingEnvStore{
		envs: map[string]store.Environment{
			"env_a": {ID: "env_a", Name: "staging"},
			"env_b": {ID: "env_b", Name: "production"},
		},
	}
	rt := &Router{environments: fake}

	svcs := make([]store.DesiredService, 0, 20)
	for i := 0; i < 10; i++ {
		svcs = append(svcs, store.DesiredService{Name: "app-a", EnvironmentID: "env_a"})
		svcs = append(svcs, store.DesiredService{Name: "app-b", EnvironmentID: "env_b"})
	}
	svcs = append(svcs, store.DesiredService{Name: "app-c", EnvironmentID: ""})

	names := rt.environmentNames(context.Background(), svcs)

	if fake.getByIDs != 1 {
		t.Errorf("GetEnvironmentsByIDs calls = %d, want 1 (one batched call regardless of row count)", fake.getByIDs)
	}
	if fake.getEnvironment != 0 {
		t.Errorf("GetEnvironment calls = %d, want 0 (per-row lookup should never run)", fake.getEnvironment)
	}
	if names["env_a"] != "staging" || names["env_b"] != "production" {
		t.Errorf("names = %+v, want env_a=staging env_b=production", names)
	}
	if _, ok := names[""]; ok {
		t.Errorf("names unexpectedly contains an entry for empty environment id")
	}
}

package application

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/GLINCKER/levelrail/internal/rolloutpolicy"
	"github.com/GLINCKER/levelrail/internal/store"
)

type progressStore struct {
	*fakeStore
	mu   sync.Mutex
	rows map[string][]byte
}

func (p *progressStore) SaveRolloutProgress(_ context.Context, name string, payload []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.rows[name] = payload
	return nil
}

func (p *progressStore) LoadRolloutProgress(_ context.Context, name string) ([]byte, bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	b, ok := p.rows[name]
	return b, ok, nil
}

func TestRollout_ProgressPersistedAndRestored(t *testing.T) {
	desired := &store.DesiredService{Name: "durable-a", Image: "img:v1", Port: 80, Strategy: "rolling", Replicas: 2}
	ps := &progressStore{fakeStore: &fakeStore{svc: desired}, rows: map[string][]byte{}}
	c := New("durable-a", ps, newFakeRuntime(0))
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	var saved rolloutpolicy.Progress
	if err := json.Unmarshal(ps.rows["durable-a"], &saved); err != nil {
		t.Fatalf("saved payload: %v", err)
	}
	if saved.Phase != rolloutpolicy.PhaseComplete || saved.Updated != 2 {
		t.Fatalf("saved = %+v, want complete with 2 updated", saved)
	}

	// A fresh process has no in-memory entry: an interrupted rollout is restored.
	raw, _ := json.Marshal(rolloutpolicy.Progress{Active: true, Phase: rolloutpolicy.PhaseRolling, Desired: 2, Updated: 1})
	ps2 := &progressStore{fakeStore: &fakeStore{svc: &store.DesiredService{Name: "durable-b", Image: "img:v1", Port: 80, Strategy: "rolling", Replicas: 2}}, rows: map[string][]byte{"durable-b": raw}}
	if !New("durable-b", ps2, newFakeRuntime(0)).hydrateProgress(context.Background()) {
		t.Fatal("hydrateProgress() = false, want the active rollout restored")
	}
	if p, ok := rolloutpolicy.Lookup("durable-b"); !ok || p.Updated != 1 {
		t.Fatalf("Lookup = %+v, %v, want the restored progress", p, ok)
	}
}

func TestRollout_WaveStartsReplicasInParallelSafely(t *testing.T) {
	t.Setenv(rolloutpolicy.EnvMaxSurge, "3")
	rt := newFakeRuntime(0)
	desired := &store.DesiredService{Name: "wavefast", Image: "img:v1", Port: 80, Strategy: "rolling", Replicas: 4}
	c := New("wavefast", &fakeStore{svc: desired}, rt)
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if rt.createCalls != 4 || rt.count() != 4 {
		t.Fatalf("createCalls = %d, count = %d, want 4 and 4", rt.createCalls, rt.count())
	}
}

func TestRollout_CrashedReplicaDoesNotCountTowardFloor(t *testing.T) {
	t.Setenv(rolloutpolicy.EnvMaxSurge, "1")
	rt := newFakeRuntime(0)
	rt.seed(replicaContainerName("floorapp", "img:v1", "", 0), true)
	rt.seed(replicaContainerName("floorapp", "img:v1", "", 1), false)
	desired := &store.DesiredService{Name: "floorapp", Image: "img:v2", Port: 80, Strategy: "rolling", Replicas: 2}
	c := New("floorapp", &fakeStore{svc: desired}, rt)
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	for i := 0; i < 2; i++ {
		if _, ok := rt.containers[replicaContainerName("floorapp", "img:v2", "", i)]; !ok {
			t.Errorf("replica %d of v2 missing among %v", i, rt.names())
		}
	}
}

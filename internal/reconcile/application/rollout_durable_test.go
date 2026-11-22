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

func TestRollout_PauseHoldsAfterWaveAndResumes(t *testing.T) {
	t.Setenv(rolloutpolicy.EnvMaxSurge, "1")
	t.Cleanup(func() { rolloutpolicy.ClearHold("pausesvc") })
	rolloutpolicy.Record("pausesvc", rolloutpolicy.Progress{Active: true, Phase: rolloutpolicy.PhaseRolling})
	if _, ok := rolloutpolicy.RequestPause("pausesvc"); !ok {
		t.Fatal("RequestPause refused an active rollout")
	}
	rt := newFakeRuntime(0)
	desired := &store.DesiredService{Name: "pausesvc", Image: "img:v1", Port: 80, Strategy: "rolling", Replicas: 2}
	c := New("pausesvc", &fakeStore{svc: desired}, rt)

	res, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("held Reconcile() error = %v", err)
	}
	if rt.count() != 1 {
		t.Fatalf("containers while paused = %d, want 1 (one wave done)", rt.count())
	}
	if p, _ := rolloutpolicy.Lookup("pausesvc"); !p.Paused || p.Phase != rolloutpolicy.PhasePaused || p.Wave != 2 {
		t.Fatalf("progress = %+v, want paused before wave 2", p)
	}
	if len(res.Conditions) == 0 || res.Conditions[0].Reason != "RolloutPaused" {
		t.Fatalf("conditions = %+v, want RolloutPaused", res.Conditions)
	}

	if _, err := c.Reconcile(context.Background()); err != nil || rt.count() != 1 {
		t.Fatalf("second held pass: err = %v, containers = %d, want still 1", err, rt.count())
	}
	if _, ok := rolloutpolicy.Resume("pausesvc"); !ok {
		t.Fatal("Resume found nothing to resume")
	}
	if _, err := c.Reconcile(context.Background()); err != nil || rt.count() != 2 {
		t.Fatalf("resumed pass: err = %v, containers = %d, want 2", err, rt.count())
	}
	if p, _ := rolloutpolicy.Lookup("pausesvc"); p.Paused || p.Phase != rolloutpolicy.PhaseComplete {
		t.Fatalf("progress after resume = %+v, want complete and not paused", p)
	}
}

package database

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/dbparams"
	"github.com/GLINCKER/levelrail/internal/dbprovision"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestParamsWatchVerdict(t *testing.T) {
	w := paramsWatch{vanished: 1}
	if settled, _ := watchVerdict("VanishedAfterStart", w, 1, true); settled {
		t.Fatal("one exit right after start must not settle the watch")
	}
	w.vanished = 2
	if _, blame := watchVerdict("VanishedAfterStart", w, 0, true); blame {
		t.Fatal("clean exits do not blame the new args")
	}
	if _, blame := watchVerdict("VanishedAfterStart", w, 1, true); !blame {
		t.Fatal("repeated failing exits blame the new args")
	}
	for _, r := range []string{reasonNotAccepting, dbprovision.ReasonCrashLoop} {
		if settled, blame := watchVerdict(r, w, 0, false); !settled || !blame {
			t.Fatalf("%s must blame the new args", r)
		}
	}
	for _, r := range []string{reasonVolumeFailed, "ReplaceFailed", "Starting"} {
		if settled, _ := watchVerdict(r, w, 1, true); settled {
			t.Fatalf("%s must not settle the watch", r)
		}
	}
}

func TestParamsRollbackEligible(t *testing.T) {
	applied := map[string]string{"maxclients": "500"}
	label := dbparams.Fingerprint(dbparams.Args("redis", applied, dbparams.Limits{}))
	base := store.DatabaseEngineParams{Applied: applied, Previous: map[string]string{}, AppliedAt: "2026-11-09T12:00:00Z"}
	if !paramsRollbackEligible("redis", base, label) {
		t.Fatal("a failed watched start of the applied set must roll back")
	}
	rolled := base
	rolled.RolledBackAt = "2026-11-09T12:05:00Z"
	same := base
	same.Previous = applied
	for name, p := range map[string]store.DatabaseEngineParams{"already rolled back": rolled, "previous is the same set": same} {
		if paramsRollbackEligible("redis", p, label) {
			t.Errorf("%s: rolled back", name)
		}
	}
	if paramsRollbackEligible("redis", base, "other") {
		t.Error("args that are not the applied set must not roll back")
	}
}

// paramsStore is an in memory store with the rollback compare and set.
type paramsStore struct {
	mu        sync.Mutex
	db        store.DesiredDatabase
	p         store.DatabaseEngineParams
	rollbacks int
}

func (s *paramsStore) GetDesiredDatabase(context.Context, string) (*store.DesiredDatabase, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d := s.db
	d.EngineParams = ""
	if len(s.p.Applied) > 0 {
		raw, _ := json.Marshal(s.p.Applied)
		d.EngineParams = store.EngineParamSet(raw)
	}
	return &d, nil
}

func (s *paramsStore) GetDatabaseEngineParams(context.Context, string) (store.DatabaseEngineParams, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.p, nil
}

func (s *paramsStore) RollbackDatabaseEngineParams(_ context.Context, _ string, _ map[string]string, appliedAt, reason string, at time.Time, _ store.AuditEntry) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.p.RolledBackAt != "" || s.p.AppliedAt != appliedAt {
		return false, nil
	}
	s.p.Applied, s.p.Previous = s.p.Previous, s.p.Applied
	s.p.RolledBackAt, s.p.RolledBackReason = at.Format(time.RFC3339), reason
	s.rollbacks++
	return true, nil
}

// addressedRuntime gives every created container an address so the ready probe runs.
type addressedRuntime struct{ *fakeRuntime }

func (r *addressedRuntime) Create(ctx context.Context, spec docker.ContainerSpec) (string, error) {
	id, err := r.fakeRuntime.Create(ctx, spec)
	if err == nil {
		r.mu.Lock()
		r.containers[spec.Name].Networks = []docker.ContainerNetwork{{Name: "n", IPAddress: "10.0.0.5"}}
		r.mu.Unlock()
	}
	return id, err
}

// runBadApply deploys a ready database, applies a set the engine never
// listens with (and optionally a new version), then runs passes over the ready
// timeout with a fresh Controller each pass, as the boot wiring does.
func runBadApply(t *testing.T, newVersion string, opts ...Option) (*paramsStore, *addressedRuntime) {
	t.Helper()
	t.Setenv(EnvReadyTimeout, "1m")
	start := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	now := start
	var listening sync.Map
	listening.Store("on", true)
	probe := func(context.Context, string) error {
		if on, _ := listening.Load("on"); on == true {
			return nil
		}
		return errors.New("refused")
	}
	st := &paramsStore{db: store.DesiredDatabase{Name: "main", Engine: store.EngineRedis, Version: "7"}, p: store.DatabaseEngineParams{Applied: map[string]string{}, Previous: map[string]string{}}}
	rt := &addressedRuntime{newFakeRuntime()}
	shared := NewSharedState()
	pass := func() {
		c := New("main", st, rt, append([]Option{WithReadyProbe(probe), WithSharedState(shared)}, opts...)...)
		c.now = func() time.Time { return now }
		if _, err := c.Reconcile(context.Background()); err != nil {
			t.Fatalf("pass at %s: %v", now.Sub(start), err)
		}
	}
	pass()

	st.mu.Lock()
	st.p = store.DatabaseEngineParams{Applied: map[string]string{"maxclients": "500"}, Previous: map[string]string{}, AppliedAt: now.Format(time.RFC3339Nano)}
	if newVersion != "" {
		st.db.Version = newVersion
	}
	st.mu.Unlock()
	listening.Store("on", false)
	for _, at := range []time.Duration{0, 2 * time.Minute, 2 * time.Minute, 4 * time.Minute, 6 * time.Minute} {
		now = start.Add(at)
		pass()
	}
	return st, rt
}

func TestParamsRollback_ReadyDatabaseBadSetRollsBackOnce(t *testing.T) {
	st, rt := runBadApply(t, "")
	if st.rollbacks != 1 {
		t.Fatalf("rollbacks = %d, want exactly 1", st.rollbacks)
	}
	if st.p.Previous["maxclients"] != "500" || len(st.p.Applied) != 0 {
		t.Fatalf("params after rollback = %+v", st.p)
	}
	if !rt.volumes[dataVolumeName("main")] {
		t.Fatal("data volume was touched")
	}
}

func TestParamsRollback_NotWithImageChangeOrRemoteNode(t *testing.T) {
	if st, _ := runBadApply(t, "7.2"); st.rollbacks != 0 {
		t.Fatalf("version change failing rolled back params %d times", st.rollbacks)
	}
	if st, _ := runBadApply(t, "", WithRemoteNode(true)); st.rollbacks != 0 {
		t.Fatalf("remote node rolled back params %d times", st.rollbacks)
	}
}

func TestSharedStateRetainForgetsDeletedDatabases(t *testing.T) {
	s := NewSharedState()
	s.setWatch("gone", paramsWatch{label: "x"})
	s.setWatch("kept", paramsWatch{label: "y"})
	_ = s.guard("gone")
	s.Retain([]string{"kept"})
	if _, ok := s.watch("gone"); ok || len(s.guards) != 0 {
		t.Fatal("state of a deleted database was kept")
	}
	if _, ok := s.watch("kept"); !ok {
		t.Fatal("state of a live database was dropped")
	}
}

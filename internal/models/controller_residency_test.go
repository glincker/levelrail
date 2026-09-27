package models

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/gpu"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

var residencyNow = time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)

func onDemandModel(lastActive time.Time, state string) *store.Model {
	m := testModel()
	m.Residency = store.ResidencyOnDemand
	m.IdleTTLSeconds = 600
	m.LastActiveAt = lastActive
	m.ResidencyState = state
	return m
}

func residencyController(st *fakeStore, rt *fakeRuntime, node NodeInfo, probe Status) *Controller {
	return New("chat", st, fakeNodes{info: node}, rt, fakeProber{probe}, WithClock(func() time.Time { return residencyNow }))
}

func runningContainer(rt *fakeRuntime, c *Controller, st *fakeStore) string {
	name := c.containerName(st.model, c.imageFor(st.model.Engine), false)
	rt.containers[name] = &docker.ContainerState{ID: "id-" + name, Name: name, Running: true,
		Ports: []docker.PortBinding{{ContainerPort: 11434, HostPort: 40000}}}
	return name
}

func TestResidency_IdleStopsTheEngineOnce(t *testing.T) {
	st := &fakeStore{model: onDemandModel(residencyNow.Add(-11*time.Minute), store.ResidencyAwake)}
	rt := newFakeRuntime()
	c := residencyController(st, rt, goodNode, Status{Phase: PhaseReady})
	name := runningContainer(rt, c, st)

	res, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if cond := reason(t, res); cond.Reason != "Idle" || cond.Status != reconcile.ConditionFalse {
		t.Fatalf("condition = %+v", cond)
	}
	if len(rt.stopped) != 1 || rt.containers[name].Running {
		t.Fatalf("stopped = %v, running = %v", rt.stopped, rt.containers[name].Running)
	}
	if st.model.ResidencyState != store.ResidencyAsleep {
		t.Fatalf("state = %q, want asleep", st.model.ResidencyState)
	}

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(rt.stopped) != 1 || len(rt.started) != 0 || len(rt.created) != 0 {
		t.Fatalf("an idle pass must be a no-op: stopped=%v started=%v created=%d", rt.stopped, rt.started, len(rt.created))
	}
}

func TestResidency_RecentUseKeepsTheEngineRunning(t *testing.T) {
	st := &fakeStore{model: onDemandModel(residencyNow.Add(-2*time.Minute), store.ResidencyAwake)}
	rt := newFakeRuntime()
	c := residencyController(st, rt, goodNode, Status{Phase: PhaseReady})
	runningContainer(rt, c, st)

	res, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if cond := reason(t, res); cond.Reason != "ModelLoaded" || len(rt.stopped) != 0 {
		t.Fatalf("condition = %+v stopped=%v", cond, rt.stopped)
	}
}

func TestResidency_WakeStartsThenMarksAwakeWhenReady(t *testing.T) {
	st := &fakeStore{model: onDemandModel(residencyNow.Add(-time.Second), store.ResidencyAsleep)}
	rt := newFakeRuntime()
	probe := Status{Phase: PhaseLoading, Detail: "loading weights"}
	c := residencyController(st, rt, goodNode, probe)
	name := c.containerName(st.model, c.imageFor(st.model.Engine), false)
	rt.containers[name] = &docker.ContainerState{ID: "id-" + name, Name: name}

	res, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if cond := reason(t, res); cond.Reason != "WakingUp" {
		t.Fatalf("first wake pass = %+v, want WakingUp", cond)
	}
	if st.model.ResidencyState != store.ResidencyWaking || len(rt.started) != 1 {
		t.Fatalf("state = %q started = %v", st.model.ResidencyState, rt.started)
	}

	res, err = c.Reconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if cond := reason(t, res); cond.Reason != "WakingUp" || cond.Message != "loading weights" {
		t.Fatalf("while loading = %+v", cond)
	}

	c.prober = fakeProber{Status{Phase: PhaseReady}}
	res, err = c.Reconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if cond := reason(t, res); cond.Reason != "ModelLoaded" || cond.Status != reconcile.ConditionTrue {
		t.Fatalf("when ready = %+v", cond)
	}
	if st.model.ResidencyState != store.ResidencyAwake {
		t.Fatalf("state = %q, want awake", st.model.ResidencyState)
	}
}

func TestResidency_StopSucceededButStateWriteFailedRecovers(t *testing.T) {
	st := &fakeStore{model: onDemandModel(residencyNow.Add(-time.Hour), store.ResidencyAwake), stateErr: errors.New("db busy")}
	rt := newFakeRuntime()
	c := residencyController(st, rt, goodNode, Status{Phase: PhaseReady})
	name := runningContainer(rt, c, st)

	res, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("a failed state write must surface")
	}
	if cond := reason(t, res); cond.Reason != "StoreError" {
		t.Fatalf("condition = %+v", cond)
	}
	if rt.containers[name].Running {
		t.Fatal("the engine was already stopped when the write failed")
	}

	st.stateErr = nil
	res, err = c.Reconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if cond := reason(t, res); cond.Reason != "Idle" || st.model.ResidencyState != store.ResidencyAsleep {
		t.Fatalf("recovered = %+v state=%q", cond, st.model.ResidencyState)
	}
	if len(rt.stopped) != 1 {
		t.Fatalf("stop must not repeat: %v", rt.stopped)
	}
}

func TestResidency_StopFailureIsRetried(t *testing.T) {
	st := &fakeStore{model: onDemandModel(residencyNow.Add(-time.Hour), store.ResidencyAwake)}
	rt := newFakeRuntime()
	rt.stopErr = errors.New("daemon busy")
	c := residencyController(st, rt, goodNode, Status{Phase: PhaseReady})
	runningContainer(rt, c, st)

	res, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("expected the stop error")
	}
	if cond := reason(t, res); cond.Reason != "StopFailed" || st.model.ResidencyState != store.ResidencyAwake {
		t.Fatalf("condition = %+v state=%q", cond, st.model.ResidencyState)
	}
}

func TestResidency_RequestLandingDuringSleepDecisionWins(t *testing.T) {
	st := &fakeStore{model: onDemandModel(residencyNow.Add(-time.Hour), store.ResidencyAwake)}
	rt := newFakeRuntime()
	c := residencyController(st, rt, goodNode, Status{Phase: PhaseReady})
	runningContainer(rt, c, st)
	// The gateway touches the model between this pass's first read and the
	// re-check made just before stopping.
	st.onRead = func(n int, m *store.Model) {
		if n == 2 {
			m.LastActiveAt = residencyNow.Add(-time.Second)
		}
	}

	res, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(rt.stopped) != 0 {
		t.Fatalf("the engine must not be stopped under a fresh request: %v", rt.stopped)
	}
	if cond := reason(t, res); cond.Reason != "ModelLoaded" {
		t.Fatalf("condition = %+v", cond)
	}
}

func TestResidency_WakeWaitsForGPUHeldByOthers(t *testing.T) {
	full := NodeInfo{BindIP: "127.0.0.1", DialHost: "127.0.0.1", GPU: gpu.Info{Present: true, RuntimeInstalled: true,
		Devices: []gpu.Device{{Index: 0, UUID: "GPU-a", VRAMTotalMiB: 24576, VRAMUsedMiB: 23000}}}}
	empty := full
	empty.GPU.Devices = []gpu.Device{{Index: 0, UUID: "GPU-a", VRAMTotalMiB: 24576, VRAMUsedMiB: 200}}
	tests := []struct {
		name       string
		ref        string
		node       NodeInfo
		wantReason string
		wantStart  bool
	}{
		{"fits an empty GPU but another workload holds it", "llama3.1:8b", full, "WaitingForGPU", false},
		{"fits and there is room", "llama3.1:8b", empty, "WakingUp", true},
		{"too big even for an empty GPU is not held back", "llama3.1:70b", full, "WakingUp", true},
		{"unknown size is not held back", "mistral", full, "WakingUp", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := onDemandModel(residencyNow.Add(-time.Second), store.ResidencyAsleep)
			m.ModelRef = tc.ref
			st := &fakeStore{model: m}
			rt := newFakeRuntime()
			c := residencyController(st, rt, tc.node, Status{Phase: PhaseLoading})
			name := c.containerName(m, c.imageFor(m.Engine), false)
			rt.containers[name] = &docker.ContainerState{ID: "id-" + name, Name: name}

			res, err := c.Reconcile(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if cond := reason(t, res); cond.Reason != tc.wantReason {
				t.Fatalf("reason = %q (%s), want %q", cond.Reason, cond.Message, tc.wantReason)
			}
			if (len(rt.started) == 1) != tc.wantStart {
				t.Fatalf("started = %v, want start=%v", rt.started, tc.wantStart)
			}
			if !tc.wantStart && st.model.ResidencyState != store.ResidencyAsleep {
				t.Fatalf("a held-back wake must stay asleep, got %q", st.model.ResidencyState)
			}
		})
	}
}

func TestResidency_AlwaysResidentIgnoresIdleness(t *testing.T) {
	m := testModel()
	m.LastActiveAt = residencyNow.Add(-24 * time.Hour)
	st := &fakeStore{model: m}
	rt := newFakeRuntime()
	c := residencyController(st, rt, goodNode, Status{Phase: PhaseReady})
	runningContainer(rt, c, st)

	res, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if cond := reason(t, res); cond.Reason != "ModelLoaded" || len(rt.stopped) != 0 {
		t.Fatalf("condition = %+v stopped=%v", cond, rt.stopped)
	}
}

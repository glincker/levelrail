package models

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/gpu"
	"github.com/GLINCKER/levelrail/internal/store"
)

// dynamicNodes returns before on its first call and after on every
// later call, simulating a GPU's free VRAM changing once a sibling is
// stopped: waitForGPUWithSwap re-reads node info after evicting, so a
// test can prove it actually rechecks fit against the freed VRAM
// instead of the stale snapshot it started with.
type dynamicNodes struct {
	before, after NodeInfo
	calls         int
}

func (d *dynamicNodes) NodeInfo(context.Context, string) (NodeInfo, error) {
	d.calls++
	if d.calls == 1 {
		return d.before, nil
	}
	return d.after, nil
}

func fullGPUNode() NodeInfo {
	return NodeInfo{BindIP: "127.0.0.1", DialHost: "127.0.0.1", GPU: gpu.Info{Present: true, RuntimeInstalled: true,
		Devices: []gpu.Device{{Index: 0, UUID: "GPU-a", VRAMTotalMiB: 24576, VRAMUsedMiB: 23000}}}}
}

func emptyGPUNode() NodeInfo {
	n := fullGPUNode()
	n.GPU.Devices = []gpu.Device{{Index: 0, UUID: "GPU-a", VRAMTotalMiB: 24576, VRAMUsedMiB: 200}}
	return n
}

func swapModel(name, residencyState string) store.Model {
	m := onDemandModel(residencyNow.Add(-time.Second), residencyState)
	m.Name = name
	m.SwapGroup = "gpu0"
	return *m
}

func TestSwapGroup_EvictsResidentSiblingThenWakes(t *testing.T) {
	m := swapModel("chat", store.ResidencyAsleep)
	sibling := swapModel("other", store.ResidencyAwake)
	st := &fakeStore{model: &m, swapGroupModels: []store.Model{sibling}}
	rt := newFakeRuntime()
	nodes := &dynamicNodes{before: fullGPUNode(), after: emptyGPUNode()}
	c := New("chat", st, nodes, rt, fakeProber{Status{Phase: PhaseLoading}}, WithClock(func() time.Time { return residencyNow }))

	siblingContainer := c.containerName(&sibling, c.imageFor(sibling.Engine), false)
	rt.containers[siblingContainer] = &docker.ContainerState{ID: "id-" + siblingContainer, Name: siblingContainer, Running: true}

	res, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if cond := reason(t, res); cond.Reason != "WakingUp" {
		t.Fatalf("reason = %q (%s), want WakingUp", cond.Reason, cond.Message)
	}
	if len(rt.stopped) != 1 || rt.stopped[0] != "id-"+siblingContainer {
		t.Fatalf("stopped = %v, want the sibling's container stopped", rt.stopped)
	}
	if len(rt.started) != 1 {
		t.Fatalf("started = %v, want chat's engine to start once the sibling freed VRAM", rt.started)
	}
}

func TestSwapGroup_NoEvictableSiblingStillWaits(t *testing.T) {
	m := swapModel("chat", store.ResidencyAsleep)
	// The only "sibling" is asleep already: nothing to evict.
	sibling := swapModel("other", store.ResidencyAsleep)
	st := &fakeStore{model: &m, swapGroupModels: []store.Model{sibling}}
	rt := newFakeRuntime()
	nodes := &dynamicNodes{before: fullGPUNode(), after: fullGPUNode()}
	c := New("chat", st, nodes, rt, fakeProber{Status{Phase: PhaseLoading}}, WithClock(func() time.Time { return residencyNow }))

	res, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if cond := reason(t, res); cond.Reason != "WaitingForGPU" {
		t.Fatalf("reason = %q, want WaitingForGPU", cond.Reason)
	}
	if len(rt.started) != 0 {
		t.Fatalf("started = %v, want no start with nothing evictable", rt.started)
	}
}

func TestSwapGroup_EvictionStopFailureReportsWaitingForGPU(t *testing.T) {
	m := swapModel("chat", store.ResidencyAsleep)
	sibling := swapModel("other", store.ResidencyAwake)
	st := &fakeStore{model: &m, swapGroupModels: []store.Model{sibling}}
	rt := newFakeRuntime()
	rt.stopErr = errors.New("daemon busy")
	nodes := &dynamicNodes{before: fullGPUNode(), after: emptyGPUNode()}
	c := New("chat", st, nodes, rt, fakeProber{Status{Phase: PhaseLoading}}, WithClock(func() time.Time { return residencyNow }))

	siblingContainer := c.containerName(&sibling, c.imageFor(sibling.Engine), false)
	rt.containers[siblingContainer] = &docker.ContainerState{ID: "id-" + siblingContainer, Name: siblingContainer, Running: true}

	res, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if cond := reason(t, res); cond.Reason != "WaitingForGPU" {
		t.Fatalf("reason = %q (%s), want WaitingForGPU when eviction itself fails", cond.Reason, cond.Message)
	}
	if len(rt.started) != 0 {
		t.Fatalf("started = %v, want no start when the sibling could not be stopped", rt.started)
	}
}

func TestSwapGroup_EmptyGroupNeverEvictsAnything(t *testing.T) {
	m := onDemandModel(residencyNow.Add(-time.Second), store.ResidencyAsleep)
	m.ModelRef = "llama3.1:8b"
	st := &fakeStore{model: m}
	rt := newFakeRuntime()
	c := residencyController(st, rt, fullGPUNode(), Status{Phase: PhaseLoading})
	name := c.containerName(m, c.imageFor(m.Engine), false)
	rt.containers[name] = &docker.ContainerState{ID: "id-" + name, Name: name}

	res, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if cond := reason(t, res); cond.Reason != "WaitingForGPU" {
		t.Fatalf("reason = %q, want WaitingForGPU", cond.Reason)
	}
	if len(st.swapGroupModels) != 0 {
		t.Fatal("no swap group models were configured, ListModelsInSwapGroup should never be consulted for an unset group")
	}
}

// TestSwapGroup_ConcurrentWakesAreSerialized simulates the racing-wake
// case called out for this feature: two asleep group members (a, b)
// both racing to wake at once, both needing to evict the same resident
// sibling (c) first. Each goes through waitForGPUWithSwap, the locked
// entry point (swap_lock.go): without the lock, both goroutines' Stop
// calls would mutate the shared fake runtime's containers map at once,
// a real data race go test -race catches. The assertion is that both
// calls complete cleanly (no panic, no race) and c ends up stopped.
func TestSwapGroup_ConcurrentWakesAreSerialized(t *testing.T) {
	a := swapModel("a", store.ResidencyAsleep)
	b := swapModel("b", store.ResidencyAsleep)
	c := swapModel("c", store.ResidencyAwake)
	rt := newFakeRuntime()
	nodes := &dynamicNodes{before: fullGPUNode(), after: emptyGPUNode()}
	cA := New("a", &fakeStore{model: &a, swapGroupModels: []store.Model{b, c}}, nodes, rt, fakeProber{Status{Phase: PhaseLoading}}, WithClock(func() time.Time { return residencyNow }))
	cB := New("b", &fakeStore{model: &b, swapGroupModels: []store.Model{a, c}}, nodes, rt, fakeProber{Status{Phase: PhaseLoading}}, WithClock(func() time.Time { return residencyNow }))

	cContainer := cA.containerName(&c, cA.imageFor(c.Engine), false)
	rt.containers[cContainer] = &docker.ContainerState{ID: "id-" + cContainer, Name: cContainer, Running: true}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		cA.waitForGPUWithSwap(context.Background(), &a, fullGPUNode())
	}()
	go func() {
		defer wg.Done()
		cB.waitForGPUWithSwap(context.Background(), &b, fullGPUNode())
	}()
	wg.Wait()

	if rt.containers[cContainer].Running {
		t.Fatal("c should have been stopped by whichever of a/b acquired the lock first")
	}
	if len(rt.stopped) == 0 {
		t.Fatal("expected at least one Stop call on c's container")
	}
}

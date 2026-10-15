package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Phase 5 chaos suite: one named test per failure mode. Each asserts the
// failed pass reports a stable condition reason, the old release keeps
// serving, nothing is orphaned, and a later pass converges.

func runningNames(rt *fakeRuntime) []string {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	var out []string
	for name, cs := range rt.containers {
		if cs.Running {
			out = append(out, name)
		}
	}
	return out
}

func TestChaos_DockerDaemonDiesMidDeploy_NoOrphanAndConverges(t *testing.T) {
	for _, withOld := range []bool{true, false} {
		name := "fresh deploy"
		if withOld {
			name = "redeploy over running v1"
		}
		t.Run(name, func(t *testing.T) {
			v1 := ContainerName("web", "img:v1", "")
			v2 := ContainerName("web", "img:v2", "")
			rt := newFakeRuntime(0)
			if withOld {
				rt.seed(v1, true)
			}
			desired := &store.DesiredService{Name: "web", Image: "img:v2", Port: 80}
			c := New("web", &fakeStore{svc: desired}, rt)

			daemonDown := errors.New("Cannot connect to the Docker daemon at unix:///var/run/docker.sock")
			rt.startErr = daemonDown
			rt.removeErr = daemonDown
			res, err := c.Reconcile(context.Background())
			if err == nil {
				t.Fatal("pass with the daemon dying mid-deploy error = nil, want failure")
			}
			if cond := conditionOf(t, res); cond.Status != reconcile.ConditionFalse || cond.Reason == "" {
				t.Fatalf("condition = %+v, want False with a stable reason", cond)
			}
			if withOld {
				if cs := rt.containers[v1]; cs == nil || !cs.Running {
					t.Fatal("old release not running after the failed deploy")
				}
			}

			rt.startErr, rt.removeErr = nil, nil
			res, err = c.Reconcile(context.Background())
			if err != nil {
				t.Fatalf("pass after daemon recovery error = %v", err)
			}
			if cond := conditionOf(t, res); cond.Status != reconcile.ConditionTrue {
				t.Fatalf("condition after recovery = %+v, want True", cond)
			}
			assertNames(t, "after recovery", rt.names(), []string{v2})
			assertNames(t, "running after recovery", runningNames(rt), []string{v2})
		})
	}
}

func TestChaos_DiskFullDuringImagePull_OldKeepsServing(t *testing.T) {
	v1 := ContainerName("web", "img:v1", "")
	rt := newFakeRuntime(0)
	rt.seed(v1, true)
	desired := &store.DesiredService{Name: "web", Image: "img:v2", Port: 80}
	c := New("web", &fakeStore{svc: desired}, rt)

	rt.createErr = errors.New("write /var/lib/docker/tmp/GetImageBlob: no space left on device")
	res, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("Reconcile() error = nil, want the disk-full failure")
	}
	cond := conditionOf(t, res)
	if cond.Status != reconcile.ConditionFalse || cond.Reason != "CreateFailed" {
		t.Fatalf("condition = %+v, want False/CreateFailed", cond)
	}
	assertNames(t, "after disk-full pull", rt.names(), []string{v1})
	if !rt.containers[v1].Running {
		t.Fatal("old release stopped by a pull that never finished")
	}

	rt.createErr = nil
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("pass after space was freed error = %v", err)
	}
	assertNames(t, "after space freed", rt.names(), []string{ContainerName("web", "img:v2", "")})
}

func TestChaos_CorruptOrMissingImageAtRollback_CurrentKeepsServing(t *testing.T) {
	tests := []struct {
		name      string
		createErr string
	}{
		{"image missing", "Error response from daemon: No such image: img:v1"},
		{"layer corrupt", "Error response from daemon: failed to register layer: archive/tar: invalid tar header"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v2 := ContainerName("web", "img:v2", "")
			rt := newFakeRuntime(0)
			rt.seed(v2, true)
			rt.createErr = errors.New(tt.createErr)
			rollbackTarget := &store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Strategy: "blue-green"}
			c := New("web", &fakeStore{svc: rollbackTarget}, rt)

			res, err := c.Reconcile(context.Background())
			if err == nil {
				t.Fatal("Reconcile() error = nil, want the rollback failure surfaced")
			}
			if cond := conditionOf(t, res); cond.Status != reconcile.ConditionFalse || cond.Reason != "CreateFailed" {
				t.Fatalf("condition = %+v, want False/CreateFailed", cond)
			}
			assertNames(t, "after failed rollback", rt.names(), []string{v2})
			if !rt.containers[v2].Running || rt.removeCalls != 0 || rt.stopCalls != 0 {
				t.Fatalf("serving release disturbed: running=%v remove=%d stop=%d", rt.containers[v2].Running, rt.removeCalls, rt.stopCalls)
			}
		})
	}
}

func TestChaos_OOMKillDuringReadinessWait_OldKeepsServing(t *testing.T) {
	srv := neverHealthy()
	defer srv.Close()

	rt := newFakeRuntime(serverPort(t, srv))
	v1 := ContainerName("web", "img:v1", "")
	v2 := ContainerName("web", "img:v2", "")
	rt.seed(v1, true)
	desired := &store.DesiredService{
		Name: "web", Image: "img:v2", Port: 80,
		Health: &store.ServiceHealth{Readiness: &store.ServiceProbe{Path: "/healthz", Interval: 10 * time.Millisecond, Timeout: 50 * time.Millisecond}},
	}
	time.AfterFunc(40*time.Millisecond, func() { rt.crashContainer(v2, true, 137) })
	c := New("web", &fakeStore{svc: desired}, rt, WithReadyBudget(2*time.Second))

	start := time.Now()
	res, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("Reconcile() error = nil, want the OOM kill surfaced")
	}
	if time.Since(start) > time.Second {
		t.Errorf("took %v, want a fast fail well under the readiness budget", time.Since(start))
	}
	assertConditionReasonFalse(t, res, "OOMKilledDuringReadiness")
	if cs := rt.containers[v1]; cs == nil || !cs.Running {
		t.Fatal("old release not serving after the replacement was OOM-killed")
	}
	for _, n := range runningNames(rt) {
		if n != v1 {
			t.Errorf("unexpected running container %q, want only the old release", n)
		}
	}
}

func TestChaos_ControlPlaneKilledBetweenDesiredStateAndCreate_ConvergesOnRestart(t *testing.T) {
	v1 := ContainerName("web", "img:v1", "")
	v2 := ContainerName("web", "img:v2", "")
	desired := &store.DesiredService{Name: "web", Image: "img:v2", Port: 80, Strategy: "blue-green"}

	tests := []struct {
		name     string
		leftover func(rt *fakeRuntime)
	}{
		{"nothing created yet", func(rt *fakeRuntime) { rt.seed(v1, true) }},
		{"replacement created but never started", func(rt *fakeRuntime) {
			rt.seed(v1, true)
			rt.seed(v2, false)
		}},
		{"fresh app, nothing at all", func(_ *fakeRuntime) {}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := newFakeRuntime(0)
			tt.leftover(rt)

			// A new controller stands in for the restarted process: it
			// carries no memory of the pass the dead one was in.
			c := New("web", &fakeStore{svc: desired}, rt)
			res, err := c.Reconcile(context.Background())
			if err != nil {
				t.Fatalf("first pass after restart error = %v", err)
			}
			if cond := conditionOf(t, res); cond.Status != reconcile.ConditionTrue {
				t.Fatalf("condition = %+v, want True", cond)
			}
			assertNames(t, "after restart", rt.names(), []string{v2})
			assertNames(t, "running after restart", runningNames(rt), []string{v2})

			createsBefore := rt.createCalls
			if _, err := New("web", &fakeStore{svc: desired}, rt).Reconcile(context.Background()); err != nil {
				t.Fatalf("second pass error = %v", err)
			}
			if rt.createCalls != createsBefore {
				t.Errorf("second pass created %d more containers, want a no-op", rt.createCalls-createsBefore)
			}
		})
	}
}

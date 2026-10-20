package application

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploysafety"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

// exitOnStartRuntime is fakeRuntime whose new containers exit with code
// exitCode the moment they start, like an image with no default command.
type exitOnStartRuntime struct {
	*fakeRuntime
	exitCode int
	exitImg  string
	logs     []string
}

func (r *exitOnStartRuntime) Start(ctx context.Context, id string) error {
	if err := r.fakeRuntime.Start(ctx, id); err != nil {
		return err
	}
	r.mu.Lock()
	var name, image string
	for n, cs := range r.containers {
		if cs.ID == id {
			name, image = n, cs.Image
		}
	}
	r.mu.Unlock()
	if image == r.exitImg {
		now := time.Now()
		r.crashContainer(name, false, r.exitCode)
		r.setExitState(name, &docker.ExitState{ExitCode: r.exitCode, StartedAt: now.Add(-2 * time.Second), FinishedAt: now})
	} else {
		r.setExitState(name, &docker.ExitState{Running: true, StartedAt: time.Now()})
	}
	return nil
}

func (r *exitOnStartRuntime) Logs(_ context.Context, _ string, _ bool, _ time.Time) (<-chan docker.LogLine, <-chan error) {
	out := make(chan docker.LogLine, len(r.logs))
	errs := make(chan error, 1)
	for _, l := range r.logs {
		out <- docker.LogLine{Stream: "stdout", Message: l}
	}
	close(out)
	close(errs)
	return out, errs
}

type fakeExitRecorder struct {
	mu    sync.Mutex
	exits []store.DeployExit
}

func (f *fakeExitRecorder) RecordDeployExit(_ context.Context, e store.DeployExit) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.exits = append(f.exits, e)
	return nil
}

func (f *fakeExitRecorder) last(t *testing.T) store.DeployExit {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.exits) == 0 {
		t.Fatal("no exit captured")
	}
	return f.exits[len(f.exits)-1]
}

// A new release whose container exits 0 right after start must fail the
// deploy, keep the previous release serving and capture its last output.
func TestController_ExitOnStart_FailsDeployAndKeepsPrevious(t *testing.T) {
	slowProbe := &store.ServiceHealth{Readiness: &store.ServiceProbe{Path: "/healthz", Interval: 30 * time.Second, Timeout: time.Second}}
	tests := []struct {
		name     string
		strategy string
		port     int
		health   *store.ServiceHealth
		exitCode int
		maxWait  time.Duration
	}{
		{name: "blue-green worker, no readiness", strategy: "blue-green", exitCode: 0, maxWait: 3 * time.Second},
		{name: "rolling worker, nonzero exit", strategy: "rolling", exitCode: 2, maxWait: 3 * time.Second},
		{name: "blue-green web, 30s probe interval caught by crash watch", strategy: "blue-green", port: 80, health: slowProbe, exitCode: 0, maxWait: 4 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hostPort := 0
			if tt.port != 0 {
				hostPort = closedPort(t)
			}
			rt := &exitOnStartRuntime{fakeRuntime: newFakeRuntime(hostPort), exitCode: tt.exitCode, exitImg: "img:v2", logs: []string{"NAME:", "   app - usage"}}
			old := replicaContainerName("web", "img:v1", "", 0)
			rt.seed(old, true)
			desired := &store.DesiredService{Name: "web", Image: "img:v2", Port: tt.port, Health: tt.health, Strategy: tt.strategy, Replicas: 1}
			rec := &fakeExitRecorder{}
			c := New("web", &fakeStore{svc: desired}, rt, WithStartupWatch(time.Second), WithExitRecorder(rec), WithReadyBudget(10*time.Second))

			start := time.Now()
			res, err := c.Reconcile(context.Background())
			if took := time.Since(start); took > tt.maxWait {
				t.Fatalf("Reconcile took %s, want under %s", took, tt.maxWait)
			}
			if err == nil {
				t.Fatal("Reconcile error = nil, want the exit to fail the deploy")
			}
			cond := conditionOf(t, res)
			if cond.Status != reconcile.ConditionFalse || cond.Reason != deploysafety.ReasonExitedDuringReadiness {
				t.Fatalf("condition = %+v, want False/%s", cond, deploysafety.ReasonExitedDuringReadiness)
			}
			wantMsg := "Container exited with code " + strconv.Itoa(tt.exitCode) + " after 2s, before it became ready"
			if cond.Message != wantMsg {
				t.Errorf("message = %q, want %q", cond.Message, wantMsg)
			}
			if cs := rt.containers[old]; cs == nil || !cs.Running {
				t.Fatalf("previous release %q running = false, want it still serving", old)
			}
			got := rec.last(t)
			if got.ExitCode != tt.exitCode || got.Image != "img:v2" || got.Reason != deploysafety.ReasonExitedDuringReadiness {
				t.Errorf("capture = %+v", got)
			}
			if len(got.LogTail) != 2 || !strings.Contains(got.LogTail[1].Message, "usage") {
				t.Errorf("captured log tail = %+v, want the container's output", got.LogTail)
			}
		})
	}
}

func TestController_StartupWatch_PassesStableWorker(t *testing.T) {
	tests := []struct {
		name  string
		watch time.Duration
	}{
		{name: "watch configured", watch: 300 * time.Millisecond},
		{name: "watch off", watch: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := newFakeRuntime(0)
			desired := &store.DesiredService{Name: "web", Image: "img:v1", Replicas: 1}
			rec := &fakeExitRecorder{}
			c := New("web", &fakeStore{svc: desired}, rt, WithStartupWatch(tt.watch), WithExitRecorder(rec))
			res, err := c.Reconcile(context.Background())
			if err != nil {
				t.Fatalf("Reconcile: %v", err)
			}
			if cond := conditionOf(t, res); cond.Status != reconcile.ConditionTrue {
				t.Fatalf("condition = %+v, want True", cond)
			}
			if len(rec.exits) != 0 {
				t.Fatalf("captured %d exits for a healthy worker", len(rec.exits))
			}
		})
	}
}

// With a stable window configured the gate already watches the container,
// so its exit is reported by the gate with the same wording.
func TestController_StabilityGateExit_UsesExitMessage(t *testing.T) {
	rt := &exitOnStartRuntime{fakeRuntime: newFakeRuntime(0), exitImg: "never"}
	desired := &store.DesiredService{Name: "web", Image: "img:v1", Replicas: 1}
	rec := &fakeExitRecorder{}
	defaults := deploysafety.Effective{SuccessThreshold: 1, StableWindow: 5 * time.Second}
	c := New("web", &fakeStore{svc: desired}, rt, WithStabilityGate(defaults, nil), WithGateTick(50*time.Millisecond), WithExitRecorder(rec))
	target := replicaContainerName("web", "img:v1", "", 0)
	go func() {
		time.Sleep(200 * time.Millisecond)
		now := time.Now()
		rt.crashContainer(target, false, 0)
		rt.setExitState(target, &docker.ExitState{ExitCode: 0, StartedAt: now.Add(-3 * time.Second), FinishedAt: now})
	}()
	res, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("Reconcile error = nil, want the gate to fail")
	}
	cond := conditionOf(t, res)
	if cond.Reason != deploysafety.ReasonExitedDuringWindow {
		t.Fatalf("reason = %q, want %q", cond.Reason, deploysafety.ReasonExitedDuringWindow)
	}
	if want := "Container exited with code 0 after 3s, before it became ready"; cond.Message != want {
		t.Errorf("message = %q, want %q", cond.Message, want)
	}
	if got := rec.last(t); got.Reason != deploysafety.ReasonExitedDuringWindow {
		t.Errorf("capture reason = %q", got.Reason)
	}
}

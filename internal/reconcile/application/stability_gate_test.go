package application

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploysafety"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeGateRecorder struct {
	mu      sync.Mutex
	events  []store.DeployGateEvent
	reasons []string
}

func (f *fakeGateRecorder) RecordGateEvent(_ context.Context, e store.DeployGateEvent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, e)
	return nil
}

func (f *fakeGateRecorder) MarkRolloutFailureReason(_ context.Context, _, _, reason string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reasons = append(f.reasons, reason)
	return nil
}

func (f *fakeGateRecorder) has(phase, status string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, e := range f.events {
		if e.Phase == phase && e.Status == status {
			return true
		}
	}
	return false
}

type fixedSafety store.DeploySafetySettings

func (f fixedSafety) GetDeploySafetySettings(context.Context, string) (store.DeploySafetySettings, error) {
	return store.DeploySafetySettings(f), nil
}

func gateDesired() *store.DesiredService {
	return &store.DesiredService{
		Name: "web", Image: "img:v2", Port: 80,
		Health: &store.ServiceHealth{Readiness: &store.ServiceProbe{Path: "/healthz", Interval: 10 * time.Millisecond, Timeout: 50 * time.Millisecond}},
	}
}

func gateOptions(rec *fakeGateRecorder, threshold int, window, drain time.Duration) []Option {
	cfg := deploysafety.Effective{SuccessThreshold: threshold, StableWindow: window, DrainTimeout: drain}
	return []Option{WithStabilityGate(cfg, nil), WithGateRecorder(rec), WithGateTick(10 * time.Millisecond), WithReadyBudget(2 * time.Second)}
}

// alternating answers 200 and 503 in turn, a readiness probe that never
// strings together consecutive passes.
func alternating() *httptest.Server {
	var n atomic.Int64
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if n.Add(1)%2 == 0 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
}

func TestStabilityGate_PassesAfterWindowAndConsecutiveChecks(t *testing.T) {
	srv := alwaysHealthy()
	defer srv.Close()
	rt := newFakeRuntime(serverPort(t, srv))
	desired := gateDesired()
	target := ContainerName("web", desired.Image, "")
	rt.setExitState(target, &docker.ExitState{Running: true})
	rec := &fakeGateRecorder{}

	c := New("web", &fakeStore{svc: desired}, rt, gateOptions(rec, 3, 150*time.Millisecond, 0)...)
	start := time.Now()
	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if elapsed := time.Since(start); elapsed < 150*time.Millisecond {
		t.Errorf("Reconcile() returned after %v, before the 150ms stable window", elapsed)
	}
	if cond := conditionOf(t, result); cond.Status != reconcile.ConditionTrue || cond.Reason != "Deployed" {
		t.Errorf("condition = %+v, want True/Deployed", cond)
	}
	for _, want := range [][2]string{
		{deploysafety.PhaseReadiness, deploysafety.StatusPassed},
		{deploysafety.PhaseStability, deploysafety.StatusStarted},
		{deploysafety.PhaseStability, deploysafety.StatusPassed},
		{deploysafety.PhaseCutover, deploysafety.StatusPassed},
	} {
		if !rec.has(want[0], want[1]) {
			t.Errorf("no %s/%s gate event recorded: %+v", want[0], want[1], rec.events)
		}
	}
}

func TestStabilityGate_FailuresKeepOldReleaseServing(t *testing.T) {
	tests := []struct {
		name       string
		breakIt    func(rt *fakeRuntime, target string)
		server     func() *httptest.Server
		wantReason string
	}{
		{
			name:       "oom killed inside window",
			breakIt:    func(rt *fakeRuntime, target string) { rt.crashContainer(target, true, 137) },
			server:     alwaysHealthy,
			wantReason: deploysafety.ReasonOOMKilledDuringWindow,
		},
		{
			name:       "exited inside window",
			breakIt:    func(rt *fakeRuntime, target string) { rt.crashContainer(target, false, 2) },
			server:     alwaysHealthy,
			wantReason: deploysafety.ReasonExitedDuringWindow,
		},
		{
			name: "restarted inside window",
			breakIt: func(rt *fakeRuntime, target string) {
				rt.setExitState(target, &docker.ExitState{Running: true, RestartCount: 1})
			},
			server:     alwaysHealthy,
			wantReason: deploysafety.ReasonRestartedDuringWindow,
		},
		{
			name:       "probe never strings passes together",
			server:     alternating,
			wantReason: deploysafety.ReasonGateTimeout,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := tt.server()
			defer srv.Close()
			rt := newFakeRuntime(serverPort(t, srv))
			oldName := ContainerName("web", "img:v1", "")
			rt.seed(oldName, true)
			desired := gateDesired()
			target := ContainerName("web", desired.Image, "")
			rt.setExitState(target, &docker.ExitState{Running: true})
			if tt.breakIt != nil {
				time.AfterFunc(80*time.Millisecond, func() { tt.breakIt(rt, target) })
			}
			rec := &fakeGateRecorder{}
			opts := append(gateOptions(rec, 3, 400*time.Millisecond, 0), WithReadyBudget(300*time.Millisecond))

			c := New("web", &fakeStore{svc: desired}, rt, opts...)
			result, err := c.Reconcile(context.Background())
			if err == nil {
				t.Fatal("Reconcile() error = nil, want the gate failure surfaced")
			}
			if cond := conditionOf(t, result); cond.Status != reconcile.ConditionFalse || cond.Reason != tt.wantReason {
				t.Errorf("condition = %+v, want False/%s", cond, tt.wantReason)
			}
			old, _ := rt.InspectByName(context.Background(), oldName)
			if old == nil || !old.Running {
				t.Errorf("previous release %s = %+v, want it still running and serving", oldName, old)
			}
			if !rec.has(deploysafety.PhaseStability, deploysafety.StatusFailed) || rec.has(deploysafety.PhaseCutover, deploysafety.StatusPassed) {
				t.Errorf("events = %+v, want a failed stability event and no cutover", rec.events)
			}
			if len(rec.reasons) == 0 || rec.reasons[len(rec.reasons)-1] != tt.wantReason {
				t.Errorf("attempt reasons = %v, want %s recorded", rec.reasons, tt.wantReason)
			}
			if cur, _ := rt.InspectByName(context.Background(), target); cur != nil && cur.Running {
				t.Errorf("gate-failed container %s still running; it must be stopped so the next pass restarts it under backoff", target)
			}
		})
	}
}

func TestStabilityGate_InspectErrorIsUnverifiedNotPassed(t *testing.T) {
	srv := alwaysHealthy()
	defer srv.Close()
	rt := newFakeRuntime(serverPort(t, srv))
	oldName := ContainerName("web", "img:v1", "")
	rt.seed(oldName, true)
	rt.inspectExitErr = errors.New("docker daemon unreachable")
	desired := gateDesired()

	rec := &fakeGateRecorder{}
	opts := append(gateOptions(rec, 1, 50*time.Millisecond, 0), WithReadyBudget(200*time.Millisecond))
	c := New("web", &fakeStore{svc: desired}, rt, opts...)
	if _, err := c.Reconcile(context.Background()); err == nil {
		t.Fatal("Reconcile() error = nil, want the unverified container to fail the gate")
	}
	if rec.has(deploysafety.PhaseCutover, deploysafety.StatusPassed) {
		t.Errorf("events = %+v, an uninspectable container must not reach cutover", rec.events)
	}
	old, _ := rt.InspectByName(context.Background(), oldName)
	if old == nil || !old.Running {
		t.Errorf("previous release = %+v, want it still running", old)
	}
}

// Half-succeeded: replica 0 clears the gate, replica 1 is OOM-killed inside
// its window. Nothing from the old release may be retired.
func TestStabilityGate_HalfSucceededRolloutRetiresNothing(t *testing.T) {
	srv := alwaysHealthy()
	defer srv.Close()
	rt := newFakeRuntime(serverPort(t, srv))
	old0, old1 := ReplicaContainerName("web", "img:v1", "", 0), ReplicaContainerName("web", "img:v1", "", 1)
	rt.seed(old0, true)
	rt.seed(old1, true)
	desired := gateDesired()
	desired.Replicas = 2
	desired.Strategy = strategyBlueGreen
	new0, new1 := ReplicaContainerName("web", desired.Image, "", 0), ReplicaContainerName("web", desired.Image, "", 1)
	rt.setExitState(new0, &docker.ExitState{Running: true})
	rt.setExitState(new1, &docker.ExitState{Running: true})
	rec := &fakeGateRecorder{}

	go func() {
		for {
			if cs, _ := rt.InspectByName(context.Background(), new1); cs != nil && cs.Running {
				time.Sleep(30 * time.Millisecond)
				rt.crashContainer(new1, true, 137)
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
	}()

	c := New("web", &fakeStore{svc: desired}, rt, gateOptions(rec, 2, 150*time.Millisecond, 0)...)
	result, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("Reconcile() error = nil, want replica 1's OOM kill to fail the rollout")
	}
	if cond := conditionOf(t, result); cond.Reason != deploysafety.ReasonOOMKilledDuringWindow {
		t.Errorf("condition = %+v, want %s", cond, deploysafety.ReasonOOMKilledDuringWindow)
	}
	for _, name := range []string{old0, old1} {
		if cs, _ := rt.InspectByName(context.Background(), name); cs == nil || !cs.Running {
			t.Errorf("old replica %s = %+v, want still running after a half-succeeded rollout", name, cs)
		}
	}
	if cs, _ := rt.InspectByName(context.Background(), new0); cs == nil || !cs.Running {
		t.Errorf("new replica 0 = %+v, want it left running (it passed its gate)", cs)
	}
}

func TestDrain_PreviousReleaseOutlivesCutover(t *testing.T) {
	srv := alwaysHealthy()
	defer srv.Close()
	rt := newFakeRuntime(serverPort(t, srv))
	oldName := ContainerName("web", "img:v1", "")
	rt.seed(oldName, true)
	desired := gateDesired()
	rt.setExitState(ContainerName("web", desired.Image, ""), &docker.ExitState{Running: true})
	rec := &fakeGateRecorder{}
	tracker := NewDrainTracker()
	opts := append(gateOptions(rec, 1, 0, time.Hour), WithDrainTracker(tracker))

	c := New("web", &fakeStore{svc: desired}, rt, opts...)
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if cs, _ := rt.InspectByName(context.Background(), oldName); cs == nil || !cs.Running {
		t.Fatalf("old release stopped at cutover, want it draining: %+v", cs)
	}
	if !rec.has(deploysafety.PhaseDrain, deploysafety.StatusStarted) {
		t.Errorf("no drain started event: %+v", rec.events)
	}

	c = New("web", &fakeStore{svc: desired}, rt, opts...)
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("second Reconcile() error = %v", err)
	}
	if cs, _ := rt.InspectByName(context.Background(), oldName); cs == nil {
		t.Fatal("old release removed while its drain window is still open")
	}

	tracker.mark("web", time.Now().Add(-2*time.Hour))
	c = New("web", &fakeStore{svc: desired}, rt, opts...)
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("third Reconcile() error = %v", err)
	}
	if cs, _ := rt.InspectByName(context.Background(), oldName); cs != nil {
		t.Errorf("old release %+v still present after its drain window closed", cs)
	}
	if !rec.has(deploysafety.PhaseDrain, deploysafety.StatusPassed) {
		t.Errorf("no drain passed event: %+v", rec.events)
	}
}

func TestDrain_ZeroTimeoutRetiresAtCutover(t *testing.T) {
	srv := alwaysHealthy()
	defer srv.Close()
	rt := newFakeRuntime(serverPort(t, srv))
	oldName := ContainerName("web", "img:v1", "")
	rt.seed(oldName, true)
	desired := gateDesired()
	c := New("web", &fakeStore{svc: desired}, rt, WithStabilityGate(deploysafety.Effective{SuccessThreshold: 1}, nil))
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if cs, _ := rt.InspectByName(context.Background(), oldName); cs != nil {
		t.Errorf("old release %+v kept with no drain and no hold configured", cs)
	}
}

func TestSafetyFor_PerAppOverride(t *testing.T) {
	five := 5
	c := New("web", &fakeStore{}, newFakeRuntime(0), WithStabilityGate(deploysafety.Effective{SuccessThreshold: 2}, fixedSafety{SuccessThreshold: &five}))
	if got := c.safetyFor(context.Background(), &store.DesiredService{Name: "web"}); got.SuccessThreshold != 5 {
		t.Errorf("SuccessThreshold = %d, want the per-app 5", got.SuccessThreshold)
	}
}

// A release that already proved itself and only crashed comes back after
// readiness, not after another stability window.
func TestStabilityGate_CrashRestartSkipsWindow(t *testing.T) {
	srv := alwaysHealthy()
	defer srv.Close()
	rt := newFakeRuntime(serverPort(t, srv))
	desired := gateDesired()
	target := ContainerName("web", desired.Image, "")
	rt.seed(target, false)
	rt.setExitState(target, &docker.ExitState{Running: true})
	rec := &fakeGateRecorder{}

	c := New("web", &fakeStore{svc: desired}, rt, gateOptions(rec, 3, 2*time.Second, 0)...)
	start := time.Now()
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if elapsed := time.Since(start); elapsed >= time.Second {
		t.Errorf("crash restart took %v, it must not wait for the stability window", elapsed)
	}
	if rec.has(deploysafety.PhaseStability, deploysafety.StatusStarted) {
		t.Errorf("stability gate ran on a crash restart: %+v", rec.events)
	}
}

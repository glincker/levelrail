package application

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/failure"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Chaos pack: fast fault-injection tests that extend chaos_phase5_test.go.
// docs/chaos-coverage.md maps each scenario to its test. Every test ends by
// proving the level-triggered property: after recovery one more pass is a no-op.

type mutationCounts struct{ create, remove, stop, start int }

func countsOf(rt *fakeRuntime) mutationCounts {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return mutationCounts{rt.createCalls, rt.removeCalls, rt.stopCalls, rt.startCalls}
}

// assertNoopPass runs one more pass and requires success with no container mutation.
func assertNoopPass(t *testing.T, c *Controller, rt *fakeRuntime) {
	t.Helper()
	before := countsOf(rt)
	res, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("no-op pass error = %v", err)
	}
	if cond := conditionOf(t, res); cond.Status != reconcile.ConditionTrue {
		t.Fatalf("no-op pass condition = %+v, want True", cond)
	}
	if after := countsOf(rt); after != before {
		t.Fatalf("pass after convergence mutated containers: before=%+v after=%+v, want a no-op", before, after)
	}
}

func assertFailedPass(t *testing.T, res reconcile.Result, err error) reconcile.Condition {
	t.Helper()
	if err == nil {
		t.Fatal("pass error = nil, want the injected failure surfaced")
	}
	cond := conditionOf(t, res)
	if cond.Status != reconcile.ConditionFalse || cond.Reason == "" {
		t.Fatalf("condition = %+v, want False with a stable reason", cond)
	}
	return cond
}

func assertRunning(t *testing.T, when string, rt *fakeRuntime, want ...string) {
	t.Helper()
	assertNames(t, "running "+when, runningNames(rt), want)
}

// Daemon blips at every inspect step of a deploy: whichever call fails, the
// old release is never taken down and the next pass converges.
func TestChaosPack_DaemonBlipAtEveryInspectStep_ConvergesWithoutOrphans(t *testing.T) {
	v1 := ContainerName("web", "img:v1", "")
	v2 := ContainerName("web", "img:v2", "")
	for _, strategy := range []string{"blue-green", "rolling", "recreate"} {
		for call := 1; call <= 12; call++ {
			t.Run(fmt.Sprintf("%s/inspect call %d", strategy, call), func(t *testing.T) {
				rt := newFakeRuntime(0)
				rt.seed(v1, true)
				rt.inspectErrOnCall = call
				desired := &store.DesiredService{Name: "web", Image: "img:v2", Port: 80, Strategy: strategy}
				c := New("web", &fakeStore{svc: desired}, rt)

				_, err := c.Reconcile(context.Background())
				if err != nil && strategy != "recreate" {
					if cs := rt.containers[v1]; cs == nil || !cs.Running {
						t.Fatal("old release not running after a daemon blip mid-deploy")
					}
				}
				if got := len(rt.names()); got > 2 {
					t.Fatalf("containers after blip = %v, want at most the old and the new release", rt.names())
				}

				rt.inspectErrOnCall = 0
				for pass := 0; pass < 3; pass++ {
					if _, err := c.Reconcile(context.Background()); err == nil {
						break
					}
				}
				assertNames(t, "after recovery", rt.names(), []string{v2})
				assertRunning(t, "after recovery", rt, v2)
				assertNoopPass(t, c, rt)
			})
		}
	}
}

// Create succeeds, then the daemon dies before or during start and stays down
// through the next pass. The old release survives and a restarted control
// plane (new controller) converges once the daemon returns.
func TestChaosPack_DaemonDiesAfterCreate_OldReleaseSurvivesAndConverges(t *testing.T) {
	v1 := ContainerName("web", "img:v1", "")
	v2 := ContainerName("web", "img:v2", "")
	down := errors.New("cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?")

	tests := []struct {
		name   string
		inject func(rt *fakeRuntime)
	}{
		{"start fails, cleanup works", func(rt *fakeRuntime) { rt.startErr = down }},
		{"start fails, cleanup fails too", func(rt *fakeRuntime) { rt.startErr, rt.removeErr, rt.stopErr = down, down, down }},
		{"start fails, then every inspect fails", func(rt *fakeRuntime) { rt.startErr, rt.removeErr = down, down }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := newFakeRuntime(0)
			rt.seed(v1, true)
			desired := &store.DesiredService{Name: "web", Image: "img:v2", Port: 80, Strategy: "blue-green"}
			tt.inject(rt)

			res, err := New("web", &fakeStore{svc: desired}, rt).Reconcile(context.Background())
			assertFailedPass(t, res, err)
			if rt.createCalls != 1 {
				t.Fatalf("createCalls = %d, want 1: the daemon died after create", rt.createCalls)
			}
			if cs := rt.containers[v1]; cs == nil || !cs.Running {
				t.Fatal("old release not running after the daemon died mid-deploy")
			}

			if tt.name == "start fails, then every inspect fails" {
				rt.inspectErr = down
				before := countsOf(rt)
				res, err := New("web", &fakeStore{svc: desired}, rt).Reconcile(context.Background())
				assertFailedPass(t, res, err)
				if countsOf(rt) != before {
					t.Fatal("pass with the daemon fully down mutated containers")
				}
				rt.inspectErr = nil
			}

			rt.startErr, rt.removeErr, rt.stopErr = nil, nil, nil
			c := New("web", &fakeStore{svc: desired}, rt)
			res, err = c.Reconcile(context.Background())
			if err != nil {
				t.Fatalf("pass after the daemon returned error = %v", err)
			}
			if cond := conditionOf(t, res); cond.Status != reconcile.ConditionTrue {
				t.Fatalf("condition after recovery = %+v, want True", cond)
			}
			assertNames(t, "after recovery", rt.names(), []string{v2})
			assertRunning(t, "after recovery", rt, v2)
			assertNoopPass(t, c, rt)
		})
	}
}

// Disk fills at each point a deploy writes to disk. The failure is typed as
// disk_full, the old release keeps serving, and freeing space converges.
func TestChaosPack_DiskFull_AtEachWriteStep_OldKeepsServingAndIsTyped(t *testing.T) {
	v1 := ContainerName("web", "img:v1", "")
	v2 := ContainerName("web", "img:v2", "")
	tests := []struct {
		name   string
		inject func(rt *fakeRuntime, err error)
		clear  func(rt *fakeRuntime)
	}{
		{"image pull", func(rt *fakeRuntime, err error) { rt.createErr = err }, func(rt *fakeRuntime) { rt.createErr = nil }},
		{"container start writes its log", func(rt *fakeRuntime, err error) { rt.startErr = err }, func(rt *fakeRuntime) { rt.startErr = nil }},
		{"volume ensure", func(rt *fakeRuntime, err error) { rt.ensureVolumeErr = err }, func(rt *fakeRuntime) { rt.ensureVolumeErr = nil }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := newFakeRuntime(0)
			rt.seed(v1, true)
			desired := &store.DesiredService{Name: "web", Image: "img:v2", Port: 80, Strategy: "blue-green"}
			if tt.name == "volume ensure" {
				desired.Volumes = []store.ServiceVolume{{Name: "data", ContainerPath: "/data"}}
			}
			c := New("web", &fakeStore{svc: desired}, rt)

			tt.inject(rt, errors.New("write /var/lib/docker/overlay2/x/diff: no space left on device"))
			res, err := c.Reconcile(context.Background())
			cond := assertFailedPass(t, res, err)
			got, ok := failure.Classify(failure.Input{Status: "failed", Error: cond.Message + " " + err.Error()}, failure.Options{})
			if !ok || got.Code != failure.CodeDiskFull {
				t.Fatalf("Classify(%q) = %q, %v, want %q", cond.Message, got.Code, ok, failure.CodeDiskFull)
			}
			if cs := rt.containers[v1]; cs == nil || !cs.Running {
				t.Fatal("old release stopped by a deploy that ran out of disk")
			}
			assertRunning(t, "after disk-full", rt, v1)

			tt.clear(rt)
			res, err = c.Reconcile(context.Background())
			if err != nil {
				t.Fatalf("pass after space was freed error = %v", err)
			}
			if cond := conditionOf(t, res); cond.Status != reconcile.ConditionTrue {
				t.Fatalf("condition after recovery = %+v, want True", cond)
			}
			assertNames(t, "after recovery", rt.names(), []string{v2})
			assertNoopPass(t, c, rt)
		})
	}
}

// A metrics write that hits a full disk after the cutover must not undo the
// deploy: the release stays Ready and the next pass neither redeploys nor
// records a second deploy.
func TestChaosPack_DiskFullDuringMetricWrite_DeployStaysReadyAndIsNotRepeated(t *testing.T) {
	v2 := ContainerName("web", "img:v2", "")
	rt := newFakeRuntime(0)
	rt.seed(ContainerName("web", "img:v1", ""), true)
	rec := &fakeDeployRecorder{err: errors.New("write telemetry.db-wal: no space left on device")}
	desired := &store.DesiredService{Name: "web", Image: "img:v2", Port: 80, Strategy: "blue-green"}
	c := New("web", &fakeStore{svc: desired}, rt, WithDeployRecorder(rec))

	res, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("pass error = nil, want the metric write failure surfaced")
	}
	cond := conditionOf(t, res)
	if cond.Status != reconcile.ConditionTrue || cond.Reason != "DeployedMetricRecordFailed" {
		t.Fatalf("condition = %+v, want True/DeployedMetricRecordFailed", cond)
	}
	assertNames(t, "after metric failure", rt.names(), []string{v2})
	assertRunning(t, "after metric failure", rt, v2)

	rec.err = nil
	assertNoopPass(t, c, rt)
	if rec.calls != 1 {
		t.Fatalf("RecordDeploy calls = %d, want 1: a converged pass is not a new deploy", rec.calls)
	}
}

// severableRuntime stands in for a node agent transport whose connection can
// drop and come back. While severed every call fails the way a broken gRPC
// stream does; the node's containers keep running regardless.
type severableRuntime struct {
	*fakeRuntime
	severed  atomic.Bool
	afterOps map[string]func()
}

var errSevered = errors.New("rpc error: code = Unavailable desc = transport is closing")

func (s *severableRuntime) InspectByName(ctx context.Context, name string) (*docker.ContainerState, error) {
	if s.severed.Load() {
		return nil, errSevered
	}
	return s.fakeRuntime.InspectByName(ctx, name)
}

func (s *severableRuntime) ListByPrefix(ctx context.Context, prefix string) ([]docker.ContainerState, error) {
	if s.severed.Load() {
		return nil, errSevered
	}
	return s.fakeRuntime.ListByPrefix(ctx, prefix)
}

func (s *severableRuntime) Create(ctx context.Context, spec docker.ContainerSpec) (string, error) {
	if s.severed.Load() {
		return "", errSevered
	}
	id, err := s.fakeRuntime.Create(ctx, spec)
	if err == nil {
		if fn := s.afterOps["create"]; fn != nil {
			fn()
		}
	}
	return id, err
}

func (s *severableRuntime) Start(ctx context.Context, id string) error {
	if s.severed.Load() {
		return errSevered
	}
	return s.fakeRuntime.Start(ctx, id)
}

func (s *severableRuntime) Stop(ctx context.Context, id string, timeout time.Duration) error {
	if s.severed.Load() {
		return errSevered
	}
	return s.fakeRuntime.Stop(ctx, id, timeout)
}

func (s *severableRuntime) Remove(ctx context.Context, id string, force bool) error {
	if s.severed.Load() {
		return errSevered
	}
	return s.fakeRuntime.Remove(ctx, id, force)
}

// The agent link drops before a pass: nothing is mutated, the apps keep
// serving, and when the link returns the pass converges without duplicates.
func TestChaosPack_AgentSeveredBeforePass_NothingMutatedAndReapplyIsIdempotent(t *testing.T) {
	v1 := ContainerName("web", "img:v1", "")
	v2 := ContainerName("web", "img:v2", "")
	inner := newFakeRuntime(0)
	inner.seed(v1, true)
	rt := &severableRuntime{fakeRuntime: inner}
	desired := &store.DesiredService{Name: "web", Image: "img:v2", Port: 80, Strategy: "blue-green"}
	c := New("web", &fakeStore{svc: desired}, rt)

	rt.severed.Store(true)
	before := countsOf(inner)
	for i := 0; i < 3; i++ {
		res, err := c.Reconcile(context.Background())
		assertFailedPass(t, res, err)
	}
	if countsOf(inner) != before {
		t.Fatal("passes against a severed agent mutated containers")
	}
	assertRunning(t, "while severed", inner, v1)

	rt.severed.Store(false)
	res, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("pass after reconnect error = %v", err)
	}
	if cond := conditionOf(t, res); cond.Status != reconcile.ConditionTrue {
		t.Fatalf("condition after reconnect = %+v, want True", cond)
	}
	assertNames(t, "after reconnect", inner.names(), []string{v2})
	assertNoopPass(t, c, inner)
}

// The agent link drops right after the replacement is created. The control
// plane keeps no state across the drop, so reconnecting builds a new
// controller; it must adopt the half-created replacement, not duplicate it.
func TestChaosPack_AgentSeveredMidReconcile_ReconnectAdoptsHalfCreatedRelease(t *testing.T) {
	v1 := ContainerName("web", "img:v1", "")
	v2 := ContainerName("web", "img:v2", "")
	for _, strategy := range []string{"blue-green", "rolling", "recreate"} {
		t.Run(strategy, func(t *testing.T) {
			inner := newFakeRuntime(0)
			inner.seed(v1, true)
			rt := &severableRuntime{fakeRuntime: inner}
			rt.afterOps = map[string]func(){"create": func() { rt.severed.Store(true) }}
			desired := &store.DesiredService{Name: "web", Image: "img:v2", Port: 80, Strategy: strategy}

			res, err := New("web", &fakeStore{svc: desired}, rt).Reconcile(context.Background())
			assertFailedPass(t, res, err)
			if strategy != "recreate" {
				assertRunning(t, "while severed", inner, v1)
			}

			rt.afterOps = nil
			rt.severed.Store(false)
			c := New("web", &fakeStore{svc: desired}, rt)
			res, err = c.Reconcile(context.Background())
			if err != nil {
				t.Fatalf("pass after reconnect error = %v", err)
			}
			if cond := conditionOf(t, res); cond.Status != reconcile.ConditionTrue {
				t.Fatalf("condition after reconnect = %+v, want True", cond)
			}
			assertNames(t, "after reconnect", inner.names(), []string{v2})
			assertRunning(t, "after reconnect", inner, v2)
			assertNoopPass(t, c, inner)
		})
	}
}

func toggleHealth(unhealthy *atomic.Bool) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if unhealthy.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
}

// recordingPinner stands in for the rollback pinner: it only ever adds pins,
// like the real one at cutover, so a failed deploy must leave the set alone.
type recordingPinner struct {
	mu   sync.Mutex
	pins []string
}

func (p *recordingPinner) PinServing(_ context.Context, d store.DesiredService, _ string) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	ref := d.Image
	p.pins = append(p.pins, ref)
	return ref, nil
}

func (p *recordingPinner) snapshot() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.pins...)
}

// A corrupt or missing image at deploy is typed, leaves the serving release
// untouched and pinned, pins nothing for the failed release, and converges
// once the image is repaired.
func TestChaosPack_CorruptOrMissingImageAtDeploy_TypedAndPreviousStaysPinned(t *testing.T) {
	v1 := ContainerName("web", "img:v1", "")
	v2 := ContainerName("web", "img:v2", "")
	tests := []struct {
		name     string
		createEr string
		wantCode string
	}{
		{"corrupt layer", "Error response from daemon: failed to register layer: archive/tar: invalid tar header", failure.CodeImageCorrupt},
		{"digest mismatch", "layer sha256:abc: digest mismatch", failure.CodeImageCorrupt},
		{"manifest unknown", "Error response from daemon: manifest unknown: manifest unknown", failure.CodeImagePullFailed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := newFakeRuntime(0)
			pinner := &recordingPinner{}
			old := &store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Strategy: "blue-green"}
			sv := &fakeStore{svc: old}
			c := New("web", sv, rt, WithRollbackPinner(pinner))
			if _, err := c.Reconcile(context.Background()); err != nil {
				t.Fatalf("initial deploy error = %v", err)
			}
			assertStrings(t, "pins after v1", pinner.snapshot(), []string{"img:v1"})

			sv.svc = &store.DesiredService{Name: "web", Image: "img:v2", Port: 80, Strategy: "blue-green"}
			rt.createErr = errors.New(tt.createEr)
			res, err := c.Reconcile(context.Background())
			cond := assertFailedPass(t, res, err)
			got, ok := failure.Classify(failure.Input{Status: "failed", Error: cond.Message + " " + err.Error()}, failure.Options{})
			if !ok || got.Code != tt.wantCode {
				t.Fatalf("Classify() = %q, %v, want %q", got.Code, ok, tt.wantCode)
			}
			if cs := rt.containers[v1]; cs == nil || !cs.Running || rt.removeCalls != 0 || rt.stopCalls != 0 {
				t.Fatalf("serving release disturbed: remove=%d stop=%d", rt.removeCalls, rt.stopCalls)
			}
			assertStrings(t, "pins after failed v2", pinner.snapshot(), []string{"img:v1"})

			rt.createErr = nil
			res, err = c.Reconcile(context.Background())
			if err != nil {
				t.Fatalf("pass after the image was repaired error = %v", err)
			}
			if cond := conditionOf(t, res); cond.Status != reconcile.ConditionTrue {
				t.Fatalf("condition after repair = %+v, want True", cond)
			}
			assertNames(t, "after repair", rt.names(), []string{v2})
			assertStrings(t, "pins after v2", pinner.snapshot(), []string{"img:v1", "img:v2"})
			assertNoopPass(t, c, rt)
			assertStrings(t, "pins after no-op", pinner.snapshot(), []string{"img:v1", "img:v2"})
		})
	}
}

func assertStrings(t *testing.T, what string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s = %v, want %v", what, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s = %v, want %v", what, got, want)
		}
	}
}

// Half-succeeded OOM: with two replicas, the first new replica is ready and
// the second is OOM-killed during its readiness wait. The service must keep
// serving throughout, report the typed reason, and converge once the
// memory problem is fixed.
func TestChaosPack_OOMOnSecondReplicaDuringReadiness_KeepsServingAndConverges(t *testing.T) {
	var unhealthy atomic.Bool
	srv := toggleHealth(&unhealthy)
	defer srv.Close()

	inner := newFakeRuntime(serverPort(t, srv))
	oldNames := []string{replicaContainerName("web", "img:v1", "", 0), replicaContainerName("web", "img:v1", "", 1)}
	newNames := []string{replicaContainerName("web", "img:v2", "", 0), replicaContainerName("web", "img:v2", "", 1)}
	for _, n := range oldNames {
		inner.seed(n, true)
	}
	var oomArmed atomic.Bool
	oomArmed.Store(true)
	rt := &startHookRuntime{fakeRuntime: inner, onStart: func(name string) {
		if name == newNames[1] && oomArmed.Load() {
			unhealthy.Store(true)
			time.AfterFunc(30*time.Millisecond, func() { inner.crashContainer(name, true, 137) })
		}
	}}
	desired := &store.DesiredService{
		Name: "web", Image: "img:v2", Port: 80, Replicas: 2, Strategy: "rolling",
		Health: &store.ServiceHealth{Readiness: &store.ServiceProbe{Path: "/healthz", Interval: 10 * time.Millisecond, Timeout: 50 * time.Millisecond}},
	}
	c := New("web", &fakeStore{svc: desired}, rt, WithReadyBudget(2*time.Second))

	res, err := c.Reconcile(context.Background())
	assertFailedPass(t, res, err)
	assertConditionReasonFalse(t, res, "OOMKilledDuringReadiness")
	if len(runningNames(inner)) == 0 {
		t.Fatal("no container left serving after a replica was OOM-killed mid-rollout")
	}
	if got := len(inner.names()); got > 4 {
		t.Fatalf("containers = %v, want no more than old and new replica sets", inner.names())
	}

	oomArmed.Store(false)
	unhealthy.Store(false)
	inner.mu.Lock()
	delete(inner.exitStates, newNames[1])
	inner.mu.Unlock()
	for pass := 0; pass < 3; pass++ {
		if _, err := c.Reconcile(context.Background()); err == nil {
			break
		}
	}
	assertNames(t, "after recovery", inner.names(), newNames)
	assertRunning(t, "after recovery", inner, newNames...)
	assertNoopPass(t, c, inner)
}

// OOM on a single replica, then the cause is fixed: the next pass promotes
// the replacement and the pass after it is a no-op.
func TestChaosPack_OOMDuringReadiness_NextPassAfterFixPromotesAndIsThenNoop(t *testing.T) {
	var unhealthy atomic.Bool
	unhealthy.Store(true)
	srv := toggleHealth(&unhealthy)
	defer srv.Close()

	rt := newFakeRuntime(serverPort(t, srv))
	v1 := ContainerName("web", "img:v1", "")
	v2 := ContainerName("web", "img:v2", "")
	rt.seed(v1, true)
	desired := &store.DesiredService{
		Name: "web", Image: "img:v2", Port: 80, Strategy: "blue-green",
		Health: &store.ServiceHealth{Readiness: &store.ServiceProbe{Path: "/healthz", Interval: 10 * time.Millisecond, Timeout: 50 * time.Millisecond}},
	}
	time.AfterFunc(40*time.Millisecond, func() { rt.crashContainer(v2, true, 137) })
	c := New("web", &fakeStore{svc: desired}, rt, WithReadyBudget(2*time.Second))

	res, err := c.Reconcile(context.Background())
	assertFailedPass(t, res, err)
	assertConditionReasonFalse(t, res, "OOMKilledDuringReadiness")
	assertRunning(t, "after OOM", rt, v1)

	unhealthy.Store(false)
	rt.mu.Lock()
	delete(rt.exitStates, v2)
	rt.mu.Unlock()
	for pass := 0; pass < 3; pass++ {
		if _, err := c.Reconcile(context.Background()); err == nil {
			break
		}
	}
	assertNames(t, "after fix", rt.names(), []string{v2})
	assertRunning(t, "after fix", rt, v2)
	assertNoopPass(t, c, rt)
}

// startHookRuntime reports each container start so a test can inject a fault
// at a precise point of a multi-replica rollout.
type startHookRuntime struct {
	*fakeRuntime
	onStart func(name string)
}

func (s *startHookRuntime) Start(ctx context.Context, id string) error {
	if err := s.fakeRuntime.Start(ctx, id); err != nil {
		return err
	}
	s.mu.Lock()
	var name string
	for n, cs := range s.containers {
		if cs.ID == id {
			name = n
		}
	}
	s.mu.Unlock()
	if s.onStart != nil {
		s.onStart(name)
	}
	return nil
}

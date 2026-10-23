package application

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	dockerclient "github.com/docker/docker/client"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/dockertest"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

// faultRuntime wraps a real Docker runtime and fails selected calls the way a
// dead daemon or a full disk does, so the surrounding containers are real.
type faultRuntime struct {
	docker.Runtime
	daemonDown    atomic.Bool
	diskFull      atomic.Bool
	downAfterMake atomic.Bool
}

var (
	errLiveDaemonDown = errors.New("cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?")
	errLiveDiskFull   = errors.New("write /var/lib/docker/tmp/GetImageBlob: no space left on device")
)

func (f *faultRuntime) InspectByName(ctx context.Context, name string) (*docker.ContainerState, error) {
	if f.daemonDown.Load() {
		return nil, errLiveDaemonDown
	}
	return f.Runtime.InspectByName(ctx, name)
}

func (f *faultRuntime) ListByPrefix(ctx context.Context, prefix string) ([]docker.ContainerState, error) {
	if f.daemonDown.Load() {
		return nil, errLiveDaemonDown
	}
	return f.Runtime.ListByPrefix(ctx, prefix)
}

func (f *faultRuntime) Create(ctx context.Context, spec docker.ContainerSpec) (string, error) {
	if f.daemonDown.Load() {
		return "", errLiveDaemonDown
	}
	if f.diskFull.Load() {
		return "", errLiveDiskFull
	}
	id, err := f.Runtime.Create(ctx, spec)
	if err == nil && f.downAfterMake.Load() {
		f.daemonDown.Store(true)
	}
	return id, err
}

func (f *faultRuntime) Start(ctx context.Context, id string) error {
	if f.daemonDown.Load() {
		return errLiveDaemonDown
	}
	return f.Runtime.Start(ctx, id)
}

func (f *faultRuntime) Stop(ctx context.Context, id string, timeout time.Duration) error {
	if f.daemonDown.Load() {
		return errLiveDaemonDown
	}
	return f.Runtime.Stop(ctx, id, timeout)
}

func (f *faultRuntime) Remove(ctx context.Context, id string, force bool) error {
	if f.daemonDown.Load() {
		return errLiveDaemonDown
	}
	return f.Runtime.Remove(ctx, id, force)
}

// liveChaosSetup connects to a real daemon, makes two local tags of the
// fixture image, deploys v1 and returns everything a fault test needs.
func liveChaosSetup(t *testing.T, serviceName string) (rawRT docker.Runtime, fault *faultRuntime, db *store.DB, desired store.DesiredService, v1, v2 string) {
	t.Helper()
	dockertest.SkipIfShort(t)
	rt, err := docker.NewClient()
	if err != nil {
		t.Skipf("no docker client available: %v", err)
	}
	rawCli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		t.Skipf("no docker client available: %v", err)
	}
	pingCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := rawCli.Ping(pingCtx); err != nil {
		t.Skipf("docker daemon not reachable: %v", err)
	}
	t.Cleanup(func() { _ = rt.Close() })

	ctx := context.Background()
	if err := pullIfMissing(ctx, t, rawCli); err != nil {
		t.Fatalf("pull fixture image: %v", err)
	}
	v1, v2 = "nginx:alpine", "nginx:alpine-levelrail-chaos-v2"
	if err := rawCli.ImageTag(ctx, v1, v2); err != nil {
		t.Fatalf("tag %s as %s: %v", v1, v2, err)
	}
	cleanupContainers(ctx, t, rt, serviceName)
	t.Cleanup(func() { cleanupContainers(context.Background(), t, rt, serviceName) })

	db = openLiveStore(t)
	desired = store.DesiredService{
		Name: serviceName, Image: v1, Port: 80, Strategy: "blue-green",
		Health: &store.ServiceHealth{Readiness: &store.ServiceProbe{Path: "/", Interval: 200 * time.Millisecond, Timeout: time.Second}},
	}
	if err := db.SaveDesiredService(ctx, desired); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}
	fault = &faultRuntime{Runtime: rt}
	ctrl := New(serviceName, db, fault, WithReadyBudget(20*time.Second))
	if _, err := ctrl.Reconcile(ctx); err != nil {
		t.Fatalf("initial deploy error = %v", err)
	}
	return rt, fault, db, desired, v1, v2
}

func liveRunning(t *testing.T, rt docker.Runtime, name string) bool {
	t.Helper()
	cs, err := rt.InspectByName(context.Background(), name)
	if err != nil {
		t.Fatalf("InspectByName(%q) error = %v", name, err)
	}
	return cs != nil && cs.Running
}

// Live: the daemon connection dies right after the replacement is created.
// The real old container keeps running, and once the daemon is back the
// service converges to the new release and a further pass is a no-op.
func TestChaosPack_Live_DaemonDiesMidDeploy_OldKeepsServingAndConverges(t *testing.T) {
	const svc = "levelrail-test-chaos-daemon"
	rt, fault, db, desired, v1, v2 := liveChaosSetup(t, svc)
	ctx := context.Background()
	oldName, newName := ContainerName(svc, v1, ""), ContainerName(svc, v2, "")

	desired.Image = v2
	if err := db.SaveDesiredService(ctx, desired); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}
	fault.downAfterMake.Store(true)
	if _, err := New(svc, db, fault, WithReadyBudget(20*time.Second)).Reconcile(ctx); err == nil {
		t.Fatal("pass with the daemon dying mid-deploy error = nil, want failure")
	}
	fault.downAfterMake.Store(false)
	if !liveRunning(t, rt, oldName) {
		t.Fatal("old release not running after the daemon connection died mid-deploy")
	}

	fault.daemonDown.Store(false)
	c := New(svc, db, fault, WithReadyBudget(20*time.Second))
	res, err := c.Reconcile(ctx)
	if err != nil {
		t.Fatalf("pass after the daemon returned error = %v", err)
	}
	if cond := conditionOf(t, res); cond.Status != reconcile.ConditionTrue {
		t.Fatalf("condition after recovery = %+v, want True", cond)
	}
	if !liveRunning(t, rt, newName) {
		t.Fatal("new release not running after recovery")
	}
	if cs, _ := rt.InspectByName(ctx, oldName); cs != nil {
		t.Fatalf("old release %q still present after recovery: orphaned", oldName)
	}
	before := containerCount(ctx, t, rt, svc)
	if _, err := c.Reconcile(ctx); err != nil {
		t.Fatalf("no-op pass error = %v", err)
	}
	if after := containerCount(ctx, t, rt, svc); after != before || after != 1 {
		t.Fatalf("container count before/after no-op pass = %d/%d, want 1/1", before, after)
	}
}

// Live: the disk fills while the next release is being created. The real old
// container keeps running with a clear failure reason, and freeing space
// converges the service.
func TestChaosPack_Live_DiskFullDuringDeploy_OldKeepsServingAndConverges(t *testing.T) {
	const svc = "levelrail-test-chaos-disk"
	rt, fault, db, desired, v1, v2 := liveChaosSetup(t, svc)
	ctx := context.Background()
	oldName, newName := ContainerName(svc, v1, ""), ContainerName(svc, v2, "")

	desired.Image = v2
	if err := db.SaveDesiredService(ctx, desired); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}
	fault.diskFull.Store(true)
	c := New(svc, db, fault, WithReadyBudget(20*time.Second))
	res, err := c.Reconcile(ctx)
	if err == nil {
		t.Fatal("pass on a full disk error = nil, want failure")
	}
	if cond := conditionOf(t, res); cond.Status != reconcile.ConditionFalse || cond.Reason != "CreateFailed" {
		t.Fatalf("condition = %+v, want False/CreateFailed", cond)
	}
	if !liveRunning(t, rt, oldName) {
		t.Fatal("old release stopped by a deploy that ran out of disk")
	}
	if got := containerCount(ctx, t, rt, svc); got != 1 {
		t.Fatalf("container count after disk-full pass = %d, want 1 (no partial release)", got)
	}

	fault.diskFull.Store(false)
	res, err = c.Reconcile(ctx)
	if err != nil {
		t.Fatalf("pass after space was freed error = %v", err)
	}
	if cond := conditionOf(t, res); cond.Status != reconcile.ConditionTrue {
		t.Fatalf("condition after recovery = %+v, want True", cond)
	}
	if !liveRunning(t, rt, newName) {
		t.Fatal("new release not running after space was freed")
	}
	before := containerCount(ctx, t, rt, svc)
	if _, err := c.Reconcile(ctx); err != nil {
		t.Fatalf("no-op pass error = %v", err)
	}
	if after := containerCount(ctx, t, rt, svc); after != before || after != 1 {
		t.Fatalf("container count before/after no-op pass = %d/%d, want 1/1", before, after)
	}
}

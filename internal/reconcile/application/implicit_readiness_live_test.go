package application

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	dockerclient "github.com/docker/docker/client"

	"github.com/GLINCKER/levelrail/internal/deploysafety"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/dockertest"
	"github.com/GLINCKER/levelrail/internal/store"
)

// An image that never listens on its declared port, with no readiness probe,
// must fail the deploy and leave the old release serving with zero failed
// requests. The published-port proxy accepts connections even when nothing
// listens behind it, which is the case a plain connect check would miss.
func TestController_NoProbeNeverListening_KeepsOldReleaseServing_Live(t *testing.T) {
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
	if _, err := rawCli.Ping(pingCtx); err != nil {
		cancel()
		t.Skipf("docker daemon not reachable: %v", err)
	}
	cancel()
	t.Cleanup(func() { _ = rt.Close() })

	const serviceName = "levelrail-test-implicit-ready"
	image1 := "nginx:alpine"
	image2 := "nginx:alpine-levelrail-test-never-listens"
	ctx := context.Background()
	if err := pullIfMissing(ctx, t, rawCli); err != nil {
		t.Fatalf("pull %s: %v", image1, err)
	}
	if err := rawCli.ImageTag(ctx, image1, image2); err != nil {
		t.Fatalf("tag: %v", err)
	}
	cleanupContainers(ctx, t, rt, serviceName)
	t.Cleanup(func() { cleanupContainers(context.Background(), t, rt, serviceName) })

	db := openLiveStore(t)
	rec := &fakeGateRecorder{}
	cfg := deploysafety.Effective{SuccessThreshold: 1, StableWindow: 0}
	ctrl := New(serviceName, db, rt, WithStabilityGate(cfg, nil), WithGateRecorder(rec), WithReadyBudget(3*time.Second))

	desired := store.DesiredService{Name: serviceName, Image: image1, Port: 80}
	if err := db.SaveDesiredService(ctx, desired); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}
	if _, err := ctrl.Reconcile(ctx); err != nil {
		t.Fatalf("first deploy (listening port) error = %v", err)
	}
	oldName := ContainerName(serviceName, image1, "")
	oldState, err := rt.InspectByName(ctx, oldName)
	if err != nil || oldState == nil || len(oldState.Ports) == 0 {
		t.Fatalf("old release state = %+v, err = %v", oldState, err)
	}
	oldURL := fmt.Sprintf("http://127.0.0.1:%d/", oldState.Ports[0].HostPort)

	var failed, total atomic.Int64
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		client := &http.Client{Timeout: time.Second}
		for {
			select {
			case <-stop:
				return
			default:
			}
			total.Add(1)
			resp, err := client.Get(oldURL)
			if err != nil || resp.StatusCode != http.StatusOK {
				failed.Add(1)
			}
			if resp != nil {
				_ = resp.Body.Close()
			}
			time.Sleep(20 * time.Millisecond)
		}
	}()

	desired.Image = image2
	desired.Port = 3000
	if err := db.SaveDesiredService(ctx, desired); err != nil {
		t.Fatalf("SaveDesiredService() redeploy error = %v", err)
	}
	_, reconcileErr := ctrl.Reconcile(ctx)
	close(stop)
	<-done

	if reconcileErr == nil {
		t.Fatal("Reconcile() error = nil, want the never-listens release to fail its readiness gate")
	}
	for _, e := range rec.events {
		if e.Image == image2 && e.Phase == deploysafety.PhaseCutover {
			t.Errorf("cutover event for the never-listening release: %+v", e)
		}
	}
	if st, _ := rt.InspectByName(ctx, oldName); st == nil || !st.Running {
		t.Errorf("old release = %+v, want it still running", st)
	}
	if failed.Load() != 0 || total.Load() == 0 {
		t.Errorf("old release served %d failed of %d requests, want 0 failed", failed.Load(), total.Load())
	}
}

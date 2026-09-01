package application

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	dockerclient "github.com/docker/docker/client"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/dockertest"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// TestController_Reconcile_Live_RecordsDeployMetric is TASKS.md 2.1's
// remaining-gap proof: a real telemetry.DB, wired via WithDeployRecorder,
// actually gains a queryable deploy_count sample after a real Reconcile
// performs a real deploy cutover, and gains no second sample on the
// following no-op reconcile, verified against the store directly rather
// than trusting Reconcile's returned Result.
func TestController_Reconcile_Live_RecordsDeployMetric(t *testing.T) {
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
	t.Cleanup(func() {
		if err := rt.Close(); err != nil {
			t.Errorf("closing docker client: %v", err)
		}
	})

	const serviceName = "levelrail-test-deploy-metric"
	image := "nginx:alpine"

	longCtx := context.Background()
	if err := pullIfMissing(longCtx, t, rawCli); err != nil {
		t.Fatalf("pull %s: %v", image, err)
	}

	cleanupContainers(longCtx, t, rt, serviceName)
	t.Cleanup(func() { cleanupContainers(context.Background(), t, rt, serviceName) })

	db := openLiveStore(t)

	telemetryDB, err := telemetry.Open(longCtx, filepath.Join(t.TempDir(), "telemetry.db"))
	if err != nil {
		t.Fatalf("telemetry.Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := telemetryDB.Close(); err != nil {
			t.Errorf("closing telemetry store: %v", err)
		}
	})

	desired := store.DesiredService{Name: serviceName, Image: image, Port: 80}
	if err := db.SaveDesiredService(longCtx, desired); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}

	ctrl := New(serviceName, db, rt, WithReadyBudget(20*time.Second), WithDeployRecorder(telemetryDB))

	before := time.Now()
	if _, err := ctrl.Reconcile(longCtx); err != nil {
		t.Fatalf("first Reconcile() error = %v", err)
	}
	after := time.Now()

	resourceID := "service:" + serviceName
	samples, err := telemetryDB.Query(longCtx, resourceID, telemetry.MetricDeployCount, before.Add(-time.Second), after.Add(time.Second))
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if len(samples) != 1 {
		t.Fatalf("Query() after first (deploying) reconcile = %d samples, want 1", len(samples))
	}
	if samples[0].Value != 1 {
		t.Errorf("samples[0].Value = %v, want 1", samples[0].Value)
	}

	// Second reconcile, same desired state: a no-op, must not record a
	// second deploy.
	if _, err := ctrl.Reconcile(longCtx); err != nil {
		t.Fatalf("second (no-op) Reconcile() error = %v", err)
	}
	samples, err = telemetryDB.Query(longCtx, resourceID, telemetry.MetricDeployCount, before.Add(-time.Second), time.Now().Add(time.Second))
	if err != nil {
		t.Fatalf("Query() after no-op reconcile error = %v", err)
	}
	if len(samples) != 1 {
		t.Errorf("Query() after a no-op reconcile = %d samples, want still 1 (a no-op tick is not a deploy)", len(samples))
	}
}

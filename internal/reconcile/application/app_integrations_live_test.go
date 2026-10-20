package application

import (
	"context"
	"slices"
	"testing"
	"time"

	dockerclient "github.com/docker/docker/client"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/dockertest"
	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

// TestController_Reconcile_Live_AppIntegration is the whole-chain proof
// for attached internal/integrations: a real master key, a real store,
// a real secrets.Manager, and a real Controller.Reconcile that creates
// a real container. Independently verified via the raw Docker Engine
// API (not this controller's own return value) that the container's
// actual environment contains the integration's resolved env var, and
// via the store directly that only ciphertext (never the plaintext)
// ever reached disk. A second phase then detaches the integration and
// proves its env var is gone from the next container Reconcile creates,
// the same "cleans up on next deploy" behavior the detach API endpoint
// promises.
func TestController_Reconcile_Live_AppIntegration(t *testing.T) {
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

	const serviceName = "levelrail-test-app-integration"
	const image = "nginx:alpine"
	longCtx := context.Background()

	if err := pullIfMissing(longCtx, t, rawCli); err != nil {
		t.Fatalf("pull %s: %v", image, err)
	}

	cleanupContainers(longCtx, t, rt, serviceName)
	t.Cleanup(func() { cleanupContainers(context.Background(), t, rt, serviceName) })

	db := openLiveStore(t)

	mk, err := secrets.GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() error = %v", err)
	}
	manager := secrets.NewManager(db, mk)

	const plaintext = "https://real-key@o0.ingest.sentry.io/0"
	namespace := store.AppIntegrationSecretsKey(serviceName, "sentry")
	if err := manager.SetValue(longCtx, namespace, "SENTRY_DSN", plaintext); err != nil {
		t.Fatalf("SetValue() error = %v", err)
	}

	desired := store.DesiredService{
		Name: serviceName, Image: image, Port: 80,
		Env: map[string]string{"NODE_ENV": "production"},
	}
	if err := db.SaveDesiredService(longCtx, desired); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}

	now := time.Now().UTC()
	if err := db.SaveAppIntegration(longCtx, store.AppIntegration{ID: "appint_live1", ServiceName: serviceName, IntegrationKey: "sentry", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("SaveAppIntegration() error = %v", err)
	}

	ctrl := New(serviceName, db, rt, WithAppIntegrations(db), WithSecretResolver(manager))

	result, err := ctrl.Reconcile(longCtx)
	if err != nil {
		t.Fatalf("Reconcile() error = %v, result = %+v", err, result)
	}
	if cond := conditionOf(t, result); cond.Status != "True" {
		t.Fatalf("Reconcile() condition = %+v, want Status=True", cond)
	}

	target := ContainerName(serviceName, image, "")
	inspect, err := rawCli.ContainerInspect(longCtx, target)
	if err != nil {
		t.Fatalf("raw ContainerInspect(%q) error = %v", target, err)
	}
	if !slices.Contains(inspect.Config.Env, "SENTRY_DSN="+plaintext) {
		t.Errorf("container env = %v, want it to contain SENTRY_DSN=%s", inspect.Config.Env, plaintext)
	}
	if !slices.Contains(inspect.Config.Env, "NODE_ENV=production") {
		t.Errorf("container env = %v, want the literal NODE_ENV=production preserved alongside the resolved integration value", inspect.Config.Env)
	}

	// Independent verification the plaintext never reached disk: the
	// store's own secret_values table must hold ciphertext, not this
	// exact string, under the integration's own namespace.
	ciphertext, err := db.GetSecretValue(longCtx, namespace, "SENTRY_DSN")
	if err != nil {
		t.Fatalf("GetSecretValue() error = %v", err)
	}
	if string(ciphertext) == plaintext {
		t.Fatal("stored integration field value equals the plaintext verbatim, encryption did not happen")
	}

	// Detach: the same two calls handleDetachAppIntegration makes
	// (clear the secrets namespace, then remove the attachment row).
	if err := manager.DeleteAll(longCtx, namespace); err != nil {
		t.Fatalf("DeleteAll() error = %v", err)
	}
	if err := db.DeleteAppIntegration(longCtx, "appint_live1"); err != nil {
		t.Fatalf("DeleteAppIntegration() error = %v", err)
	}

	// Simulate the next deploy's cleanup-then-recreate cycle: remove the
	// existing container (same as a rolling deploy's cutover step) and
	// reconcile again from a clean slate.
	cleanupContainers(longCtx, t, rt, serviceName)

	result2, err := ctrl.Reconcile(longCtx)
	if err != nil {
		t.Fatalf("second Reconcile() error = %v, result = %+v", err, result2)
	}
	if cond := conditionOf(t, result2); cond.Status != "True" {
		t.Fatalf("second Reconcile() condition = %+v, want Status=True", cond)
	}

	inspect2, err := rawCli.ContainerInspect(longCtx, target)
	if err != nil {
		t.Fatalf("raw ContainerInspect(%q) after detach error = %v", target, err)
	}
	for _, kv := range inspect2.Config.Env {
		if slices.Contains([]string{"SENTRY_DSN=" + plaintext}, kv) {
			t.Errorf("container env after detach = %v, want SENTRY_DSN removed", inspect2.Config.Env)
		}
	}
}

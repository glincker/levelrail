package application

import (
	"context"
	"slices"
	"testing"
	"time"

	dockerclient "github.com/docker/docker/client"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

// TestController_Reconcile_Live_SecretEnv is the whole-chain proof for
// TASKS.md 1.7's env-injection follow-up: a real master key, a real
// store, a real secrets.Manager, and a real Controller.Reconcile that
// creates a real container. Independently verified via the raw Docker
// Engine API, not this controller's own return value, that the
// container's actual environment contains the decrypted secret value,
// and via the store directly that only ciphertext (never the plaintext)
// ever reached disk.
func TestController_Reconcile_Live_SecretEnv(t *testing.T) {
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

	const serviceName = "levelrail-test-secret-env"
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

	const plaintext = "sk-real-secret-abc123" //nolint:gosec // fake fixture, the whole test proves it never reaches the store in this form
	if err := manager.SetValue(longCtx, serviceName, "API_KEY", plaintext); err != nil {
		t.Fatalf("SetValue() error = %v", err)
	}

	desired := store.DesiredService{
		Name: serviceName, Image: image, Port: 80,
		Env:       map[string]string{"NODE_ENV": "production"},
		SecretEnv: []string{"API_KEY"},
	}
	if err := db.SaveDesiredService(longCtx, desired); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}

	ctrl := New(serviceName, db, rt, WithSecretResolver(manager))

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
	if !slices.Contains(inspect.Config.Env, "API_KEY="+plaintext) {
		t.Errorf("container env = %v, want it to contain API_KEY=%s", inspect.Config.Env, plaintext)
	}
	if !slices.Contains(inspect.Config.Env, "NODE_ENV=production") {
		t.Errorf("container env = %v, want the literal NODE_ENV=production preserved alongside the resolved secret", inspect.Config.Env)
	}

	// Independent verification the plaintext never reached disk: the
	// store's own secret_values table must hold ciphertext, not this
	// exact string.
	ciphertext, err := db.GetSecretValue(longCtx, serviceName, "API_KEY")
	if err != nil {
		t.Fatalf("GetSecretValue() error = %v", err)
	}
	if string(ciphertext) == plaintext {
		t.Fatal("stored secret value equals the plaintext verbatim, encryption did not happen")
	}
}

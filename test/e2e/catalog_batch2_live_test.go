// TestServiceTemplates_Live_Batch2Boots is this package's live proof for
// a sample of internal/catalog's third Coolify-catalog import wave
// (ADR 015, templates_catalog_batch2.go), the same shape as
// catalog_devtools_batch_live_test.go: catalog_test.go already proves
// every entry's Compose parses and translates to an active-probe
// readiness check in isolation; this proves POSTing to the real
// one-click deploy endpoint and reconciling the result with a real
// application.Controller produces real, running Docker containers.
package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/brand"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/secrets"
)

const (
	e2eBatch2AdminUsername = "e2e-batch2-admin"
	e2eBatch2AdminPassword = "e2e-batch2-correct-horse" //nolint:gosec // test fixture credential, not a real secret
)

// TestServiceTemplates_Live_Batch2Boots deploys five of the wave's
// entries through the real one-click path and reconciles every
// resulting service with a real application.Controller, proving each
// one actually boots: three single-container entries with no backing
// service, one that generates its own auth file at container start
// (mosquitto, the same pattern as the devtools-batch's
// docker-registry-auth entry), and matrix-synapse-postgres, the
// heaviest and most config-generation-reliant entry in the wave.
func TestServiceTemplates_Live_Batch2Boots(t *testing.T) {
	env := newLiveBuildEnv(t)
	runtime := env.Runtime

	svcStore := openLiveStore(t)

	mk, err := secrets.GenerateMasterKey()
	if err != nil {
		t.Fatalf("secrets.GenerateMasterKey() error = %v", err)
	}
	secretsManager := secrets.NewManager(svcStore, mk)

	logger := discardTestLogger()
	b := &brand.Brand{Name: "E2E Test Platform", BinaryName: "e2e-test-platform"}
	router := api.NewRouter(logger, b, svcStore, api.WithComposeSecrets(secretsManager))
	ts := newE2ETestServer(t, router)

	ctx := context.Background()
	if err := api.BootstrapAdmin(ctx, svcStore, e2eBatch2AdminUsername, e2eBatch2AdminPassword); err != nil {
		t.Fatalf("BootstrapAdmin() error = %v", err)
	}
	client := loginE2EClient(t, ts.URL, e2eBatch2AdminUsername, e2eBatch2AdminPassword)

	tests := []struct {
		templateID  string
		readyBudget time.Duration
	}{
		{"whoogle", 60 * time.Second},
		{"pairdrop", 60 * time.Second},
		{"phpmyadmin", 60 * time.Second},
		{"mosquitto", 60 * time.Second},
		{"matrix-synapse-postgres", 120 * time.Second},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.templateID, func(t *testing.T) {
			if tt.templateID == "matrix-synapse-postgres" && testing.Short() {
				t.Skip("too slow for the PR fast lane's 18m budget, covered by the nightly full suite")
			}
			deployCtx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
			defer cancel()

			resp := deployServiceTemplateNow(t, client, ts.URL, tt.templateID)
			if len(resp.Services) == 0 {
				t.Fatalf("deploy %q: got 0 services, want at least 1", tt.templateID)
			}

			t.Cleanup(func() {
				cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cleanupCancel()
				for _, svc := range resp.Services {
					cleanupContainers(cleanupCtx, t, runtime, svc.Name)
				}
				removeAppVolumes(cleanupCtx, t, env.DockerCli, resp.AppID)
				if err := runtime.RemoveNetwork(cleanupCtx, application.NetworkName("", resp.AppID)); err != nil {
					t.Logf("cleanup: RemoveNetwork(%q) error = %v", resp.AppID, err)
				}
			})

			// Services come back from compose.ToDesiredServices in
			// sorted-key order; matrix-synapse-postgres's own "postgres"
			// key sorts before "synapse", a real dependency-first order
			// (see this file's own doc comment), so reconciling in
			// response order is sufficient here same as the devtools
			// batch's own live test.
			for _, svc := range resp.Services {
				ctrl := application.New(svc.Name, svcStore, runtime,
					application.WithReadyBudget(tt.readyBudget),
					application.WithSecretResolver(secretsManager),
				)
				// One Reconcile is not enough on a cold runner: the image pull and
				// first start can outlast a single pass. Retry until Ready or twice
				// the ready budget has passed.
				if result, err := reconcileUntilReady(deployCtx, ctrl, 2*tt.readyBudget); err != nil {
					t.Fatalf("service %q: %v (result = %+v)", svc.Name, err, result)
				}

				target := application.ContainerName(svc.Name, svc.Image, "")
				state, err := runtime.InspectByName(deployCtx, target)
				if err != nil {
					t.Fatalf("InspectByName(%q) error = %v", target, err)
				}
				if state == nil || !state.Running {
					t.Fatalf("InspectByName(%q) = %+v, want a running container", target, state)
				}
			}
		})
	}
}

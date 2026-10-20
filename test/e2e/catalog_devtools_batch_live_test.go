// TestServiceTemplates_Live_DevtoolsBatchBoots is this package's live
// proof for a sample of internal/catalog's devtools-batch import (ADR
// 015, templates_devtools_batch.go): catalog_test.go already proves
// every entry's Compose parses and translates to an active-probe
// readiness check in isolation; this proves POSTing to the real
// one-click deploy endpoint and reconciling the result with a real
// application.Controller produces real, running Docker containers.
package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/volume"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/brand"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/test/e2e/testenv"
)

const (
	e2eDevtoolsBatchAdminUsername = "e2e-devtools-batch-admin"
	e2eDevtoolsBatchAdminPassword = "e2e-devtools-batch-correct-horse" //nolint:gosec // test fixture credential, not a real secret
)

// TestServiceTemplates_Live_DevtoolsBatchBoots deploys four of the
// batch's simplest, fastest entries through the real one-click path and
// reconciles every resulting service with a real application.Controller,
// proving each one actually boots. The heavier or operationally-gated
// entries are left to catalog_test.go's static checks; see this PR's
// description for why (startup time, or, for vault, sealed by design).
func TestServiceTemplates_Live_DevtoolsBatchBoots(t *testing.T) {
	testenv.RequireFullLive(t)
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
	if err := api.BootstrapAdmin(ctx, svcStore, e2eDevtoolsBatchAdminUsername, e2eDevtoolsBatchAdminPassword); err != nil {
		t.Fatalf("BootstrapAdmin() error = %v", err)
	}
	client := loginE2EClient(t, ts.URL, e2eDevtoolsBatchAdminUsername, e2eDevtoolsBatchAdminPassword)

	tests := []struct {
		templateID  string
		readyBudget time.Duration
	}{
		{"jupyter-notebook", 60 * time.Second},
		{"docker-registry-auth", 60 * time.Second},
		{"grafana-postgres", 90 * time.Second},
		{"gitea-postgres", 90 * time.Second},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.templateID, func(t *testing.T) {
			deployCtx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
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
				// application.Controller creates this app's shared network
				// on first use but never deletes it itself (only
				// NetworkCleanupController does, store.App-row-driven, not
				// run in this test); removing it here avoids leaking one
				// per run against Docker's limited default address pool.
				if err := runtime.RemoveNetwork(cleanupCtx, application.NetworkName("", resp.AppID)); err != nil {
					t.Logf("cleanup: RemoveNetwork(%q) error = %v", resp.AppID, err)
				}
			})

			// Services come back from compose.ToDesiredServices in
			// sorted-key order, which for every template picked here
			// means a db/backing service before the app that depends on
			// it (see this file's own doc comment), so reconciling in
			// response order respects that ordering without extra logic.
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

func deployServiceTemplateNow(t *testing.T, client *http.Client, baseURL, templateID string) composeDeployResponseMirror {
	t.Helper()
	status, body := postJSON(t, client, baseURL+"/api/v1/service-templates/"+templateID+"/deploy", "")
	if status != http.StatusOK {
		t.Fatalf("deploy %q: status = %d, want %d, body = %s", templateID, status, http.StatusOK, body)
	}
	var got composeDeployResponseMirror
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("deploy %q: decode response: %v, body = %s", templateID, err, body)
	}
	return got
}

// composeDeployResponseMirror is POST .../service-templates/{id}/deploy's
// response shape (internal/api's own composeDeployResponse, package-
// private), re-declared field-for-field so this external test package
// can decode it without exporting an internal API response type just
// for a test.
type composeDeployResponseMirror struct {
	AppID    string `json:"app_id"`
	Services []struct {
		Name  string `json:"name"`
		Image string `json:"image"`
	} `json:"services"`
}

// removeAppVolumes removes every named Docker volume
// compose.ToDesiredServices created for appID (internal/compose's own
// volumeName convention: "app-<appID>-<serviceKey>-<logicalName>"),
// which application.Controller's own container removal never deletes
// (cleanupContainers calls Runtime.Remove with force, not
// remove-volumes), so a repeat test run would otherwise leak one named
// volume per service per run.
func removeAppVolumes(ctx context.Context, t *testing.T, cli volumeRemover, appID string) {
	t.Helper()
	prefix := "app-" + appID + "-"
	list, err := cli.VolumeList(ctx, volume.ListOptions{Filters: filters.NewArgs()})
	if err != nil {
		t.Logf("removeAppVolumes: VolumeList() error = %v", err)
		return
	}
	for _, v := range list.Volumes {
		if len(v.Name) < len(prefix) || v.Name[:len(prefix)] != prefix {
			continue
		}
		if err := cli.VolumeRemove(ctx, v.Name, true); err != nil {
			t.Logf("removeAppVolumes: VolumeRemove(%q) error = %v", v.Name, err)
		}
	}
}

// volumeRemover is the narrow slice of *dockerclient.Client this file
// needs, named here so removeAppVolumes doesn't import the client
// package's full concrete type just to call two methods already used
// elsewhere in this package (live_api_helpers.go's removeContainerAndVolumes).
type volumeRemover interface {
	VolumeList(ctx context.Context, options volume.ListOptions) (volume.ListResponse, error)
	VolumeRemove(ctx context.Context, volumeID string, force bool) error
}

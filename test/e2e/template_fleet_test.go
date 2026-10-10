// TestTemplateFleet_Live_DeploysAndTearsDownCleanly closes
// docs/feature-status.md's own gap: no catalog template had ever been
// live-deployed and verified in CI. It drives a diverse sample through
// the real one-click deploy route, a real application.Controller per
// service, then real teardown, asserting nothing leaks. One t.Run
// subtest per template, so a broken one fails individually.
package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/volume"
	dockerclient "github.com/docker/docker/client"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/brand"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/test/e2e/testenv"
)

const (
	templateFleetAdminUsername = "e2e-template-fleet-admin"
	templateFleetAdminPassword = "e2e-template-fleet-correct-horse" //nolint:gosec // test fixture credential, not a real secret

	templateFleetDefaultReadyBudget    = 60 * time.Second
	templateFleetDefaultServiceTimeout = 3 * time.Minute
	templateFleetPollInterval          = 3 * time.Second
	templateFleetReconcileCallTimeout  = 3 * time.Minute
)

// templateFleetCase is one catalog.Templates entry picked for live
// deploy verification. serviceOrder encodes this test's own
// dependency-first ordering: dependencyBlock only checks that a
// depends_on: target has started, it never reconciles it, and most of
// this sample's own db-backed templates declare no depends_on: at all.
type templateFleetCase struct {
	id string
	// serviceOrder lists this template's own compose service keys
	// (catalog's own Compose body, not always equal to id: wikijs's
	// own service is keyed "wiki"), dependency first.
	serviceOrder []string
	// readyBudget overrides application.WithReadyBudget's per-attempt
	// probe wait; zero uses templateFleetDefaultReadyBudget. Set higher
	// for templates whose own healthcheck declares a long start_period.
	readyBudget time.Duration
	// serviceTimeout overrides the per-service outer retry-loop
	// deadline (reconcileUntilReady); zero uses
	// templateFleetDefaultServiceTimeout.
	serviceTimeout time.Duration
}

func (tc templateFleetCase) readyBudgetOrDefault() time.Duration {
	if tc.readyBudget > 0 {
		return tc.readyBudget
	}
	return templateFleetDefaultReadyBudget
}

func (tc templateFleetCase) serviceTimeoutOrDefault() time.Duration {
	if tc.serviceTimeout > 0 {
		return tc.serviceTimeout
	}
	return templateFleetDefaultServiceTimeout
}

// templateFleetSample is a deliberately diverse ~16-template slice of
// the 200+ catalog, not an attempt at full coverage: single- and
// multi-service, six Postgres-backed templates with no depends_on:
// (the race above), two Starter Kits entries that do declare it, across
// nine categories and four runtimes.
var templateFleetSample = []templateFleetCase{
	{id: "uptime-kuma", serviceOrder: []string{"uptime-kuma"}},
	{id: "homepage", serviceOrder: []string{"homepage"}},
	{id: "vaultwarden", serviceOrder: []string{"vaultwarden"}},
	{id: "code-server", serviceOrder: []string{"code-server"}},
	{id: "n8n", serviceOrder: []string{"n8n"}},
	{id: "mealie", serviceOrder: []string{"mealie"}, readyBudget: 90 * time.Second, serviceTimeout: 4 * time.Minute},
	{id: "nocodb", serviceOrder: []string{"nocodb"}, readyBudget: 90 * time.Second, serviceTimeout: 4 * time.Minute},
	{id: "qdrant", serviceOrder: []string{"qdrant"}},
	{id: "gitea", serviceOrder: []string{"db", "gitea"}, serviceTimeout: 4 * time.Minute},
	{id: "vikunja", serviceOrder: []string{"db", "vikunja"}, serviceTimeout: 4 * time.Minute},
	{id: "miniflux", serviceOrder: []string{"db", "miniflux"}, serviceTimeout: 4 * time.Minute},
	{id: "umami", serviceOrder: []string{"db", "umami"}, serviceTimeout: 4 * time.Minute},
	{id: "healthchecks", serviceOrder: []string{"db", "healthchecks"}, readyBudget: 90 * time.Second, serviceTimeout: 4 * time.Minute},
	{id: "wikijs", serviceOrder: []string{"db", "wiki"}, readyBudget: 90 * time.Second, serviceTimeout: 4 * time.Minute},
	{id: "node-postgres-starter", serviceOrder: []string{"db", "web"}},
	{id: "redis-cache-starter", serviceOrder: []string{"cache", "web"}},
	{id: "mailpit", serviceOrder: []string{"mailpit"}},
	{id: "pocketbase", serviceOrder: []string{"pocketbase"}},
	{id: "minio", serviceOrder: []string{"minio"}},
	{id: "grafana", serviceOrder: []string{"grafana"}},
	{id: "metabase", serviceOrder: []string{"metabase"}, readyBudget: 120 * time.Second, serviceTimeout: 5 * time.Minute},
	{id: "nextcloud", serviceOrder: []string{"nextcloud"}, readyBudget: 120 * time.Second, serviceTimeout: 5 * time.Minute},
	{id: "ghost", serviceOrder: []string{"db", "ghost"}, readyBudget: 120 * time.Second, serviceTimeout: 5 * time.Minute},
	{id: "wordpress", serviceOrder: []string{"db", "wordpress"}, readyBudget: 120 * time.Second, serviceTimeout: 5 * time.Minute},
	{id: "listmonk", serviceOrder: []string{"db", "listmonk"}, serviceTimeout: 4 * time.Minute},
	{id: "documenso", serviceOrder: []string{"database", "documenso"}, readyBudget: 120 * time.Second, serviceTimeout: 5 * time.Minute},
	{id: "plausible", serviceOrder: []string{"db", "clickhouse", "plausible"}, readyBudget: 120 * time.Second, serviceTimeout: 6 * time.Minute},
	{id: "linkwarden", serviceOrder: []string{"db", "meilisearch", "linkwarden"}, readyBudget: 120 * time.Second, serviceTimeout: 6 * time.Minute},
}

// templateFleetSmokeIDs is the PR subset: one single-service template, one
// tiny one, and one db-backed multi-service template.
var templateFleetSmokeIDs = map[string]bool{"uptime-kuma": true, "mailpit": true, "gitea": true}

// templateFleetParallelism is how many templates deploy at once, from
// LEVELRAIL_FLEET_PARALLEL (default 3, 1 runs them one at a time).
func templateFleetParallelism() int {
	if n, err := strconv.Atoi(os.Getenv("LEVELRAIL_FLEET_PARALLEL")); err == nil && n >= 1 {
		return n
	}
	return 3
}

func templateFleetCases() []templateFleetCase {
	if !testenv.LiveSmokeOnly() {
		return templateFleetSample
	}
	var out []templateFleetCase
	for _, tc := range templateFleetSample {
		if templateFleetSmokeIDs[tc.id] {
			out = append(out, tc)
		}
	}
	return out
}

// templateFleetDeployResponse mirrors internal/api's own (unexported)
// composeDeployResponse wire shape: only the fields this test needs to
// drive the rest of the deploy.
type templateFleetDeployResponse struct {
	AppID    string `json:"app_id"`
	Services []struct {
		Name string `json:"name"`
	} `json:"services"`
}

func TestTemplateFleet_Live_DeploysAndTearsDownCleanly(t *testing.T) {
	dockerCli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		t.Skipf("no docker client available: %v", err)
	}
	t.Cleanup(func() { _ = dockerCli.Close() })

	pingCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	_, err = dockerCli.Ping(pingCtx)
	cancel()
	if err != nil {
		t.Skipf("docker daemon not reachable: %v", err)
	}

	runtime, err := docker.NewClient()
	if err != nil {
		t.Fatalf("docker.NewClient() error = %v", err)
	}
	t.Cleanup(func() {
		if err := runtime.Close(); err != nil {
			t.Errorf("closing docker.Client: %v", err)
		}
	})

	svcStore := openLiveStore(t)

	// A real secrets.Manager under a real, freshly generated master key:
	// every multi-secret template in this sample (vaultwarden's
	// ADMIN_TOKEN, qdrant's API key, every Postgres password) fails its
	// deploy with "no secrets master key configured" without one
	// (apps_compose.go's generateComposeSecret), the exact same wiring a
	// real control plane always has in production.
	masterKey, err := secrets.GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() error = %v", err)
	}
	secretsManager := secrets.NewManager(svcStore, masterKey)

	logger := discardTestLogger()
	b := &brand.Brand{Name: "E2E Test Platform", BinaryName: "e2e-test-platform"}
	router := api.NewRouter(logger, b, svcStore, api.WithComposeSecrets(secretsManager))
	ts := newE2ETestServer(t, router)

	if err := api.BootstrapAdmin(context.Background(), svcStore, templateFleetAdminUsername, templateFleetAdminPassword); err != nil {
		t.Fatalf("BootstrapAdmin() error = %v", err)
	}
	client := loginE2EClient(t, ts.URL, templateFleetAdminUsername, templateFleetAdminPassword)

	limit := templateFleetParallelism()
	sem := make(chan struct{}, limit)
	for _, tc := range templateFleetCases() {
		tc := tc
		t.Run(tc.id, func(t *testing.T) {
			if limit > 1 {
				t.Parallel()
				sem <- struct{}{}
				defer func() { <-sem }()
			}
			runTemplateFleetCase(t, client, ts.URL, svcStore, runtime, dockerCli, secretsManager, tc)
		})
	}
}

// runTemplateFleetCase deploys tc's template through the real one-click
// API route, converges every one of its services through a real
// application.Controller (dependency-first, per tc.serviceOrder),
// verifies each is independently running, then tears every bit of it
// back down and asserts nothing leaked.
func runTemplateFleetCase(t *testing.T, client *http.Client, baseURL string, svcStore *store.DB, runtime docker.Runtime, dockerCli *dockerclient.Client, secretsManager *secrets.Manager, tc templateFleetCase) {
	t.Helper()

	status, body := postJSON(t, client, baseURL+"/api/v1/service-templates/"+tc.id+"/deploy", "")
	if status != http.StatusOK {
		t.Fatalf("deploy %q: status = %d, want %d, body = %s", tc.id, status, http.StatusOK, body)
	}

	var resp templateFleetDeployResponse
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("decode deploy response for %q: %v; body = %s", tc.id, err, body)
	}
	if resp.AppID == "" {
		t.Fatalf("deploy %q: response has no app_id; body = %s", tc.id, body)
	}
	if len(resp.Services) != len(tc.serviceOrder) {
		t.Fatalf("deploy %q: got %d service(s), want %d (serviceOrder = %v); body = %s", tc.id, len(resp.Services), len(tc.serviceOrder), tc.serviceOrder, body)
	}

	appID := resp.AppID
	serviceNames := make([]string, len(tc.serviceOrder))
	for i, key := range tc.serviceOrder {
		serviceNames[i] = appID + "-" + key
	}

	// Registered before any reconcile call below: a readiness failure
	// partway through must still tear down every container, volume, and
	// network this subtest's own deploy created, the same
	// "clean up no matter where this fails" shape every other live test
	// in this package already applies via t.Cleanup.
	t.Cleanup(func() {
		teardownTemplateDeploy(context.Background(), t, svcStore, runtime, dockerCli, appID, serviceNames)
	})

	for i, serviceName := range serviceNames {
		ctrl := application.New(serviceName, svcStore, runtime,
			application.WithSecretResolver(secretsManager),
			application.WithReadyBudget(tc.readyBudgetOrDefault()),
		)

		result, err := reconcileUntilReady(context.Background(), ctrl, tc.serviceTimeoutOrDefault())
		if err != nil {
			t.Fatalf("template %q service %q: %v", tc.id, serviceName, err)
		}
		persistReadyCondition(context.Background(), t, svcStore, ctrl.Name(), result.Conditions)

		containers, err := runtime.ListByPrefix(context.Background(), serviceName+"-")
		if err != nil {
			t.Fatalf("template %q service %q: ListByPrefix() error = %v", tc.id, serviceName, err)
		}
		if len(containers) == 0 {
			t.Fatalf("template %q service %q: no container found after a Ready reconcile", tc.id, serviceName)
		}
		if !containers[0].Running {
			t.Fatalf("template %q service %q: container %+v is not running after a Ready reconcile", tc.id, serviceName, containers[0])
		}

		// Stagger before the next service: a db with no declared
		// healthcheck reports Ready the moment its process starts, not
		// once it accepts connections. reconcileUntilReady's retry loop
		// still recovers a too-early app crash either way.
		if i < len(serviceNames)-1 {
			time.Sleep(5 * time.Second)
		}
	}
}

// reconcileUntilReady calls ctrl.Reconcile repeatedly until it reports a
// True Ready condition or overallTimeout elapses: the same resync-loop
// shape reconcile.Engine applies in production (Run's resyncInterval
// ticks), needed here because Docker's own RestartPolicy is
// deliberately disabled (internal/docker/client.go) and a service that
// crashes before its dependency is ready only comes back on the next
// reconcile pass, never on its own.
func reconcileUntilReady(parent context.Context, ctrl *application.Controller, overallTimeout time.Duration) (reconcile.Result, error) {
	deadline := time.Now().Add(overallTimeout)
	var lastResult reconcile.Result
	var lastErr error
	for {
		callCtx, cancel := context.WithTimeout(parent, templateFleetReconcileCallTimeout)
		lastResult, lastErr = ctrl.Reconcile(callCtx)
		cancel()
		if lastErr == nil && len(lastResult.Conditions) > 0 && lastResult.Conditions[0].Status == reconcile.ConditionTrue {
			return lastResult, nil
		}
		if time.Now().After(deadline) {
			return lastResult, fmt.Errorf("did not reach a True Ready condition within %s: last error = %v, last result = %+v", overallTimeout, lastErr, lastResult)
		}
		time.Sleep(templateFleetPollInterval)
	}
}

// teardownTemplateDeploy mirrors handleDeleteApp's own teardown, then
// also removes the per-app network and named volumes a real delete
// deliberately leaves behind (so a redeploy under the same name can
// find them again; this test's deploys never redeploy, so keeping them
// would be a genuine leak), and re-checks all three independently.
func teardownTemplateDeploy(ctx context.Context, t *testing.T, svcStore *store.DB, runtime docker.Runtime, dockerCli *dockerclient.Client, appID string, serviceNames []string) {
	t.Helper()

	for _, serviceName := range serviceNames {
		ctrl := application.New(serviceName, svcStore, runtime)
		if err := ctrl.Teardown(ctx); err != nil {
			t.Errorf("Teardown(%q) error = %v", serviceName, err)
		}
		if err := svcStore.DeleteDesiredService(ctx, serviceName); err != nil {
			t.Errorf("DeleteDesiredService(%q) error = %v", serviceName, err)
		}
	}

	networkName := application.NetworkName("", appID)
	if err := runtime.RemoveNetwork(ctx, networkName); err != nil {
		t.Errorf("RemoveNetwork(%q) error = %v", networkName, err)
	}

	volumePrefix := "app-" + appID + "-"
	for _, name := range listVolumesByPrefix(ctx, t, dockerCli, volumePrefix) {
		if err := dockerCli.VolumeRemove(ctx, name, true); err != nil {
			t.Errorf("VolumeRemove(%q) error = %v", name, err)
		}
	}

	for _, serviceName := range serviceNames {
		containers, err := runtime.ListByPrefix(ctx, serviceName+"-")
		if err != nil {
			t.Errorf("post-teardown ListByPrefix(%q) error = %v", serviceName, err)
			continue
		}
		if len(containers) != 0 {
			t.Errorf("service %q: %d container(s) leaked after teardown: %+v", serviceName, len(containers), containers)
		}
	}

	networks, err := runtime.ListNetworksByPrefix(ctx, networkName)
	if err != nil {
		t.Errorf("post-teardown ListNetworksByPrefix(%q) error = %v", networkName, err)
	} else if len(networks) != 0 {
		t.Errorf("app %q: network leaked after teardown: %+v", appID, networks)
	}

	if leaked := listVolumesByPrefix(ctx, t, dockerCli, volumePrefix); len(leaked) != 0 {
		t.Errorf("app %q: volume(s) leaked after teardown: %v", appID, leaked)
	}
}

// listVolumesByPrefix returns every Docker volume name starting with
// prefix, failing the calling test (not fatally: this is cleanup-path
// plumbing, a listing failure here shouldn't mask whichever assertion
// is actually driving the subtest) rather than panicking on a transient
// Docker API error.
func listVolumesByPrefix(ctx context.Context, t *testing.T, dockerCli *dockerclient.Client, prefix string) []string {
	t.Helper()
	resp, err := dockerCli.VolumeList(ctx, volume.ListOptions{})
	if err != nil {
		t.Errorf("VolumeList() error = %v", err)
		return nil
	}
	var out []string
	for _, v := range resp.Volumes {
		if v != nil && strings.HasPrefix(v.Name, prefix) {
			out = append(out, v.Name)
		}
	}
	return out
}

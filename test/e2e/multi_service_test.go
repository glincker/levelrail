// TestDeploySpec_Live_MultiServiceFanOut is docs/roadmap.md's multi-service
// apps entry's own named gap: every other live test in this package proves
// a single-service chain end to end, none of them prove the fan-out itself.
// This test calls internal/deploy.Pipeline.DeploySpec directly, the same
// call POST /api/v1/apps/{name}/deploy-spec and "apps deploy-spec" make,
// with two services built from one shared checkout (each scoped to its own
// build.BaseDirectory, the way a real monorepo app.yaml would), then
// reconciles both into real containers and routes both through one real
// ingress pass, so a passing run means: one shared checkout really produces
// two independent images, both land under the same store.App, both become
// real running containers, and both are independently reachable over
// HTTPS, not just independently deployed.
package e2e

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/image"

	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	ingressreconcile "github.com/GLINCKER/levelrail/internal/reconcile/ingress"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	multiServiceWebBody    = "hello from levelrail e2e multi web"
	multiServiceWorkerBody = "hello from levelrail e2e multi worker"
	multiServiceWorkerPort = 9091
)

func TestDeploySpec_Live_MultiServiceFanOut(t *testing.T) {
	env := newLiveBuildEnv(t)

	const (
		appName      = "levelrail-test-e2e-multi"
		webDomain    = "e2e-multi-web.levelrail.internal"
		workerDomain = "e2e-multi-worker.levelrail.internal"
	)
	imageRepoBase := "levelrail/test-e2e-multi"
	sha := "e2emulti1"
	webTag := imageRepoBase + "-web:" + sha
	workerTag := imageRepoBase + "-worker:" + sha

	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = env.DockerCli.ImageRemove(cleanupCtx, webTag, image.RemoveOptions{Force: true})
		_, _ = env.DockerCli.ImageRemove(cleanupCtx, workerTag, image.RemoveOptions{Force: true})
	})

	webServiceName := appName + "-web"
	workerServiceName := appName + "-worker"
	cleanupContainers(context.Background(), t, env.Runtime, webServiceName)
	cleanupContainers(context.Background(), t, env.Runtime, workerServiceName)
	t.Cleanup(func() {
		cleanupContainers(context.Background(), t, env.Runtime, webServiceName)
		cleanupContainers(context.Background(), t, env.Runtime, workerServiceName)
	})

	svcStore := openLiveStore(t)
	pipeline := deploy.New(env.BuildClient, svcStore, deploy.WithAppStore(svcStore))

	deployCtx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	// Step 1: one DeploySpec call builds both services from one shared
	// checkout, each scoped to its own subdirectory, exactly what
	// handleDeploySpec (internal/api/apps_multi.go) does for a real
	// POST .../deploy-spec request.
	byKey := deployMultiService(deployCtx, t, pipeline, appName, imageRepoBase, sha, webDomain, workerDomain)
	if byKey["web"].Image != webTag {
		t.Errorf("web image = %q, want %q", byKey["web"].Image, webTag)
	}
	if byKey["worker"].Image != workerTag {
		t.Errorf("worker image = %q, want %q", byKey["worker"].Image, workerTag)
	}

	assertSharedApp(deployCtx, t, svcStore, appName, webServiceName, workerServiceName)

	// Step 2: a real application.Controller converges each fanned-out
	// service into its own real running container.
	reconcileAndAssertRunning(deployCtx, t, svcStore, env.Runtime, webServiceName, webTag)
	reconcileAndAssertRunning(deployCtx, t, svcStore, env.Runtime, workerServiceName, workerTag)

	// Step 3: one real ingress.Controller reconcile pass routes both
	// domains, proving the fan-out's two services are independently
	// reachable, not just independently running.
	client := newLiveIngressClient(t, svcStore, env.Runtime, "e2e-multi", "Routed2Services")

	assertBodyContains(t, client, "https://"+webDomain+"/", multiServiceWebBody)
	assertBodyContains(t, client, "https://"+workerDomain+"/", multiServiceWorkerBody)
}

// deployMultiService runs pipeline.DeploySpec for a "web"/"worker" pair
// scoped to sibling subdirectories of test/fixtures/multi-service-e2e,
// failing the test on any fan-out error, and returns each outcome keyed
// by its service key for the caller's own assertions.
func deployMultiService(ctx context.Context, t *testing.T, pipeline *deploy.Pipeline, appName, imageRepoBase, sha, webDomain, workerDomain string) map[string]deploy.ServiceOutcome {
	t.Helper()

	outcomes, err := pipeline.DeploySpec(ctx, deploy.MultiRequest{
		AppName: appName,
		Services: map[string]spec.Service{
			"web": {
				Build:   spec.Build{Type: spec.BuildDockerfile, BaseDirectory: "web"},
				Port:    8080,
				Domains: []string{webDomain},
				Health:  &spec.Health{Readiness: &spec.Probe{Path: "/"}},
			},
			"worker": {
				Build:   spec.Build{Type: spec.BuildDockerfile, BaseDirectory: "worker"},
				Port:    multiServiceWorkerPort,
				Domains: []string{workerDomain},
				Health:  &spec.Health{Readiness: &spec.Probe{Path: "/"}},
			},
		},
		SourceDir:     "../fixtures/multi-service-e2e",
		CommitSHA:     sha,
		ImageRepoBase: imageRepoBase,
	}, nil)
	if err != nil {
		t.Fatalf("DeploySpec() error = %v", err)
	}
	if len(outcomes) != 2 {
		t.Fatalf("DeploySpec() returned %d outcomes, want 2: %+v", len(outcomes), outcomes)
	}

	byKey := make(map[string]deploy.ServiceOutcome, len(outcomes))
	for _, o := range outcomes {
		if o.Err != nil {
			t.Fatalf("service %q failed: %v", o.ServiceKey, o.Err)
		}
		byKey[o.ServiceKey] = o
	}
	return byKey
}

// assertSharedApp fails the test unless every named service is linked to
// the same real store.App: the part a hypothetical implementation that
// just ran unrelated single-service Deploy calls under similarly-prefixed
// names would get wrong.
func assertSharedApp(ctx context.Context, t *testing.T, svcStore *store.DB, appName string, serviceNames ...string) {
	t.Helper()

	app, err := svcStore.GetAppByName(ctx, appName)
	if err != nil {
		t.Fatalf("GetAppByName(%q) error = %v", appName, err)
	}
	for _, sn := range serviceNames {
		desired, err := svcStore.GetDesiredService(ctx, sn)
		if err != nil {
			t.Fatalf("GetDesiredService(%q) error = %v", sn, err)
		}
		if desired.AppID != app.ID {
			t.Errorf("service %q: AppID = %q, want %q (every fanned-out service must share one app)", sn, desired.AppID, app.ID)
		}
	}
}

// reconcileAndAssertRunning converges serviceName with a real
// application.Controller and fails the test unless the result is a True
// Ready condition backed by an actually-running container built from tag.
func reconcileAndAssertRunning(ctx context.Context, t *testing.T, svcStore *store.DB, runtime docker.Runtime, serviceName, tag string) {
	t.Helper()

	ctrl := application.New(serviceName, svcStore, runtime)
	result, err := ctrl.Reconcile(ctx)
	if err != nil {
		t.Fatalf("application Controller.Reconcile(%q) error = %v, result = %+v", serviceName, err, result)
	}
	if len(result.Conditions) == 0 || result.Conditions[0].Status != "True" {
		t.Fatalf("application Controller.Reconcile(%q) result = %+v, want a True Ready condition", serviceName, result)
	}

	state, err := runtime.InspectByName(ctx, application.ContainerName(serviceName, tag, ""))
	if err != nil {
		t.Fatalf("InspectByName(%q) error = %v", serviceName, err)
	}
	if state == nil || !state.Running {
		t.Fatalf("InspectByName(%q) = %+v, want a running container", serviceName, state)
	}
}

// newLiveIngressClient reconciles a real ingress.Controller on ephemeral
// ports, asserts it reports wantReason (e.g. "Routed2Services") on a True
// Ready condition, and returns an HTTP client dialed straight at Caddy's
// listener, the same TLS-skipping pattern deploy_test.go's own
// TestDeploy_Live_BuildToHTTPS already establishes.
func newLiveIngressClient(t *testing.T, svcStore *store.DB, runtime docker.Runtime, serverName, wantReason string) *http.Client {
	t.Helper()

	caddyPort := freePort(t)
	caddyAddr := fmt.Sprintf("127.0.0.1:%d", caddyPort)
	adminAddr := fmt.Sprintf("127.0.0.1:%d", freePort(t))

	driver := ingress.New(nil)
	t.Cleanup(func() {
		if err := driver.Stop(context.Background()); err != nil {
			t.Errorf("Driver.Stop() error = %v", err)
		}
	})

	ingressCtrl := ingressreconcile.New(svcStore, runtime, driver,
		ingressreconcile.WithServerName(serverName),
		ingressreconcile.WithListenAddr(caddyAddr),
		ingressreconcile.WithAdminListen(adminAddr),
		ingressreconcile.WithStorageDir(t.TempDir()),
	)

	ingressCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result, err := ingressCtrl.Reconcile(ingressCtx)
	if err != nil {
		t.Fatalf("ingress Controller.Reconcile() error = %v, result = %+v", err, result)
	}
	if len(result.Conditions) == 0 || result.Conditions[0].Status != "True" {
		t.Fatalf("ingress Controller.Reconcile() result = %+v, want a True Ready condition", result)
	}
	if result.Conditions[0].Reason != wantReason {
		t.Fatalf("ingress Controller.Reconcile() reason = %q, want %q", result.Conditions[0].Reason, wantReason)
	}

	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, network, caddyAddr)
			},
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // deliberate, see deploy_test.go's identical comment
		},
	}
	return client
}

func assertBodyContains(t *testing.T, client *http.Client, url, want string) {
	t.Helper()
	body := getBodyWithRetry(t, client, url)
	if !strings.Contains(body, want) {
		t.Fatalf("response for %s = %q, want it to contain %q", url, body, want)
	}
}

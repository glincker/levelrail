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
	dockerclient "github.com/docker/docker/client"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	ingressreconcile "github.com/GLINCKER/levelrail/internal/reconcile/ingress"
	"github.com/GLINCKER/levelrail/internal/spec"
)

const (
	multiServiceWebBody    = "hello from levelrail e2e multi web"
	multiServiceWorkerBody = "hello from levelrail e2e multi worker"
	multiServiceWorkerPort = 9091
)

func TestDeploySpec_Live_MultiServiceFanOut(t *testing.T) {
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

	connectCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	buildClient, err := build.NewClient(connectCtx, dockerCli)
	cancel()
	if err != nil {
		t.Skipf("could not connect to buildkit: %v", err)
	}
	t.Cleanup(func() { _ = buildClient.Close() })

	runtime, err := docker.NewClient()
	if err != nil {
		t.Fatalf("docker.NewClient() error = %v", err)
	}
	t.Cleanup(func() {
		if err := runtime.Close(); err != nil {
			t.Errorf("closing docker.Client: %v", err)
		}
	})

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
		_, _ = dockerCli.ImageRemove(cleanupCtx, webTag, image.RemoveOptions{Force: true})
		_, _ = dockerCli.ImageRemove(cleanupCtx, workerTag, image.RemoveOptions{Force: true})
	})

	webServiceName := appName + "-web"
	workerServiceName := appName + "-worker"
	cleanupContainers(context.Background(), t, runtime, webServiceName)
	cleanupContainers(context.Background(), t, runtime, workerServiceName)
	t.Cleanup(func() {
		cleanupContainers(context.Background(), t, runtime, webServiceName)
		cleanupContainers(context.Background(), t, runtime, workerServiceName)
	})

	svcStore := openLiveStore(t)
	pipeline := deploy.New(buildClient, svcStore, deploy.WithAppStore(svcStore))

	deployCtx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	// Step 1: one DeploySpec call builds both services from one shared
	// checkout, each scoped to its own subdirectory, exactly what
	// handleDeploySpec (internal/api/apps_multi.go) does for a real
	// POST .../deploy-spec request.
	outcomes, err := pipeline.DeploySpec(deployCtx, deploy.MultiRequest{
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
	if byKey["web"].Image != webTag {
		t.Errorf("web image = %q, want %q", byKey["web"].Image, webTag)
	}
	if byKey["worker"].Image != workerTag {
		t.Errorf("worker image = %q, want %q", byKey["worker"].Image, workerTag)
	}

	// Assertion: both services are linked under the same real store.App.
	// This is the part a hypothetical implementation that just ran two
	// unrelated single-service Deploy calls under similarly-prefixed
	// names would get wrong.
	app, err := svcStore.GetAppByName(deployCtx, appName)
	if err != nil {
		t.Fatalf("GetAppByName(%q) error = %v", appName, err)
	}
	webDesired, err := svcStore.GetDesiredService(deployCtx, webServiceName)
	if err != nil {
		t.Fatalf("GetDesiredService(%q) error = %v", webServiceName, err)
	}
	if webDesired.AppID != app.ID {
		t.Errorf("web service AppID = %q, want %q", webDesired.AppID, app.ID)
	}
	workerDesired, err := svcStore.GetDesiredService(deployCtx, workerServiceName)
	if err != nil {
		t.Fatalf("GetDesiredService(%q) error = %v", workerServiceName, err)
	}
	if workerDesired.AppID != app.ID {
		t.Errorf("worker service AppID = %q, want the same app ID as web: %q", workerDesired.AppID, app.ID)
	}

	// Step 2: a real application.Controller converges each fanned-out
	// service into its own real running container.
	for _, sn := range []string{webServiceName, workerServiceName} {
		ctrl := application.New(sn, svcStore, runtime)
		result, err := ctrl.Reconcile(deployCtx)
		if err != nil {
			t.Fatalf("application Controller.Reconcile(%q) error = %v, result = %+v", sn, err, result)
		}
		if len(result.Conditions) == 0 || result.Conditions[0].Status != "True" {
			t.Fatalf("application Controller.Reconcile(%q) result = %+v, want a True Ready condition", sn, result)
		}
	}

	for sn, tag := range map[string]string{webServiceName: webTag, workerServiceName: workerTag} {
		state, err := runtime.InspectByName(deployCtx, application.ContainerName(sn, tag, ""))
		if err != nil {
			t.Fatalf("InspectByName(%q) error = %v", sn, err)
		}
		if state == nil || !state.Running {
			t.Fatalf("InspectByName(%q) = %+v, want a running container", sn, state)
		}
	}

	// Step 3: one real ingress.Controller reconcile pass routes both
	// domains, proving the fan-out's two services are independently
	// reachable, not just independently running.
	caddyPort := freePort(t)
	caddyAddr := fmt.Sprintf("127.0.0.1:%d", caddyPort)
	adminPort := freePort(t)
	adminAddr := fmt.Sprintf("127.0.0.1:%d", adminPort)

	driver := ingress.New(nil)
	t.Cleanup(func() {
		if err := driver.Stop(context.Background()); err != nil {
			t.Errorf("Driver.Stop() error = %v", err)
		}
	})

	ingressCtrl := ingressreconcile.New(svcStore, runtime, driver,
		ingressreconcile.WithServerName("e2e-multi"),
		ingressreconcile.WithListenAddr(caddyAddr),
		ingressreconcile.WithAdminListen(adminAddr),
		ingressreconcile.WithStorageDir(t.TempDir()),
	)

	ingressCtx, ingressCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer ingressCancel()
	ingressResult, err := ingressCtrl.Reconcile(ingressCtx)
	if err != nil {
		t.Fatalf("ingress Controller.Reconcile() error = %v, result = %+v", err, ingressResult)
	}
	if len(ingressResult.Conditions) == 0 || ingressResult.Conditions[0].Status != "True" {
		t.Fatalf("ingress Controller.Reconcile() result = %+v, want a True Ready condition", ingressResult)
	}
	if ingressResult.Conditions[0].Reason != "Routed2Services" {
		t.Fatalf("ingress Controller.Reconcile() reason = %q, want %q (both fanned-out services routed)", ingressResult.Conditions[0].Reason, "Routed2Services")
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

	webBody := getBodyWithRetry(t, client, "https://"+webDomain+"/")
	if !strings.Contains(webBody, multiServiceWebBody) {
		t.Fatalf("response for %s = %q, want it to contain %q", webDomain, webBody, multiServiceWebBody)
	}
	workerBody := getBodyWithRetry(t, client, "https://"+workerDomain+"/")
	if !strings.Contains(workerBody, multiServiceWorkerBody) {
		t.Fatalf("response for %s = %q, want it to contain %q", workerDomain, workerBody, multiServiceWorkerBody)
	}
}

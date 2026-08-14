// Package e2e is the whole-chain proof this repo's other live tests only
// prove piece by piece: internal/build builds a real image from a real
// fixture, internal/store saves it as desired state with a domain, a real
// application.Controller converges it into a real running container, a
// real ingress.Controller converges Caddy's routing for that domain, and
// a real HTTPS request through Caddy comes back with the fixture's own
// response body. Every one of those five steps already has its own live
// test (internal/build, internal/deploy, internal/reconcile/application,
// internal/reconcile/ingress); nothing before this package proved they
// compose into one pipeline the way cmd/levelrail/main.go's dynamicSource
// actually wires them at runtime.
//
// See TASKS.md's Phase 1 section (after 1.10) for exactly what this
// proves and, as importantly, what it deliberately does not: no rollback,
// no webhook/git-push path, a single service only, no multi-node.
package e2e

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/image"
	dockerclient "github.com/docker/docker/client"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	ingressreconcile "github.com/GLINCKER/levelrail/internal/reconcile/ingress"
	"github.com/GLINCKER/levelrail/internal/store"
)

// helloBody is the fixture's distinctive response body (see
// test/fixtures/hello-e2e/Dockerfile). Asserting on this specific string,
// not just a 200 status, is what actually proves traffic reached the
// container this test built and deployed, rather than some other backend
// or a Caddy default response.
const helloBody = "hello from levelrail e2e"

// TestDeploy_Live_BuildToHTTPS builds test/fixtures/hello-e2e, saves it as
// desired state with a domain, reconciles it through a real application
// controller and a real ingress controller, and makes a real HTTPS
// request through Caddy for the domain. Skips cleanly, does not fail, if
// Docker or BuildKit aren't reachable, matching the pattern
// internal/deploy/deploy_live_test.go and
// internal/reconcile/ingress/controller_live_test.go already use.
func TestDeploy_Live_BuildToHTTPS(t *testing.T) {
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
		serviceName = "levelrail-test-e2e-deploy"
		domain      = "e2e-test.levelrail.internal"
	)
	repo := "levelrail/test-e2e-deploy"
	sha := "e2elivetest1"
	tag := repo + ":" + sha

	// Image cleanup: registered before the build even runs, matching
	// deploy_live_test.go's convention, so a failure partway through this
	// test still leaves nothing behind on a re-run.
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = dockerCli.ImageRemove(cleanupCtx, tag, image.RemoveOptions{Force: true})
	})

	// Container cleanup: same "clean before, clean after" shape
	// deploy_live_test.go uses, so repeated runs never collide with a
	// leftover container from a prior failed run.
	cleanupContainers(context.Background(), t, runtime, serviceName)
	t.Cleanup(func() { cleanupContainers(context.Background(), t, runtime, serviceName) })

	svcStore := openLiveStore(t)

	buildCtx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	// Step 1: a real build via internal/build, mirroring
	// internal/deploy.Pipeline.deployDockerfile's own call shape, called
	// directly rather than through internal/deploy so this test owns the
	// desired-state fields (Domains, Health.Readiness) that package's
	// Request/translate.go don't currently expose a path to set (see
	// TASKS.md 1.4's own scope note: domains flow through spec.Service.Domains
	// today, but this test's fixture has no app.yaml, just a raw build).
	res, err := buildClient.Build(buildCtx, build.Request{
		ContextDir: "../fixtures/hello-e2e",
		Tag:        tag,
	}, nil)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if res.Tag != tag {
		t.Fatalf("Build() returned tag %q, want %q", res.Tag, tag)
	}

	// Step 2: save desired state, a real domain and a real readiness
	// probe pointed at the fixture's actual serving path.
	desired := store.DesiredService{
		Name:    serviceName,
		Image:   res.Tag,
		Port:    8080,
		Domains: []string{domain},
		Health: &store.ServiceHealth{
			Readiness: &store.ServiceProbe{Path: "/"},
		},
	}
	if err := svcStore.SaveDesiredService(buildCtx, desired); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}

	// Step 3: a real application.Controller converges a real container.
	appCtrl := application.New(serviceName, svcStore, runtime)
	appResult, err := appCtrl.Reconcile(buildCtx)
	if err != nil {
		t.Fatalf("application Controller.Reconcile() error = %v, result = %+v", err, appResult)
	}
	if len(appResult.Conditions) == 0 || appResult.Conditions[0].Status != "True" {
		t.Fatalf("application Controller.Reconcile() result = %+v, want a True Ready condition", appResult)
	}

	// Independent verification, not trusting the returned Result: inspect
	// the container directly through docker.Runtime, the same rigor
	// deploy_live_test.go applies, before trusting anything downstream of
	// it (ingress, HTTP) actually has something real to route to.
	containers, err := runtime.ListByPrefix(buildCtx, serviceName+"-")
	if err != nil {
		t.Fatalf("ListByPrefix() error = %v", err)
	}
	if len(containers) != 1 {
		t.Fatalf("expected exactly 1 container for %q after reconcile, got %d: %+v", serviceName, len(containers), containers)
	}
	if !containers[0].Running {
		t.Fatalf("container %+v is not running", containers[0])
	}
	if containers[0].Image != tag {
		t.Fatalf("running container's image = %q, want the image Build() produced: %q", containers[0].Image, tag)
	}

	state, err := runtime.InspectByName(buildCtx, application.ContainerName(serviceName, tag, ""))
	if err != nil {
		t.Fatalf("InspectByName() error = %v", err)
	}
	if state == nil || !state.Running {
		t.Fatalf("InspectByName() = %+v, want a running container", state)
	}

	// Step 4: a real ingress.Controller converges Caddy's routing,
	// constructed the same way internal/reconcile/ingress/controller_live_test.go
	// does, against a real ingress.Driver on ephemeral ports so this test
	// never collides with a real control plane's :443/:2019.
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
		ingressreconcile.WithServerName("e2e-deploy"),
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
	if ingressResult.Conditions[0].Reason != "Routed1Services" {
		t.Fatalf("ingress Controller.Reconcile() reason = %q, want %q (exactly this service routed)", ingressResult.Conditions[0].Reason, "Routed1Services")
	}

	// Step 5: a real HTTPS request through Caddy, TLS client pattern
	// reused from controller_live_test.go: Caddy's internal issuer cert
	// is never installed into this process's trust store on purpose.
	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, network, caddyAddr)
			},
			// Caddy's internal issuer root is a locally-generated CA this
			// test process never installs into a trust store (see
			// internal/ingress/driver_test.go and controller_live_test.go's
			// identical comment): only the handshake and routing to this
			// test's own fixture container are being proven here.
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // deliberate, see comment above
		},
	}

	body := getBodyWithRetry(t, client, "https://"+domain+"/")
	if !strings.Contains(body, helloBody) {
		t.Fatalf("response for %s = %q, want it to contain the fixture's distinctive body %q", domain, body, helloBody)
	}
}

func cleanupContainers(ctx context.Context, t *testing.T, rt docker.Runtime, serviceName string) {
	t.Helper()
	found, err := rt.ListByPrefix(ctx, serviceName+"-")
	if err != nil {
		return
	}
	for _, cs := range found {
		_ = rt.Stop(ctx, cs.ID, 3*time.Second)
		_ = rt.Remove(ctx, cs.ID, true)
	}
}

func openLiveStore(t *testing.T) *store.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "levelrail.db")
	db, err := store.Open(context.Background(), path)
	if err != nil {
		t.Fatalf("store.Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("closing store: %v", err)
		}
	})
	return db
}

// freePort asks the OS for an unused TCP port on 127.0.0.1, the same
// approach internal/reconcile/ingress/controller_live_test.go uses for the
// identical reason: there's an inherent (tiny) race between releasing the
// probe port here and Caddy binding it a few lines later, but this is the
// standard way to get an ephemeral port for a test.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("freePort: listen: %v", err)
	}
	defer func() {
		if err := l.Close(); err != nil {
			t.Logf("freePort: closing probe listener: %v", err)
		}
	}()
	return l.Addr().(*net.TCPAddr).Port
}

// getBodyWithRetry issues GET requests against url until one succeeds or
// the deadline passes. Caddy's listeners come up asynchronously inside
// caddy.Load, and the internal-TLS case also has to finish a synchronous
// certificate issuance during Provision, so a short retry loop here is
// standalone-test plumbing, matching
// internal/reconcile/ingress/controller_live_test.go's identical helper.
func getBodyWithRetry(t *testing.T, client *http.Client, url string) string {
	t.Helper()

	deadline := time.Now().Add(8 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		body, status, err := doGet(client, url)
		if err != nil {
			lastErr = err
			time.Sleep(100 * time.Millisecond)
			continue
		}
		if status != http.StatusOK {
			lastErr = fmt.Errorf("status %d", status)
			time.Sleep(100 * time.Millisecond)
			continue
		}
		return body
	}

	t.Fatalf("GET %s never succeeded within the retry window, last error: %v", url, lastErr)
	return ""
}

func doGet(client *http.Client, url string) (body string, status int, err error) {
	resp, err := client.Get(url) //nolint:noctx // test helper, no context to plumb through
	if err != nil {
		return "", 0, err
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("closing response body: %w", closeErr)
		}
	}()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", resp.StatusCode, fmt.Errorf("reading response body: %w", err)
	}
	return string(b), resp.StatusCode, nil
}

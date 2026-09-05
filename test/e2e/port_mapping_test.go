// This file proves one specific thing test/e2e/deploy_test.go's chain
// does not: every fixture that test uses, and every fixture in this
// directory before this file existed, listens on 8080. A test suite where
// every fixture agrees on the same port cannot tell the difference
// between "the port field is honored end-to-end" and "8080 is hardcoded
// somewhere in the pipeline and the field is silently ignored." This file
// deploys a fixture on 9090 instead, through the identical five-step
// chain (build, save desired state, application reconcile, ingress
// reconcile, real HTTPS request) deploy_test.go already proves works, and
// adds two assertions deploy_test.go does not need: that the container's
// actual published port mapping is 9090, not 8080 or anything else, and
// that Caddy's routed response is the alt-port fixture's own distinctive
// body, not some other backend's.
//
// It shares deploy_test.go's helpers (freePort, getBodyWithRetry,
// openLiveStore, cleanupContainers) since both files are package e2e, and
// mirrors its Docker/BuildKit reachability skip pattern exactly. See
// TASKS.md's Phase 1 section for what this suite as a whole proves and
// deliberately does not: no rollback, no webhook/git-push path, a single
// service only, no multi-node.
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

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	ingressreconcile "github.com/GLINCKER/levelrail/internal/reconcile/ingress"
	"github.com/GLINCKER/levelrail/internal/store"
)

// altPortBody is test/fixtures/hello-e2e-altport/Dockerfile's distinctive
// response body. It differs from deploy_test.go's helloBody on purpose:
// asserting on this exact string is what proves traffic reached this
// fixture's container specifically, not hello-e2e's, and reached it on
// port 9090, not 8080.
const altPortBody = "hello from levelrail e2e alt port"

// altPort is the non-default port this test's fixture listens on. Every
// other fixture under test/fixtures uses 8080; this constant exists
// precisely so nothing here can accidentally match that default.
const altPort = 9090

// TestPortMapping_Live_NonDefaultPortRouting builds
// test/fixtures/hello-e2e-altport (listening on 9090, not the 8080 every
// other fixture uses), saves it as desired state with Port: 9090,
// reconciles it through a real application.Controller and a real
// ingress.Controller, then proves two things deploy_test.go's identical
// chain does not: the container's actual published port mapping is 9090
// (via runtime.InspectByName, not trusting the reconcile Result), and a
// real HTTPS request through Caddy returns this fixture's own distinctive
// body, meaning ingress routed to port 9090 specifically. Skips cleanly,
// does not fail, if Docker or BuildKit aren't reachable, matching
// deploy_test.go's own skip pattern exactly.
func TestPortMapping_Live_NonDefaultPortRouting(t *testing.T) {
	env := newLiveBuildEnv(t)
	dockerCli, buildClient, runtime := env.DockerCli, env.BuildClient, env.Runtime

	// Service name and domain are deliberately distinct from
	// deploy_test.go's (and any other e2e test's), since other agents are
	// concurrently adding sibling e2e tests against the same shared local
	// Docker daemon and name collisions would break both tests.
	const (
		serviceName = "levelrail-test-e2e-altport"
		domain      = "e2e-altport-test.levelrail.internal"
	)
	repo := "levelrail/test-e2e-altport"
	sha := "e2elivetestaltport1"
	tag := repo + ":" + sha

	// Image cleanup: registered before the build even runs, same
	// convention deploy_test.go and deploy_live_test.go use, so a
	// failure partway through this test still leaves nothing behind on a
	// re-run.
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = dockerCli.ImageRemove(cleanupCtx, tag, image.RemoveOptions{Force: true})
	})

	// Container cleanup: same "clean before, clean after" shape
	// deploy_test.go uses, so repeated runs never collide with a
	// leftover container from a prior failed run.
	cleanupContainers(context.Background(), t, runtime, serviceName)
	t.Cleanup(func() { cleanupContainers(context.Background(), t, runtime, serviceName) })

	svcStore := openLiveStore(t)

	buildCtx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	// Step 1: a real build via internal/build, of the alt-port fixture.
	res, err := buildClient.Build(buildCtx, build.Request{
		ContextDir: "../fixtures/hello-e2e-altport",
		Tag:        tag,
	}, nil)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if res.Tag != tag {
		t.Fatalf("Build() returned tag %q, want %q", res.Tag, tag)
	}

	// Step 2: save desired state with Port: 9090, the whole point of this
	// test, and a readiness probe pointed at the fixture's actual serving
	// path.
	desired := store.DesiredService{
		Name:    serviceName,
		Image:   res.Tag,
		Port:    altPort,
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
	// the container directly through docker.Runtime before trusting
	// anything downstream (ingress, HTTP) actually has something real to
	// route to. Same rigor deploy_test.go applies.
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

	// Assertion (a): the container's actual published port mapping
	// matches 9090, the port this test's DesiredService declared, not
	// 8080 (every other fixture's port) or any other value. This is the
	// assertion deploy_test.go has no reason to make, since its fixture's
	// port and the pipeline's hypothetical hardcoded default would be
	// indistinguishable.
	if len(state.Ports) == 0 {
		t.Fatalf("InspectByName() state.Ports is empty, want a binding for container port %d", altPort)
	}
	foundContainerPort := false
	for _, pb := range state.Ports {
		if pb.ContainerPort == altPort {
			foundContainerPort = true
			break
		}
	}
	if !foundContainerPort {
		t.Fatalf("InspectByName() state.Ports = %+v, want a binding with ContainerPort = %d", state.Ports, altPort)
	}

	// Step 4: a real ingress.Controller converges Caddy's routing,
	// constructed the same way deploy_test.go and
	// internal/reconcile/ingress/controller_live_test.go do, against a
	// real ingress.Driver on its own ephemeral ports (distinct from any
	// other concurrently running e2e test's) so this test never collides
	// with a real control plane's :443/:2019 or another test's Caddy
	// instance.
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
		ingressreconcile.WithServerName("e2e-altport"),
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
	// reused from deploy_test.go and controller_live_test.go: Caddy's
	// internal issuer cert is never installed into this process's trust
	// store on purpose.
	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, network, caddyAddr)
			},
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // deliberate, see comment above
		},
	}

	// Assertion (b): the response body is this fixture's own distinctive
	// string, proving ingress routed to the container listening on 9090
	// specifically, not some other backend or a Caddy default response.
	body := getBodyWithRetry(t, client, "https://"+domain+"/")
	if !strings.Contains(body, altPortBody) {
		t.Fatalf("response for %s = %q, want it to contain the alt-port fixture's distinctive body %q", domain, body, altPortBody)
	}
}

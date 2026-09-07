// TestMaintenanceMode_Live_TogglesResponseWithoutStoppingContainer is
// this feature's own whole-chain proof, the same shape
// TestDeploy_Live_BuildToHTTPS establishes: a real build, a real
// running container, a real ingress reconcile pass, and a real HTTPS
// request through Caddy. What it specifically proves that no other live
// test does: enabling maintenance mode on a domain changes what an HTTPS
// visitor actually receives (503 plus the fixed maintenance body,
// instead of the real app's response) without the container itself ever
// stopping, and disabling it again restores real traffic, both changes
// taking effect on the very next ingress reconcile pass with no
// redeploy.
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

func TestMaintenanceMode_Live_TogglesResponseWithoutStoppingContainer(t *testing.T) {
	env := newLiveBuildEnv(t)

	const (
		serviceName = "levelrail-test-e2e-maintenance"
		domain      = "e2e-maintenance.levelrail.internal"
	)
	repo := "levelrail/test-e2e-maintenance"
	tag := repo + ":e2emaintenance1"

	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = env.DockerCli.ImageRemove(cleanupCtx, tag, image.RemoveOptions{Force: true})
	})
	cleanupContainers(context.Background(), t, env.Runtime, serviceName)
	t.Cleanup(func() { cleanupContainers(context.Background(), t, env.Runtime, serviceName) })

	buildCtx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	res, err := env.BuildClient.Build(buildCtx, build.Request{ContextDir: "../fixtures/hello-e2e", Tag: tag}, nil)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	svcStore := openLiveStore(t)
	desired := store.DesiredService{
		Name:    serviceName,
		Image:   res.Tag,
		Port:    8080,
		Domains: []string{domain},
		Health:  &store.ServiceHealth{Readiness: &store.ServiceProbe{Path: "/"}},
	}
	if err := svcStore.SaveDesiredService(buildCtx, desired); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}

	// Step 1: a real application.Controller converges a real running
	// container, exactly like TestDeploy_Live_BuildToHTTPS.
	appCtrl := application.New(serviceName, svcStore, env.Runtime)
	appResult, err := appCtrl.Reconcile(buildCtx)
	if err != nil {
		t.Fatalf("application Controller.Reconcile() error = %v, result = %+v", err, appResult)
	}
	if len(appResult.Conditions) == 0 || appResult.Conditions[0].Status != "True" {
		t.Fatalf("application Controller.Reconcile() result = %+v, want a True Ready condition", appResult)
	}

	caddyPort := freePort(t)
	caddyAddr := fmt.Sprintf("127.0.0.1:%d", caddyPort)
	adminAddr := fmt.Sprintf("127.0.0.1:%d", freePort(t))

	driver := ingress.New(nil)
	t.Cleanup(func() {
		if err := driver.Stop(context.Background()); err != nil {
			t.Errorf("Driver.Stop() error = %v", err)
		}
	})
	ingressCtrl := ingressreconcile.New(svcStore, env.Runtime, driver,
		ingressreconcile.WithServerName("e2e-maintenance"),
		ingressreconcile.WithListenAddr(caddyAddr),
		ingressreconcile.WithAdminListen(adminAddr),
		ingressreconcile.WithStorageDir(t.TempDir()),
	)
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

	// Step 2: before maintenance mode, the domain serves the real app.
	ingressCtx, ingressCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer ingressCancel()
	if _, err := ingressCtrl.Reconcile(ingressCtx); err != nil {
		t.Fatalf("ingress Controller.Reconcile() (before maintenance) error = %v", err)
	}
	body := getBodyWithRetry(t, client, "https://"+domain+"/")
	if !strings.Contains(body, helloBody) {
		t.Fatalf("response before maintenance mode = %q, want it to contain the real app's body %q", body, helloBody)
	}

	// Step 3: enabling maintenance mode, then reconciling ingress again
	// (no redeploy, no container restart), switches the domain to the
	// fixed maintenance response.
	if err := svcStore.SetDomainMaintenance(context.Background(), domain); err != nil {
		t.Fatalf("SetDomainMaintenance() error = %v", err)
	}
	if _, err := ingressCtrl.Reconcile(ingressCtx); err != nil {
		t.Fatalf("ingress Controller.Reconcile() (during maintenance) error = %v", err)
	}

	status, body := getStatusAndBodyWithRetry(t, client, "https://"+domain+"/", http.StatusServiceUnavailable)
	if status != http.StatusServiceUnavailable {
		t.Fatalf("status during maintenance = %d, want %d", status, http.StatusServiceUnavailable)
	}
	if strings.Contains(body, helloBody) {
		t.Fatalf("response during maintenance mode = %q, must not contain the real app's body", body)
	}

	// The container itself must still be running: maintenance mode
	// never stops it, only reroutes traffic at the ingress layer.
	state, err := env.Runtime.InspectByName(context.Background(), application.ContainerName(serviceName, res.Tag, ""))
	if err != nil {
		t.Fatalf("InspectByName() error = %v", err)
	}
	if state == nil || !state.Running {
		t.Fatalf("InspectByName() = %+v, want the container still running during maintenance mode", state)
	}

	// Step 4: disabling maintenance mode restores real traffic on the
	// next reconcile pass.
	if err := svcStore.DeleteDomainMaintenance(context.Background(), domain); err != nil {
		t.Fatalf("DeleteDomainMaintenance() error = %v", err)
	}
	if _, err := ingressCtrl.Reconcile(ingressCtx); err != nil {
		t.Fatalf("ingress Controller.Reconcile() (after maintenance) error = %v", err)
	}
	body = getBodyWithRetry(t, client, "https://"+domain+"/")
	if !strings.Contains(body, helloBody) {
		t.Fatalf("response after clearing maintenance mode = %q, want it to contain the real app's body %q again", body, helloBody)
	}
}

// getStatusAndBodyWithRetry mirrors getBodyWithRetry's own retry loop,
// except it accepts wantStatus as success instead of always requiring
// 200: a maintenance-mode response is a deliberate 503, and Caddy's own
// route swap on reconcile is asynchronous the same way certificate
// issuance is, so this needs the identical retry-until-settled shape.
func getStatusAndBodyWithRetry(t *testing.T, client *http.Client, url string, wantStatus int) (status int, body string) {
	t.Helper()

	deadline := time.Now().Add(8 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		b, s, err := doGet(client, url)
		if err != nil {
			lastErr = err
			time.Sleep(100 * time.Millisecond)
			continue
		}
		if s != wantStatus {
			lastErr = fmt.Errorf("status %d, want %d", s, wantStatus)
			time.Sleep(100 * time.Millisecond)
			continue
		}
		return s, b
	}

	t.Fatalf("GET %s never returned status %d within the retry window, last error: %v", url, wantStatus, lastErr)
	return 0, ""
}

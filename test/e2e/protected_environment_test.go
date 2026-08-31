// TestProtectedEnvironment_Live_DeployBlockedThenAllowed is this
// package's live proof for the confirm: true gate internal/api's
// TestHandleTriggerDeploy_ProtectedEnvironment (deploys_test.go) already
// covers at the handler level: that test asserts a 409 and an unchanged
// store row using httptest.NewRecorder against the handler directly.
// What it cannot show is the thing that actually matters operationally:
// whether the block genuinely stops a container from changing, or only
// stops a JSON field from changing while some other path still mutates
// the running deployment. This test drives the same gate over a real
// TCP connection, a real cookie-authenticated session, and a real
// application.Controller reconciling real Docker containers, so an
// unconfirmed request is proven to leave the exact running container
// untouched, and a confirmed one is proven to actually cut over.
package e2e

import (
	"context"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/image"
	dockerclient "github.com/docker/docker/client"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/brand"
	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	e2eProtectedEnvAdminUsername = "e2e-protected-env-admin"
	e2eProtectedEnvAdminPassword = "e2e-protected-env-correct-horse" //nolint:gosec // test fixture credential, not a real secret
)

func TestProtectedEnvironment_Live_DeployBlockedThenAllowed(t *testing.T) {
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

	const serviceName = "levelrail-test-e2e-protected-env"
	repo := "levelrail/test-e2e-protected-env"
	tagA := repo + ":e2eprotectedenva"
	tagB := repo + ":e2eprotectedenvb"

	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = dockerCli.ImageRemove(cleanupCtx, tagA, image.RemoveOptions{Force: true})
		_, _ = dockerCli.ImageRemove(cleanupCtx, tagB, image.RemoveOptions{Force: true})
	})

	cleanupContainers(context.Background(), t, runtime, serviceName)
	t.Cleanup(func() { cleanupContainers(context.Background(), t, runtime, serviceName) })

	svcStore := openLiveStore(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Two real builds of the identical fixture content under distinct
	// tags, the same "container identity is keyed by image tag, not
	// content" reasoning rollback_test.go's own package comment
	// documents: what's under test is whether the confirmation gate lets
	// an image change reach Docker, not what the served body is.
	resA, err := buildClient.Build(ctx, build.Request{ContextDir: "../fixtures/hello-e2e", Tag: tagA}, nil)
	if err != nil {
		t.Fatalf("Build(A) error = %v", err)
	}
	resB, err := buildClient.Build(ctx, build.Request{ContextDir: "../fixtures/hello-e2e", Tag: tagB}, nil)
	if err != nil {
		t.Fatalf("Build(B) error = %v", err)
	}

	// Step 1: a real project, a real protected environment, and a real
	// app already tagged into it and already running image A, matching
	// the state an operator's real production environment would be in
	// before anyone tries to deploy again.
	if err := svcStore.SaveProject(ctx, store.Project{ID: "proj_e2e_protected", Name: "e2e-protected", CreatedAt: time.Now().UTC().Format(time.RFC3339)}); err != nil {
		t.Fatalf("SaveProject() error = %v", err)
	}
	if err := svcStore.SaveEnvironment(ctx, store.Environment{ID: "env_e2e_protected_prod", ProjectID: "proj_e2e_protected", Name: "production", Protected: true, CreatedAt: time.Now().UTC().Format(time.RFC3339)}); err != nil {
		t.Fatalf("SaveEnvironment() error = %v", err)
	}
	if err := svcStore.SaveDesiredService(ctx, store.DesiredService{
		Name:  serviceName,
		Image: resA.Tag,
		Port:  8080,
		Health: &store.ServiceHealth{
			Readiness: &store.ServiceProbe{Path: "/"},
		},
	}); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}
	if err := svcStore.SetServiceEnvironment(ctx, serviceName, "env_e2e_protected_prod"); err != nil {
		t.Fatalf("SetServiceEnvironment() error = %v", err)
	}

	appCtrl := application.New(serviceName, svcStore, runtime)
	nameA := application.ContainerName(serviceName, resA.Tag, "")
	nameB := application.ContainerName(serviceName, resB.Tag, "")

	if result, err := appCtrl.Reconcile(ctx); err != nil {
		t.Fatalf("initial Reconcile() error = %v, result = %+v", err, result)
	}
	assertRunningWithImage(ctx, t, runtime, serviceName, nameA, resA.Tag)

	// Step 2: a real *api.Router, a real HTTP server, and a real
	// cookie-authenticated session, the same construction
	// test/e2e/metrics_test.go and auth_lifecycle_test.go already use.
	logger := discardTestLogger()
	b := &brand.Brand{Name: "E2E Test Platform", BinaryName: "e2e-test-platform"}
	router := api.NewRouter(logger, b, svcStore)
	ts := newE2ETestServer(t, router)

	if err := api.BootstrapAdmin(ctx, svcStore, e2eProtectedEnvAdminUsername, e2eProtectedEnvAdminPassword); err != nil {
		t.Fatalf("BootstrapAdmin() error = %v", err)
	}
	client := loginE2EClient(t, ts.URL, e2eProtectedEnvAdminUsername, e2eProtectedEnvAdminPassword)

	// Step 3: the unconfirmed deploy. The real HTTP response must be
	// 409, and, more importantly, the running container must remain
	// exactly image A after a second real reconcile: this is the actual
	// safety property, not the status code alone.
	status, body := postJSON(t, client, ts.URL+"/api/v1/apps/"+serviceName+"/deploys", `{"image":"`+resB.Tag+`"}`)
	if status != http.StatusConflict {
		t.Fatalf("unconfirmed deploy: status = %d, want %d, body = %s", status, http.StatusConflict, body)
	}
	if !strings.Contains(body, "protected") {
		t.Errorf("unconfirmed deploy: body = %s, want it to mention the environment is protected", body)
	}

	svc, err := svcStore.GetDesiredService(ctx, serviceName)
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}
	if svc.Image != resA.Tag {
		t.Fatalf("desired state Image = %q after a rejected unconfirmed deploy, want unchanged %q", svc.Image, resA.Tag)
	}

	if result, err := appCtrl.Reconcile(ctx); err != nil {
		t.Fatalf("post-block Reconcile() error = %v, result = %+v", err, result)
	}
	assertRunningWithImage(ctx, t, runtime, serviceName, nameA, resA.Tag)
	assertContainerAbsent(ctx, t, runtime, nameB, "B")

	// Step 4: the confirmed deploy. A real 202, a real store update, and
	// a real reconcile that genuinely swaps the running container to
	// image B.
	status, body = postJSON(t, client, ts.URL+"/api/v1/apps/"+serviceName+"/deploys", `{"image":"`+resB.Tag+`","confirm":true}`)
	if status != http.StatusAccepted {
		t.Fatalf("confirmed deploy: status = %d, want %d, body = %s", status, http.StatusAccepted, body)
	}

	svc, err = svcStore.GetDesiredService(ctx, serviceName)
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}
	if svc.Image != resB.Tag {
		t.Fatalf("desired state Image = %q after a confirmed deploy, want %q", svc.Image, resB.Tag)
	}

	if result, err := appCtrl.Reconcile(ctx); err != nil {
		t.Fatalf("post-confirm Reconcile() error = %v, result = %+v", err, result)
	}
	assertRunningWithImage(ctx, t, runtime, serviceName, nameB, resB.Tag)
	assertContainerAbsent(ctx, t, runtime, nameA, "A")
}

// assertRunningWithImage verifies, directly against docker.Runtime, that
// exactly one container exists for serviceName, is running, and matches
// both wantName and wantImage: the same independent-of-Reconcile's-own-
// return-value rigor this package's other live tests apply throughout.
func assertRunningWithImage(ctx context.Context, t *testing.T, runtime docker.Runtime, serviceName, wantName, wantImage string) {
	t.Helper()
	containers, err := runtime.ListByPrefix(ctx, serviceName+"-")
	if err != nil {
		t.Fatalf("ListByPrefix() error = %v", err)
	}
	if len(containers) != 1 {
		t.Fatalf("expected exactly 1 container for %q, got %d: %+v", serviceName, len(containers), containers)
	}
	if !containers[0].Running {
		t.Fatalf("container %+v is not running", containers[0])
	}
	if containers[0].Name != wantName {
		t.Fatalf("running container's name = %q, want %q", containers[0].Name, wantName)
	}
	if containers[0].Image != wantImage {
		t.Fatalf("running container's image = %q, want %q", containers[0].Image, wantImage)
	}
}

// loginE2EClient logs in against a real POST /api/v1/auth/login route
// through a fresh cookie jar, the same pattern
// test/e2e/metrics_test.go's TestMetrics_Live_ContainerToHTTP already
// uses, factored out here since two more live tests in this package
// (compose healthcheck and master key rotation) need the identical
// login flow.
func loginE2EClient(t *testing.T, baseURL, username, password string) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar.New() error = %v", err)
	}
	client := &http.Client{Jar: jar, Timeout: e2eHTTPTimeout}

	status, body := postJSON(t, client, baseURL+"/api/v1/auth/login", `{"username":"`+username+`","password":"`+password+`"}`)
	if status != http.StatusOK {
		t.Fatalf("login: status = %d, want %d, body = %s", status, http.StatusOK, body)
	}
	return client
}

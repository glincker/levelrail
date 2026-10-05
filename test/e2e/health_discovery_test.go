// TestHealthDiscovery_Live proves POST /api/v1/apps/{name}/health/discover
// (internal/api/apps_health_discover.go) against real containers, not a
// fake docker.Runtime: a stock nginx:alpine answers "/" with 200 and
// nothing else, so discovery must find exactly that path and nothing
// else; a container with no HTTP server at all must come back empty,
// never a fabricated guess.
package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/brand"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	e2eHealthDiscoveryAdminUsername = "admin-health-discovery"
	e2eHealthDiscoveryAdminPassword = "e2e-health-discovery-pw-1!" //nolint:gosec // test fixture credential, not a real one
)

// healthDiscoveryAttemptE2E/healthDiscoveryResponseE2E decode
// internal/api's unexported wire shape by field name rather than
// importing it, the same arm's-length approach this package's other
// live tests take toward internal/api response bodies.
type healthDiscoveryAttemptE2E struct {
	Path      string `json:"path"`
	Success   bool   `json:"success"`
	Error     string `json:"error,omitempty"`
	LatencyMs int64  `json:"latency_ms"`
}

type healthDiscoveryResponseE2E struct {
	Name     string                      `json:"name"`
	Attempts []healthDiscoveryAttemptE2E `json:"attempts"`
	Found    string                      `json:"found,omitempty"`
}

func TestHealthDiscovery_Live_FindsTheRealWorkingPath(t *testing.T) {
	env := newLiveBuildEnv(t)
	runtime := env.Runtime

	const name = "levelrail-test-e2e-health-discovery-found"
	const image = "nginx:alpine"
	// The discover handler resolves the container the same way exec.go
	// does: by the reconciler's own computed name, not the app's logical
	// name (application.ContainerName's own doc comment), so the fixture
	// container has to be created under that exact name too.
	containerName := application.ContainerName(name, image, "")
	removeContainerAndVolumes(env.DockerCli, containerName)
	t.Cleanup(func() { removeContainerAndVolumes(env.DockerCli, containerName) })

	ctx := context.Background()
	id, err := runtime.Create(ctx, docker.ContainerSpec{
		Name: containerName, Image: image,
		Ports: []docker.PortBinding{{ContainerPort: 80, HostIP: "127.0.0.1"}},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := runtime.Start(ctx, id); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	baseURL, client := newHealthDiscoveryLiveServerWithApp(t, runtime, name, image, 80)

	// nginx needs a moment to start accepting connections; retry the
	// whole discover call rather than sleeping a fixed guess, since the
	// endpoint itself is cheap, idempotent, and side-effect free.
	var resp healthDiscoveryResponseE2E
	deadline := time.Now().Add(20 * time.Second)
	for {
		status, body := postJSON(t, client, baseURL+"/api/v1/apps/"+name+"/health/discover", "")
		if status != http.StatusOK {
			t.Fatalf("discover status = %d, want 200, body = %s", status, body)
		}
		if err := json.Unmarshal([]byte(body), &resp); err != nil {
			t.Fatalf("decode discover response: %v, body = %s", err, body)
		}
		if resp.Found != "" || time.Now().After(deadline) {
			break
		}
		time.Sleep(300 * time.Millisecond)
	}

	if resp.Found != "/" {
		t.Fatalf("Found = %q, want / (nginx's own default welcome page is the only real match): attempts = %+v", resp.Found, resp.Attempts)
	}
	for _, a := range resp.Attempts {
		if a.Path != "/" && a.Success {
			t.Errorf("path %q reported success against stock nginx, want only / to answer", a.Path)
		}
	}
}

func TestHealthDiscovery_Live_NothingFoundForAPathlessService(t *testing.T) {
	env := newLiveBuildEnv(t)
	runtime := env.Runtime

	const name = "levelrail-test-e2e-health-discovery-nomatch"
	const image = "busybox"
	containerName := application.ContainerName(name, image, "")
	removeContainerAndVolumes(env.DockerCli, containerName)
	t.Cleanup(func() { removeContainerAndVolumes(env.DockerCli, containerName) })

	ctx := context.Background()
	// busybox with a long-running shell and a declared port nothing
	// inside the container ever listens on: every candidate path must
	// fail to connect, proving discovery reports "nothing found" rather
	// than inventing a match.
	id, err := runtime.Create(ctx, docker.ContainerSpec{
		Name:    containerName,
		Image:   image,
		Ports:   []docker.PortBinding{{ContainerPort: 8080, HostIP: "127.0.0.1"}},
		Command: []string{"sleep", "300"},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := runtime.Start(ctx, id); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	baseURL, client := newHealthDiscoveryLiveServerWithApp(t, runtime, name, image, 8080)

	status, body := postJSON(t, client, baseURL+"/api/v1/apps/"+name+"/health/discover", "")
	if status != http.StatusOK {
		t.Fatalf("discover status = %d, want 200, body = %s", status, body)
	}
	var resp healthDiscoveryResponseE2E
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("decode discover response: %v, body = %s", err, body)
	}
	if resp.Found != "" {
		t.Fatalf("Found = %q, want empty: no server is listening on this container's port", resp.Found)
	}
	if len(resp.Attempts) == 0 {
		t.Fatal("Attempts is empty, want every candidate path attempted and reported")
	}
	for _, a := range resp.Attempts {
		if a.Success {
			t.Errorf("path %q reported success against a container with no HTTP server", a.Path)
		}
		if a.Error == "" {
			t.Errorf("path %q has no Error text despite failing", a.Path)
		}
	}
}

// newHealthDiscoveryLiveServerWithApp wires a real *api.Router with a
// real docker.Runtime behind WithExecRuntime, seeds the store row for
// name/image/port, and logs a real admin session in over a real TCP
// connection. Shared by both subtests, which differ only in which
// container they target.
func newHealthDiscoveryLiveServerWithApp(t *testing.T, runtime docker.Runtime, name, image string, port int) (baseURL string, client *http.Client) {
	t.Helper()
	svcStore := openLiveStore(t)
	if err := svcStore.SaveDesiredService(context.Background(), store.DesiredService{Name: name, Image: image, Port: port}); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}
	b := &brand.Brand{Name: "E2E Test Platform", BinaryName: "e2e-test-platform"}
	router := api.NewRouter(discardTestLogger(), b, svcStore,
		api.WithExecRuntime(func(string) (docker.Runtime, error) { return runtime, nil }))
	ts := newE2ETestServer(t, router)

	if err := api.BootstrapAdmin(context.Background(), svcStore, e2eHealthDiscoveryAdminUsername+"-"+name, e2eHealthDiscoveryAdminPassword); err != nil {
		t.Fatalf("BootstrapAdmin() error = %v", err)
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar.New() error = %v", err)
	}
	client = &http.Client{Jar: jar, Timeout: 25 * time.Second}
	loginBody := `{"username":"` + e2eHealthDiscoveryAdminUsername + "-" + name + `","password":"` + e2eHealthDiscoveryAdminPassword + `"}`
	if status, body := postJSON(t, client, ts.URL+"/api/v1/auth/login", loginBody); status != http.StatusOK {
		t.Fatalf("login status = %d, want 200, body = %s", status, body)
	}
	return ts.URL, client
}

// Package e2e: this file is the live proof behind docs/resilience.md.
// cmd/levelrail embeds Caddy in-process (internal/ingress/driver.go's own
// doc comment: caddy.Load drives a package-global Caddy instance, not a
// sibling process, per the locked architecture decision on embedded
// ingress). A control plane process death necessarily takes its ingress
// listener down with it. What this file proves, precisely: a running app
// container is not a child of the control plane process and keeps
// serving traffic directly through its published host port across a
// SIGKILL; the embedded Caddy listener does not survive the same kill,
// and stops accepting connections the instant the process dies; and a
// restarted control plane's level-triggered reconciler picks the
// already-running container back up without recreating or restarting
// it, and Caddy resumes routing once it reconciles again.
package reconcile

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/test/e2e/testenv"
)

const breakGlassToken = "dev-root-token" //nolint:gosec // dev-mode fixture token from dev-fixtures.yml

// breakGlassProcess is one real OS process running a binary this package
// built, with its exit tracked so a test can SIGKILL it and know exactly
// when it's gone: the same shape startControlPlane already uses in
// agent_loop_harness_test.go, but exposed here for a test that needs to
// kill and restart the process mid-test rather than only at cleanup.
type breakGlassProcess struct {
	cmd     *exec.Cmd
	exited  chan struct{}
	logPath string
}

func startBreakGlassProcess(t *testing.T, bin string, env []string, logPath string) *breakGlassProcess {
	t.Helper()
	logFile, err := os.Create(logPath) //nolint:gosec // path under t.TempDir
	if err != nil {
		t.Fatalf("create log %s: %v", logPath, err)
	}
	cmd := exec.Command(bin) //nolint:gosec,noctx // binary built by this test; lifetime is managed by the caller
	cmd.Env = env
	cmd.Stdout, cmd.Stderr = logFile, logFile
	if err := cmd.Start(); err != nil {
		t.Fatalf("start %s: %v", bin, err)
	}
	exited := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(exited)
	}()
	p := &breakGlassProcess{cmd: cmd, exited: exited, logPath: logPath}
	t.Cleanup(func() {
		select {
		case <-exited:
		default:
			_ = cmd.Process.Kill()
			<-exited
		}
		_ = logFile.Close()
		if t.Failed() {
			dumpTail(t, "process log "+logPath, logPath, 80)
		}
	})
	return p
}

// kill sends SIGKILL, the hard-crash case this file's tests are about,
// not a graceful shutdown: proving what survives when the process gets
// no chance to run any defer or signal handler at all.
func (p *breakGlassProcess) kill(t *testing.T) {
	t.Helper()
	if err := p.cmd.Process.Kill(); err != nil {
		t.Fatalf("kill process: %v", err)
	}
	select {
	case <-p.exited:
	case <-time.After(10 * time.Second):
		t.Fatal("process did not exit after SIGKILL")
	}
}

// envWithOverrides starts from base and replaces (not appends) every key
// present in overrides: exec.Cmd hands its Env slice to the OS as-is, and
// which of two entries for the same key wins if both are present is not
// something this file wants to depend on.
func envWithOverrides(base []string, overrides ...string) []string {
	keys := make(map[string]bool, len(overrides))
	for _, kv := range overrides {
		if i := strings.IndexByte(kv, '='); i >= 0 {
			keys[kv[:i]] = true
		}
	}
	out := make([]string, 0, len(base)+len(overrides))
	for _, kv := range base {
		if i := strings.IndexByte(kv, '='); i >= 0 && keys[kv[:i]] {
			continue
		}
		out = append(out, kv)
	}
	return append(out, overrides...)
}

func waitAPIUp(t *testing.T, client *apiclient.Client, exited <-chan struct{}) {
	t.Helper()
	pollUntil(t, 60*time.Second, "control plane to answer /api/v1/apps", func() (bool, string) {
		select {
		case <-exited:
			t.Fatalf("control plane exited during startup")
		default:
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_, err := client.ListApps(ctx)
		if err != nil {
			return false, err.Error()
		}
		return true, ""
	})
}

func waitAppReady(t *testing.T, client *apiclient.Client, name string) {
	t.Helper()
	pollUntil(t, 3*time.Minute, name+" to be Ready", func() (bool, string) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		conds, err := client.GetDeployStatus(ctx, name)
		if err != nil {
			return false, err.Error()
		}
		for _, c := range conds {
			if c.Type == "Ready" && c.Status == "True" {
				return true, ""
			}
		}
		return false, fmt.Sprintf("conditions %+v", conds)
	})
}

func waitAppNetwork(t *testing.T, client *apiclient.Client, name string) apiclient.NetworkResource {
	t.Helper()
	var netw apiclient.NetworkResource
	pollUntil(t, 30*time.Second, name+" network to publish a host port", func() (bool, string) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		got, err := client.GetAppNetwork(ctx, name)
		if err != nil {
			return false, err.Error()
		}
		netw = got
		if !got.Running || got.HostPort == 0 {
			return false, fmt.Sprintf("network %+v", got)
		}
		return true, ""
	})
	return netw
}

// breakGlassInspect independently confirms a service's single container
// through the real Docker daemon, never through the (possibly dead)
// control plane API: the point of this file's tests is to know the
// truth regardless of whether the control plane process is up.
func breakGlassInspect(t *testing.T, env liveBuildEnv, serviceName string) container.InspectResponse {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	containers, err := env.Runtime.ListByPrefix(ctx, serviceName+"-")
	if err != nil {
		t.Fatalf("ListByPrefix(%q) error = %v", serviceName, err)
	}
	if len(containers) != 1 {
		t.Fatalf("expected exactly 1 container for %q, got %d: %+v", serviceName, len(containers), containers)
	}
	insp, err := env.DockerCli.ContainerInspect(ctx, containers[0].ID)
	if err != nil {
		t.Fatalf("ContainerInspect(%q) error = %v", containers[0].ID, err)
	}
	return insp
}

func breakGlassDialFails(addr string) bool {
	conn, err := (&net.Dialer{Timeout: 2 * time.Second}).Dial("tcp", addr)
	if err != nil {
		return true
	}
	_ = conn.Close()
	return false
}

func breakGlassCaddyClient(caddyAddr string) *http.Client {
	return &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, network, caddyAddr)
			},
			// Caddy's internal issuer root is never installed into this
			// process's trust store, the same deliberate choice
			// deploy_test.go's identical comment documents: only the
			// handshake and routing are being proven here.
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // deliberate, see comment above
		},
	}
}

// TestBreakGlass_Live_ControlPlaneDeath_WorkloadAndIngress is backlog
// wave-1 item 9: does a running workload and Caddy's routing survive the
// control plane process dying outright.
//
// What it actually measures, precisely, because the honest answer has
// two different parts (internal/ingress/driver.go: caddy.Load drives a
// package-global Caddy instance inside this same process, the locked
// embedded-ingress architecture decision):
//
//  1. The app container is not a child of the control plane process. It
//     keeps running and keeps serving direct requests to its published
//     host port across a SIGKILL, with zero restarts.
//  2. Caddy's HTTPS listener dies in the same instant the process does:
//     domain routing has a real outage window from the moment the
//     control plane dies until a restarted one reconciles ingress
//     again. This is not a bug this test fixes; it is the documented,
//     measured shape of the embedded-ingress design (docs/resilience.md).
//  3. A restarted control plane, same data dir, reconciles the existing
//     container back into Ready without recreating or restarting it
//     (RestartCount stays 0, container ID is unchanged), and Caddy
//     resumes routing the domain once it reconciles.
//
// Skips cleanly if Docker or BuildKit aren't reachable, matching this
// package's other live tests.
func TestBreakGlass_Live_ControlPlaneDeath_WorkloadAndIngress(t *testing.T) {
	testenv.RequireFullLive(t)
	env := newLiveBuildEnv(t)

	const (
		serviceName = "levelrail-breakglass-cpdeath"
		domain      = "breakglass-cpdeath.levelrail.internal"
	)
	tag := "levelrail/breakglass-cpdeath:1"

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = env.DockerCli.ImageRemove(ctx, tag, image.RemoveOptions{Force: true})
	})
	cleanupContainers(context.Background(), t, env.Runtime, serviceName)
	t.Cleanup(func() { cleanupContainers(context.Background(), t, env.Runtime, serviceName) })

	buildCtx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	res, err := env.BuildClient.Build(buildCtx, build.Request{ContextDir: "../../fixtures/hello-e2e", Tag: tag}, nil)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	repoRoot, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	binDir := t.TempDir()
	cpBin := goBuild(t, repoRoot, binDir, "./cmd/levelrail")

	workDir := t.TempDir()
	dataDir := filepath.Join(workDir, "data")
	homeDir := filepath.Join(workDir, "home")
	httpAddr := fmt.Sprintf("127.0.0.1:%d", freePort(t))
	httpsAddr := fmt.Sprintf("127.0.0.1:%d", freePort(t))
	ingressHTTPAddr := fmt.Sprintf("127.0.0.1:%d", freePort(t))
	apiURL := "http://" + httpAddr

	cpEnv := envWithOverrides(hermeticEnv(
		"APP_DEV_MODE=1",
		"APP_BRAND_FILE="+filepath.Join(repoRoot, "brand.yaml"),
		"APP_DEV_FIXTURES_FILE="+filepath.Join(repoRoot, "dev-fixtures.yml"),
		"APP_DATA_DIR="+dataDir,
		"APP_HTTP_ADDR="+httpAddr,
		"APP_AGENT_ADDR=127.0.0.1:0",
		"APP_INGRESS_HTTPS_ADDR="+httpsAddr,
		"APP_INGRESS_HTTP_ADDR="+ingressHTTPAddr,
	),
		// Caddy's own cert/ACME storage has no APP_DATA_DIR-scoped
		// override (main.go never calls WithStorageDir for the real
		// binary), so it falls back to its OS default under $HOME.
		// Pinning HOME (and XDG_DATA_HOME for a Linux CI runner) to this
		// test's own temp dir keeps that storage, and the internal CA it
		// generates for the fake domain below, out of the real machine.
		"HOME="+homeDir,
		"XDG_DATA_HOME="+filepath.Join(homeDir, ".local", "share"),
		"XDG_CONFIG_HOME="+filepath.Join(homeDir, ".config"),
	)

	proc1 := startBreakGlassProcess(t, cpBin, cpEnv, filepath.Join(workDir, "server1.log"))
	client := apiclient.NewClient(apiURL, breakGlassToken)
	waitAPIUp(t, client, proc1.exited)

	createCtx, cancelCreate := context.WithTimeout(context.Background(), 30*time.Second)
	_, err = client.CreateApp(createCtx, apiclient.AppResource{
		Name:    serviceName,
		Image:   res.Tag,
		Port:    8080,
		Domains: []string{domain},
		Health:  &apiclient.ServiceHealth{Readiness: &apiclient.ServiceProbe{Path: "/"}},
	})
	cancelCreate()
	if err != nil {
		t.Fatalf("CreateApp() error = %v", err)
	}

	waitAppReady(t, client, serviceName)
	netw := waitAppNetwork(t, client, serviceName)
	directURL := fmt.Sprintf("http://127.0.0.1:%d/", netw.HostPort)
	httpClient := &http.Client{Timeout: 3 * time.Second}
	caddyClient := breakGlassCaddyClient(httpsAddr)

	if body := getBodyWithRetry(t, httpClient, directURL); !strings.Contains(body, helloBody) {
		t.Fatalf("direct request before kill: body = %q, want it to contain %q", body, helloBody)
	}
	if body := getBodyWithRetry(t, caddyClient, "https://"+domain+"/"); !strings.Contains(body, helloBody) {
		t.Fatalf("domain request before kill: body = %q, want it to contain %q", body, helloBody)
	}

	before := breakGlassInspect(t, env, serviceName)
	if !before.State.Running {
		t.Fatalf("container not running before kill: %+v", before.State)
	}

	// The chaos step: SIGKILL, no graceful shutdown, no defer runs.
	proc1.kill(t)

	// (1) The container answers directly: it is not a child of the
	// control plane process, and Docker does not tie its lifecycle to it.
	after := breakGlassInspect(t, env, serviceName)
	if !after.State.Running {
		t.Fatalf("container not running after control plane SIGKILL: %+v", after.State)
	}
	if body := getBodyWithRetry(t, httpClient, directURL); !strings.Contains(body, helloBody) {
		t.Fatalf("direct request after control plane death: body = %q, want it to contain %q", body, helloBody)
	}

	// (2) Caddy is gone: it's embedded in the process that just died, so
	// its listener died with it. This is the precise, real gap: routing
	// has an outage window here until the control plane restarts.
	if !breakGlassDialFails(httpsAddr) {
		t.Fatalf("expected Caddy's HTTPS listener at %s to be gone after control plane SIGKILL, but a connection succeeded", httpsAddr)
	}

	// Restart: same binary, same data dir, same ports, a fresh process.
	proc2 := startBreakGlassProcess(t, cpBin, cpEnv, filepath.Join(workDir, "server2.log"))
	client2 := apiclient.NewClient(apiURL, breakGlassToken)
	waitAPIUp(t, client2, proc2.exited)
	waitAppReady(t, client2, serviceName)

	// (3) The restart reconciled cleanly: the exact same container, never
	// recreated or restarted just because the control plane came back.
	final := breakGlassInspect(t, env, serviceName)
	if final.ID != after.ID {
		t.Fatalf("container was recreated across control plane restart: before-restart id=%s after-restart id=%s", after.ID, final.ID)
	}
	if final.RestartCount != 0 {
		t.Fatalf("container RestartCount = %d, want 0: the reconciler must never restart an already-running container just because the control plane restarted", final.RestartCount)
	}

	// Ingress resumes once the control plane (and its embedded Caddy) is
	// back and has reconciled at least once.
	if body := getBodyWithRetry(t, caddyClient, "https://"+domain+"/"); !strings.Contains(body, helloBody) {
		t.Fatalf("domain routing did not resume after control plane restart: body = %q, want it to contain %q", body, helloBody)
	}
}

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	dockerclient "github.com/docker/docker/client"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/brand"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/test/e2e/testenv"
)

const (
	e2eUDPAdminUsername = "e2e-udp-admin"
	e2eUDPAdminPassword = "e2e-udp-correct-horse" //nolint:gosec // test fixture credential, not a real secret
	e2eUDPApp           = "udpecho"
	e2eUDPService       = e2eUDPApp + "-echo"
	e2eUDPImage         = "python:3.12-alpine3.20"
	e2eUDPReadyBudget   = 90 * time.Second
)

// udpEchoCompose is the docs/udp-ports.md echo server: a UDP-only service
// with no HTTP port, so readiness falls back to the container running.
func udpEchoCompose(hostPort int) string {
	ports := "    ports:\n      - \"" + strconv.Itoa(hostPort) + ":9000/udp\"\n"
	return `services:
  echo:
    image: ` + e2eUDPImage + `
    command:
      - python3
      - -c
      - |
        import socket
        s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
        s.bind(("0.0.0.0", 9000))
        while True:
            d, a = s.recvfrom(65535)
            s.sendto(d, a)
` + ports
}

// TestPublishedPorts_Live_UDPEcho drives a UDP-only compose app through the
// real API and reconcile loop on real Docker and proves the port answers.
func TestPublishedPorts_Live_UDPEcho(t *testing.T) {
	testenv.RequireFullLive(t)
	env := newLiveBuildEnv(t)
	svcStore := openLiveStore(t)

	b := &brand.Brand{Name: "E2E Test Platform", BinaryName: "e2e-test-platform"}
	router := api.NewRouter(discardTestLogger(), b, svcStore)
	ts := newE2ETestServer(t, router)
	ctx := context.Background()
	if err := api.BootstrapAdmin(ctx, svcStore, e2eUDPAdminUsername, e2eUDPAdminPassword); err != nil {
		t.Fatalf("BootstrapAdmin() error = %v", err)
	}
	client := loginE2EClient(t, ts.URL, e2eUDPAdminUsername, e2eUDPAdminPassword)

	cleanupContainers(ctx, t, env.Runtime, e2eUDPService)
	t.Cleanup(func() {
		cctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cleanupContainers(cctx, t, env.Runtime, e2eUDPService)
		if err := env.Runtime.RemoveNetwork(cctx, application.NetworkName("", e2eUDPApp)); err != nil {
			t.Logf("cleanup: RemoveNetwork(%q) error = %v", e2eUDPApp, err)
		}
	})

	hostPort := freeUDPPort(t)
	ctrl := application.New(e2eUDPService, svcStore, env.Runtime, application.WithReadyBudget(e2eUDPReadyBudget))
	reconcileUDP := func(step string) {
		t.Helper()
		rctx, cancel := context.WithTimeout(ctx, 4*time.Minute)
		defer cancel()
		result, err := reconcileUntilReady(rctx, ctrl, 2*e2eUDPReadyBudget)
		if err != nil {
			t.Fatalf("%s: reconcile: %v (result = %+v)", step, err, result)
		}
		t.Logf("%s: reconcile %+v%s", step, result.Conditions, serviceContainers(t, env.DockerCli))
	}

	status, body := requestJSON(t, client, http.MethodPost, ts.URL+"/api/v1/apps/"+e2eUDPApp+"/compose", udpEchoCompose(hostPort))
	if status != http.StatusOK {
		t.Fatalf("compose deploy: status = %d, body = %s", status, body)
	}
	reconcileUDP("deploy")
	if err := udpEcho(hostPort, "deploy", 60*time.Second); err != nil {
		t.Fatalf("deploy: %v", err)
	}
	t.Logf("deploy: 127.0.0.1:%d/udp echoed", hostPort)
	firstID := runningContainerID(t, env.DockerCli)

	// PUT is a full replace, so edit the app the way the dashboard does: GET, change env, send it back.
	detail := appDetail(t, client, ts.URL)
	detail["env"] = json.RawMessage(`{"E2E_EDIT":"1"}`)
	edited, err := json.Marshal(detail)
	if err != nil {
		t.Fatalf("encode edit: %v", err)
	}
	status, body = requestJSON(t, client, http.MethodPut, ts.URL+"/api/v1/apps/"+e2eUDPService, string(edited))
	if status != http.StatusOK {
		t.Fatalf("env edit PUT: status = %d, body = %s", status, body)
	}
	if got := appPublishedPorts(t, client, ts.URL); got != fmt.Sprintf("%d/udp", hostPort) {
		t.Fatalf("after env edit: published_ports = %q, want %d/udp", got, hostPort)
	}
	// An env edit only marks the app dirty; the restart is the redeploy that applies it.
	status, body = requestJSON(t, client, http.MethodPost, ts.URL+"/api/v1/apps/"+e2eUDPService+"/restart", "")
	if status >= 300 {
		t.Fatalf("restart: status = %d, body = %s", status, body)
	}
	reconcileUDP("env edit")
	if err := udpEcho(hostPort, "after env edit", 60*time.Second); err != nil {
		t.Fatalf("after env edit: %v\ncontainers: %s", err, serviceContainers(t, env.DockerCli))
	}
	if runningContainerID(t, env.DockerCli) == firstID {
		t.Fatal("env edit: the container was not replaced, so the edit proved nothing")
	}
	t.Logf("env edit and restart: container replaced, port kept and 127.0.0.1:%d/udp still echoes", hostPort)

	t.Run("admission errors", func(t *testing.T) {
		expectComposeCode(t, client, ts.URL, "dupe", udpEchoCompose(hostPort), http.StatusConflict, "published_port_conflict")
		expectComposeCode(t, client, ts.URL, "quic", udpEchoCompose(8080), http.StatusConflict, "published_port_conflict")

		tcpPort := testenv.FreePort(t)
		status, body := requestJSON(t, client, http.MethodPost, ts.URL+"/api/v1/apps",
			fmt.Sprintf(`{"name":"tcpclash","image":"alpine:3.20","port":80,"host_port":%d,"published_ports":[{"container_port":53,"host_port":%d,"protocol":"tcp"}]}`, tcpPort, tcpPort))
		if status != http.StatusConflict || !strings.Contains(string(body), `"code":"published_port_conflict"`) {
			t.Errorf("tcp host_port and published tcp port on one number: status = %d, body = %s", status, body)
		}
		// UDP and TCP on the same number are two ports, so this one is admitted.
		status, body = requestJSON(t, client, http.MethodPost, ts.URL+"/api/v1/apps",
			fmt.Sprintf(`{"name":"tcpbeside","image":"alpine:3.20","port":80,"published_ports":[{"container_port":9000,"host_port":%d,"protocol":"tcp"}]}`, hostPort))
		if status >= 300 {
			t.Errorf("tcp beside the same udp number: status = %d, body = %s", status, body)
		}

		if runtime.GOOS != "linux" {
			t.Logf("host listener check reads the Linux socket table, not run on %s", runtime.GOOS)
			return
		}
		busy, err := net.ListenPacket("udp4", "0.0.0.0:0")
		if err != nil {
			t.Fatalf("listen udp: %v", err)
		}
		defer func() { _ = busy.Close() }()
		busyPort := busy.LocalAddr().(*net.UDPAddr).Port
		expectComposeCode(t, client, ts.URL, "busy", udpEchoCompose(busyPort), http.StatusConflict, "published_port_in_use")
	})

	detail = appDetail(t, client, ts.URL)
	detail["published_ports"] = json.RawMessage(`[]`)
	cleared, err := json.Marshal(detail)
	if err != nil {
		t.Fatalf("encode clear: %v", err)
	}
	status, body = requestJSON(t, client, http.MethodPut, ts.URL+"/api/v1/apps/"+e2eUDPService, string(cleared))
	if status != http.StatusOK {
		t.Fatalf("clear PUT: status = %d, body = %s", status, body)
	}
	status, body = requestJSON(t, client, http.MethodPost, ts.URL+"/api/v1/apps/"+e2eUDPService+"/restart", "")
	if status >= 300 {
		t.Fatalf("restart after clear: status = %d, body = %s", status, body)
	}
	reconcileUDP("clear")
	if bound := udpBindings(t, env.DockerCli); len(bound) != 0 {
		t.Fatalf("after clear: running container still binds %v", bound)
	}
	if err := udpEcho(hostPort, "after clear", 3*time.Second); err == nil {
		t.Fatalf("after clear: 127.0.0.1:%d/udp still echoes", hostPort)
	}
	t.Logf("clear: binding removed, 127.0.0.1:%d/udp silent", hostPort)
}

func freeUDPPort(t *testing.T) int {
	t.Helper()
	pc, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen udp: %v", err)
	}
	port := pc.LocalAddr().(*net.UDPAddr).Port
	_ = pc.Close()
	return port
}

// udpEcho retries because the port forward and the python loop come up a
// moment after the container reports running.
func udpEcho(port int, payload string, budget time.Duration) error {
	deadline := time.Now().Add(budget)
	var lastErr error
	for time.Now().Before(deadline) {
		lastErr = udpEchoOnce(port, payload)
		if lastErr == nil {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("no echo from 127.0.0.1:%d/udp within %s: %w", port, budget, lastErr)
}

func udpEchoOnce(port int, payload string) error {
	conn, err := net.Dial("udp4", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer func() { _ = conn.Close() }()
	if err := conn.SetDeadline(time.Now().Add(time.Second)); err != nil {
		return fmt.Errorf("deadline: %w", err)
	}
	if _, err := conn.Write([]byte(payload)); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	buf := make([]byte, 256)
	n, err := conn.Read(buf)
	if err != nil {
		return fmt.Errorf("read: %w", err)
	}
	if got := string(buf[:n]); got != payload {
		return fmt.Errorf("echo = %q, want %q", got, payload)
	}
	return nil
}

func appDetail(t *testing.T, client *http.Client, baseURL string) map[string]json.RawMessage {
	t.Helper()
	status, body := requestJSON(t, client, http.MethodGet, baseURL+"/api/v1/apps/"+e2eUDPService, "")
	if status != http.StatusOK {
		t.Fatalf("GET app: status = %d, body = %s", status, body)
	}
	var detail map[string]json.RawMessage
	if err := json.Unmarshal(body, &detail); err != nil {
		t.Fatalf("decode GET app: %v, body = %s", err, body)
	}
	return detail
}

func appPublishedPorts(t *testing.T, client *http.Client, baseURL string) string {
	t.Helper()
	status, body := requestJSON(t, client, http.MethodGet, baseURL+"/api/v1/apps/"+e2eUDPService, "")
	if status != http.StatusOK {
		t.Fatalf("GET app: status = %d, body = %s", status, body)
	}
	var got struct {
		PublishedPorts []struct {
			HostPort int    `json:"host_port"`
			Protocol string `json:"protocol"`
		} `json:"published_ports"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode GET app: %v, body = %s", err, body)
	}
	parts := make([]string, 0, len(got.PublishedPorts))
	for _, p := range got.PublishedPorts {
		parts = append(parts, fmt.Sprintf("%d/%s", p.HostPort, p.Protocol))
	}
	return strings.Join(parts, ",")
}

func expectComposeCode(t *testing.T, client *http.Client, baseURL, app, compose string, wantStatus int, wantCode string) {
	t.Helper()
	status, body := requestJSON(t, client, http.MethodPost, baseURL+"/api/v1/apps/"+app+"/compose", compose)
	if status != wantStatus || !strings.Contains(string(body), `"code":"`+wantCode+`"`) {
		t.Errorf("compose %s: status = %d, body = %s, want %d %s", app, status, body, wantStatus, wantCode)
		return
	}
	t.Logf("compose %s: %d %s", app, status, wantCode)
}

// serviceContainers describes every container of the service, for a failure message.
func serviceContainers(t *testing.T, cli *dockerclient.Client) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	list, err := cli.ContainerList(ctx, container.ListOptions{All: true, Filters: filters.NewArgs(filters.Arg("name", e2eUDPService))})
	if err != nil {
		return err.Error()
	}
	var b strings.Builder
	for _, c := range list {
		fmt.Fprintf(&b, "\n  %v %s %s ports=%+v", c.Names, c.State, c.Status, c.Ports)
	}
	return b.String()
}

func runningContainerID(t *testing.T, cli *dockerclient.Client) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	list, err := cli.ContainerList(ctx, container.ListOptions{Filters: filters.NewArgs(filters.Arg("name", e2eUDPService))})
	if err != nil || len(list) != 1 {
		t.Fatalf("ContainerList: %d running (err %v), want 1", len(list), err)
	}
	return list[0].ID
}

// udpBindings lists the UDP host bindings of the service's running containers.
func udpBindings(t *testing.T, cli *dockerclient.Client) []string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	list, err := cli.ContainerList(ctx, container.ListOptions{Filters: filters.NewArgs(filters.Arg("name", e2eUDPService))})
	if err != nil {
		t.Fatalf("ContainerList: %v", err)
	}
	if len(list) == 0 {
		t.Fatal("no running container for the service after reconcile")
	}
	var out []string
	for _, c := range list {
		for _, p := range c.Ports {
			if p.Type == "udp" && p.PublicPort != 0 {
				out = append(out, fmt.Sprintf("%s:%d/udp", p.IP, p.PublicPort))
			}
		}
	}
	return out
}

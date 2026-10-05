package sshprovision

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	gliderssh "github.com/gliderlabs/ssh"
	"golang.org/x/crypto/ssh"
)

// fakeHost is an in-process SSH server standing in for the remote
// machine Provision would otherwise dial over a real network: real SSH
// client/server code is exercised end to end, no live host required. It
// answers the fixed sequence of commands Provision runs by name, the
// same testing technique gliderlabs/ssh's own docs recommend for SSH
// client code.
type fakeHost struct {
	dockerPresent  bool
	systemdPresent bool
	unsupportedOS  bool
	unsupportedArc bool
	failInstall    bool

	mu       sync.Mutex
	envFile  string
	unitFile string
}

func newFakeHost(t *testing.T, password string, authorizedKey ssh.PublicKey) (addr string, host *fakeHost) {
	t.Helper()
	host = &fakeHost{dockerPresent: true, systemdPresent: true}

	srv := &gliderssh.Server{
		Handler: host.handle,
	}
	if password != "" {
		srv.PasswordHandler = func(_ gliderssh.Context, pass string) bool { return pass == password }
	}
	if authorizedKey != nil {
		srv.PublicKeyHandler = func(_ gliderssh.Context, key gliderssh.PublicKey) bool {
			return gliderssh.KeysEqual(key, authorizedKey)
		}
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() { _ = srv.Serve(ln) }()
	return ln.Addr().String(), host
}

func (h *fakeHost) handle(s gliderssh.Session) {
	cmd := s.RawCommand()
	switch {
	case cmd == "uname -s":
		if h.unsupportedOS {
			_, _ = io.WriteString(s, "Darwin\n")
		} else {
			_, _ = io.WriteString(s, "Linux\n")
		}
		_ = s.Exit(0)
	case cmd == "uname -m":
		if h.unsupportedArc {
			_, _ = io.WriteString(s, "riscv64\n")
		} else {
			_, _ = io.WriteString(s, "x86_64\n")
		}
		_ = s.Exit(0)
	case strings.HasPrefix(cmd, "cat /etc/os-release"):
		_, _ = io.WriteString(s, "PRETTY_NAME=\"Test Linux 1.0\"\n")
		_ = s.Exit(0)
	case cmd == "command -v systemctl >/dev/null 2>&1":
		_ = s.Exit(exitCode(h.systemdPresent))
	case cmd == "command -v docker >/dev/null 2>&1":
		_ = s.Exit(exitCode(h.dockerPresent))
	case strings.HasPrefix(cmd, "curl -fsSL https://get.docker.com"):
		if h.failInstall {
			_, _ = io.WriteString(s, "docker install failed\n")
			_ = s.Exit(1)
			return
		}
		_, _ = io.WriteString(s, "installing docker...\n")
		_ = s.Exit(0)
	case cmd == "systemctl enable --now docker":
		_ = s.Exit(0)
	case strings.HasPrefix(cmd, "mkdir -p /var/lib/acme-agent-data"):
		_ = s.Exit(0)
	case strings.HasPrefix(cmd, "install -m 600 /dev/stdin "+testNames.envPath()):
		content, _ := io.ReadAll(s)
		h.mu.Lock()
		h.envFile = string(content)
		h.mu.Unlock()
		_ = s.Exit(0)
	case strings.HasPrefix(cmd, "install -m 644 /dev/stdin "+testNames.unitPath()):
		content, _ := io.ReadAll(s)
		h.mu.Lock()
		h.unitFile = string(content)
		h.mu.Unlock()
		_ = s.Exit(0)
	case cmd == "systemctl daemon-reload && systemctl enable --now acme-agent":
		_ = s.Exit(0)
	case strings.HasPrefix(cmd, "sleep 2 && systemctl is-active acme-agent"):
		if h.failInstall {
			_, _ = io.WriteString(s, "failed\n")
			_ = s.Exit(3)
			return
		}
		_, _ = io.WriteString(s, "active\n")
		_ = s.Exit(0)
	case strings.HasPrefix(cmd, "journalctl -u acme-agent"):
		_, _ = io.WriteString(s, "-- no useful diagnostic in test --\n")
		_ = s.Exit(0)
	default:
		_, _ = io.WriteString(s, "unrecognized command in test fake: "+cmd+"\n")
		_ = s.Exit(127)
	}
}

func exitCode(present bool) int {
	if present {
		return 0
	}
	return 1
}

func (h *fakeHost) envFileContent() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.envFile
}

func (h *fakeHost) unitFileContent() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.unitFile
}

func generateTestKey(t *testing.T) (pemBytes []byte, pub ssh.PublicKey) {
	t.Helper()
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(privKey)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	block := &pem.Block{Type: "PRIVATE KEY", Bytes: der}
	signerPub, err := ssh.NewPublicKey(pubKey)
	if err != nil {
		t.Fatalf("public key: %v", err)
	}
	return pem.EncodeToMemory(block), signerPub
}

func hostPort(t *testing.T, addr string) (string, int) {
	t.Helper()
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("split host port %q: %v", addr, err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("parse port %q: %v", portStr, err)
	}
	return host, port
}

func testParams() InstallParams {
	return InstallParams{
		ControlPlaneAddr: "control-plane.example:9443",
		JoinToken:        "test-join-token",
		CAFingerprint:    "sha256:deadbeef",
		NodeName:         "ssh-test-node",
	}
}

func TestProvisionPasswordAuthDockerAlreadyPresent(t *testing.T) {
	addr, host := newFakeHost(t, "s3cret", nil)
	h, p := hostPort(t, addr)

	var events []Event
	det, err := New("acme-agent", "Acme").Provision(context.Background(), Credentials{
		Host: h, Port: p, Username: "root", Auth: AuthPassword, Password: "s3cret",
	}, testParams(), func(e Event) { events = append(events, e) })
	if err != nil {
		t.Fatalf("Provision: %v", err)
	}
	if det.OS != "linux" || det.Arch != "amd64" {
		t.Fatalf("unexpected detected host: %+v", det)
	}
	if det.Distro != "Test Linux 1.0" {
		t.Fatalf("distro = %q", det.Distro)
	}
	if !det.DockerPresent || !det.SystemdPresent {
		t.Fatalf("expected docker and systemd present, got %+v", det)
	}

	if !strings.Contains(host.envFileContent(), "APP_JOIN_TOKEN=test-join-token") {
		t.Fatalf("env file missing join token: %q", host.envFileContent())
	}
	if !strings.Contains(host.envFileContent(), "APP_CONTROL_PLANE_ADDR=control-plane.example:9443") {
		t.Fatalf("env file missing control plane addr: %q", host.envFileContent())
	}
	if !strings.Contains(host.unitFileContent(), "ghcr.io/glincker/acme-agent") {
		t.Fatalf("unit file missing agent image: %q", host.unitFileContent())
	}

	for _, e := range events {
		if strings.Contains(e.Message, "test-join-token") {
			t.Fatalf("join token leaked into an event message: %q", e.Message)
		}
	}
	sawDone := false
	for _, e := range events {
		if e.Step == StepDone {
			sawDone = true
		}
	}
	if !sawDone {
		t.Fatalf("expected a StepDone event, got %+v", events)
	}
}

func TestProvisionKeyAuthInstallsDocker(t *testing.T) {
	keyPEM, pub := generateTestKey(t)
	addr, host := newFakeHost(t, "", pub)
	host.dockerPresent = false
	h, p := hostPort(t, addr)

	det, err := New("acme-agent", "Acme").Provision(context.Background(), Credentials{
		Host: h, Port: p, Username: "root", Auth: AuthKey, PrivateKey: string(keyPEM),
	}, testParams(), nil)
	if err != nil {
		t.Fatalf("Provision: %v", err)
	}
	if det.DockerPresent {
		t.Fatalf("expected DockerPresent to reflect the pre-install probe (false), got true")
	}
	if host.envFileContent() == "" {
		t.Fatalf("expected the agent env file to have been written")
	}
}

func TestProvisionRejectsUnsupportedOS(t *testing.T) {
	addr, host := newFakeHost(t, "s3cret", nil)
	host.unsupportedOS = true
	h, p := hostPort(t, addr)

	_, err := New("acme-agent", "Acme").Provision(context.Background(), Credentials{
		Host: h, Port: p, Username: "root", Auth: AuthPassword, Password: "s3cret",
	}, testParams(), nil)
	if err == nil || !strings.Contains(err.Error(), "unsupported OS") {
		t.Fatalf("expected an unsupported OS error, got %v", err)
	}
}

func TestProvisionRejectsUnsupportedArch(t *testing.T) {
	addr, host := newFakeHost(t, "s3cret", nil)
	host.unsupportedArc = true
	h, p := hostPort(t, addr)

	_, err := New("acme-agent", "Acme").Provision(context.Background(), Credentials{
		Host: h, Port: p, Username: "root", Auth: AuthPassword, Password: "s3cret",
	}, testParams(), nil)
	if err == nil || !strings.Contains(err.Error(), "unsupported CPU architecture") {
		t.Fatalf("expected an unsupported architecture error, got %v", err)
	}
}

func TestProvisionRejectsNoSystemd(t *testing.T) {
	addr, host := newFakeHost(t, "s3cret", nil)
	host.systemdPresent = false
	h, p := hostPort(t, addr)

	_, err := New("acme-agent", "Acme").Provision(context.Background(), Credentials{
		Host: h, Port: p, Username: "root", Auth: AuthPassword, Password: "s3cret",
	}, testParams(), nil)
	if err == nil || !strings.Contains(err.Error(), "no systemd") {
		t.Fatalf("expected a no-systemd error, got %v", err)
	}
}

func TestProvisionFailsOnWrongPassword(t *testing.T) {
	addr, _ := newFakeHost(t, "s3cret", nil)
	h, p := hostPort(t, addr)

	_, err := New("acme-agent", "Acme").Provision(context.Background(), Credentials{
		Host: h, Port: p, Username: "root", Auth: AuthPassword, Password: "wrong",
	}, testParams(), nil)
	if err == nil || !strings.Contains(err.Error(), "authenticate") {
		t.Fatalf("expected an authentication error, got %v", err)
	}
}

func TestProvisionFailsWhenAgentServiceDoesNotStayActive(t *testing.T) {
	addr, host := newFakeHost(t, "s3cret", nil)
	host.failInstall = true
	h, p := hostPort(t, addr)

	_, err := New("acme-agent", "Acme").Provision(context.Background(), Credentials{
		Host: h, Port: p, Username: "root", Auth: AuthPassword, Password: "s3cret",
	}, testParams(), nil)
	if err == nil || !strings.Contains(err.Error(), "did not stay active") {
		t.Fatalf("expected an agent-not-active error, got %v", err)
	}
}

func TestProvisionRequiresParams(t *testing.T) {
	addr, _ := newFakeHost(t, "s3cret", nil)
	h, p := hostPort(t, addr)

	_, err := New("acme-agent", "Acme").Provision(context.Background(), Credentials{
		Host: h, Port: p, Username: "root", Auth: AuthPassword, Password: "s3cret",
	}, InstallParams{}, nil)
	if err == nil {
		t.Fatalf("expected an error for missing required params")
	}
}

func TestProvisionUnreachableHostFailsFast(t *testing.T) {
	p := &Provisioner{DialTimeout: 500 * time.Millisecond}
	_, err := p.Provision(context.Background(), Credentials{
		Host: "127.0.0.1", Port: 1, Username: "root", Auth: AuthPassword, Password: "x",
	}, testParams(), nil)
	if err == nil {
		t.Fatalf("expected a dial error")
	}
	var netErr *net.OpError
	if !errors.As(err, &netErr) {
		t.Fatalf("expected a net.OpError, got %T: %v", err, err)
	}
}

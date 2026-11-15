package dockertest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// ForgeKind selects which self-hosted forge StartForge runs.
type ForgeKind struct {
	Name   string
	Image  string
	Binary string
	// EnvPrefix is the app.ini override prefix the image understands.
	EnvPrefix string
}

var (
	// Gitea is the official rootless Gitea image.
	Gitea = ForgeKind{Name: "gitea", Image: "gitea/gitea:1.22-rootless", Binary: "gitea", EnvPrefix: "GITEA__"}
	// Forgejo is the official rootless Forgejo image.
	Forgejo = ForgeKind{Name: "forgejo", Image: "codeberg.org/forgejo/forgejo:11-rootless", Binary: "forgejo", EnvPrefix: "FORGEJO__"}
)

// Forge is one running throwaway forge container.
type Forge struct {
	Kind     ForgeKind
	URL      string
	Admin    string
	Password string
	// Token is an admin access token with every scope; never log it.
	Token     string
	container string
}

// StartForge runs a forge on a random loopback port with no volumes, creates
// an admin user and token through the container's own CLI, and removes the
// container when the test ends. Images stay for the caller to remove.
func StartForge(t *testing.T, kind ForgeKind) *Forge {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	name := fmt.Sprintf("lr-forge-%s-%s", kind.Name, randHex(4))
	p := kind.EnvPrefix
	args := []string{
		"run", "-d", "--name", name,
		"-p", "127.0.0.1::3000",
		// Lets the forge deliver webhooks to the test receiver on the host.
		"--add-host", "host.docker.internal:host-gateway",
		"-e", p + "security__INSTALL_LOCK=true",
		"-e", p + "database__DB_TYPE=sqlite3",
		"-e", p + "webhook__ALLOWED_HOST_LIST=*",
		"-e", p + "server__DISABLE_SSH=true",
		"-e", p + "service__DISABLE_REGISTRATION=true",
		"-e", p + "log__LEVEL=Warn",
		kind.Image,
	}
	if out, err := docker(ctx, args...); err != nil {
		t.Fatalf("start %s: %v: %s", kind.Name, err, out)
	}
	t.Cleanup(func() { _, _ = docker(context.Background(), "rm", "-f", name) })

	port, err := docker(ctx, "port", name, "3000/tcp")
	if err != nil {
		t.Fatalf("port of %s: %v", name, err)
	}
	// Output is "127.0.0.1:49153", possibly followed by an IPv6 line.
	hostPort := strings.TrimSpace(strings.Split(port, "\n")[0])
	f := &Forge{Kind: kind, URL: "http://" + hostPort, Admin: "lradmin", Password: randHex(12), container: name}
	f.waitReady(ctx, t)
	f.bootstrap(ctx, t)
	return f
}

func (f *Forge) waitReady(ctx context.Context, t *testing.T) {
	t.Helper()
	for ctx.Err() == nil {
		out, err := docker(ctx, "exec", f.container, "wget", "-qO-", "http://127.0.0.1:3000/api/healthz")
		if err == nil && strings.Contains(out, "pass") {
			return
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("%s did not become ready: %v", f.Kind.Name, ctx.Err())
}

func (f *Forge) bootstrap(ctx context.Context, t *testing.T) {
	t.Helper()
	bin := f.Kind.Binary
	if out, err := docker(ctx, "exec", f.container, bin, "admin", "user", "create",
		"--username", f.Admin, "--password", f.Password, "--email", f.Admin+"@example.invalid",
		"--admin", "--must-change-password=false"); err != nil {
		t.Fatalf("create admin: %v: %s", err, redact(out, f.Password))
	}
	out, err := docker(ctx, "exec", f.container, bin, "admin", "user", "generate-access-token",
		"--username", f.Admin, "--token-name", "lr-live", "--scopes", "all", "--raw")
	if err != nil {
		t.Fatalf("create token: %v: %s", err, redact(out, f.Password))
	}
	f.Token = strings.TrimSpace(out)
}

// docker runs the docker CLI; arguments are fixed test inputs, never user data.
func docker(ctx context.Context, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput() //nolint:gosec // fixed binary, test-only arguments
	return string(out), err
}

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func redact(s, secret string) string {
	return strings.ReplaceAll(s, secret, "[redacted]")
}

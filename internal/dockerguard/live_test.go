package dockerguard

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	dockerclient "github.com/docker/docker/client"
	dockerbuildkit "github.com/docker/docker/client/buildkit"
	bkclient "github.com/moby/buildkit/client"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/dockertest"
)

// TestGuard_Live_RealDaemon drives a real daemon through an enforcing guard:
// the hardened create path works end to end, a privileged create does not.
func TestGuard_Live_RealDaemon(t *testing.T) {
	dockertest.SkipIfShort(t)
	upstream, err := UpstreamSocket(docker.DetectRuntimeSocket(os.LookupEnv).Host)
	if err != nil {
		t.Skipf("daemon not on a unix socket: %v", err)
	}
	tun, _ := TunablesFromEnv(func(string) (string, bool) { return "", false })
	grants := NewGrants()
	hardening := docker.HardeningConfig{Mode: docker.HardeningEnforce, PidsLimit: docker.DefaultPidsLimit}
	sink := &memorySink{}
	g := New(Config{Mode: ModeEnforce, Upstream: upstream, Grants: grants, Tunables: tun, Sink: sink,
		Policy: PolicyFromHardening(hardening, tun), Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	srv, err := Listen(ctx, g, filepath.Join(shortTempDir(t), "g", SocketName))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = srv.Close() }()

	client, err := docker.NewClient(docker.WithHost(srv.Host()), docker.WithCreateDeclarer(grants), docker.WithHardening(hardening))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Close() }()
	if err := client.Ping(ctx); err != nil {
		t.Skipf("docker daemon not reachable through the guard: %v", err)
	}

	name := "dockerguard-live-" + strings.ReplaceAll(time.Now().Format("150405.000000"), ".", "")
	id, err := client.Create(ctx, docker.ContainerSpec{Name: name, Image: "hello-world:latest", Labels: map[string]string{"dockerguard-live": "1"}})
	if err != nil {
		t.Fatalf("hardened create through the guard: %v", err)
	}
	if err := client.Start(ctx, id); err != nil {
		t.Errorf("start through the guard: %v", err)
	}
	waitCtx, waitCancel := context.WithTimeout(ctx, 30*time.Second)
	for waitCtx.Err() == nil {
		if st, err := client.InspectByName(waitCtx, name); err == nil && st != nil && !st.Running {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	waitCancel()
	lines, logErrs := client.Logs(ctx, id, false, time.Time{})
	var logText strings.Builder
	for l := range lines {
		logText.WriteString(l.Message)
	}
	if err := <-logErrs; err != nil || !strings.Contains(logText.String(), "Hello from Docker") {
		t.Errorf("logs through the guard: %v (%d bytes)", err, logText.Len())
	}
	if err := client.Remove(ctx, id, true); err != nil {
		t.Errorf("remove through the guard: %v", err)
	}

	raw, err := dockerclient.NewClientWithOpts(dockerclient.WithHost(srv.Host()), dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()
	_, err = raw.ContainerCreate(ctx, &container.Config{Image: "hello-world:latest"}, &container.HostConfig{Privileged: true}, nil, nil, name+"-priv")
	if err == nil {
		_ = raw.ContainerRemove(ctx, name+"-priv", container.RemoveOptions{Force: true})
		t.Fatal("privileged create reached the daemon in enforce mode")
	}
	if !strings.Contains(err.Error(), RulePrivileged) {
		t.Fatalf("privileged create error = %v, want rule %s", err, RulePrivileged)
	}
	if _, ierr := raw.ContainerInspect(ctx, name+"-priv"); ierr == nil {
		t.Fatal("denied container exists on the daemon")
	}

	bk, err := bkclient.New(ctx, "", dockerbuildkit.ClientOpts(raw)...)
	if err != nil {
		t.Fatalf("buildkit through the guard: %v", err)
	}
	defer func() { _ = bk.Close() }()
	if _, err := bk.ListWorkers(ctx); err != nil {
		t.Fatalf("buildkit /grpc and /session upgrade through the guard: %v", err)
	}
}

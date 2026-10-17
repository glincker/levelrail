package main

import (
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/GLINCKER/levelrail/internal/plugin"
	"github.com/GLINCKER/levelrail/internal/plugin/pluginpb"
)

func TestFixtureManifestIsValid(t *testing.T) {
	raw, err := os.ReadFile("plugin.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := plugin.ParseManifest(raw); err != nil {
		t.Fatalf("plugin.yaml: %v", err)
	}
}

func TestFixtureSpeaksTheContract(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "pn")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, "p.sock")
	lis, err := (&net.ListenConfig{}).Listen(context.Background(), "unix", sock)
	if err != nil {
		t.Skipf("unix sockets unavailable: %v", err)
	}
	s := newServer()
	go func() { _ = s.Serve(lis) }()
	t.Cleanup(s.Stop)

	conn, err := grpc.NewClient("unix://"+sock, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	d, err := pluginpb.NewPluginServiceClient(conn).Describe(ctx, &pluginpb.DescribeRequest{HostContractVersions: plugin.ContractVersions})
	if err != nil || d.GetName() != pluginName || d.GetKind() != plugin.KindNotificationChannel {
		t.Fatalf("Describe = %v, %v", d, err)
	}
	if _, err := pluginpb.NewPluginServiceClient(conn).Describe(ctx, &pluginpb.DescribeRequest{HostContractVersions: []uint32{99}}); err == nil {
		t.Fatal("a host with no common contract version was accepted")
	}
	r, err := pluginpb.NewNotificationChannelServiceClient(conn).Send(ctx, &pluginpb.SendRequest{Event: &pluginpb.Event{Id: "e", Title: "t"}, Secret: "s"})
	if err != nil || !r.GetDelivered() || r.GetDetail() != "logged" {
		t.Fatalf("Send = %v, %v", r, err)
	}
}

// TestFixtureBuildsAsImage needs a Docker daemon and is skipped without one; a real
// plugin run under the supervisor is not covered here.
func TestFixtureBuildsAsImage(t *testing.T) {
	if testing.Short() {
		t.Skip("image build skipped under -short")
	}
	if err := exec.CommandContext(context.Background(), "docker", "info").Run(); err != nil {
		t.Skip("no Docker daemon available")
	}
	tmp := t.TempDir()
	build := exec.CommandContext(context.Background(), "go", "build", "-o", filepath.Join(tmp, "plugin"), ".") //nolint:gosec // fixed arguments, temp dir
	build.Env = append(cleanEnv(), "CGO_ENABLED=0", "GOOS=linux")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	dockerfile, err := os.ReadFile("Dockerfile")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "Dockerfile"), dockerfile, 0o600); err != nil { //nolint:gosec // temp dir
		t.Fatal(err)
	}
	tag := "plugin-notify-fixture:test"
	if out, err := exec.CommandContext(context.Background(), "docker", "build", "-t", tag, tmp).CombinedOutput(); err != nil { //nolint:gosec // fixed arguments, temp dir
		t.Fatalf("docker build: %v\n%s", err, out)
	}
	t.Cleanup(func() { _ = exec.CommandContext(context.Background(), "docker", "rmi", "-f", tag).Run() })
}

func cleanEnv() []string {
	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "GIT_") && !strings.HasPrefix(kv, "GOOS=") && !strings.HasPrefix(kv, "GOARCH=") {
			env = append(env, kv)
		}
	}
	return env
}

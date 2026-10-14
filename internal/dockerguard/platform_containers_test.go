package dockerguard

import (
	"context"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
)

// TestEnforceAllowsContainersThePlatformCreates drives the real internal/docker
// client through an enforcing guard: every container shape the control plane
// and agent build themselves must reach the daemon.
func TestEnforceAllowsContainersThePlatformCreates(t *testing.T) {
	h := startGuard(t, ModeEnforce)
	hardening := docker.HardeningConfig{Mode: docker.HardeningEnforce, PidsLimit: docker.DefaultPidsLimit}
	client, err := docker.NewClient(docker.WithHost(h.server.Host()), docker.WithCreateDeclarer(h.grants), docker.WithHardening(hardening))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	ctx := context.Background()

	creates := []struct {
		name string
		run  func() error
	}{
		{"app with volume and declared bind mount", func() error {
			_, err := client.Create(ctx, docker.ContainerSpec{
				Name: "app-1", Image: "alpine:3",
				Volumes:    []docker.VolumeMount{{Name: "vol1", ContainerPath: "/data"}},
				BindMounts: []docker.BindMount{{HostPath: "/srv/shared/app", ContainerPath: "/mnt", ReadOnly: true}},
			})
			return err
		}},
		{"egress sidecar with NET_ADMIN sharing the app netns", func() error {
			_, err := client.Create(ctx, docker.ContainerSpec{
				Name: "egress-1", Image: "alpine:3", CapAdd: []string{"NET_ADMIN", "NET_RAW"},
				NetworkMode: "container:c1",
			})
			return err
		}},
		{"gpu app", func() error {
			_, err := client.Create(ctx, docker.ContainerSpec{Name: "gpu-1", Image: "alpine:3", GPU: &docker.GPURequest{Count: 1}})
			return err
		}},
		{"one-shot scanner", func() error {
			_, err := client.RunOneShot(ctx, docker.OneShotSpec{
				Name: "scan-1", Image: "alpine:3", Command: []string{"true"},
				Volumes: map[string]string{"scan-cache": "/cache"},
			})
			return err
		}},
		{"browser preview", func() error {
			b, err := client.StartBrowser(ctx, docker.BrowserSpec{
				Name: "browser-1", Image: "alpine:3", DebugPort: 9222,
				Tmpfs: map[string]string{"/tmp": "rw"}, User: "1000",
			})
			if b != nil {
				b.Close()
			}
			return err
		}},
		{"network share volume", func() error {
			return client.EnsureNetworkVolume(ctx, "nfs-1", docker.NetworkShareVolumeOpts{Protocol: docker.NetworkShareProtocolNFS, Host: "10.0.0.5", RemotePath: "/exports"})
		}},
	}
	for _, c := range creates {
		t.Run(c.name, func(t *testing.T) {
			if err := c.run(); err != nil && (strings.Contains(err.Error(), "403") || strings.Contains(err.Error(), "denied")) {
				t.Fatalf("enforce denied a platform container: %v", err)
			}
		})
	}
	if err := client.PauseContainer(ctx, "c1"); err != nil && strings.Contains(err.Error(), "403") {
		t.Fatalf("pause denied: %v", err)
	}
	if err := client.UnpauseContainer(ctx, "c1"); err != nil && strings.Contains(err.Error(), "403") {
		t.Fatalf("unpause denied: %v", err)
	}

	const wantCreates = 5
	creating := 0
	for _, r := range h.daemon.requests() {
		if strings.HasSuffix(r.Path, "/containers/create") {
			creating++
		}
	}
	if creating < wantCreates {
		t.Fatalf("only %d of %d containers reached the daemon", creating, wantCreates)
	}
	for _, d := range h.sink.all() {
		if d.Denied {
			t.Errorf("platform request denied: %s %s %+v", d.Method, d.Path, d.Violations)
		}
	}
}

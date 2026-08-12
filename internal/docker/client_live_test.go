package docker

import (
	"context"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
)

// liveClient returns a Client connected to a real daemon, or skips the
// test cleanly if none is reachable, the same pattern internal/build and
// internal/ingress use, so CI without Docker-in-Docker skips instead of
// failing.
func liveClient(t *testing.T) *Client {
	t.Helper()
	c, err := NewClient()
	if err != nil {
		t.Skipf("no docker client available: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := c.cli.Ping(ctx); err != nil {
		t.Skipf("docker daemon not reachable: %v", err)
	}
	t.Cleanup(func() {
		if err := c.Close(); err != nil {
			t.Errorf("closing client: %v", err)
		}
	})
	return c
}

// TestClient_Create_Live_PortsEnvAndNoRestartPolicy proves the extended
// ContainerSpec fields actually reach a real container, not just that
// the Docker API calls don't error. Creates a real container, published
// port and all, and inspects it back through both this package's own
// InspectByName and a raw ContainerInspect call, independently.
func TestClient_Create_Live_PortsEnvAndNoRestartPolicy(t *testing.T) {
	c := liveClient(t)
	ctx := context.Background()

	name := "levelrail-test-docker-wrapper"

	// Clean up anything left over from a previous failed run before
	// starting, and always clean up after.
	removeIfExists(ctx, t, c, name)
	t.Cleanup(func() { removeIfExists(ctx, t, c, name) })

	id, err := c.Create(ctx, ContainerSpec{
		Name:  name,
		Image: "nginx:alpine",
		Ports: []PortBinding{{ContainerPort: 80}}, // HostPort 0: let Docker assign one
		Env:   map[string]string{"LEVELRAIL_TEST": "1"},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := c.Start(ctx, id); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Give Docker a moment to actually bind the published port; Start
	// returning success doesn't guarantee the port binding is visible
	// to a ContainerList query yet.
	var state *ContainerState
	for range 20 {
		state, err = c.InspectByName(ctx, name)
		if err != nil {
			t.Fatalf("InspectByName() error = %v", err)
		}
		if state != nil && len(state.Ports) > 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if state == nil {
		t.Fatal("InspectByName() = nil, want the container we just created")
	}
	if !state.Running {
		t.Error("expected the container to be running")
	}
	if len(state.Ports) != 1 {
		t.Fatalf("expected 1 observed port binding, got %d: %+v", len(state.Ports), state.Ports)
	}
	if state.Ports[0].ContainerPort != 80 {
		t.Errorf("ContainerPort = %d, want 80", state.Ports[0].ContainerPort)
	}
	if state.Ports[0].HostPort == 0 {
		t.Error("HostPort = 0, want a real assigned port")
	}

	// Independent verification, not trusting this package's own
	// InspectByName: inspect the raw container directly and check the
	// env var and restart policy landed exactly as specified.
	raw, err := c.cli.ContainerInspect(ctx, id)
	if err != nil {
		t.Fatalf("raw ContainerInspect() error = %v", err)
	}
	foundEnv := false
	for _, e := range raw.Config.Env {
		if e == "LEVELRAIL_TEST=1" {
			foundEnv = true
		}
	}
	if !foundEnv {
		t.Errorf("env var LEVELRAIL_TEST=1 not found in raw container config: %v", raw.Config.Env)
	}
	if raw.HostConfig.RestartPolicy.Name != "no" {
		t.Errorf("restart policy = %q, want \"no\" (the reconciler owns restart, not Docker)", raw.HostConfig.RestartPolicy.Name)
	}
}

// TestClient_ListImages_Live uses nginx:alpine, already pulled onto this
// daemon by the port/env test above (ensureImage is a no-op the second
// time), to prove ListImages actually finds a real local image by
// repository rather than just constructing a well-formed, empty-handed
// request.
func TestClient_ListImages_Live(t *testing.T) {
	c := liveClient(t)
	ctx := context.Background()

	if err := c.ensureImage(ctx, "nginx:alpine"); err != nil {
		t.Fatalf("ensureImage() error = %v", err)
	}

	images, err := c.ListImages(ctx, "nginx")
	if err != nil {
		t.Fatalf("ListImages() error = %v", err)
	}

	found := false
	for _, img := range images {
		if img.Tag == "nginx:alpine" {
			found = true
			if img.CreatedAt.IsZero() {
				t.Error("CreatedAt is zero, want a real timestamp")
			}
		}
	}
	if !found {
		t.Errorf("expected nginx:alpine in ListImages() result, got %+v", images)
	}
}

// TestClient_Events_Live proves the event stream this package's Runtime
// interface promises (and every reconcile controller depends on for its
// event-driven trigger, per internal/reconcile.Engine.Run) actually
// carries real start/die events end to end. This machinery has been in
// production use since Phase 0's nginxdemo controller, but had never
// been directly tested at this layer until now.
func TestClient_Events_Live(t *testing.T) {
	c := liveClient(t)

	name := "levelrail-test-docker-events"
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	removeIfExists(ctx, t, c, name)
	t.Cleanup(func() { removeIfExists(context.Background(), t, c, name) })

	eventCh, errCh := c.Events(ctx)
	go func() {
		for err := range errCh {
			if err != nil {
				t.Logf("event stream error (informational): %v", err)
			}
		}
	}()

	if err := c.ensureImage(ctx, "nginx:alpine"); err != nil {
		t.Fatalf("ensureImage() error = %v", err)
	}
	id, err := c.Create(ctx, ContainerSpec{Name: name, Image: "nginx:alpine"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := c.Start(ctx, id); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if !waitForEvent(t, eventCh, name, EventStart, 10*time.Second) {
		t.Error("did not observe a start event for the container we just started")
	}

	if err := c.cli.ContainerKill(ctx, id, "KILL"); err != nil {
		t.Fatalf("ContainerKill() error = %v", err)
	}

	if !waitForEvent(t, eventCh, name, EventDie, 10*time.Second) {
		t.Error("did not observe a die event after killing the container")
	}
}

// waitForEvent drains eventCh until it sees an event matching name and
// action, or the timeout expires. Other events (from unrelated
// containers on a shared daemon, or events this test doesn't care about)
// are consumed and ignored, not treated as failures.
func waitForEvent(t *testing.T, eventCh <-chan Event, name string, action EventAction, timeout time.Duration) bool {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case ev, ok := <-eventCh:
			if !ok {
				return false
			}
			if ev.ContainerName == name && ev.Action == action {
				return true
			}
		case <-deadline:
			return false
		}
	}
}

func removeIfExists(ctx context.Context, t *testing.T, c *Client, name string) {
	t.Helper()
	state, err := c.InspectByName(ctx, name)
	if err != nil || state == nil {
		return
	}
	_ = c.cli.ContainerRemove(ctx, state.ID, container.RemoveOptions{Force: true})
}

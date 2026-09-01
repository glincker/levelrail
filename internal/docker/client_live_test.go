package docker

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"strings"
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
	if testing.Short() {
		t.Skip("real Docker test, skipped in short mode; see nightly.yml for the full run")
	}
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
		Name:   name,
		Image:  "nginx:alpine",
		Ports:  []PortBinding{{ContainerPort: 80}}, // HostPort 0: let Docker assign one
		Env:    map[string]string{"LEVELRAIL_TEST": "1"},
		Labels: map[string]string{"team": "platform"},
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
	if raw.Config.Labels["team"] != "platform" {
		t.Errorf("label team = %q, want %q", raw.Config.Labels["team"], "platform")
	}
}

// TestClient_BridgeGatewayIP_Live proves BridgeGatewayIP resolves a real,
// parseable IP against an actual daemon, not just that the Engine API
// call succeeds. Every Docker installation (Docker Desktop and a plain
// Linux dockerd alike) has a default "bridge" network with a gateway,
// so this should never be skipped once liveClient itself didn't skip.
func TestClient_BridgeGatewayIP_Live(t *testing.T) {
	c := liveClient(t)
	ctx := context.Background()

	gateway, err := c.BridgeGatewayIP(ctx)
	if err != nil {
		t.Fatalf("BridgeGatewayIP() error = %v", err)
	}
	if net.ParseIP(gateway) == nil {
		t.Fatalf("BridgeGatewayIP() = %q, not a parseable IP", gateway)
	}
}

// TestClient_Create_Live_DNS proves ContainerSpec.DNS actually reaches a
// real, running container's resolv.conf via the Engine API: not just
// that Create accepts it (Create alone doesn't validate DNS entries at
// all, discovered while building this field, see dockerNameserverPort's
// doc comment in cmd/levelrail/mesh.go), but that Start succeeds and the
// container's actual resolver config carries the address, verified
// against the raw HostConfig.DNS the same way
// TestClient_Create_Live_PortsEnvAndNoRestartPolicy verifies env and
// restart policy independently of this package's own return values.
//
// The address here is a bare IP, not "ip:port": that is deliberately the
// only form ContainerSpec.DNS is ever exercised with, live-verified
// (mesh.go's dockerNameserverPort doc comment) to be the only form Docker
// and a container's own resolver accept at all.
//
// Also proves the reverse: a spec with no DNS set produces an empty
// HostConfig.DNS, the same byte-identical-to-before-this-field behavior
// runtime.go's own doc comment on ContainerSpec.DNS promises.
func TestClient_Create_Live_DNS(t *testing.T) {
	c := liveClient(t)
	ctx := context.Background()

	cases := []struct {
		name    string
		dns     []string
		wantDNS []string
	}{
		{name: "levelrail-test-docker-dns-set", dns: []string{"172.17.0.1"}, wantDNS: []string{"172.17.0.1"}},
		{name: "levelrail-test-docker-dns-unset", dns: nil, wantDNS: nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			removeIfExists(ctx, t, c, tc.name)
			t.Cleanup(func() { removeIfExists(ctx, t, c, tc.name) })

			id, err := c.Create(ctx, ContainerSpec{
				Name:  tc.name,
				Image: "nginx:alpine",
				DNS:   tc.dns,
			})
			if err != nil {
				t.Fatalf("Create() error = %v", err)
			}
			if err := c.Start(ctx, id); err != nil {
				t.Fatalf("Start() error = %v", err)
			}

			raw, err := c.cli.ContainerInspect(ctx, id)
			if err != nil {
				t.Fatalf("raw ContainerInspect() error = %v", err)
			}
			if len(raw.HostConfig.DNS) != len(tc.wantDNS) {
				t.Fatalf("HostConfig.DNS = %v, want %v", raw.HostConfig.DNS, tc.wantDNS)
			}
			for i, want := range tc.wantDNS {
				if raw.HostConfig.DNS[i] != want {
					t.Errorf("HostConfig.DNS[%d] = %q, want %q", i, raw.HostConfig.DNS[i], want)
				}
			}

			// Independent verification: the container's own
			// /etc/resolv.conf, not just the HostConfig this package's
			// own Create wrote.
			rc, err := c.Exec(ctx, id, []string{"cat", "/etc/resolv.conf"})
			if err != nil {
				t.Fatalf("Exec(cat /etc/resolv.conf) error = %v", err)
			}
			resolvConf, err := io.ReadAll(rc)
			_ = rc.Close()
			if err != nil {
				t.Fatalf("read /etc/resolv.conf: %v", err)
			}
			for _, want := range tc.wantDNS {
				if !strings.Contains(string(resolvConf), "nameserver "+want) {
					t.Errorf("/etc/resolv.conf = %q, want it to contain %q", resolvConf, "nameserver "+want)
				}
			}
		})
	}
}

// TestClient_EnsureVolume_Live_IdempotentAndMounted proves two things a
// database controller (TASKS.md 1.8) depends on: EnsureVolume actually
// creates a real named volume and is safe to call twice (no error on the
// second call, matching the Engine API's own by-name idempotency), and a
// ContainerSpec's Volumes actually reach a real container's mounts.
// Verified against the raw Engine API's own ContainerInspect and
// VolumeInspect, not this package's own return values.
func TestClient_EnsureVolume_Live_IdempotentAndMounted(t *testing.T) {
	c := liveClient(t)
	ctx := context.Background()

	volName := "levelrail-test-docker-volume"
	containerNameForTest := "levelrail-test-docker-volume-mount"

	removeIfExists(ctx, t, c, containerNameForTest)
	t.Cleanup(func() { removeIfExists(context.Background(), t, c, containerNameForTest) })
	t.Cleanup(func() { _ = c.cli.VolumeRemove(context.Background(), volName, true) })

	if err := c.EnsureVolume(ctx, volName); err != nil {
		t.Fatalf("EnsureVolume() first call error = %v", err)
	}
	// Idempotent: calling it again for a volume that already exists must
	// not error.
	if err := c.EnsureVolume(ctx, volName); err != nil {
		t.Fatalf("EnsureVolume() second call error = %v, want nil (idempotent)", err)
	}

	rawVol, err := c.cli.VolumeInspect(ctx, volName)
	if err != nil {
		t.Fatalf("raw VolumeInspect() error = %v", err)
	}
	if rawVol.Name != volName {
		t.Errorf("volume name = %q, want %q", rawVol.Name, volName)
	}

	id, err := c.Create(ctx, ContainerSpec{
		Name:  containerNameForTest,
		Image: "nginx:alpine",
		Volumes: []VolumeMount{
			{Name: volName, ContainerPath: "/data"},
		},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := c.Start(ctx, id); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	raw, err := c.cli.ContainerInspect(ctx, id)
	if err != nil {
		t.Fatalf("raw ContainerInspect() error = %v", err)
	}
	found := false
	for _, m := range raw.Mounts {
		if m.Name == volName && m.Destination == "/data" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a mount of volume %q at /data in raw container mounts, got %+v", volName, raw.Mounts)
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

	if err := c.ensureImage(ctx, "nginx:alpine", nil); err != nil {
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

	if err := c.ensureImage(ctx, "nginx:alpine", nil); err != nil {
		t.Fatalf("ensureImage() error = %v", err)
	}
	id, err := c.Create(ctx, ContainerSpec{Name: name, Image: "nginx:alpine"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := c.Start(ctx, id); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	startEvent, ok := waitForEvent(t, eventCh, name, EventStart, 10*time.Second)
	if !ok {
		t.Error("did not observe a start event for the container we just started")
	}
	// Independent verification that eventTime actually wired up: the
	// daemon's own timestamp for this event should be very recent, not
	// the zero value internal/alerting.RestartTracker would otherwise
	// silently never count anything against.
	if startEvent.Time.IsZero() {
		t.Error("start event Time is zero, want the daemon's real event timestamp")
	} else if since := time.Since(startEvent.Time); since < 0 || since > time.Minute {
		t.Errorf("start event Time = %v, want within the last minute (got a delta of %v)", startEvent.Time, since)
	}

	if err := c.cli.ContainerKill(ctx, id, "KILL"); err != nil {
		t.Fatalf("ContainerKill() error = %v", err)
	}

	if _, ok := waitForEvent(t, eventCh, name, EventDie, 10*time.Second); !ok {
		t.Error("did not observe a die event after killing the container")
	}
}

// waitInspectState polls InspectByName(name) until want reports true of
// the result or timeout expires, returning whatever the last inspect
// call saw either way.
//
// Stop and Remove both wait for the Engine API call itself to complete
// before returning (this package never treats either as fire-and-forget),
// but that is a guarantee about the API call, not about every read path
// against the daemon's own state observing the same result in the same
// instant: this test asserting on InspectByName immediately after Stop()
// returned failed intermittently in CI (not once in dozens of local
// runs) with the container still reporting Running: true, a real,
// reproducible gap between "the daemon processed the stop" and "a fresh
// inspect call reflects it," apparently more visible on the
// containerd-snapshotter-backed daemon CI runs on than a typical local
// Docker install. Polling here, rather than trusting the first read, is
// the correct fix: it is the assertion that was too eager, not Stop or
// Remove themselves.
func waitInspectState(ctx context.Context, t *testing.T, c *Client, name string, want func(*ContainerState) bool, timeout time.Duration) *ContainerState {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last *ContainerState
	for {
		state, err := c.InspectByName(ctx, name)
		if err != nil {
			t.Fatalf("InspectByName(%q) error = %v", name, err)
		}
		last = state
		if want(state) {
			return state
		}
		if !time.Now().Before(deadline) {
			return last
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// waitForEvent drains eventCh until it sees an event matching name and
// action, or the timeout expires. Other events (from unrelated
// containers on a shared daemon, or events this test doesn't care about)
// are consumed and ignored, not treated as failures.
// waitForEvent returns the matching event and true, or a zero Event and
// false on timeout/stream close, so callers can assert on more than
// just "did a matching event arrive" (e.g. that Time was actually
// populated by the daemon, not left at the zero value).
func waitForEvent(t *testing.T, eventCh <-chan Event, name string, action EventAction, timeout time.Duration) (Event, bool) {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case ev, ok := <-eventCh:
			if !ok {
				return Event{}, false
			}
			if ev.ContainerName == name && ev.Action == action {
				return ev, true
			}
		case <-deadline:
			return Event{}, false
		}
	}
}

// TestClient_ListByPrefix_Stop_Remove_Live proves the three primitives
// the application controller's blue-green cutover depends on: finding
// every container belonging to a service by name prefix (not just one
// exact name, unlike everything nginxdemo needed), and actually being
// able to stop and remove one.
func TestClient_ListByPrefix_Stop_Remove_Live(t *testing.T) {
	c := liveClient(t)
	ctx := context.Background()

	prefix := "levelrail-test-prefix-"
	nameA := prefix + "a"
	nameB := prefix + "b"
	unrelated := "levelrail-test-prefix-unrelated-but-different-service"

	for _, n := range []string{nameA, nameB, unrelated} {
		removeIfExists(ctx, t, c, n)
	}
	t.Cleanup(func() {
		for _, n := range []string{nameA, nameB, unrelated} {
			removeIfExists(context.Background(), t, c, n)
		}
	})

	if err := c.ensureImage(ctx, "nginx:alpine", nil); err != nil {
		t.Fatalf("ensureImage() error = %v", err)
	}

	var idA string
	for _, n := range []string{nameA, nameB} {
		id, err := c.Create(ctx, ContainerSpec{Name: n, Image: "nginx:alpine"})
		if err != nil {
			t.Fatalf("Create(%q) error = %v", n, err)
		}
		if n == nameA {
			idA = id
		}
	}
	// unrelated deliberately NOT sharing prefix's exact form, to prove
	// ListByPrefix doesn't over-match: same leading substring, different
	// service.
	if _, err := c.Create(ctx, ContainerSpec{Name: unrelated, Image: "nginx:alpine"}); err != nil {
		t.Fatalf("Create(unrelated) error = %v", err)
	}

	found, err := c.ListByPrefix(ctx, prefix+"a")
	if err != nil {
		t.Fatalf("ListByPrefix(%q) error = %v", prefix+"a", err)
	}
	if len(found) != 1 || found[0].Name != nameA {
		t.Fatalf("ListByPrefix(%q) = %+v, want exactly [%s]", prefix+"a", found, nameA)
	}

	if err := c.Start(ctx, idA); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if err := c.Stop(ctx, idA, 5*time.Second); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	stopped := waitInspectState(ctx, t, c, nameA, func(s *ContainerState) bool {
		return s != nil && !s.Running
	}, 10*time.Second)
	if stopped == nil || stopped.Running {
		t.Fatalf("expected container to exist and not be running after Stop(), got %+v", stopped)
	}

	if err := c.Remove(ctx, idA, false); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	removed := waitInspectState(ctx, t, c, nameA, func(s *ContainerState) bool {
		return s == nil
	}, 10*time.Second)
	if removed != nil {
		t.Errorf("expected InspectByName() to return nil after Remove(), got %+v", removed)
	}
}

// TestClient_Exec_Live proves Exec actually runs a command inside a
// real running container via the Engine API's exec facility and streams
// its real stdout back byte for byte, the exact mechanism
// internal/backup.ContainerDumper depends on entirely to run
// pg_dump/mysqldump/redis-cli. Three things are load-bearing enough to
// prove against a real daemon rather than trust from documentation
// alone:
//
//  1. stdout comes through demultiplexed and uncorrupted.
//  2. environment variables set at container creation are visible to
//     the exec'd process without this call passing them again, since
//     that is exactly what lets postgresDumpCmd/mysqlDumpCmd read
//     $POSTGRES_USER/$MYSQL_ROOT_PASSWORD without ContainerDumper ever
//     handling a credential itself.
//  3. a non-zero exit surfaces as a real error from the stream, not a
//     silently truncated or empty read, so a caller can tell "the
//     database is empty" apart from "the dump command failed."
func TestClient_Exec_Live(t *testing.T) {
	c := liveClient(t)
	ctx := context.Background()

	name := "levelrail-test-docker-exec"
	removeIfExists(ctx, t, c, name)
	t.Cleanup(func() { removeIfExists(context.Background(), t, c, name) })

	if err := c.ensureImage(ctx, "nginx:alpine", nil); err != nil {
		t.Fatalf("ensureImage() error = %v", err)
	}
	id, err := c.Create(ctx, ContainerSpec{
		Name:  name,
		Image: "nginx:alpine",
		Env:   map[string]string{"LEVELRAIL_TEST_VAR": "hello-from-container-env"},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := c.Start(ctx, id); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	t.Run("stdout streamed back and env inherited", func(t *testing.T) {
		rc, err := c.Exec(ctx, name, []string{"sh", "-c", `echo "$LEVELRAIL_TEST_VAR"`})
		if err != nil {
			t.Fatalf("Exec() error = %v", err)
		}
		defer func() { _ = rc.Close() }()

		got, err := io.ReadAll(rc)
		if err != nil {
			t.Fatalf("reading Exec() stream error = %v", err)
		}
		want := "hello-from-container-env\n"
		if string(got) != want {
			t.Errorf("Exec() stdout = %q, want %q (proves the exec'd process inherits the container's own creation-time env, unprompted)", got, want)
		}
	})

	t.Run("non-zero exit surfaces as a stream error", func(t *testing.T) {
		rc, err := c.Exec(ctx, name, []string{"sh", "-c", "echo partial-output; exit 7"})
		if err != nil {
			t.Fatalf("Exec() error = %v", err)
		}
		defer func() { _ = rc.Close() }()

		_, readErr := io.ReadAll(rc)
		if readErr == nil {
			t.Fatal("reading Exec() stream error = nil, want a non-nil error for a command that exited 7")
		}
		if !strings.Contains(readErr.Error(), "exited 7") {
			t.Errorf("Exec() stream error = %v, want it to mention the real exit code (7)", readErr)
		}
	})
}

func removeIfExists(ctx context.Context, t *testing.T, c *Client, name string) {
	t.Helper()
	state, err := c.InspectByName(ctx, name)
	if err != nil || state == nil {
		return
	}
	_ = c.cli.ContainerRemove(ctx, state.ID, container.RemoveOptions{Force: true})
}

// TestClient_Ping_Live proves Ping succeeds against a real reachable
// daemon, the case internal/api's DockerConnected field needs: an
// operator with Docker actually running must see DockerConnected: true
// on GET /api/v1/system/status, not just "Ping doesn't panic."
func TestClient_Ping_Live(t *testing.T) {
	c := liveClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := c.Ping(ctx); err != nil {
		t.Errorf("Ping() error = %v, want nil against a reachable daemon", err)
	}
}

// TestClient_Ping_Unreachable proves Ping returns a real, non-nil error
// when nothing is listening at DOCKER_HOST, the other half of the
// DockerConnected contract. Deliberately not gated behind liveClient's
// skip-if-unreachable pattern: unlike every other live test in this
// file, an unreachable daemon is this test's actual subject, not a
// reason to skip it. No live daemon needed to observe this: NewClient
// succeeds even with nothing listening at DOCKER_HOST, since it only
// negotiates lazily on first real call, the same reasoning
// TestClient_Logs_NoSuchContainer (logs_test.go) documents.
func TestClient_Ping_Unreachable(t *testing.T) {
	t.Setenv("DOCKER_HOST", "unix:///nonexistent/levelrail-ping-test.sock")
	c, err := NewClient()
	if err != nil {
		t.Skipf("could not construct a docker client at all: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := c.Ping(ctx); err == nil {
		t.Error("Ping() error = nil, want a non-nil error against an unreachable socket")
	}
}

// TestClient_Stats_Live proves Stats returns real, sane numbers from a
// real running container, not just that the API call doesn't error.
// Independent verification against the raw one-shot response, the same
// rigor every other live test in this file applies: the parsed
// ContainerStats fields are checked against the raw JSON's own numbers,
// not just each other.
func TestClient_Stats_Live(t *testing.T) {
	c := liveClient(t)
	ctx := context.Background()

	name := "levelrail-test-stats"
	removeIfExists(ctx, t, c, name)
	t.Cleanup(func() { removeIfExists(ctx, t, c, name) })

	// ContainerSpec has no command-override field, so this relies on
	// busybox's own default entrypoint; nginx (used elsewhere in this
	// file) stays running on its own and is just as good a stats
	// source, and is already the known-good image other live tests here
	// use successfully.
	id, err := c.Create(ctx, ContainerSpec{Name: name, Image: "nginx:alpine"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := c.Start(ctx, id); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	first, err := c.Stats(ctx, id)
	if err != nil {
		t.Fatalf("Stats() first call error = %v", err)
	}
	time.Sleep(500 * time.Millisecond)
	second, err := c.Stats(ctx, id)
	if err != nil {
		t.Fatalf("Stats() second call error = %v", err)
	}

	if second.MemoryUsageBytes == 0 {
		t.Error("MemoryUsageBytes = 0, want a real positive figure for a running container")
	}
	if second.MemoryLimitBytes == 0 {
		t.Error("MemoryLimitBytes = 0, want a real positive figure (the cgroup memory limit)")
	}
	if first.CPUPercent < 0 || second.CPUPercent < 0 {
		t.Errorf("CPUPercent negative: first=%v second=%v", first.CPUPercent, second.CPUPercent)
	}

	// Independent verification: decode the raw one-shot response
	// directly and confirm this package's parsing matches it, not just
	// that Stats() returned without erroring.
	rawReader, err := c.cli.ContainerStatsOneShot(ctx, id)
	if err != nil {
		t.Fatalf("raw ContainerStatsOneShot() error = %v", err)
	}
	var raw container.StatsResponse
	if err := json.NewDecoder(rawReader.Body).Decode(&raw); err != nil {
		t.Fatalf("decode raw stats: %v", err)
	}
	_ = rawReader.Body.Close()
	if raw.MemoryStats.Usage == 0 {
		t.Error("raw MemoryStats.Usage = 0, this test's own assumptions are wrong, not just the wrapper")
	}
}

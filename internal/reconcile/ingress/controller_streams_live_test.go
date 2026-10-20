package ingress

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	dockerclient "github.com/docker/docker/client"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/dockertest"
	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

// TestController_Reconcile_AppStream_Live is the real end-to-end proof
// for streams, the same rigor TestController_Reconcile_Live already
// establishes for HTTP routes: a real Docker container, a real
// in-process Caddy layer4 listener, and a real TCP client speaking
// real Redis protocol through it. Also the regression test for
// streamPortBindings's port-collision bug (see its own doc comment).
func TestController_Reconcile_AppStream_Live(t *testing.T) {
	dockertest.SkipIfShort(t)
	rt, err := docker.NewClient()
	if err != nil {
		t.Skipf("no docker client available: %v", err)
	}
	rawCli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		t.Skipf("no docker client available: %v", err)
	}
	pingCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	if _, err := rawCli.Ping(pingCtx); err != nil {
		cancel()
		t.Skipf("docker daemon not reachable: %v", err)
	}
	cancel()
	t.Cleanup(func() {
		if err := rt.Close(); err != nil {
			t.Errorf("closing docker client: %v", err)
		}
	})

	const serviceName = "levelrail-test-ingress-stream"
	redisImage := "redis:7-alpine"

	longCtx := context.Background()
	if err := pullIfMissing(longCtx, t, rawCli, redisImage); err != nil {
		t.Fatalf("pull %s: %v", redisImage, err)
	}

	cleanupContainers(longCtx, t, rt, serviceName)
	t.Cleanup(func() { cleanupContainers(context.Background(), t, rt, serviceName) })

	db := openLiveStore(t)

	streamHostPort := freePort(t)
	// Port: 0, deliberately: this service has no HTTP/reverse-proxy port
	// of its own (the realistic shape for a database-only backend like
	// Postgres or Redis), only the stream below. A non-zero main Port
	// equal to the stream's own ContainerPort would ask Docker to
	// publish the same container port twice, an unrelated conflict this
	// test isn't about.
	desired := store.DesiredService{Name: serviceName, Image: redisImage}
	if err := db.SaveDesiredService(longCtx, desired); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}
	if err := db.SaveAppStream(longCtx, store.AppStream{
		ID: "stream_live_test", ServiceName: serviceName,
		ContainerPort: 6379, HostPort: streamHostPort, Protocol: store.AppStreamProtocolTCP,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("SaveAppStream() error = %v", err)
	}

	appCtrl := application.New(serviceName, db, rt)
	if _, err := appCtrl.Reconcile(longCtx); err != nil {
		t.Fatalf("application Reconcile() error = %v", err)
	}

	// Confirm the real container published container port 6379 to some
	// Docker-assigned ephemeral host port, never to streamHostPort
	// itself: see streamPortBindings's own doc comment for why
	// publishing straight to streamHostPort would make Docker and
	// Caddy's own layer4 listener below race for the same port.
	target := application.ContainerName(serviceName, redisImage, "")
	state, err := rt.InspectByName(longCtx, target)
	if err != nil {
		t.Fatalf("InspectByName(%q) error = %v", target, err)
	}
	var ephemeralPort int
	for _, p := range state.Ports {
		if p.ContainerPort == 6379 {
			ephemeralPort = p.HostPort
		}
	}
	if ephemeralPort == 0 {
		t.Fatalf("container did not publish a host port for container port 6379: %+v", state.Ports)
	}
	if ephemeralPort == streamHostPort {
		t.Fatalf("container published container port 6379 directly to the stream's own HostPort %d; want a Docker-assigned ephemeral port instead", streamHostPort)
	}

	// redis-server takes a moment after the container reports Running to
	// actually respond on its port; wait on the real backend directly
	// (not the stream) so a timing issue here can't be confused with a
	// Caddy layer4 problem below.
	waitForPong(t, fmt.Sprintf("127.0.0.1:%d", ephemeralPort))

	driver := ingress.New(discardLogger())
	t.Cleanup(func() {
		if err := driver.Stop(context.Background()); err != nil {
			t.Errorf("Driver.Stop() error = %v", err)
		}
	})
	ingressCtrl := New(db, rt, driver,
		WithServerName("ingress-stream-live"),
		WithListenAddr(fmt.Sprintf("127.0.0.1:%d", freePort(t))),
		WithAdminListen(fmt.Sprintf("127.0.0.1:%d", freePort(t))),
		WithLogger(discardLogger()),
	)

	reconcileCtx, reconcileCancel := context.WithTimeout(longCtx, 30*time.Second)
	defer reconcileCancel()
	if _, err := ingressCtrl.Reconcile(reconcileCtx); err != nil {
		t.Fatalf("ingress Reconcile() error = %v", err)
	}

	// The real proof: dial the stream's own host port (Caddy's own
	// listener, not the container's ephemeral one) and speak real
	// Redis wire protocol through it, proving actual byte-for-byte TCP
	// passthrough to the real container, not just that some response
	// came back. waitForPong's own retry also absorbs Caddy's listener
	// taking a moment to come up after Apply returns.
	waitForPong(t, fmt.Sprintf("127.0.0.1:%d", streamHostPort))
}

// pingOnce dials addr, sends a raw Redis PING, and returns the raw
// response line (or an error: connection refused/reset, a read
// timeout, or an immediate EOF, each a distinct real failure mode this
// test wants to tell apart from a wrong response body).
func pingOnce(addr string) (string, error) {
	conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
	if err != nil {
		return "", fmt.Errorf("dial: %w", err)
	}
	defer func() { _ = conn.Close() }()
	if _, err := conn.Write([]byte("PING\r\n")); err != nil {
		return "", fmt.Errorf("write: %w", err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		return "", fmt.Errorf("set read deadline: %w", err)
	}
	buf := make([]byte, 64)
	n, err := conn.Read(buf)
	if err != nil {
		return "", fmt.Errorf("read: %w", err)
	}
	return string(buf[:n]), nil
}

// waitForPong retries pingOnce against addr until it returns the
// expected Redis PONG, up to 10s: redis-server takes a moment after
// the container reports Running to actually respond on its port.
func waitForPong(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	var lastErr error
	var last string
	for time.Now().Before(deadline) {
		got, err := pingOnce(addr)
		if err == nil && got == "+PONG\r\n" {
			return
		}
		lastErr, last = err, got
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("waitForPong(%s): never got PONG, last error = %v, last response = %q", addr, lastErr, last)
}

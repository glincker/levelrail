package agent

// TestLive_EnrollAndSession is TASKS.md 3.2's real end-to-end proof:
// not mux/GRPCTransport/Execute/Server tested in isolation against
// fakes (every other test file in this package), but a real join token
// minted through a real store, a real self-signed CA, a real TLS
// listener on a real TCP port, a real gRPC server running Server's
// Enroll and Session handlers, a real agent.DialEnroll exchange, a real
// agent.RunSession holding a real mTLS connection open, and a real
// Docker container inspected through the entire stack: control plane
// caller -> mux -> gRPC -> mTLS -> gRPC -> agent recv loop -> Execute ->
// a real docker.Runtime -> the real Docker daemon, and the response
// travels the same path back. If this test passes, ADR 003's "the
// reconciler... never knows whether it's talking to a local in-process
// agent or a remote one over mTLS" is provably true for a remote one,
// not just designed to be.

import (
	"context"
	"io"
	"net"
	"testing"
	"time"

	"github.com/docker/docker/api/types/image"
	dockerclient "github.com/docker/docker/client"
	"google.golang.org/grpc"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestLive_EnrollAndSession(t *testing.T) {
	rt, err := docker.NewClient()
	if err != nil {
		t.Skipf("no docker client available: %v", err)
	}
	rawCli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		t.Skipf("no docker client available: %v", err)
	}
	pingCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	_, err = rawCli.Ping(pingCtx)
	cancel()
	if err != nil {
		t.Skipf("docker daemon not reachable: %v", err)
	}
	t.Cleanup(func() { _ = rt.Close() })

	const image1 = "nginx:alpine"
	const containerName = "levelrail-test-agent-live"
	longCtx := context.Background()
	if err := pullIfMissing(longCtx, t, rawCli, image1); err != nil {
		t.Fatalf("pull %s: %v", image1, err)
	}
	cleanupContainer(longCtx, t, rt, containerName)
	t.Cleanup(func() { cleanupContainer(context.Background(), t, rt, containerName) })

	id, err := rt.Create(longCtx, docker.ContainerSpec{Name: containerName, Image: image1})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := rt.Start(longCtx, id); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// --- Control plane side: real store, real CA, real gRPC server on
	// a real TCP listener. ---
	db := openLiveTestStore(t)
	ca, err := GenerateCA()
	if err != nil {
		t.Fatalf("GenerateCA() error = %v", err)
	}
	registry := NewRegistry()

	listener, grpcServer := startTestAgentServer(t, ca, db, registry)
	addr := listener.Addr().String()

	plaintext := "live-test-join-token"
	now := time.Now()
	if err := db.SaveNodeJoinToken(longCtx, store.NodeJoinToken{
		ID: "njt_live", TokenHash: hashJoinToken(plaintext), CreatedAt: now, ExpiresAt: now.Add(time.Hour),
	}); err != nil {
		t.Fatalf("SaveNodeJoinToken() error = %v", err)
	}

	// --- Agent side: real enrollment, real persistent session, real
	// docker.Runtime backing it. ---
	enrollCtx, enrollCancel := context.WithTimeout(longCtx, 10*time.Second)
	identity, err := DialEnroll(enrollCtx, addr, plaintext, "live-test-node")
	enrollCancel()
	if err != nil {
		t.Fatalf("DialEnroll() error = %v", err)
	}
	if identity.NodeID == "" {
		t.Fatal("DialEnroll() returned an empty NodeID")
	}

	sessionCtx, sessionCancel := context.WithCancel(longCtx)
	defer sessionCancel()
	sessionDone := make(chan error, 1)
	go func() { sessionDone <- RunSession(sessionCtx, addr, identity, rt, nil) }()

	transport := waitForTransport(t, registry, identity.NodeID)

	// The actual proof: a real container's state, round-tripped through
	// the entire mTLS/gRPC/multiplexer stack, not the local docker.Runtime
	// directly.
	callCtx, callCancel := context.WithTimeout(longCtx, 10*time.Second)
	state, err := transport.InspectByName(callCtx, containerName)
	callCancel()
	if err != nil {
		t.Fatalf("transport.InspectByName() error = %v", err)
	}
	if state == nil || !state.Running || state.Name != containerName {
		t.Fatalf("state = %+v, want a running container named %q", state, containerName)
	}
	if state.Image != image1 {
		t.Errorf("state.Image = %q, want %q", state.Image, image1)
	}

	// ListByPrefix, a second real remote call over the same connection,
	// proving the multiplexer actually multiplexes rather than only
	// ever having proven a single request/response pair.
	listCtx, listCancel := context.WithTimeout(longCtx, 10*time.Second)
	containers, err := transport.ListByPrefix(listCtx, containerName)
	listCancel()
	if err != nil {
		t.Fatalf("transport.ListByPrefix() error = %v", err)
	}
	if len(containers) != 1 || containers[0].ID != state.ID {
		t.Errorf("ListByPrefix() = %+v, want exactly the one container InspectByName already found", containers)
	}

	// Events: a real Docker event (stopping the container), relayed
	// from the agent's own local event stream, through the same
	// physical connection, back to this test as a real docker.Event,
	// not a request/response pair at all.
	eventsCtx, eventsCancel := context.WithTimeout(longCtx, 10*time.Second)
	defer eventsCancel()
	events, errs := transport.Events(eventsCtx)

	if err := rt.Stop(longCtx, id, 3*time.Second); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	sawDie := false
	for !sawDie {
		select {
		case ev, ok := <-events:
			if !ok {
				t.Fatal("events channel closed before observing the expected event")
			}
			if ev.ContainerName == containerName && (ev.Action == docker.EventDie || ev.Action == docker.EventStop) {
				sawDie = true
			}
		case err := <-errs:
			t.Fatalf("unexpected error on events stream: %v", err)
		case <-time.After(10 * time.Second):
			t.Fatal("timed out waiting for the container's stop/die event to arrive over the remote transport")
		}
	}

	// Cleanly end the session and confirm the server side notices.
	sessionCancel()
	select {
	case <-sessionDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for RunSession to return after cancellation")
	}
	grpcServer.GracefulStop()
}

func startTestAgentServer(t *testing.T, ca *CA, st EnrollStore, registry *Registry) (net.Listener, *grpc.Server) {
	t.Helper()

	creds, err := NewServerCredentials(ca, []string{"127.0.0.1"}, time.Hour)
	if err != nil {
		t.Fatalf("NewServerCredentials() error = %v", err)
	}
	// A plain TCP listener, not a raw tls.Listener: grpc.Creds below is
	// what performs the TLS handshake and populates the peer info
	// Server.Session's peerIdentity depends on, credentials.go's own doc
	// comment explains why a raw tls.Listener handed to Serve doesn't.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}

	grpcServer := grpc.NewServer(grpc.Creds(creds))
	agentpb.RegisterAgentServiceServer(grpcServer, NewServer(ca, st, registry, nil))
	go func() { _ = grpcServer.Serve(listener) }()
	t.Cleanup(grpcServer.Stop)

	return listener, grpcServer
}

func waitForTransport(t *testing.T, registry *Registry, nodeID string) Transport {
	t.Helper()
	deadline := time.After(10 * time.Second)
	for {
		if tr, err := registry.Get(nodeID); err == nil {
			return tr
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for the agent's transport to register")
			return nil
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func openLiveTestStore(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(context.Background(), t.TempDir()+"/levelrail.db")
	if err != nil {
		t.Fatalf("store.Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("closing test store: %v", err)
		}
	})
	return db
}

func pullIfMissing(ctx context.Context, t *testing.T, cli *dockerclient.Client, ref string) error {
	t.Helper()
	_, _, err := cli.ImageInspectWithRaw(ctx, ref)
	if err == nil {
		return nil
	}
	rc, err := cli.ImagePull(ctx, ref, image.PullOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = rc.Close() }()
	_, err = io.Copy(io.Discard, rc)
	return err
}

func cleanupContainer(ctx context.Context, t *testing.T, rt docker.Runtime, name string) {
	t.Helper()
	found, err := rt.ListByPrefix(ctx, name)
	if err != nil {
		return
	}
	for _, cs := range found {
		_ = rt.Stop(ctx, cs.ID, 3*time.Second)
		_ = rt.Remove(ctx, cs.ID, true)
	}
}

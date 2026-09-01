package application

// TestController_Reconcile_Live_ViaRemoteTransport is TASKS.md 3.3's
// real end-to-end proof, composing 3.1's own claim with 3.2's own live
// test rather than re-trusting it: this Controller, completely
// unmodified, given a real internal/agent.GRPCTransport (a "remote"
// node reached over real mTLS/gRPC, using this same machine's own
// Docker daemon underneath since there's only one available here)
// instead of the local docker.Runtime controller_live_test.go already
// proves it against, converges a real container exactly the same way.
// If this passes, ADR 003's "the reconciler... never knows whether it's
// talking to a local in-process agent or a remote one" is proven at the
// one layer that actually matters: the application controller itself,
// not just Transport's own isolated method calls.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"testing"
	"time"

	dockerclient "github.com/docker/docker/client"
	"google.golang.org/grpc"

	"github.com/GLINCKER/levelrail/internal/agent"
	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// hashJoinTokenForTest mirrors internal/agent's own unexported
// hashJoinToken (SHA-256 hex): duplicated rather than exported from
// that package purely for this one test, the same "small, purpose-
// specific duplication over a speculative export" call this codebase
// already makes elsewhere (e.g. internal/api's randomNodeJoinTokenID
// doc comment).
func hashJoinTokenForTest(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}

func TestController_Reconcile_Live_ViaRemoteTransport(t *testing.T) {
	if testing.Short() {
		t.Skip("real Docker test, skipped in short mode; see nightly.yml for the full run")
	}
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

	const serviceName = "levelrail-test-app-controller-remote"
	image1 := "nginx:alpine"
	if err := pullIfMissing(context.Background(), t, rawCli); err != nil {
		t.Fatalf("pull %s: %v", image1, err)
	}
	cleanupContainers(context.Background(), t, rt, serviceName)
	t.Cleanup(func() { cleanupContainers(context.Background(), t, rt, serviceName) })

	// --- Stand up a real agent connection, the same shape
	// internal/agent/live_test.go's own TestLive_EnrollAndSession
	// already proves works, reused here as the placement target rather
	// than re-verified in isolation. ---
	nodeStore, err := store.Open(context.Background(), t.TempDir()+"/levelrail.db")
	if err != nil {
		t.Fatalf("store.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = nodeStore.Close() })

	ca, err := agent.GenerateCA()
	if err != nil {
		t.Fatalf("GenerateCA() error = %v", err)
	}
	registry := agent.NewRegistry()

	creds, err := agent.NewServerCredentials(ca, []string{"127.0.0.1"}, time.Hour)
	if err != nil {
		t.Fatalf("NewServerCredentials() error = %v", err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}
	grpcServer := grpc.NewServer(grpc.Creds(creds))
	agentpb.RegisterAgentServiceServer(grpcServer, agent.NewServer(ca, nodeStore, registry, nil))
	go func() { _ = grpcServer.Serve(listener) }()
	t.Cleanup(grpcServer.Stop)
	addr := listener.Addr().String()

	plaintext := "placement-live-test-token"
	now := time.Now()
	if err := nodeStore.SaveNodeJoinToken(context.Background(), store.NodeJoinToken{
		ID: "njt_placement_live", TokenHash: hashJoinTokenForTest(plaintext), CreatedAt: now, ExpiresAt: now.Add(time.Hour),
	}); err != nil {
		t.Fatalf("SaveNodeJoinToken() error = %v", err)
	}

	enrollCtx, enrollCancel := context.WithTimeout(context.Background(), 10*time.Second)
	identity, err := agent.DialEnroll(enrollCtx, addr, plaintext, "placement-live-test-node")
	enrollCancel()
	if err != nil {
		t.Fatalf("DialEnroll() error = %v", err)
	}

	sessionCtx, sessionCancel := context.WithCancel(context.Background())
	sessionDone := make(chan struct{})
	go func() {
		defer close(sessionDone)
		_ = agent.RunSession(sessionCtx, addr, identity, rt, nil)
	}()
	// Cancel and wait for the client side to finish shutting down before
	// nodeStore.Close() runs (t.Cleanup is LIFO, so this must be
	// registered after nodeStore's own cleanup above); the extra grace
	// delay covers the server's own Session handler goroutine finishing
	// its update-node-offline cleanup shortly after the client-visible
	// stream closes, an inherently async step on the other side of this
	// same connection with nothing this test can block on directly.
	t.Cleanup(func() {
		sessionCancel()
		<-sessionDone
		time.Sleep(100 * time.Millisecond)
	})

	deadline := time.After(10 * time.Second)
	var transport agent.Transport
	for {
		if transport, err = registry.Get(identity.NodeID); err == nil {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for the agent transport to register")
		case <-time.After(20 * time.Millisecond):
		}
	}

	// --- The actual proof: an application.Controller, otherwise
	// identical to controller_live_test.go's own, given transport
	// instead of the local rt. ---
	svcStore := openLiveStore(t)
	desired := store.DesiredService{
		Name: serviceName, Image: image1, Port: 80,
		Health: &store.ServiceHealth{
			Readiness: &store.ServiceProbe{Path: "/", Interval: 200 * time.Millisecond, Timeout: 1 * time.Second},
		},
	}
	if err := svcStore.SaveDesiredService(context.Background(), desired); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}

	ctrl := New(serviceName, svcStore, transport, WithReadyBudget(20*time.Second))

	reconcileCtx, reconcileCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer reconcileCancel()
	result, err := ctrl.Reconcile(reconcileCtx)
	if err != nil {
		t.Fatalf("Reconcile() via remote transport error = %v, result = %+v", err, result)
	}
	if cond := conditionOf(t, result); cond.Status != "True" || cond.Reason != "Deployed" {
		t.Fatalf("Reconcile() condition = %+v, want Status=True Reason=Deployed", cond)
	}

	// Independent verification: inspect via the LOCAL docker.Runtime,
	// not the transport this controller itself used, confirming the
	// container that actually exists in Docker, not just what the
	// remote path claims exists.
	target := ContainerName(serviceName, image1, "")
	state, err := rt.InspectByName(context.Background(), target)
	if err != nil {
		t.Fatalf("InspectByName(%q) error = %v", target, err)
	}
	if state == nil || !state.Running {
		t.Fatalf("expected %q running after Reconcile via remote transport, got %+v", target, state)
	}
}

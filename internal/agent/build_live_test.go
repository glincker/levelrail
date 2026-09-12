package agent

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/docker/docker/api/types/image"
	dockerclient "github.com/docker/docker/client"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// TestLive_RemoteBuildDispatch is the real end-to-end proof for a
// dispatched build, the build counterpart to TestLive_EnrollAndSession:
// a real join token, a real mTLS session, a real BuildKit solve run
// through the agent's own connection, and a real image landing in the
// dispatching side's image store after travelling back over the wire.
// Nothing here is faked except the fact that both nodes happen to be this
// one machine; every frame still crosses a real gRPC connection.
func TestLive_RemoteBuildDispatch(t *testing.T) {
	rt, err := docker.NewClient()
	if err != nil {
		t.Skipf("no docker client available: %v", err)
	}
	t.Cleanup(func() { _ = rt.Close() })

	rawCli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		t.Skipf("no docker client available: %v", err)
	}
	t.Cleanup(func() { _ = rawCli.Close() })

	pingCtx, cancelPing := context.WithTimeout(context.Background(), 3*time.Second)
	_, err = rawCli.Ping(pingCtx)
	cancelPing()
	if err != nil {
		t.Skipf("docker daemon not reachable: %v", err)
	}

	connectCtx, cancelConnect := context.WithTimeout(context.Background(), 10*time.Second)
	buildClient, err := build.NewClient(connectCtx, rawCli)
	cancelConnect()
	if err != nil {
		t.Skipf("could not connect to buildkit via docker: %v", err)
	}
	t.Cleanup(func() { _ = buildClient.Close() })

	// --- Control plane side. ---
	db := openLiveTestStore(t)
	ca, err := GenerateCA()
	if err != nil {
		t.Fatalf("GenerateCA() error = %v", err)
	}
	registry := NewRegistry()
	listener, grpcServer := startTestAgentServer(t, ca, db, registry)
	t.Cleanup(grpcServer.GracefulStop)
	addr := listener.Addr().String()

	plaintext := "live-build-join-token"
	now := time.Now()
	if err := db.SaveNodeJoinToken(context.Background(), store.NodeJoinToken{
		ID: "njt_live_build", TokenHash: hashJoinToken(plaintext), CreatedAt: now, ExpiresAt: now.Add(time.Hour),
	}); err != nil {
		t.Fatalf("SaveNodeJoinToken() error = %v", err)
	}

	// --- Build node side: the same agent a real remote node runs, with
	// its own BuildKit connection wired in. ---
	enrollCtx, cancelEnroll := context.WithTimeout(context.Background(), 10*time.Second)
	identity, err := DialEnroll(enrollCtx, addr, plaintext, "live-build-node")
	cancelEnroll()
	if err != nil {
		t.Fatalf("DialEnroll() error = %v", err)
	}

	sessionCtx, cancelSession := context.WithCancel(context.Background())
	defer cancelSession()
	sessionDone := make(chan error, 1)
	go func() {
		sessionDone <- RunSession(sessionCtx, addr, identity, rt, nil, WithBuildRunner(buildClient))
	}()
	waitForTransport(t, registry, identity.NodeID)

	// --- The actual dispatch: a Router that sees exactly one node, and
	// that node is build-capable and online. ---
	nodes := func(context.Context) ([]build.NodeInfo, error) {
		return []build.NodeInfo{{ID: identity.NodeID, AcceptsBuildWorkloads: true, Online: true}}, nil
	}
	router := build.NewRouter(buildClient, nodes, NewBuildDispatcher(registry))

	contextDir := t.TempDir()
	// FROM scratch: no base image to pull, so this test needs no network
	// and no pre-seeded image, only a working BuildKit.
	writeBuildFixture(t, filepath.Join(contextDir, "Dockerfile"), "FROM scratch\nCOPY hello.txt /hello.txt\n")
	writeBuildFixture(t, filepath.Join(contextDir, "hello.txt"), "hello from the remote build node\n")

	const tag = "levelrail-remote-build:test"
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, _ = rawCli.ImageRemove(cleanupCtx, tag, image.RemoveOptions{Force: true})
	})

	buildCtx, cancelBuild := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancelBuild()

	progress := &collectBuildProgress{}
	res, err := router.Build(buildCtx, build.Request{ContextDir: contextDir, Tag: tag}, progress.fn())
	if err != nil {
		t.Fatalf("router.Build() dispatched to node %s: error = %v", identity.NodeID, err)
	}
	if res.Tag != tag {
		t.Errorf("Result.Tag = %q, want %q", res.Tag, tag)
	}
	if res.Duration <= 0 {
		t.Errorf("Result.Duration = %v, want the remote build's own measured duration", res.Duration)
	}
	if len(progress.all()) == 0 {
		t.Error("no progress events reached the control plane: a dispatched build's logs must stream back like a local one's")
	}

	// The load-bearing assertion: the image the remote node built really
	// exists in this side's image store, asked of the Engine API directly
	// rather than through any of the code under test.
	inspect, err := rawCli.ImageInspect(buildCtx, tag)
	if err != nil {
		t.Fatalf("image %q not found after a dispatched build: %v", tag, err)
	}
	if inspect.ID == "" {
		t.Error("inspected image has an empty ID")
	}

	cancelSession()
	select {
	case <-sessionDone:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for RunSession to return after cancellation")
	}
}

// TestLive_RemoteBuildDispatch_NodeOfflineMidBuild covers the failure an
// operator hits when a build node drops off: the build has to fail with
// the session error, over a real connection, not hang until the caller's
// own timeout.
func TestLive_RemoteBuildDispatch_NodeOfflineMidBuild(t *testing.T) {
	rt, err := docker.NewClient()
	if err != nil {
		t.Skipf("no docker client available: %v", err)
	}
	t.Cleanup(func() { _ = rt.Close() })

	db := openLiveTestStore(t)
	ca, err := GenerateCA()
	if err != nil {
		t.Fatalf("GenerateCA() error = %v", err)
	}
	registry := NewRegistry()
	listener, grpcServer := startTestAgentServer(t, ca, db, registry)
	t.Cleanup(grpcServer.GracefulStop)
	addr := listener.Addr().String()

	plaintext := "live-build-offline-token"
	now := time.Now()
	if err := db.SaveNodeJoinToken(context.Background(), store.NodeJoinToken{
		ID: "njt_live_build_offline", TokenHash: hashJoinToken(plaintext), CreatedAt: now, ExpiresAt: now.Add(time.Hour),
	}); err != nil {
		t.Fatalf("SaveNodeJoinToken() error = %v", err)
	}

	enrollCtx, cancelEnroll := context.WithTimeout(context.Background(), 10*time.Second)
	identity, err := DialEnroll(enrollCtx, addr, plaintext, "live-build-offline-node")
	cancelEnroll()
	if err != nil {
		t.Fatalf("DialEnroll() error = %v", err)
	}

	runner := newFakeBuildRunner()
	runner.release = make(chan struct{})

	sessionCtx, cancelSession := context.WithCancel(context.Background())
	defer cancelSession()
	go func() { _ = RunSession(sessionCtx, addr, identity, rt, nil, WithBuildRunner(runner)) }()
	transport := waitForTransport(t, registry, identity.NodeID)

	remote, ok := transport.(RemoteBuilder)
	if !ok {
		t.Fatalf("transport is %T, want a remote builder", transport)
	}

	buildCtx, cancelBuild := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancelBuild()

	req := dispatchableRequest(t)
	errc := make(chan error, 1)
	go func() {
		_, buildErr := remote.BuildOnNode(buildCtx, req, noopWriter{}, nil)
		errc <- buildErr
	}()

	runner.waitStarted(t)
	// The node vanishing, as far as the control plane can tell.
	cancelSession()

	select {
	case err := <-errc:
		if err == nil {
			t.Fatal("BuildOnNode() error = nil, want a failure once the node went offline")
		}
	case <-time.After(30 * time.Second):
		t.Fatal("timed out waiting for the dispatched build to fail after its node went offline")
	}
}

type noopWriter struct{}

func (noopWriter) Write(p []byte) (int, error) { return len(p), nil }

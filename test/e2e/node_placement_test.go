// Package e2e: this file proves the real, testable claim node placement
// ultimately rests on. See the doc comment on
// TestNodePlacement_Live_EachControllerUsesItsOwnRuntime below for
// exactly what is and is not proven, and why: in short, a single test
// environment with one Docker daemon cannot exercise the full
// NodeID-to-live-remote-agent-connection resolution
// cmd/levelrail/main.go's resolveNodeTransport and dynamicSource do at
// runtime, because that needs a second real connected agent process on
// a second real machine. What this test proves instead is the layer
// underneath that resolution: internal/reconcile/application.Controller
// only ever touches the docker.Runtime it was constructed with, never
// some other implicit one, which is the real invariant placement
// depends on once resolveNodeTransport has picked a Runtime for it.
package e2e

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"testing"
	"time"

	"github.com/docker/docker/api/types/image"
	dockerclient "github.com/docker/docker/client"
	"google.golang.org/grpc"

	"github.com/GLINCKER/levelrail/internal/agent"
	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

// hashJoinTokenForTest mirrors internal/agent's own unexported
// hashJoinToken (SHA-256 hex). Duplicated here rather than exported from
// that package for one test's sake, the same small, purpose-specific
// duplication internal/reconcile/application/placement_live_test.go
// already makes for the identical reason.
func hashJoinTokenForTest(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}

// spyRuntime wraps a real docker.Runtime and counts calls to the three
// container-management methods node placement's claim actually rests on
// (InspectByName, Create, Start), delegating every call, including
// these three, unchanged to the wrapped Runtime. This is instrumentation
// of a real Runtime, not a fake standing in for one: every observed
// container is exactly what the real Docker daemon reports, only the
// call counts are added on top.
type spyRuntime struct {
	docker.Runtime
	inspectCalls int
	createCalls  int
	startCalls   int
}

func (s *spyRuntime) InspectByName(ctx context.Context, name string) (*docker.ContainerState, error) {
	s.inspectCalls++
	return s.Runtime.InspectByName(ctx, name)
}

func (s *spyRuntime) Create(ctx context.Context, spec docker.ContainerSpec) (string, error) {
	s.createCalls++
	return s.Runtime.Create(ctx, spec)
}

func (s *spyRuntime) Start(ctx context.Context, id string) error {
	s.startCalls++
	return s.Runtime.Start(ctx, id)
}

// TestNodePlacement_Live_EachControllerUsesItsOwnRuntime is scoped
// deliberately narrower than "node placement routes a container to a
// different physical machine," because this test environment has no
// second real connected agent process and no real multi-node
// infrastructure to prove that with. cmd/levelrail/main.go's
// resolveNodeTransport and dynamicSource, which do the actual NodeID ->
// live agent connection resolution, are unexported functions in
// package main and are not reachable from this package without
// exporting them or restructuring, both out of scope for a test-only
// task.
//
// What this test DOES prove, honestly and completely: given two real,
// independently constructed docker.Runtime-compatible values, an
// application.Controller built with one of them only ever calls
// InspectByName/Create/Start through that value, never through the
// other. That is the real invariant node placement rests on once
// resolveNodeTransport has already picked which Runtime a service's
// controller should get: if a Controller silently shared global runtime
// state instead of using its own constructor argument, placement would
// be a no-op regardless of how correctly resolveNodeTransport resolved
// per-node connections, because every controller would converge through
// whichever Runtime happened to be used first.
//
// The two Runtime values here are the strongest pair this codebase
// currently offers without inventing new test-only infrastructure:
//
//  1. rtA: a direct local docker.Runtime, docker.NewClient() talking to
//     this machine's Docker socket in-process, exactly what a single
//     node deployment's own local controller uses today
//     (cmd/levelrail/main.go's dynamicSource).
//  2. transport: an agent.Transport (also structurally a docker.Runtime,
//     see internal/agent/transport.go), reached over a real mTLS/gRPC
//     connection to a real internal/agent.Server, whose own session
//     forwards every call to rtAgentSide, a SECOND, separately
//     constructed docker.NewClient(). This is the same shape
//     internal/reconcile/application/placement_live_test.go already
//     proves works for a single controller; this test reuses that
//     pattern but for two SEQUENTIAL controllers sharing one service
//     name, which that file does not exercise.
//
// Both rtA and rtAgentSide ultimately reach the same physical Docker
// daemon, since this machine only has one: that is the honest limit of
// what a single-daemon test environment can offer, explicitly called
// out rather than glossed over. The proof of "genuinely distinct
// Runtime usage" does not come from the daemon being different (it
// cannot be, here); it comes from spyRuntime's call counts, which show
// directly, per call, which Go Runtime value each controller actually
// invoked, plus the independent-detection behavior of reconciling
// through the second controller/runtime pair after the container was
// removed using only the first.
func TestNodePlacement_Live_EachControllerUsesItsOwnRuntime(t *testing.T) {
	dockerCli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		t.Skipf("no docker client available: %v", err)
	}
	t.Cleanup(func() { _ = dockerCli.Close() })

	pingCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	_, err = dockerCli.Ping(pingCtx)
	cancel()
	if err != nil {
		t.Skipf("docker daemon not reachable: %v", err)
	}

	connectCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	buildClient, err := build.NewClient(connectCtx, dockerCli)
	cancel()
	if err != nil {
		t.Skipf("could not connect to buildkit: %v", err)
	}
	t.Cleanup(func() { _ = buildClient.Close() })

	// --- Two real, independently-instantiated docker.Runtime values. ---
	rtA, err := docker.NewClient()
	if err != nil {
		t.Fatalf("docker.NewClient() for rtA error = %v", err)
	}
	t.Cleanup(func() {
		if err := rtA.Close(); err != nil {
			t.Errorf("closing rtA: %v", err)
		}
	})

	rtAgentSide, err := docker.NewClient()
	if err != nil {
		t.Fatalf("docker.NewClient() for rtAgentSide error = %v", err)
	}
	t.Cleanup(func() {
		if err := rtAgentSide.Close(); err != nil {
			t.Errorf("closing rtAgentSide: %v", err)
		}
	})

	const serviceName = "levelrail-test-e2e-node-placement"
	repo := "levelrail/test-e2e-node-placement"
	sha := "e2enodeplacement1"
	tag := repo + ":" + sha

	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = dockerCli.ImageRemove(cleanupCtx, tag, image.RemoveOptions{Force: true})
	})

	cleanupContainers(context.Background(), t, rtA, serviceName)
	t.Cleanup(func() { cleanupContainers(context.Background(), t, rtA, serviceName) })

	svcStore := openLiveStore(t)

	// --- Build the hello-e2e fixture once. ---
	buildCtx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	res, err := buildClient.Build(buildCtx, build.Request{
		ContextDir: "../fixtures/hello-e2e",
		Tag:        tag,
	}, nil)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if res.Tag != tag {
		t.Fatalf("Build() returned tag %q, want %q", res.Tag, tag)
	}

	desired := store.DesiredService{
		Name: serviceName, Image: res.Tag, Port: 8080,
		Health: &store.ServiceHealth{
			Readiness: &store.ServiceProbe{Path: "/", Interval: 200 * time.Millisecond, Timeout: 1 * time.Second},
		},
	}
	if err := svcStore.SaveDesiredService(buildCtx, desired); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}

	// --- Stand up a real agent connection so the second Runtime is a
	// real agent.Transport over real mTLS/gRPC, not just a second local
	// client, mirroring placement_live_test.go's own pattern. rtAgentSide
	// (not rtA) is what the "remote" session forwards calls to. ---
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

	plaintext := "node-placement-live-test-token"
	now := time.Now()
	if err := nodeStore.SaveNodeJoinToken(context.Background(), store.NodeJoinToken{
		ID: "njt_node_placement_live", TokenHash: hashJoinTokenForTest(plaintext), CreatedAt: now, ExpiresAt: now.Add(time.Hour),
	}); err != nil {
		t.Fatalf("SaveNodeJoinToken() error = %v", err)
	}

	enrollCtx, enrollCancel := context.WithTimeout(context.Background(), 10*time.Second)
	identity, err := agent.DialEnroll(enrollCtx, addr, plaintext, "node-placement-live-test-node")
	enrollCancel()
	if err != nil {
		t.Fatalf("DialEnroll() error = %v", err)
	}

	sessionCtx, sessionCancel := context.WithCancel(context.Background())
	sessionDone := make(chan struct{})
	go func() {
		defer close(sessionDone)
		_ = agent.RunSession(sessionCtx, addr, identity, rtAgentSide, nil)
	}()
	// t.Cleanup is LIFO: registered after nodeStore's own cleanup above,
	// so the session is torn down and given a moment to finish its
	// server-side node-offline update before the store it depends on
	// closes, the same ordering placement_live_test.go relies on.
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

	spyA := &spyRuntime{Runtime: rtA}
	spyB := &spyRuntime{Runtime: transport}

	// --- First controller/runtime pair: creates and confirms a running
	// container, entirely through spyA (rtA). ---
	ctrl1 := application.New(serviceName, svcStore, spyA, application.WithReadyBudget(20*time.Second))

	result1, err := ctrl1.Reconcile(buildCtx)
	if err != nil {
		t.Fatalf("first controller Reconcile() error = %v, result = %+v", err, result1)
	}
	if len(result1.Conditions) == 0 || result1.Conditions[0].Status != "True" || result1.Conditions[0].Reason != "Deployed" {
		t.Fatalf("first controller Reconcile() result = %+v, want a True/Deployed condition", result1)
	}

	if spyA.createCalls < 1 {
		t.Errorf("spyA.createCalls = %d, want >= 1: the first controller must create through its own Runtime (rtA)", spyA.createCalls)
	}
	if spyB.inspectCalls != 0 || spyB.createCalls != 0 || spyB.startCalls != 0 {
		t.Errorf("spyB calls after first controller's reconcile = inspect:%d create:%d start:%d, want all 0: the first controller must never touch the second Runtime",
			spyB.inspectCalls, spyB.createCalls, spyB.startCalls)
	}

	target := application.ContainerName(serviceName, tag)
	state, err := rtA.InspectByName(buildCtx, target)
	if err != nil {
		t.Fatalf("InspectByName(%q) via rtA error = %v", target, err)
	}
	if state == nil || !state.Running {
		t.Fatalf("expected %q running after first controller's Reconcile, got %+v", target, state)
	}

	// --- Remove the container using ONLY rtA (the first Runtime
	// reference). If the second controller below silently shared rtA's
	// state or a global runtime instead of using spyB/transport, this
	// removal would be invisible to it. ---
	containers, err := rtA.ListByPrefix(buildCtx, serviceName+"-")
	if err != nil {
		t.Fatalf("ListByPrefix() via rtA error = %v", err)
	}
	for _, c := range containers {
		if err := rtA.Stop(buildCtx, c.ID, 3*time.Second); err != nil {
			t.Fatalf("Stop(%q) via rtA error = %v", c.ID, err)
		}
		if err := rtA.Remove(buildCtx, c.ID, true); err != nil {
			t.Fatalf("Remove(%q) via rtA error = %v", c.ID, err)
		}
	}
	if gone, err := rtA.InspectByName(buildCtx, target); err != nil {
		t.Fatalf("InspectByName(%q) via rtA after removal error = %v", target, err)
	} else if gone != nil {
		t.Fatalf("expected %q gone after manual removal via rtA, got %+v", target, gone)
	}

	spyACreateBefore, spyAInspectBefore, spyAStartBefore := spyA.createCalls, spyA.inspectCalls, spyA.startCalls

	// --- Second controller/runtime pair, same service name, same
	// desired state, given spyB/transport instead of spyA/rtA. It must
	// independently detect the missing container and recreate it. ---
	ctrl2 := application.New(serviceName, svcStore, spyB, application.WithReadyBudget(20*time.Second))

	result2, err := ctrl2.Reconcile(buildCtx)
	if err != nil {
		t.Fatalf("second controller Reconcile() error = %v, result = %+v", err, result2)
	}
	if len(result2.Conditions) == 0 || result2.Conditions[0].Status != "True" || result2.Conditions[0].Reason != "Deployed" {
		t.Fatalf("second controller Reconcile() result = %+v, want a True/Deployed condition (recreated after removal)", result2)
	}

	if spyB.inspectCalls < 1 {
		t.Errorf("spyB.inspectCalls = %d, want >= 1: the second controller must inspect through its own Runtime (transport)", spyB.inspectCalls)
	}
	if spyB.createCalls < 1 {
		t.Errorf("spyB.createCalls = %d, want >= 1: the second controller must recreate the removed container through its own Runtime (transport)", spyB.createCalls)
	}
	if spyB.startCalls < 1 {
		t.Errorf("spyB.startCalls = %d, want >= 1: the second controller must start the recreated container through its own Runtime (transport)", spyB.startCalls)
	}
	if spyA.createCalls != spyACreateBefore || spyA.inspectCalls != spyAInspectBefore || spyA.startCalls != spyAStartBefore {
		t.Errorf("spyA calls changed during the second controller's reconcile: create %d->%d, inspect %d->%d, start %d->%d, want unchanged: the second controller must never touch the first Runtime",
			spyACreateBefore, spyA.createCalls, spyAInspectBefore, spyA.inspectCalls, spyAStartBefore, spyA.startCalls)
	}

	// Independent verification via the transport itself, not the
	// returned Result: confirm the container the second controller
	// claims exists actually exists, reached the same way that
	// controller reached it.
	finalState, err := transport.InspectByName(buildCtx, target)
	if err != nil {
		t.Fatalf("InspectByName(%q) via transport error = %v", target, err)
	}
	if finalState == nil || !finalState.Running {
		t.Fatalf("expected %q running after second controller's Reconcile, got %+v", target, finalState)
	}
}

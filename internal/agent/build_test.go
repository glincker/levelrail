package agent

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
	"github.com/GLINCKER/levelrail/internal/build"
)

// A dispatched build, like exec, is an agreement between two halves that
// cannot be tested apart: context framing, progress and image framing,
// flow control in both directions, and cancellation. So these tests run a
// real mux and a real serveSession against each other over the same
// in-memory loopback exec_test.go uses, with only the build runner faked.

// fakeBuildRunner stands in for a real *build.Client on the agent side.
// It records the context it was handed, emits whatever progress and image
// bytes the test asked for, and can block mid-build so a test can sever
// the session or cancel underneath it.
type fakeBuildRunner struct {
	progress []build.ProgressEvent
	image    []byte
	result   *build.Result
	err      error

	started   chan struct{}
	release   chan struct{}
	ctxDone   chan struct{}
	startOnce sync.Once

	mu         sync.Mutex
	req        build.RemoteRequest
	sawContext map[string]string
}

func newFakeBuildRunner() *fakeBuildRunner {
	return &fakeBuildRunner{
		result:  &build.Result{Duration: 3 * time.Second, ExporterResponse: map[string]string{"containerimage.digest": "sha256:abc"}},
		started: make(chan struct{}),
		ctxDone: make(chan struct{}),
	}
}

func (f *fakeBuildRunner) SolveRemote(ctx context.Context, req build.RemoteRequest, out io.Writer, progress func(build.ProgressEvent)) (*build.Result, error) {
	f.mu.Lock()
	f.req = req
	f.sawContext = readTree(req.ContextDir)
	f.mu.Unlock()
	f.startOnce.Do(func() { close(f.started) })

	if f.release != nil {
		select {
		case <-f.release:
		case <-ctx.Done():
			close(f.ctxDone)
			return nil, ctx.Err()
		}
	}

	for _, ev := range f.progress {
		progress(ev)
	}
	if len(f.image) > 0 {
		if _, err := out.Write(f.image); err != nil {
			return nil, err
		}
	}
	if f.err != nil {
		return nil, f.err
	}
	res := *f.result
	res.Tag = req.Tag
	return &res, nil
}

func (f *fakeBuildRunner) waitStarted(t *testing.T) {
	t.Helper()
	select {
	case <-f.started:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for the agent to start the dispatched build")
	}
}

func (f *fakeBuildRunner) context() (build.RemoteRequest, map[string]string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.req, f.sawContext
}

func readTree(dir string) map[string]string {
	out := map[string]string{}
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		data, readErr := os.ReadFile(path) //nolint:gosec // test-owned temp dir
		if readErr != nil {
			return readErr
		}
		rel, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			return relErr
		}
		out[filepath.ToSlash(rel)] = string(data)
		return nil
	})
	return out
}

func newBuildHarness(t *testing.T, runner BuildRunner) (*GRPCTransport, *loopback) {
	t.Helper()

	l := newLoopback()
	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = serveSession(ctx, agentSide{l}, newExecRuntime(), runner, testLogger()) }()

	t.Cleanup(func() {
		cancel()
		l.close()
	})
	return newGRPCTransport(newMux(controlSide{l})), l
}

// collectBuildProgress records every progress event the control plane
// saw, which is what internal/deploy hands to the SSE build-log stream.
type collectBuildProgress struct {
	mu     sync.Mutex
	events []build.ProgressEvent
}

func (c *collectBuildProgress) fn() func(build.ProgressEvent) {
	return func(ev build.ProgressEvent) {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.events = append(c.events, ev)
	}
}

func (c *collectBuildProgress) all() []build.ProgressEvent {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]build.ProgressEvent(nil), c.events...)
}

// TestBuildOnNode_RoundTrip is the whole protocol in one test: a build
// context larger than the flow-control window travels up, the build runs
// against it on the agent side, and progress plus an image tar larger
// than the window travel back. Both payloads deliberately exceed
// buildWindowFrames*buildChunkBytes so neither direction can pass by
// accident without its credit refunds working.
func TestBuildOnNode_RoundTrip(t *testing.T) {
	contextDir := t.TempDir()
	bigSource := strings.Repeat("levelrail-remote-build-context\n", 200_000)
	writeBuildFixture(t, filepath.Join(contextDir, "Dockerfile"), "FROM scratch\n")
	writeBuildFixture(t, filepath.Join(contextDir, "app", "big.txt"), bigSource)

	runner := newFakeBuildRunner()
	runner.progress = []build.ProgressEvent{
		{Step: "[1/2] FROM scratch", Completed: true, Cached: true},
		{Log: "building\n", Stream: "stdout"},
		{Log: "warning\n", Stream: "stderr"},
	}
	runner.image = bytes.Repeat([]byte("IMAGE-TAR-BYTES:"), 400_000)

	transport, _ := newBuildHarness(t, runner)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var image bytes.Buffer
	progress := &collectBuildProgress{}
	res, err := transport.BuildOnNode(ctx, build.RemoteRequest{
		Kind:           build.RemoteKindDockerfile,
		ContextDir:     contextDir,
		DockerfilePath: "Dockerfile",
		Tag:            "app:abc123",
		BuildArgs:      map[string]string{"VERSION": "1.2.3"},
		Cache:          build.CacheConfig{RegistryRef: "reg.example/cache:app", RegistryInsecure: true},
	}, &image, progress.fn())
	if err != nil {
		t.Fatalf("BuildOnNode() error = %v", err)
	}

	gotReq, gotContext := runner.context()
	if gotReq.Tag != "app:abc123" || gotReq.DockerfilePath != "Dockerfile" || gotReq.Kind != build.RemoteKindDockerfile {
		t.Errorf("agent saw request %+v, want the dispatched dockerfile build", gotReq)
	}
	if gotReq.BuildArgs["VERSION"] != "1.2.3" {
		t.Errorf("agent saw build args %v, want VERSION=1.2.3", gotReq.BuildArgs)
	}
	if gotReq.Cache.RegistryRef != "reg.example/cache:app" || !gotReq.Cache.RegistryInsecure {
		t.Errorf("agent saw cache %+v, want the registry backend carried across", gotReq.Cache)
	}
	if gotContext["Dockerfile"] != "FROM scratch\n" || gotContext["app/big.txt"] != bigSource {
		t.Errorf("agent unpacked %d files, want the dispatched context reproduced exactly (Dockerfile len %d, big.txt len %d)",
			len(gotContext), len(gotContext["Dockerfile"]), len(gotContext["app/big.txt"]))
	}

	if !bytes.Equal(image.Bytes(), runner.image) {
		t.Errorf("image stream = %d bytes, want the %d bytes the build produced", image.Len(), len(runner.image))
	}
	if res.Tag != "app:abc123" || res.Duration != 3*time.Second {
		t.Errorf("Result = %+v, want the remote build's own tag and duration", res)
	}
	if res.ExporterResponse["containerimage.digest"] != "sha256:abc" {
		t.Errorf("Result.ExporterResponse = %v, want the remote exporter metadata", res.ExporterResponse)
	}

	events := progress.all()
	if len(events) != len(runner.progress) {
		t.Fatalf("got %d progress events, want %d", len(events), len(runner.progress))
	}
	for i, want := range runner.progress {
		if events[i] != want {
			t.Errorf("progress[%d] = %+v, want %+v", i, events[i], want)
		}
	}
}

func TestBuildOnNode_RunnerFailureSurfacesWithItsReason(t *testing.T) {
	runner := newFakeBuildRunner()
	runner.err = errors.New("buildkit: failed to solve: dockerfile parse error")
	transport, _ := newBuildHarness(t, runner)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := transport.BuildOnNode(ctx, dispatchableRequest(t), io.Discard, nil)
	if err == nil {
		t.Fatal("BuildOnNode() error = nil, want the remote build's failure")
	}
	if !strings.Contains(err.Error(), "dockerfile parse error") {
		t.Errorf("BuildOnNode() error = %q, want the remote reason preserved", err)
	}
}

// TestBuildOnNode_UnsupportedProviderStaysTyped matters because
// internal/deploy branches on this error with errors.As to tell an
// operator which provider Railpack actually detected; a flattened string
// would silently degrade that message for dispatched builds only.
func TestBuildOnNode_UnsupportedProviderStaysTyped(t *testing.T) {
	runner := newFakeBuildRunner()
	runner.err = &build.UnsupportedProviderError{Provider: "python"}
	transport, _ := newBuildHarness(t, runner)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := transport.BuildOnNode(ctx, dispatchableRequest(t), io.Discard, nil)

	var unsupported *build.UnsupportedProviderError
	if !errors.As(err, &unsupported) {
		t.Fatalf("BuildOnNode() error = %v, want a *build.UnsupportedProviderError", err)
	}
	if unsupported.Provider != "python" {
		t.Errorf("Provider = %q, want %q", unsupported.Provider, "python")
	}
}

func TestBuildOnNode_AgentWithoutBuildKitRejectsTheBuild(t *testing.T) {
	transport, _ := newBuildHarness(t, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := transport.BuildOnNode(ctx, dispatchableRequest(t), io.Discard, nil)
	if err == nil {
		t.Fatal("BuildOnNode() error = nil, want a node with no BuildKit to say so")
	}
	if !strings.Contains(err.Error(), "BuildKit") {
		t.Errorf("BuildOnNode() error = %q, want it to name the missing BuildKit connection", err)
	}
}

// TestBuildOnNode_NodeGoesOfflineMidBuild is the failure an operator will
// actually hit: the build node drops off while its build is running. It
// has to end the build with a distinguishable error, not hang and not
// look like a build failure of its own.
func TestBuildOnNode_NodeGoesOfflineMidBuild(t *testing.T) {
	runner := newFakeBuildRunner()
	runner.release = make(chan struct{})
	transport, l := newBuildHarness(t, runner)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	errc := make(chan error, 1)
	go func() {
		_, err := transport.BuildOnNode(ctx, dispatchableRequest(t), io.Discard, nil)
		errc <- err
	}()

	runner.waitStarted(t)
	l.close()

	select {
	case err := <-errc:
		if !errors.Is(err, ErrSessionClosed) {
			t.Fatalf("BuildOnNode() error = %v, want it to wrap %v", err, ErrSessionClosed)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("timed out waiting for the build to fail after the session ended")
	}
}

// TestBuildOnNode_CallerGivingUpStopsTheRemoteBuild proves a cancelled
// build does not keep burning the build node's CPU for nobody.
func TestBuildOnNode_CallerGivingUpStopsTheRemoteBuild(t *testing.T) {
	runner := newFakeBuildRunner()
	runner.release = make(chan struct{})
	transport, _ := newBuildHarness(t, runner)

	ctx, cancel := context.WithCancel(context.Background())
	errc := make(chan error, 1)
	go func() {
		_, err := transport.BuildOnNode(ctx, dispatchableRequest(t), io.Discard, nil)
		errc <- err
	}()

	runner.waitStarted(t)
	cancel()

	select {
	case err := <-errc:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("BuildOnNode() error = %v, want context.Canceled", err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("timed out waiting for the cancelled build to return")
	}

	select {
	case <-runner.ctxDone:
	case <-time.After(20 * time.Second):
		t.Fatal("the remote build kept running after its caller gave up")
	}
}

// TestBuildRelay_PeerOverrunningItsWindowFailsTheBuild drives BuildRelay
// directly, since the real control plane honors its credit and so can
// never produce this: a peer that ignores the window has to fail its own
// build rather than be allowed unbounded buffering on the build node.
func TestBuildRelay_PeerOverrunningItsWindowFailsTheBuild(t *testing.T) {
	var (
		mu     sync.Mutex
		frames []*agentpb.BuildOutput
	)
	send := func(msg *agentpb.AgentMessage) {
		if out, ok := msg.GetPayload().(*agentpb.AgentMessage_BuildOutput); ok {
			mu.Lock()
			frames = append(frames, out.BuildOutput)
			mu.Unlock()
		}
	}

	runner := newFakeBuildRunner()
	runner.release = make(chan struct{})
	relay := NewBuildRelay(runner, send)
	t.Cleanup(relay.CloseAll)

	relay.Start(t.Context(), "b1", &agentpb.BuildRequest{
		Kind: agentpb.BuildKind_BUILD_KIND_DOCKERFILE,
		Tag:  "app:sha",
	})

	// Nothing is consuming credit while the build still waits for its
	// context, so these overrun the window by construction.
	for range buildWindowFrames + 2 {
		relay.Credit(&agentpb.BuildCredit{BuildId: "b1", Frames: 1})
	}

	mu.Lock()
	defer mu.Unlock()
	if len(frames) != 1 {
		t.Fatalf("got %d build output frames, want exactly one terminal failure", len(frames))
	}
	failure := frames[0].GetFailure()
	if failure == nil || !strings.Contains(failure.GetMessage(), "flow-control window") {
		t.Errorf("terminal frame = %+v, want a failure naming the flow-control window", frames[0])
	}
}

func TestBuildDispatcher_BuildOnNode(t *testing.T) {
	registry := NewRegistry()
	dispatcher := NewBuildDispatcher(registry)

	_, err := dispatcher.BuildOnNode(t.Context(), "nope", dispatchableRequest(t), io.Discard, nil)
	if !errors.Is(err, ErrNodeNotRegistered) {
		t.Fatalf("BuildOnNode() on an unknown node err = %v, want %v", err, ErrNodeNotRegistered)
	}

	// A local transport is this process's own Docker socket, which is what
	// "build locally" already means: dispatching to it must fail loudly
	// rather than look like a remote build that silently ran here.
	registry.Register("local", NewLocal(newExecRuntime()))
	_, err = dispatcher.BuildOnNode(t.Context(), "local", dispatchableRequest(t), io.Discard, nil)
	if err == nil || !strings.Contains(err.Error(), "cannot run a dispatched build") {
		t.Fatalf("BuildOnNode() on a local transport err = %v, want a clear refusal", err)
	}
}

func dispatchableRequest(t *testing.T) build.RemoteRequest {
	t.Helper()
	dir := t.TempDir()
	writeBuildFixture(t, filepath.Join(dir, "Dockerfile"), "FROM scratch\n")
	return build.RemoteRequest{
		Kind:           build.RemoteKindDockerfile,
		ContextDir:     dir,
		DockerfilePath: "Dockerfile",
		Tag:            "app:abc123",
	}
}

func writeBuildFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("creating fixture dir for %q: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing fixture %q: %v", path, err)
	}
}

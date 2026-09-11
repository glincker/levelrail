package agent

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
	"github.com/GLINCKER/levelrail/internal/docker"
)

// The exec protocol is the one operation whose two halves cannot be
// tested apart: output framing, stdin framing, flow control, and
// cancellation are all agreements between GRPCTransport and ExecRelay.
// So these tests run a real mux and a real serveSession against each
// other over in-memory streams, with only docker.Runtime faked.

// loopback connects a control-plane sessionStream to an agent
// agentClientStream. Its channels are deliberately almost unbuffered:
// the protocol's own credit window is what has to bound how much is in
// flight, and a generous buffer here would hide a failure to do that.
type loopback struct {
	toAgent   chan *agentpb.ControlMessage
	toControl chan *agentpb.AgentMessage
	done      chan struct{}
	closeOnce sync.Once
}

func newLoopback() *loopback {
	return &loopback{
		toAgent:   make(chan *agentpb.ControlMessage, 1),
		toControl: make(chan *agentpb.AgentMessage, 1),
		done:      make(chan struct{}),
	}
}

func (l *loopback) close() {
	l.closeOnce.Do(func() { close(l.done) })
}

type controlSide struct{ l *loopback }

func (c controlSide) Send(msg *agentpb.ControlMessage) error {
	select {
	case c.l.toAgent <- msg:
		return nil
	case <-c.l.done:
		return io.ErrClosedPipe
	}
}

func (c controlSide) Recv() (*agentpb.AgentMessage, error) {
	select {
	case msg := <-c.l.toControl:
		return msg, nil
	case <-c.l.done:
		return nil, io.EOF
	}
}

type agentSide struct{ l *loopback }

func (a agentSide) Send(msg *agentpb.AgentMessage) error {
	select {
	case a.l.toControl <- msg:
		return nil
	case <-a.l.done:
		return io.ErrClosedPipe
	}
}

func (a agentSide) Recv() (*agentpb.ControlMessage, error) {
	select {
	case msg := <-a.l.toAgent:
		return msg, nil
	case <-a.l.done:
		return nil, io.EOF
	}
}

func newExecHarness(t *testing.T, rt docker.Runtime) *GRPCTransport {
	t.Helper()

	l := newLoopback()
	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = serveSession(ctx, agentSide{l}, rt, testLogger()) }()

	t.Cleanup(func() {
		cancel()
		l.close()
	})
	return newGRPCTransport(newMux(controlSide{l}))
}

// trackedReadCloser reports whether the agent actually closed the local
// exec stream, which is how a cancelled remote exec is really stopped.
type trackedReadCloser struct {
	io.ReadCloser
	closed chan struct{}
	once   sync.Once
}

func (t *trackedReadCloser) Close() error {
	t.once.Do(func() { close(t.closed) })
	return t.ReadCloser.Close()
}

// fakeExecRuntime is a docker.Runtime whose exec methods hand back a
// pipe the test itself drives, so a test can produce output gradually,
// fail mid-stream, or produce nothing at all.
type fakeExecRuntime struct {
	*execRuntime

	attachErr error

	mu        sync.Mutex
	container string
	cmd       []string
	writer    *io.PipeWriter
	output    *trackedReadCloser
	ctxDone   <-chan struct{}
	started   chan struct{}

	stdinRead chan struct{}
	stdinGot  bytes.Buffer
	stdinErr  error
}

func newFakeExecRuntime() *fakeExecRuntime {
	return &fakeExecRuntime{
		execRuntime: newExecRuntime(),
		started:     make(chan struct{}),
		stdinRead:   make(chan struct{}),
	}
}

// begin records one exec attach and returns the writer half of the
// stream the agent will read, for the test to drive.
func (f *fakeExecRuntime) begin(ctx context.Context, containerID string, cmd []string) (*io.PipeWriter, io.ReadCloser, error) {
	if f.attachErr != nil {
		return nil, nil, f.attachErr
	}
	pr, pw := io.Pipe()
	tracked := &trackedReadCloser{ReadCloser: pr, closed: make(chan struct{})}

	f.mu.Lock()
	f.container, f.cmd, f.output, f.ctxDone = containerID, cmd, tracked, ctx.Done()
	f.mu.Unlock()
	close(f.started)

	return pw, tracked, nil
}

func (f *fakeExecRuntime) Exec(ctx context.Context, containerID string, cmd []string) (io.ReadCloser, error) {
	pw, rc, err := f.begin(ctx, containerID, cmd)
	if err != nil {
		return nil, err
	}
	f.mu.Lock()
	f.writer = pw
	f.mu.Unlock()
	return rc, nil
}

func (f *fakeExecRuntime) ExecWithInput(ctx context.Context, containerID string, cmd []string, stdin io.Reader) (io.ReadCloser, error) {
	pw, rc, err := f.begin(ctx, containerID, cmd)
	if err != nil {
		return nil, err
	}
	f.mu.Lock()
	f.writer = pw
	f.mu.Unlock()

	// The real client drains stdin concurrently with producing output,
	// so the fake does too: reading it inline would deadlock any command
	// whose output the caller reads before stdin is exhausted.
	go func() {
		_, copyErr := io.Copy(&f.stdinGot, stdin)
		f.mu.Lock()
		f.stdinErr = copyErr
		f.mu.Unlock()
		close(f.stdinRead)
	}()
	return rc, nil
}

func (f *fakeExecRuntime) waitStarted(t *testing.T) {
	t.Helper()
	select {
	case <-f.started:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the agent to start the exec")
	}
}

func (f *fakeExecRuntime) stream() *io.PipeWriter {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.writer
}

func (f *fakeExecRuntime) localStream() *trackedReadCloser {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.output
}

func TestExec_LargeOutput_ReassemblesAcrossFrames(t *testing.T) {
	rt := newFakeExecRuntime()
	tr := newExecHarness(t, rt)

	// Comfortably past the flow-control window (64 frames), so the
	// credit path is genuinely exercised rather than only the first
	// window's worth of frames.
	payload := patternBytes(3 << 20)

	rc, err := tr.Exec(context.Background(), "db-1", []string{"pg_dump", "app"})
	if err != nil {
		t.Fatalf("Exec() error = %v", err)
	}
	defer func() { _ = rc.Close() }()

	rt.waitStarted(t)
	go func() {
		_, _ = rt.stream().Write(payload)
		_ = rt.stream().Close()
	}()

	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if len(got) != len(payload) {
		t.Fatalf("read %d bytes, want %d", len(got), len(payload))
	}
	if !bytes.Equal(got, payload) {
		t.Error("output bytes differ from what the remote command produced")
	}

	rt.mu.Lock()
	container, cmd := rt.container, rt.cmd
	rt.mu.Unlock()
	if container != "db-1" || len(cmd) != 2 || cmd[0] != "pg_dump" {
		t.Errorf("exec ran %v in %q, want [pg_dump app] in db-1", cmd, container)
	}
}

func TestExecWithInput_StreamsStdinWhileOutputFlows(t *testing.T) {
	rt := newFakeExecRuntime()
	tr := newExecHarness(t, rt)

	dump := patternBytes(3 << 20)

	rc, err := tr.ExecWithInput(context.Background(), "db-1", []string{"psql"}, bytes.NewReader(dump))
	if err != nil {
		t.Fatalf("ExecWithInput() error = %v", err)
	}
	defer func() { _ = rc.Close() }()

	rt.waitStarted(t)

	// The command answers only once it has consumed all of stdin, which
	// is what proves stdin actually reached it rather than being
	// buffered somewhere in the middle.
	go func() {
		select {
		case <-rt.stdinRead:
		case <-time.After(10 * time.Second):
		}
		_, _ = rt.stream().Write([]byte("RESTORE OK"))
		_ = rt.stream().Close()
	}()

	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if string(got) != "RESTORE OK" {
		t.Errorf("output = %q, want RESTORE OK", got)
	}

	rt.mu.Lock()
	stdinErr := rt.stdinErr
	rt.mu.Unlock()
	if stdinErr != nil {
		t.Errorf("stdin copy error = %v", stdinErr)
	}
	if rt.stdinGot.Len() != len(dump) {
		t.Fatalf("command read %d stdin bytes, want %d", rt.stdinGot.Len(), len(dump))
	}
	if !bytes.Equal(rt.stdinGot.Bytes(), dump) {
		t.Error("stdin bytes differ from what the caller supplied")
	}
}

func TestExec_SlowReader_BlocksTheCommandInsteadOfBuffering(t *testing.T) {
	rt := newFakeExecRuntime()
	tr := newExecHarness(t, rt)

	rc, err := tr.Exec(context.Background(), "db-1", []string{"pg_dump"})
	if err != nil {
		t.Fatalf("Exec() error = %v", err)
	}
	defer func() { _ = rc.Close() }()
	rt.waitStarted(t)

	// A command producing far more than the whole pipeline may hold,
	// with nobody reading the near end yet.
	var written atomic.Int64
	go func() {
		chunk := make([]byte, 64<<10)
		for written.Load() < 32<<20 {
			n, writeErr := rt.stream().Write(chunk)
			written.Add(int64(n))
			if writeErr != nil {
				return
			}
		}
		_ = rt.stream().Close()
	}()

	plateau := waitForPlateau(t, &written)

	// Everything in flight at once: the agent's own send window, the
	// control plane's undelivered frames, and a frame in each hop
	// between them. Generous, but orders of magnitude below the 32 MiB
	// the command is trying to push.
	limit := int64(execWindowFrames*execChunkBytes)*2 + 8*execChunkBytes
	if plateau > limit {
		t.Errorf("command wrote %d bytes with nobody reading, want it blocked under %d", plateau, limit)
	}

	// And it is blocked, not dead: reading drains the window and the
	// command resumes.
	if _, err := io.CopyN(io.Discard, rc, 4<<20); err != nil {
		t.Fatalf("CopyN() error = %v", err)
	}
	deadline := time.After(5 * time.Second)
	for written.Load() <= plateau {
		select {
		case <-deadline:
			t.Fatal("the command never resumed after its output was consumed")
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// waitForPlateau returns the value counter settles at, once it has
// stopped growing across consecutive samples.
func waitForPlateau(t *testing.T, counter *atomic.Int64) int64 {
	t.Helper()
	last := int64(-1)
	stable := 0
	for range 200 {
		time.Sleep(10 * time.Millisecond)
		now := counter.Load()
		if now == last {
			stable++
			if stable == 5 {
				return now
			}
			continue
		}
		last, stable = now, 0
	}
	t.Fatal("the command's output never stopped growing, so nothing is applying backpressure")
	return 0
}

func TestExec_CloseBeforeCompletion_StopsRemoteCommand(t *testing.T) {
	rt := newFakeExecRuntime()
	tr := newExecHarness(t, rt)

	rc, err := tr.Exec(context.Background(), "db-1", []string{"sleep", "3600"})
	if err != nil {
		t.Fatalf("Exec() error = %v", err)
	}
	rt.waitStarted(t)

	// The command has produced nothing and never will: only an explicit
	// cancel can end it.
	if err := rc.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	local := rt.localStream()
	select {
	case <-local.closed:
	case <-time.After(5 * time.Second):
		t.Fatal("the agent never closed the local exec stream, so the remote command outlived its caller")
	}

	rt.mu.Lock()
	ctxDone := rt.ctxDone
	rt.mu.Unlock()
	select {
	case <-ctxDone:
	case <-time.After(5 * time.Second):
		t.Fatal("the agent never cancelled the local exec's context")
	}
}

func TestExec_NonZeroExit_SurfacesTypedError(t *testing.T) {
	rt := newFakeExecRuntime()
	tr := newExecHarness(t, rt)

	rc, err := tr.Exec(context.Background(), "db-1", []string{"pg_dump", "missing"})
	if err != nil {
		t.Fatalf("Exec() error = %v", err)
	}
	defer func() { _ = rc.Close() }()

	rt.waitStarted(t)
	go func() {
		_, _ = rt.stream().Write([]byte("partial output"))
		_ = rt.stream().CloseWithError(&docker.ExecExitError{
			Cmd:       []string{"pg_dump", "missing"},
			Container: "db-1",
			ExitCode:  7,
			Stderr:    "database \"missing\" does not exist",
		})
	}()

	got, err := io.ReadAll(rc)
	if string(got) != "partial output" {
		t.Errorf("output before the failure = %q, want %q", got, "partial output")
	}

	var exitErr *docker.ExecExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("err = %v (%T), want a *docker.ExecExitError", err, err)
	}
	if exitErr.ExitCode != 7 || exitErr.Stderr != "database \"missing\" does not exist" {
		t.Errorf("exit error = %+v, want ExitCode 7 and the remote stderr", exitErr)
	}
	if exitErr.Container != "db-1" {
		t.Errorf("exit error container = %q, want db-1", exitErr.Container)
	}
}

func TestExec_MidStreamFailure_SurfacesToCaller(t *testing.T) {
	rt := newFakeExecRuntime()
	tr := newExecHarness(t, rt)

	rc, err := tr.Exec(context.Background(), "db-1", []string{"pg_dump"})
	if err != nil {
		t.Fatalf("Exec() error = %v", err)
	}
	defer func() { _ = rc.Close() }()

	rt.waitStarted(t)
	go func() {
		_, _ = rt.stream().Write([]byte("some rows"))
		_ = rt.stream().CloseWithError(errors.New("docker: exec read output: connection reset"))
	}()

	got, err := io.ReadAll(rc)
	if string(got) != "some rows" {
		t.Errorf("output before the failure = %q, want %q", got, "some rows")
	}
	if err == nil || err.Error() != "docker: exec read output: connection reset" {
		t.Fatalf("err = %v, want the remote read failure", err)
	}
}

func TestExec_AttachFailure_ReturnsErrorImmediately(t *testing.T) {
	rt := newFakeExecRuntime()
	rt.attachErr = errors.New("docker: exec create in db-1: no such container")
	tr := newExecHarness(t, rt)

	rc, err := tr.Exec(context.Background(), "db-1", []string{"pg_dump"})
	if err == nil {
		_ = rc.Close()
		t.Fatal("Exec() error = nil, want the agent's attach failure")
	}
	if err.Error() != "docker: exec create in db-1: no such container" {
		t.Errorf("err = %v, want the attach failure verbatim", err)
	}
}

func TestExecWithInput_SourceReadError_FailsTheExec(t *testing.T) {
	rt := newFakeExecRuntime()
	tr := newExecHarness(t, rt)

	source := io.MultiReader(
		bytes.NewReader([]byte("first half of the dump")),
		&failingReader{err: errors.New("s3: connection reset mid-download")},
	)

	rc, err := tr.ExecWithInput(context.Background(), "db-1", []string{"psql"}, source)
	if err != nil {
		t.Fatalf("ExecWithInput() error = %v", err)
	}
	defer func() { _ = rc.Close() }()

	rt.waitStarted(t)

	// A truncated restore must fail loudly, not look like a complete one.
	_, err = io.ReadAll(rc)
	if err == nil {
		t.Fatal("ReadAll() error = nil, want the stdin source failure surfaced")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("s3: connection reset mid-download")) {
		t.Errorf("err = %v, want it to carry the source reader's own failure", err)
	}
}

func TestMux_ExecOutput_WindowViolationFailsOnlyThatExec(t *testing.T) {
	stream := newFakeSessionStream()
	m := newMux(stream)

	sub := m.subscribeExec("e1")
	if sub == nil {
		t.Fatal("subscribeExec() = nil")
	}

	// More unacknowledged frames than any well-behaved agent may have in
	// flight: the mux must fail this one exec rather than block recvLoop
	// or silently drop bytes.
	for range execWindowFrames + 8 {
		m.deliverExecOutput(&agentpb.ExecOutput{ExecId: "e1", Chunk: []byte("x")})
	}

	if _, ok := <-sub.out; !ok {
		t.Fatal("subscription channel closed before delivering any frame")
	}
	drainClosed(t, sub.out)

	if err := sub.failure(); !errors.Is(err, ErrExecWindowViolated) {
		t.Errorf("failure = %v, want ErrExecWindowViolated", err)
	}
	if m.execSubscription("e1") != nil {
		t.Error("the violating exec is still subscribed")
	}
}

func drainClosed(t *testing.T, ch chan *agentpb.ExecOutput) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return
			}
		case <-deadline:
			t.Fatal("timed out waiting for the failed exec's channel to close")
		}
	}
}

type failingReader struct{ err error }

func (f *failingReader) Read([]byte) (int, error) { return 0, f.err }

// patternBytes builds a deterministic payload whose every byte depends
// on its offset, so reordered or duplicated frames fail the comparison
// instead of passing on a repeating pattern.
func patternBytes(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i*31 + i/251)
	}
	return b
}

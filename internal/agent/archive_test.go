package agent

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
)

// archiveRuntime fakes the Docker archive API under a real mux and
// serveSession, so the whole remote copy path runs except Docker itself.
type archiveRuntime struct {
	*execRuntime

	mu        sync.Mutex
	outPath   string
	outMax    int64
	out       *trackedReadCloser
	outWriter *io.PipeWriter
	outErr    error
	started   chan struct{}

	inDir string
	inGot bytes.Buffer
	inErr error
	inFn  func(r io.Reader) error
}

func newArchiveRuntime() *archiveRuntime {
	return &archiveRuntime{execRuntime: newExecRuntime(), started: make(chan struct{})}
}

func (a *archiveRuntime) CopyFromContainer(_ context.Context, _, path string, maxBytes int64) (io.ReadCloser, docker.PathStat, error) {
	if a.outErr != nil {
		return nil, docker.PathStat{}, a.outErr
	}
	pr, pw := io.Pipe()
	tracked := &trackedReadCloser{ReadCloser: pr, closed: make(chan struct{})}
	a.mu.Lock()
	a.outPath, a.outMax, a.out, a.outWriter = path, maxBytes, tracked, pw
	a.mu.Unlock()
	close(a.started)
	return docker.CapReadCloser(tracked, maxBytes), docker.PathStat{Name: "dist", Size: 4096, Mode: os.ModeDir | 0o755, LinkTarget: "t"}, nil
}

func (a *archiveRuntime) CopyToContainer(_ context.Context, _, dir string, content io.Reader, maxBytes int64) error {
	a.mu.Lock()
	a.inDir = dir
	a.mu.Unlock()
	content = docker.CapReader(content, maxBytes)
	if a.inFn != nil {
		return a.inFn(content)
	}
	_, err := io.Copy(&a.inGot, content)
	return err
}

func (a *archiveRuntime) waitStarted(t *testing.T) {
	t.Helper()
	select {
	case <-a.started:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the agent to open the archive")
	}
}

func newArchiveHarness(t *testing.T, rt docker.Runtime) (*GRPCTransport, *loopback) {
	t.Helper()
	l := newLoopback()
	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = serveSession(ctx, agentSide{l}, rt, nil, nil, "", time.Hour, nil, testLogger()) }()
	t.Cleanup(func() {
		cancel()
		l.close()
	})
	return newGRPCTransport(newMux(controlSide{l})), l
}

func TestCopyFromContainer_StreamsLargeTarWithStat(t *testing.T) {
	rt := newArchiveRuntime()
	tr, _ := newArchiveHarness(t, rt)
	payload := patternBytes(5 << 20)

	rc, st, err := tr.CopyFromContainer(context.Background(), "job-1", "/workspace/dist", 0)
	if err != nil {
		t.Fatalf("CopyFromContainer() error = %v", err)
	}
	defer func() { _ = rc.Close() }()
	if st.Name != "dist" || st.Size != 4096 || !st.Mode.IsDir() || st.LinkTarget != "t" {
		t.Errorf("stat = %+v, want the agent's stat carried over the wire", st)
	}
	go func() {
		_, _ = rt.outWriter.Write(payload)
		_ = rt.outWriter.Close()
	}()
	got, err := io.ReadAll(rc)
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("read %d bytes, err %v; want the %d byte payload intact", len(got), err, len(payload))
	}
	if rt.outPath != "/workspace/dist" {
		t.Errorf("agent opened %q", rt.outPath)
	}
}

func TestCopyFromContainer_SlowReaderAppliesBackpressure(t *testing.T) {
	rt := newArchiveRuntime()
	tr, _ := newArchiveHarness(t, rt)
	rc, _, err := tr.CopyFromContainer(context.Background(), "job-1", "/workspace/dist", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rc.Close() }()

	var written atomic.Int64
	go func() {
		chunk := make([]byte, 64<<10)
		for written.Load() < 32<<20 {
			n, werr := rt.outWriter.Write(chunk)
			written.Add(int64(n))
			if werr != nil {
				return
			}
		}
	}()
	plateau := waitForPlateau(t, &written)
	if plateau >= 32<<20 {
		t.Fatalf("producer wrote %d bytes with nobody reading: the stream is buffering", plateau)
	}
}

func TestCopyFromContainer_SizeCapAbortsTheStream(t *testing.T) {
	rt := newArchiveRuntime()
	tr, _ := newArchiveHarness(t, rt)
	rc, _, err := tr.CopyFromContainer(context.Background(), "job-1", "/workspace/dist", 100<<10)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rc.Close() }()
	go func() { _, _ = rt.outWriter.Write(patternBytes(1 << 20)) }()
	_, err = io.ReadAll(rc)
	if err == nil || !strings.Contains(err.Error(), "exceeds the size limit") {
		t.Fatalf("err = %v, want the size limit failure", err)
	}
	if rt.outMax != 100<<10 {
		t.Errorf("agent saw cap %d", rt.outMax)
	}
}

func TestCopyFromContainer_MidStreamErrorSurfaces(t *testing.T) {
	rt := newArchiveRuntime()
	tr, _ := newArchiveHarness(t, rt)
	rc, _, err := tr.CopyFromContainer(context.Background(), "job-1", "/workspace/dist", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rc.Close() }()
	go func() {
		_, _ = rt.outWriter.Write(patternBytes(100 << 10))
		_ = rt.outWriter.CloseWithError(errors.New("daemon went away"))
	}()
	if _, err := io.ReadAll(rc); err == nil || !strings.Contains(err.Error(), "daemon went away") {
		t.Fatalf("err = %v, want the daemon failure, not a clean EOF", err)
	}
}

func TestCopyFromContainer_PathNotFoundFailsAtOpen(t *testing.T) {
	rt := newArchiveRuntime()
	rt.outErr = errors.New("docker: path not found in container: /workspace/nope")
	tr, _ := newArchiveHarness(t, rt)
	if _, _, err := tr.CopyFromContainer(context.Background(), "job-1", "/workspace/nope", 0); err == nil || !strings.Contains(err.Error(), "path not found") {
		t.Fatalf("err = %v", err)
	}
}

func TestCopyFromContainer_CancelStopsTheAgentSide(t *testing.T) {
	rt := newArchiveRuntime()
	tr, _ := newArchiveHarness(t, rt)
	ctx, cancel := context.WithCancel(context.Background())
	rc, _, err := tr.CopyFromContainer(ctx, "job-1", "/workspace/dist", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rc.Close() }()
	rt.waitStarted(t)
	readErr := make(chan error, 1)
	go func() {
		_, err := rc.Read(make([]byte, 10))
		readErr <- err
	}()
	cancel()
	select {
	case err := <-readErr:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("read after cancel = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("read did not return after cancel")
	}
	select {
	case <-rt.out.closed:
	case <-time.After(5 * time.Second):
		t.Fatal("agent kept the archive stream open after the control plane cancelled")
	}
}

func TestCopyFromContainer_DroppedConnectionFailsTheRead(t *testing.T) {
	rt := newArchiveRuntime()
	tr, l := newArchiveHarness(t, rt)
	rc, _, err := tr.CopyFromContainer(context.Background(), "job-1", "/workspace/dist", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rc.Close() }()
	rt.waitStarted(t)
	go func() {
		_, _ = rt.outWriter.Write(patternBytes(10 << 10))
		time.Sleep(50 * time.Millisecond)
		l.close()
	}()
	_, err = io.ReadAll(rc)
	if err == nil || errors.Is(err, io.EOF) {
		t.Fatalf("err = %v, a dropped session must not look like a complete archive", err)
	}
}

func TestCopyToContainer_StreamsLargeTar(t *testing.T) {
	rt := newArchiveRuntime()
	tr, _ := newArchiveHarness(t, rt)
	payload := patternBytes(5 << 20)
	if err := tr.CopyToContainer(context.Background(), "job-1", "/workspace", bytes.NewReader(payload), 0); err != nil {
		t.Fatalf("CopyToContainer() error = %v", err)
	}
	if !bytes.Equal(rt.inGot.Bytes(), payload) || rt.inDir != "/workspace" {
		t.Errorf("agent got %d bytes into %q, want the %d byte payload into /workspace", rt.inGot.Len(), rt.inDir, len(payload))
	}
}

func TestCopyToContainer_AgentErrorAndCapAndSourceFailure(t *testing.T) {
	tests := []struct {
		name string
		fn   func(r io.Reader) error
		src  io.Reader
		max  int64
		want string
	}{
		{"daemon refuses", func(r io.Reader) error { _, _ = io.CopyN(io.Discard, r, 1000); return errors.New("no space left") }, bytes.NewReader(patternBytes(1 << 20)), 0, "no space left"},
		{"size cap", nil, bytes.NewReader(patternBytes(1 << 20)), 100 << 10, "exceeds the size limit"},
		{"source breaks", nil, io.MultiReader(bytes.NewReader(patternBytes(200<<10)), &failingReader{errors.New("disk read failed")}), 0, "disk read failed"},
	}
	for _, tc := range tests {
		rt := newArchiveRuntime()
		rt.inFn = tc.fn
		tr, _ := newArchiveHarness(t, rt)
		err := tr.CopyToContainer(context.Background(), "job-1", "/workspace", tc.src, tc.max)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want %q", tc.name, err, tc.want)
		}
	}
}

func TestCopyToContainer_CancelAndDroppedConnection(t *testing.T) {
	rt := newArchiveRuntime()
	entered := make(chan struct{})
	rt.inFn = func(r io.Reader) error {
		close(entered)
		_, err := io.Copy(io.Discard, r)
		return err
	}
	tr, l := newArchiveHarness(t, rt)
	pr, pw := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- tr.CopyToContainer(ctx, "job-1", "/workspace", pr, 0) }()
	<-entered
	_, _ = pw.Write([]byte("partial"))
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Error("cancelled copy-in reported success")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("copy-in did not return after cancel")
	}

	rt2 := newArchiveRuntime()
	rt2.inFn = func(r io.Reader) error { _, err := io.Copy(io.Discard, r); return err }
	tr2, l2 := newArchiveHarness(t, rt2)
	pr2, pw2 := io.Pipe()
	done2 := make(chan error, 1)
	go func() { done2 <- tr2.CopyToContainer(context.Background(), "job-1", "/workspace", pr2, 0) }()
	_, _ = pw2.Write([]byte("partial"))
	time.Sleep(50 * time.Millisecond)
	l2.close()
	select {
	case err := <-done2:
		if err == nil {
			t.Error("copy-in over a dropped session reported success")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("copy-in did not return after the session dropped")
	}
	_ = l
}

func TestLocalForwardsArchiveCopy(t *testing.T) {
	rt := newArchiveRuntime()
	l := NewLocal(rt)
	if err := l.CopyToContainer(context.Background(), "c", "/workspace", strings.NewReader("tar"), 0); err != nil || rt.inGot.String() != "tar" {
		t.Errorf("CopyToContainer = %v, got %q", err, rt.inGot.String())
	}
	if _, _, err := NewLocal(newExecRuntime()).CopyFromContainer(context.Background(), "c", "/w", 0); !errors.Is(err, docker.ErrArchiveUnsupported) {
		t.Errorf("unsupported runtime = %v", err)
	}
}

// Over a real mTLS gRPC session: enroll, run an agent session with the
// fake archive runtime, and copy both directions through the registry's
// transport the way the pipeline engine would.
func TestArchiveCopyOverMTLSSession(t *testing.T) {
	db := openLiveTestStore(t)
	ca, _ := GenerateCA()
	ts := startCertTestServer(t, ca, db)
	id := enrollNode(t, db, ts, "archive-node")

	rt := newArchiveRuntime()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = RunSession(ctx, ts.addr, id, rt, nil) }()

	var tr Transport
	deadline := time.Now().Add(10 * time.Second)
	for {
		got, err := ts.srv.registry.Get(id.NodeID)
		if err == nil {
			tr = got
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("agent session never registered its transport")
		}
		time.Sleep(20 * time.Millisecond)
	}
	ar, ok := tr.(docker.ArchiveRuntime)
	if !ok {
		t.Fatalf("transport %T does not implement ArchiveRuntime", tr)
	}

	payload := patternBytes(4 << 20)
	if err := ar.CopyToContainer(ctx, "job-1", "/workspace", bytes.NewReader(payload), 0); err != nil {
		t.Fatalf("CopyToContainer over mTLS = %v", err)
	}
	if !bytes.Equal(rt.inGot.Bytes(), payload) {
		t.Fatalf("agent received %d bytes, want the %d byte payload", rt.inGot.Len(), len(payload))
	}

	rc, _, err := ar.CopyFromContainer(ctx, "job-1", "/workspace/dist", 0)
	if err != nil {
		t.Fatalf("CopyFromContainer over mTLS = %v", err)
	}
	defer func() { _ = rc.Close() }()
	go func() {
		_, _ = rt.outWriter.Write(payload)
		_ = rt.outWriter.Close()
	}()
	got, err := io.ReadAll(rc)
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("copy-out read %d bytes, err %v", len(got), err)
	}
}

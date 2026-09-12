package agent

// These run the real GRPCTransport and the real ExecRelay against each
// other over the same in-memory loopback exec_test.go establishes, with
// only the PTY itself faked, so the framing a terminal adds on top of
// exec (tty flag, initial size, resize frames, input while output is
// still streaming) is proven without needing a Docker daemon. The real
// PTY behavior is proven separately, live, in exec_tty_live_test.go.

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
)

// fakeTTYSession is a docker.ExecSession the test drives: output comes
// from a pipe the test writes, input lands in a buffer the test reads,
// and every resize is recorded in order.
type fakeTTYSession struct {
	out    *io.PipeReader
	outW   *io.PipeWriter
	closed chan struct{}

	mu      sync.Mutex
	input   strings.Builder
	resizes []docker.TTYSize
}

func newFakeTTYSession() *fakeTTYSession {
	pr, pw := io.Pipe()
	return &fakeTTYSession{out: pr, outW: pw, closed: make(chan struct{})}
}

func (s *fakeTTYSession) Read(p []byte) (int, error) { return s.out.Read(p) }

func (s *fakeTTYSession) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.input.Write(p)
}

func (s *fakeTTYSession) Resize(_ context.Context, size docker.TTYSize) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resizes = append(s.resizes, size)
	return nil
}

func (s *fakeTTYSession) Close() error {
	select {
	case <-s.closed:
	default:
		close(s.closed)
	}
	return s.out.Close()
}

func (s *fakeTTYSession) gotInput() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.input.String()
}

func (s *fakeTTYSession) lastResize() (docker.TTYSize, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.resizes) == 0 {
		return docker.TTYSize{}, false
	}
	return s.resizes[len(s.resizes)-1], true
}

// fakeTTYRuntime is a docker.Runtime that also offers interactive exec,
// handing every ExecTTY call the same test-driven session.
type fakeTTYRuntime struct {
	*execRuntime

	sess    *fakeTTYSession
	started chan struct{}

	mu      sync.Mutex
	opts    docker.ExecTTYOptions
	execErr error
}

func newFakeTTYRuntime() *fakeTTYRuntime {
	return &fakeTTYRuntime{
		execRuntime: newExecRuntime(),
		sess:        newFakeTTYSession(),
		started:     make(chan struct{}),
	}
}

func (f *fakeTTYRuntime) ExecTTY(_ context.Context, _ string, opts docker.ExecTTYOptions) (docker.ExecSession, error) {
	f.mu.Lock()
	f.opts = opts
	execErr := f.execErr
	f.mu.Unlock()
	if execErr != nil {
		return nil, execErr
	}
	close(f.started)
	return f.sess, nil
}

func (f *fakeTTYRuntime) recordedOptions() docker.ExecTTYOptions {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.opts
}

func (f *fakeTTYRuntime) waitStarted(t *testing.T) {
	t.Helper()
	select {
	case <-f.started:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the agent to open the PTY")
	}
}

func openRemoteTTY(t *testing.T, rt docker.Runtime, size docker.TTYSize) docker.ExecSession {
	t.Helper()
	transport := newExecHarness(t, rt)
	tty, ok := any(transport).(docker.TTYRuntime)
	if !ok {
		t.Fatal("GRPCTransport does not implement docker.TTYRuntime")
	}
	sess, err := tty.ExecTTY(context.Background(), "container-1", docker.ExecTTYOptions{
		Cmd:  []string{"sh"},
		Env:  []string{"TERM=xterm-256color"},
		Size: size,
	})
	if err != nil {
		t.Fatalf("ExecTTY() error = %v", err)
	}
	t.Cleanup(func() { _ = sess.Close() })
	return sess
}

func TestExecTTY_RoundTripsOutputInputAndResize(t *testing.T) {
	rt := newFakeTTYRuntime()
	sess := openRemoteTTY(t, rt, docker.TTYSize{Rows: 24, Cols: 80})
	rt.waitStarted(t)

	if got := rt.recordedOptions(); got.Size != (docker.TTYSize{Rows: 24, Cols: 80}) {
		t.Errorf("agent-side initial size = %+v, want 24x80", got.Size)
	}
	if got := rt.recordedOptions().Env; len(got) != 1 || got[0] != "TERM=xterm-256color" {
		t.Errorf("agent-side env = %v, want the requested TERM to have crossed the wire", got)
	}

	go func() { _, _ = rt.sess.outW.Write([]byte("$ ")) }()
	buf := make([]byte, 16)
	n, err := sess.Read(buf)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if string(buf[:n]) != "$ " {
		t.Errorf("Read() = %q, want %q", buf[:n], "$ ")
	}

	if _, err := io.WriteString(sess, "whoami\n"); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	waitFor(t, func() bool { return rt.sess.gotInput() == "whoami\n" }, "keystrokes to reach the PTY")

	if err := sess.Resize(context.Background(), docker.TTYSize{Rows: 40, Cols: 132}); err != nil {
		t.Fatalf("Resize() error = %v", err)
	}
	waitFor(t, func() bool {
		got, ok := rt.sess.lastResize()
		return ok && got == docker.TTYSize{Rows: 40, Cols: 132}
	}, "the resize to reach the PTY")
}

// TestExecTTY_CloseStopsRemoteSession is the leak check: an abandoned
// terminal must close the PTY on the agent side, not leave it attached
// for nobody.
func TestExecTTY_CloseStopsRemoteSession(t *testing.T) {
	rt := newFakeTTYRuntime()
	sess := openRemoteTTY(t, rt, docker.TTYSize{Rows: 24, Cols: 80})
	rt.waitStarted(t)

	if err := sess.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	select {
	case <-rt.sess.closed:
	case <-time.After(5 * time.Second):
		t.Fatal("the agent-side PTY was never closed after the caller closed the terminal")
	}
}

// TestExecTTY_UnsupportedRuntime proves a node whose runtime has no
// interactive exec fails the request outright instead of silently
// falling back to a non-interactive one.
func TestExecTTY_UnsupportedRuntime(t *testing.T) {
	transport := newExecHarness(t, newFakeExecRuntime())
	tty, ok := any(transport).(docker.TTYRuntime)
	if !ok {
		t.Fatal("GRPCTransport does not implement docker.TTYRuntime")
	}

	_, err := tty.ExecTTY(context.Background(), "container-1", docker.ExecTTYOptions{
		Cmd:  []string{"sh"},
		Size: docker.TTYSize{Rows: 24, Cols: 80},
	})
	if err == nil {
		t.Fatal("ExecTTY() succeeded against a runtime with no PTY support, want an error")
	}
	if !strings.Contains(err.Error(), "interactive exec") {
		t.Errorf("ExecTTY() error = %v, want it to name the missing interactive-exec support", err)
	}
}

// TestExecTTY_AttachFailureSurfaces proves a PTY that cannot be opened
// fails on the ExecTTY call itself, not on a later read.
func TestExecTTY_AttachFailureSurfaces(t *testing.T) {
	rt := newFakeTTYRuntime()
	rt.execErr = errors.New("no such container")
	transport := newExecHarness(t, rt)
	tty, ok := any(transport).(docker.TTYRuntime)
	if !ok {
		t.Fatal("GRPCTransport does not implement docker.TTYRuntime")
	}

	_, err := tty.ExecTTY(context.Background(), "container-1", docker.ExecTTYOptions{
		Cmd:  []string{"sh"},
		Size: docker.TTYSize{Rows: 24, Cols: 80},
	})
	if err == nil || !strings.Contains(err.Error(), "no such container") {
		t.Fatalf("ExecTTY() error = %v, want the agent-side attach failure", err)
	}
}

func waitFor(t *testing.T, cond func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

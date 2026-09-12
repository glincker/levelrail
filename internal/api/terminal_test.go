package api

// These run the terminal endpoint over a real HTTP server and a real
// WebSocket client, with only the PTY faked, so the upgrade, the
// auth gating, the binary/text frame split, and the teardown are proven
// end to end rather than asserted against a handler's shape.

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeTTYSession is a docker.ExecSession the test drives directly.
type fakeTTYSession struct {
	out    chan []byte
	closed chan struct{}

	mu      sync.Mutex
	input   strings.Builder
	resizes []docker.TTYSize

	rem []byte
}

func newFakeTTYSession() *fakeTTYSession {
	return &fakeTTYSession{out: make(chan []byte, 8), closed: make(chan struct{})}
}

func (s *fakeTTYSession) Read(p []byte) (int, error) {
	if len(s.rem) == 0 {
		select {
		case chunk, ok := <-s.out:
			if !ok {
				return 0, io.EOF
			}
			s.rem = chunk
		case <-s.closed:
			return 0, io.EOF
		}
	}
	n := copy(p, s.rem)
	s.rem = s.rem[n:]
	return n, nil
}

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
	return nil
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

// fakeTTYAppRuntime is fakeExecAppRuntime plus interactive exec.
type fakeTTYAppRuntime struct {
	*fakeExecAppRuntime

	sess    *fakeTTYSession
	started chan struct{}

	mu   sync.Mutex
	opts docker.ExecTTYOptions
}

func newFakeTTYAppRuntime() *fakeTTYAppRuntime {
	return &fakeTTYAppRuntime{
		fakeExecAppRuntime: &fakeExecAppRuntime{inspectState: &docker.ContainerState{ID: "c1", Running: true}},
		sess:               newFakeTTYSession(),
		started:            make(chan struct{}),
	}
}

func (f *fakeTTYAppRuntime) ExecTTY(_ context.Context, _ string, opts docker.ExecTTYOptions) (docker.ExecSession, error) {
	f.mu.Lock()
	f.opts = opts
	f.mu.Unlock()
	close(f.started)
	return f.sess, nil
}

func (f *fakeTTYAppRuntime) recordedOptions() docker.ExecTTYOptions {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.opts
}

// dialTestTerminal starts a real server for rt and opens an
// authenticated terminal WebSocket against it.
func dialTestTerminal(t *testing.T, rt *Router, cookie *http.Cookie, query string) *websocket.Conn {
	t.Helper()
	server := httptest.NewServer(rt.Handler())
	t.Cleanup(server.Close)

	url := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/apps/web/terminal" + query
	conn, _, err := websocket.Dial(context.Background(), url, &websocket.DialOptions{
		HTTPHeader: http.Header{"Cookie": []string{cookie.String()}},
	})
	if err != nil {
		t.Fatalf("dial terminal: %v", err)
	}
	t.Cleanup(func() { _ = conn.CloseNow() })
	return conn
}

func newTerminalTestRouter(t *testing.T, fake *fakeTTYAppRuntime) (*Router, *http.Cookie) {
	t.Helper()
	db := openTestDB(t)
	resolver := func(string) (docker.Runtime, error) { return fake, nil }
	rt := NewRouter(discardLogger(), testBrand(), db, WithExecRuntime(resolver))
	cookie := loginTestSession(t, rt, db)
	svc := store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}
	if err := db.SaveDesiredService(context.Background(), svc); err != nil {
		t.Fatalf("seed app: %v", err)
	}
	return rt, cookie
}

func TestHandleAppTerminal_RoundTripsBytesAndResize(t *testing.T) {
	fake := newFakeTTYAppRuntime()
	rt, cookie := newTerminalTestRouter(t, fake)
	conn := dialTestTerminal(t, rt, cookie, "?rows=40&cols=132")

	select {
	case <-fake.started:
	case <-time.After(5 * time.Second):
		t.Fatal("the terminal session was never opened against the runtime")
	}
	if got := fake.recordedOptions().Size; got != (docker.TTYSize{Rows: 40, Cols: 132}) {
		t.Errorf("initial PTY size = %+v, want 40x132 from the query string", got)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Output: a binary frame carrying the PTY's bytes verbatim.
	fake.sess.out <- []byte("web:/app$ ")
	kind, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if kind != websocket.MessageBinary {
		t.Errorf("output frame type = %v, want binary", kind)
	}
	if string(data) != "web:/app$ " {
		t.Errorf("output frame = %q, want %q", data, "web:/app$ ")
	}

	// Input: a binary frame of keystrokes reaches the PTY unchanged.
	if err := conn.Write(ctx, websocket.MessageBinary, []byte("ls -la\r")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	waitForCondition(t, func() bool { return fake.sess.gotInput() == "ls -la\r" }, "keystrokes to reach the PTY")

	// Control: a text frame resizes it.
	resize, err := json.Marshal(terminalControl{Type: "resize", Rows: 50, Cols: 200})
	if err != nil {
		t.Fatalf("marshal resize: %v", err)
	}
	if err := conn.Write(ctx, websocket.MessageText, resize); err != nil {
		t.Fatalf("Write(resize) error = %v", err)
	}
	waitForCondition(t, func() bool {
		got, ok := fake.sess.lastResize()
		return ok && got == docker.TTYSize{Rows: 50, Cols: 200}
	}, "the resize to reach the PTY")
}

// TestHandleAppTerminal_ClientCloseStopsSession is the leak check: a
// browser tab closing must end the shell, not orphan it.
func TestHandleAppTerminal_ClientCloseStopsSession(t *testing.T) {
	fake := newFakeTTYAppRuntime()
	rt, cookie := newTerminalTestRouter(t, fake)
	conn := dialTestTerminal(t, rt, cookie, "")

	select {
	case <-fake.started:
	case <-time.After(5 * time.Second):
		t.Fatal("the terminal session was never opened against the runtime")
	}

	_ = conn.Close(websocket.StatusNormalClosure, "tab closed")
	select {
	case <-fake.sess.closed:
	case <-time.After(5 * time.Second):
		t.Fatal("the PTY session outlived the client that opened it")
	}
}

// TestHandleAppTerminal_ShellExitReportsExitCode proves the session's
// end is reported as a structured text frame, since a raw byte stream
// has no way to carry an exit code.
func TestHandleAppTerminal_ShellExitReportsExitCode(t *testing.T) {
	fake := newFakeTTYAppRuntime()
	rt, cookie := newTerminalTestRouter(t, fake)
	conn := dialTestTerminal(t, rt, cookie, "")

	select {
	case <-fake.started:
	case <-time.After(5 * time.Second):
		t.Fatal("the terminal session was never opened against the runtime")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	close(fake.sess.out)

	kind, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if kind != websocket.MessageText {
		t.Fatalf("final frame type = %v, want text", kind)
	}
	var event terminalEvent
	if err := json.Unmarshal(data, &event); err != nil {
		t.Fatalf("unmarshal exit event: %v", err)
	}
	if event.Type != "exit" || event.ExitCode == nil || *event.ExitCode != 0 {
		t.Errorf("exit event = %+v, want type exit with exit_code 0", event)
	}
}

// TestHandleAppTerminal_NotSupportedOnNode proves a node whose runtime
// has no PTY support is refused before any upgrade, rather than silently
// handed a non-interactive exec.
func TestHandleAppTerminal_NotSupportedOnNode(t *testing.T) {
	fake := &fakeExecAppRuntime{inspectState: &docker.ContainerState{ID: "c1", Running: true}}
	rt, db := newTestRouterWithExecRuntime(t, fake)
	cookie := loginTestSession(t, rt, db)
	seedExecApp(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/terminal", ""))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNotImplemented, rec.Body.String())
	}
}

// TestTerminalRoute_RequiresRootAbility proves the terminal sits behind
// the same AbilityRoot tier one-off exec does, not a lower one.
func TestTerminalRoute_RequiresRootAbility(t *testing.T) {
	fake := newFakeTTYAppRuntime()
	rt, _ := newTerminalTestRouter(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/web/terminal", nil)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func waitForCondition(t *testing.T, cond func() bool, what string) {
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

package dockerguard

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	dockerclient "github.com/docker/docker/client"
)

// fakeDaemon is a tiny Engine API stand-in on a Unix socket: enough
// endpoints to prove streaming, hijacking and body forwarding.
type fakeDaemon struct {
	socket string
	mu     sync.Mutex
	seen   []seenRequest
	// logRelease gates the second log line so a test can prove the first
	// one was flushed through the guard before the stream ended.
	logRelease chan struct{}
}

type seenRequest struct {
	Method, Path string
	Body         []byte
}

func shortTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "dg")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func newFakeDaemon(t *testing.T) *fakeDaemon {
	t.Helper()
	f := &fakeDaemon{socket: filepath.Join(shortTempDir(t), "d.sock"), logRelease: make(chan struct{})}
	ln, err := net.Listen("unix", f.socket)
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: http.HandlerFunc(f.serve), ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	return f
}

func (f *fakeDaemon) requests() []seenRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]seenRequest(nil), f.seen...)
}

func (f *fakeDaemon) serve(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	f.mu.Lock()
	f.seen = append(f.seen, seenRequest{Method: r.Method, Path: r.URL.Path, Body: body})
	f.mu.Unlock()
	path := versionless(r.URL.Path)
	w.Header().Set("Api-Version", "1.47")
	switch {
	case path == "/_ping":
		_, _ = io.WriteString(w, "OK")
	case path == "/events":
		f.streamEvents(w, r)
	case strings.HasSuffix(path, "/logs"):
		f.streamLogs(w, r)
	case path == "/containers/create":
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"Id":"c1","Warnings":[]}`)
	case strings.HasSuffix(path, "/exec") && r.Method == http.MethodPost:
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"Id":"e1"}`)
	case strings.HasPrefix(path, "/exec/") && strings.HasSuffix(path, "/start"):
		f.hijackEcho(w)
	default:
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `[]`)
	}
}

func versionless(p string) string {
	if strings.HasPrefix(p, "/v1.") {
		if i := strings.Index(p[1:], "/"); i >= 0 {
			return p[i+1:]
		}
	}
	return p
}

func (f *fakeDaemon) streamEvents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	flusher := w.(http.Flusher)
	for i := 0; i < 3; i++ {
		_, _ = fmt.Fprintf(w, `{"Type":"container","Action":"start","Actor":{"ID":"c%d"},"time":%d}`+"\n", i, time.Now().Unix())
		flusher.Flush()
	}
	<-r.Context().Done()
}

func (f *fakeDaemon) streamLogs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	flusher := w.(http.Flusher)
	_, _ = io.WriteString(w, "line one\n")
	flusher.Flush()
	select {
	case <-f.logRelease:
	case <-r.Context().Done():
		return
	}
	_, _ = io.WriteString(w, "line two\n")
	flusher.Flush()
}

func (f *fakeDaemon) hijackEcho(w http.ResponseWriter) {
	conn, rw, err := http.NewResponseController(w).Hijack()
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()
	_, _ = rw.WriteString("HTTP/1.1 101 UPGRADED\r\nContent-Type: application/vnd.docker.raw-stream\r\nConnection: Upgrade\r\nUpgrade: tcp\r\n\r\n")
	_ = rw.Flush()
	line, err := bufio.NewReader(rw).ReadString('\n')
	if err != nil {
		return
	}
	_, _ = rw.WriteString("echo: " + line)
	_ = rw.Flush()
}

// memorySink collects decisions for assertions.
type memorySink struct {
	mu        sync.Mutex
	decisions []Decision
}

func (m *memorySink) RecordDecision(_ context.Context, d Decision) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.decisions = append(m.decisions, d)
	return nil
}

func (m *memorySink) all() []Decision {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]Decision(nil), m.decisions...)
}

type guardHarness struct {
	daemon *fakeDaemon
	server *Server
	sink   *memorySink
	grants *Grants
}

func startGuard(t *testing.T, mode Mode) *guardHarness {
	t.Helper()
	d := newFakeDaemon(t)
	sink := &memorySink{}
	grants := NewGrants()
	tun, _ := TunablesFromEnv(func(string) (string, bool) { return "", false })
	g := New(Config{
		Mode: mode, Upstream: d.socket, Grants: grants, Tunables: tun, Sink: sink,
		Policy: PolicyFromHardening(dockerHardeningForTest(), tun, "/srv/levelrail-test-data"),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	ctx, cancel := context.WithCancel(context.Background())
	srv, err := Listen(ctx, g, filepath.Join(shortTempDir(t), "g", SocketName))
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cancel()
		_ = srv.Close()
	})
	return &guardHarness{daemon: d, server: srv, sink: sink, grants: grants}
}

func (h *guardHarness) sdk(t *testing.T) *dockerclient.Client {
	t.Helper()
	cli, err := dockerclient.NewClientWithOpts(dockerclient.WithHost(h.server.Host()), dockerclient.WithVersion("1.47"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cli.Close() })
	return cli
}

func (h *guardHarness) raw() *http.Client {
	return &http.Client{Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, "unix", h.server.Socket)
	}}}
}

// waitDecisions waits for the async recorder to deliver n decisions.
func (h *guardHarness) waitDecisions(t *testing.T, n int) []Decision {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if got := h.sink.all(); len(got) >= n {
			return got
		}
		time.Sleep(5 * time.Millisecond)
	}
	got := h.sink.all()
	t.Fatalf("want %d decisions, got %d: %+v", n, len(got), got)
	return nil
}

func decodeMessage(t *testing.T, r io.Reader) string {
	t.Helper()
	var m struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r).Decode(&m); err != nil {
		t.Fatalf("decode docker error body: %v", err)
	}
	return m.Message
}

package ingress

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glincker/levelrail/internal/experimental"
)

type pipeStream struct{ net.Conn }

func (p pipeStream) CloseWrite() error {
	if cw, ok := p.Conn.(interface{ CloseWrite() error }); ok {
		return cw.CloseWrite()
	}
	return nil
}

// fakeDialer serves each tunnel with handler on a real loopback TCP pair so
// half-close is genuine.
type fakeDialer struct {
	mu      sync.Mutex
	targets []TunnelTarget
	err     error
	handler func(net.Conn)
	opens   atomic.Int64
}

func (f *fakeDialer) OpenTunnel(_ context.Context, node, svc string, port int) (TunnelStream, error) {
	f.opens.Add(1)
	f.mu.Lock()
	f.targets = append(f.targets, TunnelTarget{node, svc, port})
	err := f.err
	f.mu.Unlock()
	if err != nil {
		return nil, err
	}
	ln, _ := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	defer func() { _ = ln.Close() }()
	c, err := (&net.Dialer{}).DialContext(context.Background(), "tcp", ln.Addr().String())
	if err != nil {
		return nil, err
	}
	srv, err := ln.Accept()
	if err != nil {
		return nil, err
	}
	go func() {
		defer func() { _ = srv.Close() }()
		f.handler(srv)
	}()
	return pipeStream{c}, nil
}

func echo(c net.Conn) { _, _ = io.Copy(c, c) }

func newGW(t *testing.T, d TunnelDialer) *TunnelGateway {
	t.Helper()
	experimental.Set(experimental.AgentIngressTunnel)
	t.Cleanup(experimental.Reset)
	root, err := os.MkdirTemp("", "lrgw")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	dir := filepath.Join(root, "s")
	g, err := NewTunnelGateway(dir, d, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = g.Close() })
	return g
}

func dialSock(t *testing.T, p string) *net.UnixConn {
	t.Helper()
	c, err := (&net.Dialer{}).DialContext(context.Background(), "unix", p)
	if err != nil {
		t.Fatal(err)
	}
	return c.(*net.UnixConn)
}

var tgt = TunnelTarget{"node-a", "web", 8080}

func TestGatewayHTTPAndBinding(t *testing.T) {
	d := &fakeDialer{handler: func(c net.Conn) {
		req, err := http.ReadRequest(bufio.NewReader(c))
		if err != nil {
			return
		}
		_, _ = io.WriteString(c, "HTTP/1.1 200 OK\r\nContent-Length: 2\r\nConnection: close\r\n\r\nhi")
		_ = req
	}}
	g := newGW(t, d)
	p, err := g.Register(tgt)
	if err != nil {
		t.Fatal(err)
	}
	c := dialSock(t, p)
	defer func() { _ = c.Close() }()
	// The request tries to name another node and port; it must not matter.
	_, _ = io.WriteString(c, "GET / HTTP/1.1\r\nHost: x\r\nX-Node: other\r\nX-Port: 22\r\n\r\n")
	resp, err := http.ReadResponse(bufio.NewReader(c), nil)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	if string(b) != "hi" || resp.StatusCode != 200 {
		t.Fatalf("got %d %q", resp.StatusCode, b)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.targets) != 1 || d.targets[0] != tgt {
		t.Fatalf("target %+v", d.targets)
	}
}

func TestGatewayDistinctSocketsPerTarget(t *testing.T) {
	d := &fakeDialer{handler: echo}
	g := newGW(t, d)
	other := TunnelTarget{"node-b", "web", 8080}
	p1, _ := g.Register(tgt)
	p2, _ := g.Register(other)
	if p1 == p2 {
		t.Fatal("same socket for two targets")
	}
	c := dialSock(t, p2)
	_, _ = c.Write([]byte("x"))
	buf := make([]byte, 1)
	_, _ = io.ReadFull(c, buf)
	_ = c.Close()
	if d.targets[0] != other {
		t.Fatalf("reached %+v", d.targets[0])
	}
}

func TestGatewayBidirectionalAndHalfClose(t *testing.T) {
	d := &fakeDialer{handler: func(c net.Conn) {
		b, _ := io.ReadAll(c) // returns only after client half-close
		_, _ = c.Write([]byte("got:" + string(b)))
	}}
	g := newGW(t, d)
	p, _ := g.Register(tgt)
	c := dialSock(t, p)
	defer func() { _ = c.Close() }()
	_, _ = c.Write([]byte("ping"))
	_ = c.CloseWrite()
	b, err := io.ReadAll(c)
	if err != nil || string(b) != "got:ping" {
		t.Fatalf("%q %v", b, err)
	}
}

func TestGatewayLargeBodyBackPressure(t *testing.T) {
	const size = 32 << 20
	var got atomic.Int64
	d := &fakeDialer{handler: func(c net.Conn) {
		n, _ := io.Copy(io.Discard, c)
		got.Store(n)
		_, _ = c.Write([]byte("ok"))
	}}
	g := newGW(t, d)
	p, _ := g.Register(tgt)
	c := dialSock(t, p)
	defer func() { _ = c.Close() }()
	go func() {
		chunk := make([]byte, 64*1024)
		for sent := 0; sent < size; sent += len(chunk) {
			if _, err := c.Write(chunk); err != nil {
				return
			}
		}
		_ = c.CloseWrite()
	}()
	b, _ := io.ReadAll(c)
	if string(b) != "ok" || got.Load() != size {
		t.Fatalf("got %d bytes, reply %q", got.Load(), b)
	}
}

func TestGatewayIdleDeadline(t *testing.T) {
	d := &fakeDialer{handler: func(c net.Conn) { _, _ = io.Copy(io.Discard, c) }}
	g := newGW(t, d)
	g.idleTimeout = 100 * time.Millisecond
	p, _ := g.Register(tgt)
	c := dialSock(t, p)
	defer func() { _ = c.Close() }()
	_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, err := c.Read(make([]byte, 1)); err == nil || errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("expected server-side close, got %v", err)
	}
}

func TestGatewayOpenErrors(t *testing.T) {
	cases := []struct {
		err  error
		want int
	}{
		{ErrTunnelUnsupported, 503}, {ErrTunnelOffline, 503},
		{errors.New("refused"), 502}, {context.DeadlineExceeded, 502},
	}
	for _, tc := range cases {
		g := newGW(t, &fakeDialer{err: tc.err, handler: echo})
		p, _ := g.Register(tgt)
		c := dialSock(t, p)
		_, _ = io.WriteString(c, "GET / HTTP/1.1\r\nHost: x\r\n\r\n")
		resp, err := http.ReadResponse(bufio.NewReader(c), nil)
		if err != nil || resp.StatusCode != tc.want {
			t.Fatalf("%v: %v %v", tc.err, resp, err)
		}
		_ = c.Close()
		_ = g.Close()
	}
}

func TestGatewayConcurrencyCap(t *testing.T) {
	release := make(chan struct{})
	d := &fakeDialer{handler: func(_ net.Conn) { <-release }}
	g := newGW(t, d)
	g.maxPerTarget = 2
	p, _ := g.Register(tgt)
	var held []net.Conn
	for i := 0; i < 2; i++ {
		held = append(held, dialSock(t, p))
	}
	deadline := time.Now().Add(2 * time.Second)
	for d.opens.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	c := dialSock(t, p)
	_, _ = io.WriteString(c, "GET / HTTP/1.1\r\nHost: x\r\n\r\n")
	resp, err := http.ReadResponse(bufio.NewReader(c), nil)
	if err != nil || resp.StatusCode != 503 {
		t.Fatalf("%v %v", resp, err)
	}
	_ = c.Close()
	close(release)
	for _, h := range held {
		_ = h.Close()
	}
}

func TestGatewaySocketPermsAndStaleCleanup(t *testing.T) {
	experimental.Set(experimental.AgentIngressTunnel)
	t.Cleanup(experimental.Reset)
	dir := filepath.Join(os.TempDir(), "lrgwperm")
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	_ = os.RemoveAll(dir)
	_ = os.MkdirAll(dir, 0o750)
	stale := filepath.Join(dir, "old.sock")
	l, _ := (&net.ListenConfig{}).Listen(context.Background(), "unix", stale)
	l.(*net.UnixListener).SetUnlinkOnClose(false)
	_ = l.Close()
	keep := filepath.Join(dir, "notes.txt")
	_ = os.WriteFile(keep, []byte("x"), 0o600)

	g, err := NewTunnelGateway(dir, &fakeDialer{handler: echo}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = g.Close() }()
	if _, err := os.Lstat(stale); !os.IsNotExist(err) {
		t.Fatal("stale socket kept")
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatal("non-socket removed")
	}
	if fi, _ := os.Stat(dir); fi.Mode().Perm() != 0o700 {
		t.Fatalf("dir %v", fi.Mode())
	}
	p, _ := g.Register(tgt)
	if fi, _ := os.Stat(p); fi.Mode().Perm() != 0o600 {
		t.Fatalf("sock %v", fi.Mode())
	}
}

func TestGatewaySyncRemovesDroppedTargets(t *testing.T) {
	g := newGW(t, &fakeDialer{handler: echo})
	p, _ := g.Register(tgt)
	if err := g.Sync(nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(p); !os.IsNotExist(err) {
		t.Fatal("socket kept after sync")
	}
}

func TestGatewayFlagOffCreatesNothing(t *testing.T) {
	experimental.Reset()
	dir := filepath.Join(os.TempDir(), "lrgwoff")
	_ = os.RemoveAll(dir)
	g, err := NewTunnelGateway(dir, &fakeDialer{}, nil)
	if g != nil || !errors.Is(err, ErrTunnelGatewayDisabled) {
		t.Fatalf("%v %v", g, err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("dir created with flag off")
	}
}

func TestGatewayNoGoroutineLeak(t *testing.T) {
	before := runtime.NumGoroutine()
	d := &fakeDialer{handler: echo}
	g := newGW(t, d)
	p, _ := g.Register(tgt)
	for i := 0; i < 5; i++ {
		c := dialSock(t, p)
		_, _ = c.Write([]byte("a"))
		_, _ = io.ReadFull(c, make([]byte, 1))
		_ = c.Close()
	}
	_ = g.Close()
	deadline := time.Now().Add(3 * time.Second)
	for runtime.NumGoroutine() > before+2 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if n := runtime.NumGoroutine(); n > before+2 {
		t.Fatalf("goroutines %d -> %d", before, n)
	}
}

func TestGatewayTimeoutIs504(t *testing.T) {
	g := newGW(t, &fakeDialer{err: ErrTunnelTimeout, handler: echo})
	p, _ := g.Register(tgt)
	c := dialSock(t, p)
	_, _ = io.WriteString(c, "GET / HTTP/1.1\r\nHost: x\r\n\r\n")
	resp, err := http.ReadResponse(bufio.NewReader(c), nil)
	if err != nil || resp.StatusCode != 504 {
		t.Fatalf("%v %v", resp, err)
	}
	_ = c.Close()
}

func TestGatewayRefusesSymlinkSocketDir(t *testing.T) {
	experimental.Set(experimental.AgentIngressTunnel)
	t.Cleanup(experimental.Reset)
	root := t.TempDir()
	realDir := filepath.Join(root, "real")
	if err := os.Mkdir(realDir, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(realDir, link); err != nil {
		t.Skip("symlinks unavailable")
	}
	if _, err := NewTunnelGateway(link, &fakeDialer{handler: echo}, nil); err == nil {
		t.Fatal("symlinked socket dir accepted")
	}
	// A real directory we own with a loose mode is tightened to 0700.
	open := filepath.Join(root, "open")
	if err := os.Mkdir(open, 0o755); err != nil { //nolint:gosec // the test needs a too-open mode
		t.Fatal(err)
	}
	g, err := NewTunnelGateway(open, &fakeDialer{handler: echo}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = g.Close() }()
	if fi, _ := os.Stat(open); fi.Mode().Perm() != 0o700 {
		t.Fatalf("dir mode %v", fi.Mode())
	}
}

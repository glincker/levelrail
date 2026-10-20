package ingress

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"
)

// TestDriver_Layer4Stream_RawTCP is this feature's core proof, the exact
// same "nothing here is mocked" shape TestDriver_ReverseProxy_PlainHTTP
// already establishes for the HTTP app: a real backend (a plain TCP
// echo listener, standing in for a container's published port), a real
// in-process Caddy instance (via Driver.Apply, caddy.Load, no `caddy`
// binary), and a real net.Dial client sending bytes through Caddy's
// layer4 listener and reading the echoed response back.
func TestDriver_Layer4Stream_RawTCP(t *testing.T) {
	skipUnderLoad(t)

	backend, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen backend: %v", err)
	}
	defer func() { _ = backend.Close() }()
	go func() {
		for {
			conn, err := backend.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer func() { _ = c.Close() }()
				buf := make([]byte, 4096)
				for {
					n, err := c.Read(buf)
					if n > 0 {
						if _, werr := c.Write(buf[:n]); werr != nil {
							return
						}
					}
					if err != nil {
						return
					}
				}
			}(conn)
		}
	}()

	backendAddr := backend.Addr().String()
	caddyPort := freePort(t)
	caddyAddr := fmt.Sprintf("127.0.0.1:%d", caddyPort)

	cfg, err := BuildRoutesConfig(RoutesOptions{
		ServerName:  "spike-layer4",
		ListenAddr:  fmt.Sprintf("127.0.0.1:%d", freePort(t)), // HTTP server, unused by this test, just needs a free port of its own
		AdminListen: fmt.Sprintf("127.0.0.1:%d", freePort(t)),
		Streams: []StreamRoute{
			{ListenAddr: caddyAddr, BackendDial: backendAddr},
		},
	})
	if err != nil {
		t.Fatalf("BuildRoutesConfig() error: %v", err)
	}

	d := New(testLogger(t))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := d.Apply(ctx, cfg); err != nil {
		t.Fatalf("Driver.Apply() error: %v", err)
	}
	defer func() {
		if err := d.Stop(ctx); err != nil {
			t.Errorf("Driver.Stop() error: %v", err)
		}
	}()

	conn := dialWithRetry(t, caddyAddr)
	defer func() { _ = conn.Close() }()

	const want = "hello through the layer4 stream"
	if _, err := conn.Write([]byte(want)); err != nil {
		t.Fatalf("write to caddy layer4 listener: %v", err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	buf := make([]byte, len(want))
	if _, err := readFull(conn, buf); err != nil {
		t.Fatalf("read echoed response: %v", err)
	}
	if string(buf) != want {
		t.Errorf("echoed response = %q, want %q", string(buf), want)
	}
}

// dialWithRetry mirrors getBodyWithRetry's own "Caddy's listener takes a
// moment to come up after Apply returns" retry shape, for a raw TCP
// dial instead of an HTTP GET.
func dialWithRetry(t *testing.T, addr string) net.Conn {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			return conn
		}
		lastErr = err
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("dial %s: %v", addr, lastErr)
	return nil
}

func readFull(conn net.Conn, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := conn.Read(buf[total:])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

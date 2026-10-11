package ingress

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestStandaloneRoutes_Live boots the embedded Caddy with standalone domain
// routes and proves each upstream option against real local servers.
func TestStandaloneRoutes_Live(t *testing.T) {
	skipUnderLoad(t)
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Seen-Host", r.Host)
		w.Header().Set("X-Seen-Upgrade", r.Header.Get("Upgrade"))
		if r.Header.Get("Upgrade") == "websocket" {
			hj, ok := w.(http.Hijacker)
			if !ok {
				http.Error(w, "no hijack", http.StatusInternalServerError)
				return
			}
			conn, buf, err := hj.Hijack()
			if err != nil {
				return
			}
			defer func() { _ = conn.Close() }()
			_, _ = buf.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\nhello-ws")
			_ = buf.Flush()
			return
		}
		_, _ = fmt.Fprintf(w, "plain %s", r.URL.Path) //nolint:gosec // test upstream echoes the path
	}))
	defer plain.Close()
	secure := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Seen-Host", r.Host)
		_, _ = fmt.Fprintf(w, "secure %s", r.URL.Path) //nolint:gosec // test upstream echoes the path
	}))
	defer secure.Close()

	routes := []ProxyRoute{
		{Hosts: []string{"port.test"}, BackendDial: plain.Listener.Addr().String(), Upstream: &UpstreamOptions{ResponseHeaderTimeout: 5 * time.Second}},
		{Hosts: []string{"strip.test"}, BackendDial: plain.Listener.Addr().String(), Upstream: &UpstreamOptions{StripPrefix: "/app"}},
		{Hosts: []string{"nows.test"}, BackendDial: plain.Listener.Addr().String(), Upstream: &UpstreamOptions{DisableWebSocket: true}},
		{Hosts: []string{"tls.test"}, BackendDial: secure.Listener.Addr().String(), Upstream: &UpstreamOptions{TLS: true, InsecureSkipVerify: true, HostFromUpstream: true}},
		{Hosts: []string{"strict.test"}, BackendDial: secure.Listener.Addr().String(), Upstream: &UpstreamOptions{TLS: true}},
	}
	addr := fmt.Sprintf("127.0.0.1:%d", freePort(t))
	d := New(testLogger(t))
	t.Cleanup(func() { _ = d.Stop(context.Background()) })
	apply := func(rs []ProxyRoute) {
		t.Helper()
		cfg, err := BuildRoutesConfig(RoutesOptions{ServerName: "standalone", ListenAddr: addr, Routes: rs, HTTPPort: freePort(t), StorageDir: t.TempDir()})
		if err != nil {
			t.Fatal(err)
		}
		cfg.Apps.HTTP.Servers["standalone"].AutomaticHTTPS = &AutoHTTPSConfig{Disabled: true}
		if err := d.Apply(context.Background(), cfg); err != nil {
			t.Fatalf("apply: %v", err)
		}
	}
	apply(routes)

	get := func(host, path string) (*http.Response, string) {
		t.Helper()
		req, _ := http.NewRequest(http.MethodGet, "http://"+addr+path, nil)
		req.Host = host
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		return resp, readAll(resp)
	}

	if resp, body := get("port.test", "/hello"); body != "plain /hello" || resp.Header.Get("X-Seen-Host") != "port.test" {
		t.Errorf("port target = %q, host %q (want the client's Host preserved)", body, resp.Header.Get("X-Seen-Host"))
	}
	if _, body := get("strip.test", "/app/inner"); body != "plain /inner" {
		t.Errorf("strip prefix = %q, want plain /inner", body)
	}
	if _, body := get("strip.test", "/other"); body != "plain /other" {
		t.Errorf("strip prefix must leave other paths alone, got %q", body)
	}
	if resp, body := get("tls.test", "/x"); body != "secure /x" || resp.Header.Get("X-Seen-Host") != secure.Listener.Addr().String() {
		t.Errorf("upstream TLS = %d %q host %q", resp.StatusCode, body, resp.Header.Get("X-Seen-Host"))
	}
	if resp, _ := get("strict.test", "/x"); resp.StatusCode != http.StatusBadGateway {
		t.Errorf("verify on against a self-signed upstream = %d, want 502", resp.StatusCode)
	}

	if status, rest := rawUpgrade(t, addr, "port.test"); !strings.Contains(status, "101") || !strings.Contains(rest, "hello-ws") {
		t.Errorf("websocket upgrade = %q %q, want 101 and the upstream bytes", status, rest)
	}
	if resp, _ := get("nows.test", "/"); resp.Header.Get("X-Seen-Upgrade") != "" {
		t.Errorf("websocket disabled must strip Upgrade, upstream saw %q", resp.Header.Get("X-Seen-Upgrade"))
	}
	if status, _ := rawUpgrade(t, addr, "nows.test"); strings.Contains(status, "101") {
		t.Errorf("websocket disabled still upgraded: %q", status)
	}

	apply(routes[1:])
	if _, body := get("port.test", "/hello"); body == "plain /hello" {
		t.Error("deleted standalone domain still reaches its target")
	}
}

func rawUpgrade(t *testing.T, addr, host string) (status, rest string) {
	t.Helper()
	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	_, _ = fmt.Fprintf(conn, "GET /ws HTTP/1.1\r\nHost: %s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\nSec-WebSocket-Version: 13\r\n\r\n", host)
	r := bufio.NewReader(conn)
	status, _ = r.ReadString('\n')
	for {
		line, err := r.ReadString('\n')
		if err != nil || line == "\r\n" {
			break
		}
	}
	buf := make([]byte, 64)
	n, _ := r.Read(buf)
	return status, string(buf[:n])
}

package api

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

func echoListener(t *testing.T) int {
	t.Helper()
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() { defer func() { _ = c.Close() }(); _, _ = io.Copy(c, c) }()
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port
}

func newAppTunnelRouter(t *testing.T, routedPort int) (*Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	state := &docker.ContainerState{ID: "c1", Running: true, Networks: []docker.ContainerNetwork{{Name: "bridge", IPAddress: "127.0.0.1"}}}
	fake := &fakeExecAppRuntime{inspectState: state}
	rt := NewRouter(discardLogger(), testBrand(), db, WithExecRuntime(func(string) (docker.Runtime, error) { return fake, nil }))
	ctx := context.Background()
	seed := []store.DesiredService{
		{Name: "web", Image: "levelrail/web:1", Port: routedPort},
		{Name: "shop-db", Image: "levelrail/db:1", Port: routedPort, AppID: "shop"},
		{Name: "other-db", Image: "levelrail/db:1", Port: routedPort, AppID: "other"},
	}
	for _, id := range []string{"shop", "other"} {
		if err := db.SaveApp(ctx, store.App{ID: id, Name: id, CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-01T00:00:00Z"}); err != nil {
			t.Fatalf("seed app %s: %v", id, err)
		}
	}
	for _, s := range seed {
		if err := db.SaveDesiredService(ctx, s); err != nil {
			t.Fatalf("seed %s: %v", s.Name, err)
		}
	}
	return rt, db
}

func TestAppTunnel_RelaysBytesAndAudits(t *testing.T) {
	port := echoListener(t)
	rt, db := newAppTunnelRouter(t, port)
	cookie := loginTestSession(t, rt, db)
	srv := httptest.NewServer(rt.Handler())
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ws, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http")+"/api/v1/apps/web/tunnel?port="+fmt.Sprint(port), &websocket.DialOptions{HTTPHeader: http.Header{"Cookie": {cookie.String()}}})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	conn := websocket.NetConn(ctx, ws, websocket.MessageBinary)
	if _, err := conn.Write([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 4)
	if _, err := io.ReadFull(conn, buf); err != nil || string(buf) != "ping" {
		t.Fatalf("echo = %q, %v", buf, err)
	}
	_ = conn.Close()

	waitForCondition(t, func() bool {
		entries, _ := db.ListAuditEntries(context.Background(), 50, nil, store.AuditEntryFilter{})
		for _, e := range entries {
			if e.Method == "app.tunnel.close" && strings.Contains(e.Path, "bytes_in=4,bytes_out=4") {
				return true
			}
		}
		return false
	}, "close audit entry with byte counts")
}

func TestAppTunnel_PortConfinement(t *testing.T) {
	port := echoListener(t)
	rt, db := newAppTunnelRouter(t, port)
	cookie := loginTestSession(t, rt, db)

	for _, target := range []string{
		"/api/v1/apps/web/tunnel?port=22",
		"/api/v1/apps/web/tunnel?port=0",
		"/api/v1/apps/web/tunnel?port=70000",
		"/api/v1/apps/web/tunnel?port=abc",
	} {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, target, ""))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400, body = %s", target, rec.Code, rec.Body.String())
		}
	}
}

func TestAppTunnel_OtherAppsAreUnreachable(t *testing.T) {
	port := echoListener(t)
	rt, db := newAppTunnelRouter(t, port)
	cookie := loginTestSession(t, rt, db)

	for _, target := range []string{
		"/api/v1/apps/web/tunnel?service=shop-db",
		"/api/v1/apps/web/tunnel?service=other-db",
		"/api/v1/apps/shop/tunnel?service=other-db",
		"/api/v1/apps/shop/tunnel",
		"/api/v1/apps/nope/tunnel",
	} {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, target, ""))
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: status = %d, want 404", target, rec.Code)
		}
	}
}

func TestAppTunnel_ViewerDenied(t *testing.T) {
	port := echoListener(t)
	rt, db := newAppTunnelRouter(t, port)
	const plaintext = "viewer-token" //nolint:gosec // fake fixture
	if err := db.SaveAPIToken(context.Background(), store.APIToken{
		ID: "tok_viewer", Name: "viewer", TokenHash: hashToken(plaintext), Abilities: []string{AbilityRead, AbilityReadSensitive}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, bearerRequest(http.MethodGet, "/api/v1/apps/web/tunnel", "", plaintext))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestAppTunnel_KillSwitchAndPerUserLimit(t *testing.T) {
	port := echoListener(t)
	rt, db := newAppTunnelRouter(t, port)
	cookie := loginTestSession(t, rt, db)

	t.Setenv(envAppTunnelEnabled, "false")
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/tunnel", ""))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("disabled status = %d, want 403", rec.Code)
	}

	if !acquireAppTunnelSlot("u1", 1) || acquireAppTunnelSlot("u1", 1) {
		t.Fatal("second slot for the same user must be refused at limit 1")
	}
	releaseAppTunnelSlot("u1")
	if !acquireAppTunnelSlot("u1", 1) {
		t.Fatal("slot must be reusable after release")
	}
	releaseAppTunnelSlot("u1")
}

func TestAppTunnelPortAllowed(t *testing.T) {
	svc := &store.DesiredService{Port: 8080, PublishedPorts: []spec.PublishedPort{{ContainerPort: 5353, HostPort: 5353, Protocol: "udp"}, {ContainerPort: 9000, HostPort: 9000}}}
	cases := []struct {
		port int
		want bool
	}{{8080, true}, {9000, true}, {5353, false}, {3000, true}, {22, false}, {0, false}}
	for _, c := range cases {
		if got := appTunnelPortAllowed(svc, c.port, []string{"3000/tcp", "53/udp"}); got != c.want {
			t.Errorf("port %d = %v, want %v", c.port, got, c.want)
		}
	}
}

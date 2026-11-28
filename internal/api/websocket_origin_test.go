package api

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/glincker/levelrail/internal/cliconsole"
	"github.com/glincker/levelrail/internal/docker"
	"github.com/glincker/levelrail/internal/experimental"
	"github.com/glincker/levelrail/internal/reconcile/database"
	"github.com/glincker/levelrail/internal/store"
)

const foreignOrigin = "http://evil.example"

// dialWithOrigin opens an authenticated upgrade with an explicit Origin header.
func dialWithOrigin(t *testing.T, srv *httptest.Server, path string, cookie *http.Cookie, origin string) (int, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	conn, resp, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http")+path, &websocket.DialOptions{
		HTTPHeader: http.Header{"Cookie": {cookie.String()}, "Origin": {origin}},
	})
	status := 0
	if resp != nil {
		status = resp.StatusCode
	}
	if conn != nil {
		t.Cleanup(func() { _ = conn.CloseNow() })
	}
	return status, err
}

// assertOriginGate checks a foreign Origin is refused with 403 and the
// server's own origin gets through the upgrade.
func assertOriginGate(t *testing.T, srv *httptest.Server, path string, cookie *http.Cookie) {
	t.Helper()
	if status, err := dialWithOrigin(t, srv, path, cookie, foreignOrigin); err == nil || status != http.StatusForbidden {
		t.Fatalf("foreign origin: status = %d, err = %v, want 403", status, err)
	}
	if status, err := dialWithOrigin(t, srv, path, cookie, srv.URL); err != nil {
		t.Fatalf("same origin: status = %d, err = %v, want upgrade", status, err)
	}
}

func TestWebSocketOrigin_Terminal(t *testing.T) {
	rt, cookie := newTerminalTestRouter(t, newFakeTTYAppRuntime())
	srv := httptest.NewServer(rt.Handler())
	t.Cleanup(srv.Close)
	assertOriginGate(t, srv, "/api/v1/apps/web/terminal", cookie)
}

func TestWebSocketOrigin_AppTunnel(t *testing.T) {
	port := echoListener(t)
	rt, db := newAppTunnelRouter(t, port)
	cookie := loginTestSession(t, rt, db)
	srv := httptest.NewServer(rt.Handler())
	t.Cleanup(srv.Close)
	assertOriginGate(t, srv, "/api/v1/apps/web/tunnel?port="+strconv.Itoa(port), cookie)
}

func TestWebSocketOrigin_DatabaseTunnel(t *testing.T) {
	// The handler dials the engine's fixed port before upgrading, so
	// hold the first free one on loopback.
	var engine string
	for _, e := range []string{store.EnginePostgres, store.EngineRedis, store.EngineMySQL, store.EngineMongoDB} {
		p, _ := database.ContainerPort(e)
		ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(p)))
		if err != nil {
			continue
		}
		t.Cleanup(func() { _ = ln.Close() })
		go func() {
			for {
				c, err := ln.Accept()
				if err != nil {
					return
				}
				_ = c.Close()
			}
		}()
		engine = e
		break
	}
	if engine == "" {
		t.Skip("no database port free on loopback")
	}

	db := openTestDB(t)
	state := &docker.ContainerState{ID: "c1", Running: true, Networks: []docker.ContainerNetwork{{Name: "bridge", IPAddress: "127.0.0.1"}}}
	fake := &fakeExecAppRuntime{inspectState: state}
	rt := NewRouter(discardLogger(), testBrand(), db, WithExecRuntime(func(string) (docker.Runtime, error) { return fake, nil }))
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: engine, Version: "1"}); err != nil {
		t.Fatalf("seed database: %v", err)
	}
	cookie := loginTestSession(t, rt, db)
	srv := httptest.NewServer(rt.Handler())
	t.Cleanup(srv.Close)
	assertOriginGate(t, srv, "/api/v1/databases/main/tunnel", cookie)
}

func TestWebSocketOrigin_CLIConsole(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("credential drop needs a real unprivileged user")
	}
	bin := filepath.Join(t.TempDir(), "fake-cli")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil { //nolint:gosec // test fixture must be executable
		t.Fatal(err)
	}
	t.Setenv(cliconsole.EnvBinary, bin)
	experimental.Set(experimental.CLIConsole)
	t.Cleanup(func() { experimental.Set(experimental.All()...) })
	rt, db := newTestRouter(t)
	rt.dashboardListenAddr = "127.0.0.1:1"
	cookie := loginTestSession(t, rt, db)
	cliConsoleStepUp(t, rt, cookie)
	srv := httptest.NewServer(rt.Handler())
	t.Cleanup(srv.Close)
	assertOriginGate(t, srv, cliConsoleSessionPath, cookie)
}

package api

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeMinecraft answers every list ping with a fixed status and counts connections.
func fakeMinecraft(t *testing.T) (port int, conns *atomic.Int32) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	conns = &atomic.Int32{}
	js := `{"version":{"name":"1.21.11"},"players":{"online":3,"max":20},"description":"hello"}`
	reply := append([]byte{byte(len(js) + 2), 0x00, byte(len(js))}, js...) //nolint:gosec // small test fixture
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			conns.Add(1)
			go func() {
				defer func() { _ = c.Close() }()
				br := bufio.NewReader(c)
				buf := make([]byte, 512)
				_, _ = br.Read(buf)
				_, _ = c.Write(reply)
			}()
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port, conns
}

func TestGameStatusHandler(t *testing.T) {
	port, conns := fakeMinecraft(t)
	db := openTestDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db, WithPublicHost("203.0.113.9"))
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	for _, name := range []string{"mc", "web"} {
		if err := db.SaveDesiredService(ctx, store.DesiredService{Name: name, Image: "i:1", Port: 25565}); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.SaveAppStream(ctx, store.AppStream{ID: "s1", ServiceName: "mc", ContainerPort: 25565, HostPort: port, Protocol: store.AppStreamProtocolTCP}); err != nil {
		t.Fatal(err)
	}
	get := func(app string) (int, gameStatusResource) {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/"+app+"/game-status", ""))
		var res gameStatusResource
		_ = json.Unmarshal(rec.Body.Bytes(), &res)
		return rec.Code, res
	}

	code, res := get("mc")
	if code != 200 || !res.Online || res.Kind != "minecraft" || res.Version != "1.21.11" || res.Players != 3 || res.MaxPlayers != 20 {
		t.Fatalf("unexpected status %d %+v", code, res)
	}
	if want := "203.0.113.9:" + strconv.Itoa(port); res.JoinAddress != want || res.Exposure != gameExposureStream {
		t.Fatalf("join %q exposure %q, want %q stream", res.JoinAddress, res.Exposure, want)
	}
	before := conns.Load()
	if _, again := get("mc"); !again.Online || conns.Load() != before {
		t.Fatalf("second call within the cache window must not ping again (%d -> %d)", before, conns.Load())
	}

	code, res = get("web")
	if code != 200 || res.Exposure != gameExposureNone || res.Online || res.JoinAddress != "" {
		t.Fatalf("app without a TCP port: %d %+v", code, res)
	}
	if code, _ = get("missing"); code != 404 {
		t.Fatalf("unknown app: want 404, got %d", code)
	}
}

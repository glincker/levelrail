package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/GLINCKER/levelrail/internal/store"
)

func shortenStreamRecheck(t *testing.T) {
	t.Helper()
	old := streamRecheckInterval
	streamRecheckInterval = 20 * time.Millisecond
	t.Cleanup(func() { streamRecheckInterval = old })
}

func waitClosed(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatalf("%s did not end after access was revoked", what)
	}
}

func waitTerminalStarted(t *testing.T, fake *fakeTTYAppRuntime) {
	t.Helper()
	select {
	case <-fake.started:
	case <-time.After(5 * time.Second):
		t.Fatal("terminal never started")
	}
}

func TestTerminal_SessionRevocationEndsOpenSession(t *testing.T) {
	shortenStreamRecheck(t)
	fake := newFakeTTYAppRuntime()
	rt, cookie := newTerminalTestRouter(t, fake)
	_ = dialTestTerminal(t, rt, cookie, "")
	waitTerminalStarted(t, fake)
	rt.sessions.revoke(cookie.Value)
	waitClosed(t, fake.sess.closed, "terminal session")
}

func TestTerminal_IAMDenyAttachedMidSessionEndsOpenSession(t *testing.T) {
	shortenStreamRecheck(t)
	fake := newFakeTTYAppRuntime()
	rt, cookie := newTerminalTestRouter(t, fake)
	_ = dialTestTerminal(t, rt, cookie, "")
	waitTerminalStarted(t, fake)
	userID, ok := rt.sessions.lookup(cookie.Value)
	if !ok {
		t.Fatal("session missing")
	}
	db, ok := rt.policies.(*store.DB)
	if !ok {
		t.Fatal("test router policies is not a *store.DB")
	}
	attachTestPolicy(t, db, "deny-web", "Deny", "*", "app:web", store.PrincipalTypeUser, userID)
	waitClosed(t, fake.sess.closed, "terminal session")
}

func TestTerminal_TokenRevocationEndsOpenSession(t *testing.T) {
	shortenStreamRecheck(t)
	fake := newFakeTTYAppRuntime()
	rt, _ := newTerminalTestRouter(t, fake)
	db, ok := rt.tokens.(*store.DB)
	if !ok {
		t.Fatal("test router tokens is not a *store.DB")
	}
	plain := seedMatrixToken(t, db, "term-tok", []string{AbilityRoot})

	server := httptest.NewServer(rt.Handler())
	t.Cleanup(server.Close)
	url := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/apps/web/terminal"
	conn, _, err := websocket.Dial(context.Background(), url, &websocket.DialOptions{
		HTTPHeader: http.Header{"Authorization": []string{"Bearer " + plain}},
	})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.CloseNow() })
	waitTerminalStarted(t, fake)
	if err := db.RevokeAPIToken(context.Background(), "term-tok"); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	waitClosed(t, fake.sess.closed, "terminal session")
}

func TestTerminal_RejectsCrossOriginUpgrade(t *testing.T) {
	fake := newFakeTTYAppRuntime()
	rt, cookie := newTerminalTestRouter(t, fake)
	server := httptest.NewServer(rt.Handler())
	t.Cleanup(server.Close)
	url := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/apps/web/terminal"

	_, resp, err := websocket.Dial(context.Background(), url, &websocket.DialOptions{
		HTTPHeader: http.Header{"Cookie": []string{cookie.String()}, "Origin": []string{"https://evil.example"}},
	})
	if err == nil {
		t.Fatal("cross-origin websocket upgrade succeeded, want it refused")
	}
	if resp == nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-origin upgrade response = %v, want 403", resp)
	}
	select {
	case <-fake.started:
		t.Fatal("a shell was opened for a cross-origin request")
	case <-time.After(200 * time.Millisecond):
	}
}

func TestTerminal_UnauthenticatedRefusedBeforeUpgrade(t *testing.T) {
	fake := newFakeTTYAppRuntime()
	rt, _ := newTerminalTestRouter(t, fake)
	server := httptest.NewServer(rt.Handler())
	t.Cleanup(server.Close)
	url := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/apps/web/terminal"

	_, resp, err := websocket.Dial(context.Background(), url, nil)
	if err == nil {
		t.Fatal("anonymous websocket upgrade succeeded")
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anonymous upgrade response = %v, want 401", resp)
	}
}

func TestStreamReauth_RevokedTokenEndsBlockedStream(t *testing.T) {
	shortenStreamRecheck(t)
	rt, db, _ := newPipelineRouter(t)
	plain := seedMatrixToken(t, db, "sse-tok", []string{AbilityRead})

	mux := http.NewServeMux()
	mux.HandleFunc("GET /stream/{name}", rt.requireAbilityForResource(AbilityRead, appResourceFromPath,
		rt.withStreamReauth(appResourceFromPath, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			<-r.Context().Done()
		})))
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL+"/stream/web", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+plain)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	ended := make(chan struct{})
	go func() {
		buf := make([]byte, 16)
		for {
			if _, rerr := resp.Body.Read(buf); rerr != nil {
				close(ended)
				return
			}
		}
	}()

	select {
	case <-ended:
		t.Fatal("stream ended before any revocation")
	case <-time.After(100 * time.Millisecond):
	}
	if err := db.RevokeAPIToken(context.Background(), "sse-tok"); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	waitClosed(t, ended, "SSE stream")
}

func TestStreamingRoutesAreWrappedForReauth(t *testing.T) {
	stream := regexp.MustCompile(`mux\.HandleFunc\("[A-Z]+ [^"]+", .*rt\.(handle\w*Stream\w*)`)
	files, err := filepath.Glob("routes*.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("no routes files: %v", err)
	}
	seen := 0
	for _, f := range files {
		src, err := os.ReadFile(f) //nolint:gosec // globbed routes files in the package dir
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(string(src), "\n") {
			if m := stream.FindStringSubmatch(line); m != nil {
				seen++
				if !strings.Contains(line, "withStreamReauth(") {
					t.Errorf("%s registers %s without withStreamReauth, so a revoked caller keeps its stream", f, m[1])
				}
			}
		}
	}
	if seen < 6 {
		t.Fatalf("found only %d streaming routes, the pattern has drifted", seen)
	}
}

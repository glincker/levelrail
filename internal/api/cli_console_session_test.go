package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/GLINCKER/levelrail/internal/cliconsole"
	"github.com/GLINCKER/levelrail/internal/experimental"
)

func TestCLIConsole_SessionRunsAndRevokes(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("credential drop needs a real unprivileged user")
	}
	bin := filepath.Join(t.TempDir(), "fake-cli")
	script := "#!/bin/sh\necho \"args:$*\"\n[ -n \"$APP_API_TOKEN\" ] && echo has-token\necho oops >&2\nexit 2\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil { //nolint:gosec // test fixture must be executable
		t.Fatal(err)
	}
	t.Setenv(cliconsole.EnvBinary, bin)
	experimental.Set(experimental.CLIConsole)
	t.Cleanup(func() { experimental.Set(experimental.All()...) })
	rt, db := newTestRouter(t)
	rt.dashboardListenAddr = "127.0.0.1:1"
	admin := loginTestSession(t, rt, db)
	cliConsoleStepUp(t, rt, admin)

	server := httptest.NewServer(rt.Handler())
	t.Cleanup(server.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+cliConsoleSessionPath, &websocket.DialOptions{
		HTTPHeader: http.Header{"Cookie": []string{admin.String()}, "Origin": []string{server.URL}},
	})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.CloseNow() }()

	read := func() cliServerMessage {
		_, data, err := conn.Read(ctx)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		var m cliServerMessage
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatal(err)
		}
		return m
	}
	send := func(v string) {
		if err := conn.Write(ctx, websocket.MessageText, []byte(v)); err != nil {
			t.Fatal(err)
		}
	}
	if m := read(); m.Type != cliMsgReady {
		t.Fatalf("first message = %+v", m)
	}

	send(`{"type":"run","line":"apps list; id"}`)
	if m := read(); m.Type != cliMsgError {
		t.Fatalf("injection attempt = %+v, want error", m)
	}

	send(`{"type":"run","line":"apps list --json"}`)
	var out strings.Builder
	var exit *int
	for exit == nil {
		m := read()
		switch m.Type {
		case cliMsgOutput:
			out.WriteString(m.Data)
		case cliMsgExit:
			exit = m.Code
		default:
			t.Fatalf("unexpected %+v", m)
		}
	}
	if *exit != 2 || !strings.Contains(out.String(), "args:apps list --json") || !strings.Contains(out.String(), "has-token") || !strings.Contains(out.String(), "oops") {
		t.Fatalf("exit = %d output = %q", *exit, out.String())
	}

	tokens, err := db.ListAPITokens(ctx)
	if err != nil || len(tokens) != 1 || tokens[0].RevokedAt != nil {
		t.Fatalf("live session token = %+v (%v)", tokens, err)
	}
	_ = conn.Close(websocket.StatusNormalClosure, "")
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		tokens, _ = db.ListAPITokens(ctx)
		if len(tokens) == 1 && tokens[0].RevokedAt != nil {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("session token not revoked after the socket closed")
}

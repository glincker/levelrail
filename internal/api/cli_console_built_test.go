package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/GLINCKER/levelrail/internal/cliconsole"
	"github.com/GLINCKER/levelrail/internal/experimental"
)

// Builds only the CLI package so the gates past the binary check run for real:
// refusals still come first, then a session opens and runs a real command.
func TestCLIConsole_GateOrderWithBuiltCLI(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("credential drop needs a real unprivileged user")
	}
	bin := filepath.Join(t.TempDir(), "levelrail-cli")
	build := exec.CommandContext(t.Context(), "go", "build", "-buildvcs=false", "-o", bin, "github.com/GLINCKER/levelrail/cmd/levelrail-cli") //nolint:gosec // fixed argv, test builds one package
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build cli: %v\n%s", err, out)
	}
	t.Setenv(cliconsole.EnvBinary, bin)
	experimental.Set(experimental.CLIConsole)
	t.Cleanup(func() { experimental.Set(experimental.All()...) })
	rt, db := newTestRouter(t)
	rt.dashboardListenAddr = "127.0.0.1:1"
	admin := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, cliConsoleRequest(t, nil, cliConsoleSessionPath, "http://example.com"))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated = %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, cliConsoleRequest(t, admin, cliConsoleSessionPath, "http://example.com"))
	if rec.Code != http.StatusForbidden || errorCode(t, rec) != errCodeStepUpRequired {
		t.Fatalf("before step-up = %d %s", rec.Code, rec.Body.String())
	}
	if tokens, err := db.ListAPITokens(t.Context()); err != nil || len(tokens) != 0 {
		t.Fatalf("no token may exist before every gate passes, got %d (%v)", len(tokens), err)
	}
	cliConsoleStepUp(t, rt, admin)

	server := httptest.NewServer(rt.Handler())
	t.Cleanup(server.Close)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
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
	if m := read(); m.Type != cliMsgReady {
		t.Fatalf("first message = %+v", m)
	}
	if err := conn.Write(ctx, websocket.MessageText, []byte(`{"type":"run","line":"help"}`)); err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	for {
		m := read()
		if m.Type == cliMsgOutput {
			out.WriteString(m.Data)
			continue
		}
		if m.Type != cliMsgExit || m.Code == nil || *m.Code != 0 {
			t.Fatalf("end message = %+v output = %q", m, out.String())
		}
		break
	}
	if !strings.Contains(out.String(), "Usage") {
		t.Fatalf("help output = %q", out.String())
	}
}

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/GLINCKER/levelrail/internal/cliconsole"
	"github.com/GLINCKER/levelrail/internal/experimental"
	"github.com/GLINCKER/levelrail/internal/hostdiag"
	"github.com/GLINCKER/levelrail/internal/store"
)

type hostRouteCase struct {
	line, wantErr, wantOut string
	wantCode               int
}

func TestCLIConsole_HostRouting(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("credential drop needs a real unprivileged user")
	}
	// The fake CLI marks its output, so any host line that leaked to the binary shows up.
	bin := filepath.Join(t.TempDir(), "fake-cli")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\necho LEAKED-TO-CLI\n"), 0o755); err != nil { //nolint:gosec // test fixture must be executable
		t.Fatal(err)
	}
	t.Setenv(cliconsole.EnvBinary, bin)
	t.Setenv(hostdiag.EnvSwitch, "")
	experimental.Set(experimental.CLIConsole)
	t.Cleanup(func() { experimental.Set(experimental.All()...) })
	rt, db := newTestRouter(t)
	rt.dashboardListenAddr = "127.0.0.1:1"
	rt.dataDir = t.TempDir()
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
	run := func(line string) (string, *int, string) {
		payload, err := json.Marshal(cliClientMessage{Type: cliMsgRun, Line: line})
		if err != nil {
			t.Fatal(err)
		}
		if err := conn.Write(ctx, websocket.MessageText, payload); err != nil {
			t.Fatal(err)
		}
		var out strings.Builder
		for {
			m := read()
			switch m.Type {
			case cliMsgOutput:
				out.WriteString(m.Data)
			case cliMsgExit:
				return out.String(), m.Code, ""
			case cliMsgError:
				return out.String(), nil, m.Message
			default:
				t.Fatalf("unexpected %+v", m)
			}
		}
	}
	if m := read(); m.Type != cliMsgReady {
		t.Fatalf("first message = %+v", m)
	}

	cases := []hostRouteCase{
		{line: "host help", wantOut: "host df"},
		{line: "host", wantOut: "host service"},
		{line: "host df /etc", wantErr: "takes no arguments"},
		{line: "host free --json", wantErr: "takes no arguments"},
		{line: "host ps", wantErr: "unknown host subcommand"},
		{line: "host docker ps", wantErr: "unknown host subcommand"},
		{line: "--team t1 host df", wantErr: "first word"},
		{line: "host df; id", wantErr: "shell syntax"},
	}
	if runtime.GOOS == "linux" {
		cases = append(cases, hostRouteCase{line: "host service", wantOut: "goroutines"})
	} else {
		cases = append(cases, hostRouteCase{line: "host df", wantOut: "not supported on this platform", wantCode: 1})
	}
	for _, c := range cases {
		out, code, errMsg := run(c.line)
		if strings.Contains(out, "LEAKED-TO-CLI") {
			t.Fatalf("%q reached the CLI binary", c.line)
		}
		if c.wantErr != "" {
			if !strings.Contains(errMsg, c.wantErr) {
				t.Errorf("%q error = %q, want %q", c.line, errMsg, c.wantErr)
			}
			continue
		}
		if code == nil || *code != c.wantCode || !strings.Contains(out, c.wantOut) {
			t.Errorf("%q = code %v out %q err %q", c.line, code, out, errMsg)
		}
	}

	t.Setenv(hostdiag.EnvSwitch, "off")
	if _, _, errMsg := run("host help"); !strings.Contains(errMsg, "turned off") {
		t.Fatalf("disabled host help error = %q", errMsg)
	}
	if out, code, _ := run("apps list"); code == nil || !strings.Contains(out, "LEAKED-TO-CLI") {
		t.Fatalf("rest of the console stopped working with host off: %q", out)
	}

	entries, err := db.ListAuditEntries(ctx, 100, nil, store.AuditEntryFilter{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range entries {
		if e.Action == auditActionCLIConsoleRun && strings.HasPrefix(e.Path, "cli-console: host help (exit 0") {
			found = true
		}
	}
	if !found {
		t.Fatal("host help was not audited as cli_console.run")
	}
}

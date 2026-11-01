package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func consoleServer(t *testing.T, templateID string, console []string, gotExec *execRequest) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/apps/mc":
			_ = json.NewEncoder(w).Encode(map[string]any{"name": "mc", "template_id": templateID})
		case "/api/v1/service-templates/minecraft":
			m := map[string]any{"id": "minecraft"}
			if console != nil {
				m["manifest"] = map[string]any{"console_command": console}
			}
			_ = json.NewEncoder(w).Encode(m)
		case "/api/v1/apps/mc/exec":
			_ = json.NewDecoder(r.Body).Decode(gotExec)
			_ = json.NewEncoder(w).Encode(execResponse{Stdout: "ok\n"})
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestRun_AppsConsole_OneShot(t *testing.T) {
	var got execRequest
	srv := consoleServer(t, "minecraft", []string{"rcon-cli"}, &got)
	defer srv.Close()
	var stdout, stderr bytes.Buffer
	code := run("levelrail-cli-test", []string{"apps", "console", "mc", "--api-url", srv.URL, "--", "list"}, &stdout, &stderr, envMap())
	if code != 0 || stdout.String() != "ok\n" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if got.Command != "rcon-cli" || len(got.Args) != 1 || got.Args[0] != "list" {
		t.Errorf("exec request = %+v, want rcon-cli list", got)
	}
}

func TestRun_AppsConsole_NoConsoleDeclared(t *testing.T) {
	var got execRequest
	for _, tc := range []struct {
		name, tpl string
		console   []string
	}{{"no manifest console", "minecraft", nil}, {"not from a template", "", nil}} {
		t.Run(tc.name, func(t *testing.T) {
			srv := consoleServer(t, tc.tpl, tc.console, &got)
			defer srv.Close()
			var stdout, stderr bytes.Buffer
			code := run("levelrail-cli-test", []string{"apps", "console", "mc", "--api-url", srv.URL, "--", "list"}, &stdout, &stderr, envMap())
			if code == 0 || !strings.Contains(stderr.String(), "apps exec") {
				t.Errorf("code=%d stderr=%q, want an error pointing at apps exec", code, stderr.String())
			}
		})
	}
}

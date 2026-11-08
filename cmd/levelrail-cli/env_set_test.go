package main

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnvSetPrintsRebuildHint(t *testing.T) {
	srv := routeServer(t, map[string]any{
		"GET /api/v1/apps/web":                 appResource{Name: "web", Image: "x:1", Port: 80},
		"GET /api/v1/apps/web/secrets":         []map[string]string{},
		"GET /api/v1/apps/web/env/bound":       []map[string]string{},
		"GET /api/v1/apps/web/pending-changes": map[string]any{},
		"PUT /api/v1/apps/web":                 map[string]any{"name": "web", "rebuild_required": true, "rebuild_keys": []string{"NEXT_PUBLIC_API"}},
	}, map[string][]byte{})
	out, _ := runCLIExpectOK(t, cliArgs(srv, "env", "set", "web", "NEXT_PUBLIC_API=https://a.example"))
	if !strings.Contains(out, "NEXT_PUBLIC_API is baked in at build time") {
		t.Errorf("missing rebuild hint in %q", out)
	}
	if strings.Contains(out, "apps rebuild") || !strings.Contains(out, "deploy <dir>") {
		t.Errorf("hint must name real commands, got %q", out)
	}
}

func TestEnvSetRebuildHintGitApp(t *testing.T) {
	srv := routeServer(t, map[string]any{
		"GET /api/v1/apps/web":                 appResource{Name: "web", Image: "x:1", Port: 80},
		"GET /api/v1/apps/web/secrets":         []map[string]string{},
		"GET /api/v1/apps/web/env/bound":       []map[string]string{},
		"GET /api/v1/apps/web/pending-changes": map[string]any{},
		"GET /api/v1/apps/web/git-source":      map[string]any{"service_name": "web", "repo_url": "https://x/y.git", "branch": "main"},
		"PUT /api/v1/apps/web":                 map[string]any{"name": "web", "rebuild_required": true, "rebuild_keys": []string{"VITE_API"}},
	}, map[string][]byte{})
	out, _ := runCLIExpectOK(t, cliArgs(srv, "env", "set", "web", "VITE_API=https://a.example"))
	if !strings.Contains(out, "deploy web --latest") {
		t.Errorf("missing git app hint in %q", out)
	}
}

func envSetRoutes(extra map[string]any) map[string]any {
	r := map[string]any{
		"GET /api/v1/apps/web":                 appResource{Name: "web", Image: "x:1", Port: 80},
		"GET /api/v1/apps/web/secrets":         []map[string]string{},
		"GET /api/v1/apps/web/env/bound":       []map[string]string{},
		"GET /api/v1/apps/web/pending-changes": map[string]any{},
		"PUT /api/v1/apps/web":                 map[string]any{"name": "web"},
	}
	maps.Copy(r, extra)
	return r
}

func sentEnv(t *testing.T, bodies map[string][]byte) map[string]string {
	t.Helper()
	var sent appResource
	if err := json.Unmarshal(bodies["PUT /api/v1/apps/web"], &sent); err != nil {
		t.Fatal(err)
	}
	return sent.Env
}

func TestEnvSetValueForms(t *testing.T) {
	dir := t.TempDir()
	pem := filepath.Join(dir, "k.pem")
	if err := os.WriteFile(pem, []byte("line1\nline2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cases := []struct{ name, arg, want string }{
		{"equals", "A=x=y", "x=y"},
		{"quotes", `A=say "hi"`, `say "hi"`},
		{"newline", "A=l1\nl2", "l1\nl2"},
		{"empty", "A=", ""},
		{"spaces", "A= lead and trail ", " lead and trail "},
		{"hash", "A=#c", "#c"},
		{"file", "A=@" + pem, "line1\nline2"},
		{"escape", "A=@@x", "@x"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			bodies := map[string][]byte{}
			srv := routeServer(t, envSetRoutes(nil), bodies)
			runCLIExpectOK(t, cliArgs(srv, "env", "set", "web", c.arg))
			if got, ok := sentEnv(t, bodies)["A"]; !ok || got != c.want {
				t.Errorf("A = %q (set %v), want %q", got, ok, c.want)
			}
		})
	}
}

func TestEnvSetValueSourceStdinAndErrors(t *testing.T) {
	bodies := map[string][]byte{}
	srv := routeServer(t, envSetRoutes(nil), bodies)
	envSetStdin = strings.NewReader("s3cret\nmore\n")
	t.Cleanup(func() { envSetStdin = os.Stdin })
	runCLIExpectOK(t, cliArgs(srv, "env", "set", "web", "A=@-"))
	if got := sentEnv(t, bodies)["A"]; got != "s3cret\nmore" {
		t.Errorf("stdin value = %q", got)
	}

	dir := t.TempDir()
	big := filepath.Join(dir, "big")
	nul := filepath.Join(dir, "nul")
	_ = os.WriteFile(big, make([]byte, envValueSourceMaxBytes+1), 0o600)
	_ = os.WriteFile(nul, []byte("a\x00b"), 0o600)
	bodies = map[string][]byte{}
	srv = routeServer(t, envSetRoutes(nil), bodies)
	envSetStdin = strings.NewReader("x")
	for _, args := range [][]string{
		{"A=@-", "B=@-"},
		{"A=@" + dir},
		{"A=@" + filepath.Join(dir, "missing")},
		{"A=@" + big},
		{"A=@" + nul},
		{"A=@"},
	} {
		msg := runCLIExpectValidationError(t, cliArgs(srv, append([]string{"env", "set", "web"}, args...)...))
		if strings.Contains(msg, "a\x00b") {
			t.Errorf("error leaked content: %q", msg)
		}
	}
	if len(bodies) != 0 {
		t.Errorf("API was called despite a bad value source: %v", bodies)
	}
}

func TestEnvUnsetMultipleKeys(t *testing.T) {
	bodies := map[string][]byte{}
	routes := envSetRoutes(map[string]any{"GET /api/v1/apps/web": appResource{Name: "web", Image: "x:1", Port: 80, Env: map[string]string{"A": "1", "B": "2", "C": "3"}}})
	srv := routeServer(t, routes, bodies)
	runCLIExpectOK(t, cliArgs(srv, "env", "unset", "web", "A", "B"))
	env := sentEnv(t, bodies)
	if len(env) != 1 || env["C"] != "3" {
		t.Errorf("env = %v, want only C", env)
	}
}

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
		"POST /api/v1/apps/web/env/apply":      map[string]any{"app": "web", "changed": true, "env_version": 5, "plan": []map[string]string{{"key": "NEXT_PUBLIC_API", "action": "set"}}},
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
		"POST /api/v1/apps/web/env/apply":      map[string]any{"app": "web", "changed": true, "env_version": 5, "plan": []map[string]string{{"key": "VITE_API", "action": "set"}}},
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
		"POST /api/v1/apps/web/env/apply":      map[string]any{"app": "web", "changed": true, "env_version": 5, "plan": []map[string]string{}},
	}
	maps.Copy(r, extra)
	return r
}

func sentEnv(t *testing.T, bodies map[string][]byte) map[string]string {
	t.Helper()
	var sent struct {
		Set map[string]string `json:"set"`
	}
	if err := json.Unmarshal(bodies["POST /api/v1/apps/web/env/apply"], &sent); err != nil {
		t.Fatal(err)
	}
	return sent.Set
}

func TestEnvSetValueForms(t *testing.T) {
	cases := []struct{ name, arg, want string }{
		{"equals", "A=x=y", "x=y"},
		{"quotes", `A=say "hi"`, `say "hi"`},
		{"newline", "A=l1\nl2", "l1\nl2"},
		{"empty", "A=", ""},
		{"spaces", "A= lead and trail ", " lead and trail "},
		{"hash", "A=#c", "#c"},
		{"escape", "A=@@x", "@x"},
		{"bare at word", "A=@oncall", "@oncall"},
		{"bare at empty", "A=@", "@"},
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
		{"--allow-file-values", "A=@" + dir},
		{"--allow-file-values", "A=@" + filepath.Join(dir, "missing")},
		{"--allow-file-values", "A=@" + big},
		{"--allow-file-values", "A=@" + nul},
		{"--allow-file-values", "A=@~other/x"},
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

func TestEnvSetFileValueNeedsOptIn(t *testing.T) {
	dir := t.TempDir()
	pem := filepath.Join(dir, "k.pem")
	if err := os.WriteFile(pem, []byte("line1\nline2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	bodies := map[string][]byte{}
	srv := routeServer(t, envSetRoutes(nil), bodies)
	for _, arg := range []string{"A=@" + pem, "A=@./k.pem", "A=@../x", "A=@~/k.pem"} {
		msg := runCLIExpectValidationError(t, cliArgs(srv, "env", "set", "web", arg))
		if !strings.Contains(msg, "--allow-file-values") || strings.Contains(msg, "line1") {
			t.Errorf("%s: message must name the flag and no content, got %q", arg, msg)
		}
	}
	if len(bodies) != 0 {
		t.Errorf("API was called without the opt-in: %v", bodies)
	}

	for _, arg := range []string{"A=@" + pem, "A=@~/k.pem"} {
		bodies = map[string][]byte{}
		srv = routeServer(t, envSetRoutes(nil), bodies)
		runCLIExpectOK(t, cliArgs(srv, "env", "set", "--allow-file-values", "web", arg))
		if got := sentEnv(t, bodies)["A"]; got != "line1\nline2" {
			t.Errorf("%s: A = %q", arg, got)
		}
	}
}

func TestEnvUnsetMultipleKeys(t *testing.T) {
	bodies := map[string][]byte{}
	routes := envSetRoutes(map[string]any{"GET /api/v1/apps/web": appResource{Name: "web", Image: "x:1", Port: 80, Env: map[string]string{"A": "1", "B": "2", "C": "3"}}})
	srv := routeServer(t, routes, bodies)
	runCLIExpectOK(t, cliArgs(srv, "env", "unset", "web", "A", "B"))
	var sent struct {
		Set   map[string]string `json:"set"`
		Unset []string          `json:"unset"`
	}
	if err := json.Unmarshal(bodies["POST /api/v1/apps/web/env/apply"], &sent); err != nil {
		t.Fatal(err)
	}
	if len(sent.Set) != 0 || strings.Join(sent.Unset, ",") != "A,B" {
		t.Errorf("sent = %+v, want unset A,B in one call", sent)
	}
}

func TestEnvSetPrintsNamesOnlyDiffBeforeApply(t *testing.T) {
	bodies := map[string][]byte{}
	srv := routeServer(t, envSetRoutes(nil), bodies)
	_, errOut := runCLIExpectOK(t, cliArgs(srv, "env", "set", "web", "NEW_KEY=top-secret-value"))
	if !strings.Contains(errOut, "+ NEW_KEY (added)") || strings.Contains(errOut, "top-secret-value") {
		t.Errorf("diff = %q, want names only", errOut)
	}
	_, quietErr := runCLIExpectOK(t, cliArgs(srv, "env", "set", "web", "--quiet", "OTHER=v"))
	if strings.Contains(quietErr, "OTHER (added)") {
		t.Errorf("--quiet still printed the diff: %q", quietErr)
	}
}

func TestEnvRestoreNeedsConfirmAndPrintsNamesOnly(t *testing.T) {
	bodies := map[string][]byte{}
	srv := routeServer(t, map[string]any{
		"POST /api/v1/apps/web/env/revisions/2/restore": map[string]any{"app": "web", "restored": 2, "revision": 5, "added": []string{"A"}, "removed": []string{"B"}, "changed": []string{"C"}},
	}, bodies)
	out, _ := runCLIExpectOK(t, cliArgs(srv, "env", "restore", "web", "2", "--confirm", "web"))
	for _, want := range []string{"+ A (added)", "- B (removed)", "~ C (changed)", "new revision 5"} {
		if !strings.Contains(out, want) {
			t.Errorf("output %q missing %q", out, want)
		}
	}
	if !strings.Contains(string(bodies["POST /api/v1/apps/web/env/revisions/2/restore"]), `"confirm":"web"`) {
		t.Errorf("confirm not sent: %s", bodies["POST /api/v1/apps/web/env/revisions/2/restore"])
	}
}

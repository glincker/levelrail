package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRun_GitRemote(t *testing.T) {
	configured := false
	var calls []string
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		calls = append(calls, r.Method+" "+r.URL.Path)
		bodies = append(bodies, string(b))
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"configured":` + map[bool]string{true: "true", false: "false"}[configured] + `,"url":"https://cp.example.com/api/v1/git/web.git","remote_name":"deploy","username":"git","branch":"main","max_push_bytes":268435456}`))
		case http.MethodPost:
			configured = true
			_, _ = w.Write([]byte(`{"configured":true,"url":"https://cp.example.com/api/v1/git/web.git","remote_name":"deploy","username":"git","branch":"main","max_push_bytes":268435456,"token":"gp_abc","add_command":"git remote add deploy https://git:gp_abc@cp.example.com/api/v1/git/web.git"}`))
		case http.MethodPut:
			_, _ = w.Write([]byte(`{"configured":true,"url":"u","remote_name":"deploy","username":"git","branch":"release","max_push_bytes":1048576}`))
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer srv.Close()

	out, _ := runCLIExpectOK(t, []string{"git", "remote", "add", "web", "--api-url", srv.URL})
	if !strings.Contains(out, "git remote add deploy https://git:gp_abc@cp.example.com/api/v1/git/web.git") || !strings.Contains(out, "git push deploy main") {
		t.Errorf("add output = %q", out)
	}

	var outBuf, errBuf strings.Builder
	if got := run("levelrail-cli-test", []string{"git", "remote", "add", "web", "--api-url", srv.URL}, &outBuf, &errBuf, envMap()); got == exitOK || !strings.Contains(errBuf.String(), "git remote rotate") {
		t.Errorf("second add: exit=%d stderr=%q, want a pointer to rotate", got, errBuf.String())
	}

	out, _ = runCLIExpectOK(t, []string{"git", "remote", "show", "web", "--api-url", srv.URL})
	if strings.Contains(out, "gp_abc") || !strings.Contains(out, "git remote add deploy https://cp.example.com/api/v1/git/web.git") {
		t.Errorf("show output = %q", out)
	}

	out, _ = runCLIExpectOK(t, []string{"git", "remote", "rotate", "web", "--json", "--api-url", srv.URL})
	if !strings.Contains(out, `"token": "gp_abc"`) && !strings.Contains(out, `"token":"gp_abc"`) {
		t.Errorf("rotate --json output = %q", out)
	}

	runCLIExpectOK(t, []string{"git", "remote", "branch", "web", "release", "--api-url", srv.URL})
	if last := bodies[len(bodies)-1]; !strings.Contains(last, `"branch":"release"`) {
		t.Errorf("branch body = %q", last)
	}

	runCLIExpectOK(t, []string{"git", "remote", "revoke", "web", "--api-url", srv.URL})
	if calls[len(calls)-1] != "DELETE /api/v1/apps/web/git-remote" {
		t.Errorf("last call = %q", calls[len(calls)-1])
	}
}

func gitRemoteApplyServer(t *testing.T, remoteName, rawURL string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"configured":false}`))
			return
		}
		_, _ = w.Write([]byte(`{"configured":true,"url":"` + rawURL + `","remote_name":"` + remoteName + `","username":"git","branch":"trunk","max_push_bytes":1048576,"token":"gp_abc","add_command":"git remote add ` + remoteName + ` https://git:gp_abc@cp.example.com/x.git"}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func gitInitDir(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil { //nolint:gosec // test-only literal git
		t.Fatalf("git init: %v %s", err, out)
	}
	return dir
}

func gitRemoteURL(t *testing.T, dir, name string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", dir, "remote", "get-url", name).Output() //nolint:gosec // test-only literal git
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func TestRun_GitRemoteApplyStoreTokenOptOut(t *testing.T) {
	srv := gitRemoteApplyServer(t, "deploy", "https://cp.example.com/api/v1/git/web.git")
	dir := gitInitDir(t)

	out, _ := runCLIExpectOK(t, []string{"git", "remote", "add", "web", "--apply", "--store-token-in-config", "--dir", dir, "--api-url", srv.URL})
	if strings.Contains(out, "gp_abc") || !strings.Contains(out, "remote deploy configured in") || !strings.Contains(out, "git push deploy trunk") {
		t.Errorf("apply output = %q", out)
	}
	if got := gitRemoteURL(t, dir, "deploy"); got != "https://git:gp_abc@cp.example.com/api/v1/git/web.git" { //nolint:gosec // fake test token
		t.Errorf("remote url = %q", got)
	}

	// rotate on an existing remote must replace it, not fail.
	out, _ = runCLIExpectOK(t, []string{"git", "remote", "rotate", "web", "--apply", "--store-token-in-config", "--dir", dir, "--api-url", srv.URL})
	if strings.Contains(out, "gp_abc") {
		t.Errorf("rotate output leaked token: %q", out)
	}
}

func TestRun_GitRemoteApplyDefaultKeepsTokenOutOfConfig(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"configured":false}`))
			return
		}
		_, _ = w.Write([]byte(`{"configured":true,"url":"` + srv.URL + `/api/v1/git/web.git","remote_name":"deploy","username":"git","branch":"trunk","max_push_bytes":1048576,"token":"gp_abc"}`))
	}))
	defer srv.Close()
	dir := gitInitDir(t)
	// A remote an older CLI wrote with the token inline must be cleaned by rotate.
	runCLIExpectOK(t, []string{"git", "remote", "add", "web", "--apply", "--store-token-in-config", "--dir", dir, "--api-url", srv.URL})
	if got := gitRemoteURL(t, dir, "deploy"); !strings.Contains(got, "gp_abc") { //nolint:gosec // fake test token
		t.Fatalf("setup remote url = %q, want inline token", got)
	}
	out, _ := runCLIExpectOK(t, []string{"git", "remote", "rotate", "web", "--apply", "--dir", dir, "--api-url", srv.URL})
	if !strings.Contains(out, "LEVELRAIL_GIT_TOKEN") {
		t.Errorf("default apply output lacks export hint: %q", out)
	}
	if got := gitRemoteURL(t, dir, "deploy"); strings.Contains(got, "gp_abc") || got != "http://git@"+strings.TrimPrefix(srv.URL, "http://")+"/api/v1/git/web.git" {
		t.Errorf("remote url = %q", got)
	}
	cfg, err := exec.Command("git", "-C", dir, "config", "--local", "--list").Output() //nolint:gosec // test-only literal git
	if err != nil || strings.Contains(string(cfg), "gp_abc") || !strings.Contains(string(cfg), "helper=!f()") {
		t.Errorf("config = %q err=%v", cfg, err)
	}
	//nolint:gosec // test temp dir
	if raw, _ := os.ReadFile(filepath.Join(dir, ".git", "config")); strings.Contains(string(raw), "gp_abc") {
		t.Errorf(".git/config leaked the token")
	}
	// The empty value must come first so global helpers (keychain, store) are reset for this URL.
	vals, _ := exec.Command("git", "-C", dir, "config", "--local", "--get-regexp", `^credential\..*\.helper$`).Output() //nolint:gosec // test-only literal git
	lines := strings.Split(strings.TrimSpace(string(vals)), "\n")
	if len(lines) != 2 || len(strings.Fields(lines[0])) != 1 || !strings.Contains(lines[1], "!f()") {
		t.Errorf("helper values = %q, want empty then fixed helper", vals)
	}
	// A server naming a different host must not receive a helper that sends the token there.
	other := gitRemoteApplyServer(t, "deploy2", "https://cp.example.com/x.git")
	var outBuf, errBuf strings.Builder
	if got := run("levelrail-cli-test", []string{"git", "remote", "add", "web", "--apply", "--dir", dir, "--api-url", other.URL}, &outBuf, &errBuf, envMap()); got == exitOK || gitRemoteURL(t, dir, "deploy2") != "" {
		t.Errorf("foreign host accepted: exit=%d", got)
	}
}

func TestRun_GitRemoteApplyCredentialHelperAlias(t *testing.T) {
	srv := gitRemoteApplyServer(t, "deploy", "https://cp.example.com/x.git")
	var o, e strings.Builder
	run("levelrail-cli-test", []string{"git", "remote", "add", "web", "--apply", "--credential-helper", "--dir", t.TempDir(), "--api-url", srv.URL}, &o, &e, envMap())
	if strings.Contains(e.String(), "flag provided but not defined") {
		t.Errorf("alias rejected: %q", e.String())
	}
}

func TestRun_GitRemoteApplyNotARepo(t *testing.T) {
	srv := gitRemoteApplyServer(t, "deploy", "https://cp.example.com/api/v1/git/web.git")
	var outBuf, errBuf strings.Builder
	got := run("levelrail-cli-test", []string{"git", "remote", "add", "web", "--apply", "--dir", t.TempDir(), "--api-url", srv.URL}, &outBuf, &errBuf, envMap())
	if got == exitOK || !strings.Contains(outBuf.String(), "git remote add deploy https://git:gp_abc@") {
		t.Errorf("exit=%d stdout=%q, want failure with the token line still printed", got, outBuf.String())
	}
	if strings.Contains(errBuf.String(), "gp_abc") {
		t.Errorf("stderr leaked token: %q", errBuf.String())
	}
}

func TestRun_GitRemoteApplyRejectsBadInput(t *testing.T) {
	dir := gitInitDir(t)
	for name, c := range map[string][2]string{
		"bad name":                 {"-evil", "https://cp.example.com/x.git"},
		"bad url":                  {"deploy", "file:///etc/passwd"},
		"foreign host with helper": {"deploy", "https://evil.example.net/x.git"},
	} {
		srv := gitRemoteApplyServer(t, c[0], c[1])
		var outBuf, errBuf strings.Builder
		args := []string{"git", "remote", "add", "web", "--apply", "--dir", dir, "--api-url", srv.URL}
		got := run("levelrail-cli-test", args, &outBuf, &errBuf, envMap())
		if got == exitOK {
			t.Errorf("%s: exit ok, want failure", name)
		}
		if out, _ := exec.Command("git", "-C", dir, "remote").Output(); strings.TrimSpace(string(out)) != "" { //nolint:gosec // test-only literal git
			t.Errorf("%s: git ran, remotes = %q", name, out)
		}
	}
}

func TestRun_GitRemoteRejectsHelperWithStoreToken(t *testing.T) {
	srv := gitRemoteApplyServer(t, "deploy", "https://cp.example.com/x.git")
	var o, e strings.Builder
	got := run("levelrail-cli-test", []string{"git", "remote", "add", "web", "--apply", "--credential-helper", "--store-token-in-config", "--dir", t.TempDir(), "--api-url", srv.URL}, &o, &e, envMap())
	if got == exitOK || !strings.Contains(o.String()+e.String(), "cannot be combined") {
		t.Errorf("exit=%d out=%q err=%q, want usage error", got, o.String(), e.String())
	}
}

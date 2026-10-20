package main

import (
	"io"
	"net/http"
	"net/http/httptest"
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

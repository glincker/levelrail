package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const envApplyTestSecret = "s3cr3t-value-123"

// envApplyServer answers GET app and POST apply with a canned status and body, recording the apply request.
func envApplyServer(t *testing.T, status int, body string, sent *map[string]any) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/env/apply") {
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, sent)
			w.WriteHeader(status)
			_, _ = w.Write([]byte(body))
			return
		}
		if strings.HasSuffix(r.URL.Path, "/secrets") || strings.HasSuffix(r.URL.Path, "/env/bound") {
			_, _ = w.Write([]byte(`[]`))
			return
		}
		_, _ = w.Write([]byte(`{"name":"web","image":"x:1","port":80,"env_version":4}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func runEnvApplyCLI(t *testing.T, srv *httptest.Server, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := run("levelrail-cli-test", cliArgs(srv, args...), &out, &errb, envMap())
	return code, out.String(), errb.String()
}

func TestEnvApplyPrintsNamesNeverValues(t *testing.T) {
	var sent map[string]any
	srv := envApplyServer(t, http.StatusOK, `{"app":"web","changed":true,"env_version":5,"plan":[{"key":"A","action":"set"},{"key":"TOKEN","action":"secret_set"}]}`, &sent)
	code, out, errb := runEnvApplyCLI(t, srv, "apps", "env", "apply", "web", "--set", "A=1", "--secret-set", "TOKEN="+envApplyTestSecret, "--expected-version", "4")
	if code != exitOK {
		t.Fatalf("exit = %d stderr=%q", code, errb)
	}
	if strings.Contains(out+errb, envApplyTestSecret) {
		t.Fatalf("a value was echoed: %q %q", out, errb)
	}
	if !strings.Contains(out, "secret_set TOKEN") || !strings.Contains(out, "env version 5") {
		t.Errorf("plan not printed by name: %q", out)
	}
	if sent["expected_version"] != float64(4) {
		t.Errorf("expected_version = %v", sent["expected_version"])
	}
}

func TestEnvApplyDryRunSendsFlag(t *testing.T) {
	var sent map[string]any
	srv := envApplyServer(t, http.StatusOK, `{"app":"web","dry_run":true,"changed":false,"env_version":4,"plan":[{"key":"A","action":"set"}]}`, &sent)
	code, out, _ := runEnvApplyCLI(t, srv, "env", "apply", "web", "--unset", "A", "--dry-run")
	if code != exitOK || sent["dry_run"] != true || !strings.Contains(out, "dry run") {
		t.Fatalf("code=%d sent=%v out=%q", code, sent, out)
	}
}

func TestEnvApplyConflictHasOwnExitCode(t *testing.T) {
	var sent map[string]any
	srv := envApplyServer(t, http.StatusConflict, `{"error":"the env changed since expected_version; reload and retry","code":"env_version_conflict","current_version":9,"plan_keys":["A"]}`, &sent)
	code, out, errb := runEnvApplyCLI(t, srv, "env", "apply", "web", "--set", "A="+envApplyTestSecret, "--expected-version", "4")
	if code != exitEnvConflict {
		t.Fatalf("exit = %d, want %d (stderr=%q)", code, exitEnvConflict, errb)
	}
	if !strings.Contains(errb, "apps env get web") || strings.Contains(out+errb, envApplyTestSecret) {
		t.Errorf("conflict message = %q", errb)
	}
}

func TestEnvSetUsesVersionReadWithApp(t *testing.T) {
	var sent map[string]any
	srv := envApplyServer(t, http.StatusConflict, `{"error":"stale","code":"env_version_conflict","current_version":9}`, &sent)
	code, _, errb := runEnvApplyCLI(t, srv, "env", "set", "web", "A=1")
	if code != exitEnvConflict || sent["expected_version"] != float64(4) {
		t.Fatalf("exit=%d sent=%v stderr=%q", code, sent, errb)
	}
	if !strings.Contains(errb, "nothing was changed") {
		t.Errorf("stderr = %q", errb)
	}
}

func TestEnvApplyRejectsValueInUsageError(t *testing.T) {
	var sent map[string]any
	srv := envApplyServer(t, http.StatusOK, `{}`, &sent)
	code, out, errb := runEnvApplyCLI(t, srv, "env", "apply", "web", "--set", envApplyTestSecret)
	if code != exitValidation || strings.Contains(out+errb, envApplyTestSecret) {
		t.Fatalf("exit=%d out=%q err=%q", code, out, errb)
	}
}

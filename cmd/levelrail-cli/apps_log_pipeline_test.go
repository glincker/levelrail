package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRun_AppsLogPipelineSetSendsRules(t *testing.T) {
	var method, body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		var b bytes.Buffer
		_, _ = b.ReadFrom(r.Body)
		body = b.String()
		_, _ = w.Write([]byte(`{"rules":[{"kind":"drop","pattern":"healthz"}]}`))
	}))
	defer srv.Close()
	file := filepath.Join(t.TempDir(), "rules.json")
	if err := os.WriteFile(file, []byte(`{"rules":[{"kind":"drop","pattern":"healthz"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := run("levelrail-cli-test", []string{"apps", "log-pipeline", "set", "web", "--file", file, "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	if method != http.MethodPut || !strings.Contains(body, "healthz") {
		t.Fatalf("method=%s body=%s", method, body)
	}
	if !strings.Contains(stdout.String(), "1 rules") {
		t.Fatalf("output %q", stdout.String())
	}
}

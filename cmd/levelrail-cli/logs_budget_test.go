package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_LogsUsagePrintsTable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/logs/usage" {
			t.Errorf("path %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"default_budget_bytes":0,"apps":[{"app":"noisy","bytes":2097152,"lines":10,"rate_bytes_per_min":1024,"budget_bytes":4194304,"percent":50,"forecast_days":1.5,"trimmed_lines":7}]}`))
	}))
	defer srv.Close()
	var stdout, stderr bytes.Buffer
	if code := run("levelrail-cli-test", []string{"logs", "usage", "--api-url", srv.URL}, &stdout, &stderr, envMap()); code != exitOK {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	for _, want := range []string{"noisy", "2.0 MiB", "50%", "1.5d"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("output missing %q: %s", want, stdout.String())
		}
	}
}

func TestRun_AppsLogBudgetSetSendsMaxMB(t *testing.T) {
	var method, body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		var b bytes.Buffer
		_, _ = b.ReadFrom(r.Body)
		body = b.String()
		_, _ = w.Write([]byte(`{"max_mb":64,"default_mb":0,"effective_max_bytes":67108864}`))
	}))
	defer srv.Close()
	var stdout, stderr bytes.Buffer
	code := run("levelrail-cli-test", []string{"apps", "log-budget", "set", "web", "--max-mb", "64", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	if method != http.MethodPut || !strings.Contains(body, `"max_mb":64`) || !strings.Contains(stdout.String(), "64 MiB") {
		t.Fatalf("method=%s body=%s out=%s", method, body, stdout.String())
	}
}

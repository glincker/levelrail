package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_AppsCanary(t *testing.T) {
	var gotMethod, gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		var b bytes.Buffer
		_, _ = b.ReadFrom(r.Body)
		gotBody = b.String()
		_ = json.NewEncoder(w).Encode(canaryResource{Active: true, Image: "nginx:2", Weight: 25})
	}))
	t.Cleanup(srv.Close)

	for _, tc := range []struct {
		args               []string
		method, path, body string
	}{
		{[]string{"start", "web", "--image", "nginx:2", "--weight", "25"}, http.MethodPost, "/api/v1/apps/web/canary", `{"image":"nginx:2","weight":25}`},
		{[]string{"weight", "web", "--weight", "0"}, http.MethodPut, "/api/v1/apps/web/canary", `{"weight":0}`},
		{[]string{"status", "web"}, http.MethodGet, "/api/v1/apps/web/canary", ""},
		{[]string{"promote", "web"}, http.MethodPost, "/api/v1/apps/web/canary/promote", ""},
		{[]string{"abort", "web"}, http.MethodDelete, "/api/v1/apps/web/canary", ""},
	} {
		var stdout, stderr bytes.Buffer
		args := append([]string{"apps", "canary"}, tc.args...)
		args = append(args, "--token", "t", "--api-url", srv.URL)
		if got := run("levelrail-cli-test", args, &stdout, &stderr, envMap()); got != exitOK {
			t.Fatalf("%v: exit = %d (stderr=%q)", tc.args, got, stderr.String())
		}
		if gotMethod != tc.method || gotPath != tc.path || strings.TrimSpace(gotBody) != tc.body {
			t.Errorf("%v: %s %s %q, want %s %s %q", tc.args, gotMethod, gotPath, gotBody, tc.method, tc.path, tc.body)
		}
	}
}

func TestRun_AppsCanary_StartNeedsImage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if got := run("levelrail-cli-test", []string{"apps", "canary", "start", "web", "--token", "t", "--api-url", "http://x"}, &stdout, &stderr, envMap()); got != exitUsage {
		t.Fatalf("exit = %d, want usage", got)
	}
}

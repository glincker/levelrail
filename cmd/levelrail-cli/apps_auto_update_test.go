package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_AppsAutoUpdate(t *testing.T) {
	var gotMethod, gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		var b bytes.Buffer
		_, _ = b.ReadFrom(r.Body)
		gotBody = b.String()
		_ = json.NewEncoder(w).Encode(imageAutoUpdateResource{Enabled: true, LastResult: "up to date"})
	}))
	t.Cleanup(srv.Close)

	for verb, want := range map[string]struct{ method, path, body string }{
		"enable": {http.MethodPut, "/api/v1/apps/web/auto-update", `{"enabled":true}`},
		"check":  {http.MethodPost, "/api/v1/apps/web/auto-update/check", ""},
		"status": {http.MethodGet, "/api/v1/apps/web/auto-update", ""},
	} {
		var stdout, stderr bytes.Buffer
		got := run("levelrail-cli-test", []string{"apps", "auto-update", verb, "web", "--token", "t", "--api-url", srv.URL}, &stdout, &stderr, envMap())
		if got != exitOK {
			t.Fatalf("%s: exit = %d (stderr=%q)", verb, got, stderr.String())
		}
		if gotMethod != want.method || gotPath != want.path || strings.TrimSpace(gotBody) != want.body {
			t.Errorf("%s: %s %s %q, want %s %s %q", verb, gotMethod, gotPath, gotBody, want.method, want.path, want.body)
		}
		if !strings.Contains(stdout.String(), "up to date") {
			t.Errorf("%s: stdout = %q", verb, stdout.String())
		}
	}
}

func TestRun_AppsAutoUpdateWebhook(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/apps/web/auto-update/webhook" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"path":"/api/v1/hooks/image-update/web/tok","token":"tok"}`))
	}))
	t.Cleanup(srv.Close)

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "auto-update", "webhook", "web", "--token", "t", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK || !strings.Contains(stdout.String(), srv.URL+"/api/v1/hooks/image-update/web/tok") {
		t.Fatalf("exit = %d, stdout = %q, stderr = %q", got, stdout.String(), stderr.String())
	}
}

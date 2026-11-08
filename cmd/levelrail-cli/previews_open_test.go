package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAppsPreviewsOpenPrintsURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/apps/web/previews" {
			t.Errorf("path = %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`[{"app_name":"web","pr_number":7,"domain":"pr-7.example.test","status":"ready"}]`))
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	code := run("levelrail-cli-test", []string{"apps", "previews", "open", "web", "7", "--print", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if code != exitOK || strings.TrimSpace(stdout.String()) != "https://pr-7.example.test" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if previewOpenURL("") != "" {
		t.Fatal("empty domain must yield no URL")
	}
}

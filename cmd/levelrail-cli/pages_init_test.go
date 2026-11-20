package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRun_PagesInit_ScaffoldsAndNeverOverwrites(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "my-site")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(dir, "index.html")
	if err := os.WriteFile(keep, []byte("mine"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, _ := runCLIExpectOK(t, []string{"pages", "init", dir, "--starter", "docs"})
	if !strings.Contains(out, "skipped") {
		t.Errorf("stdout = %q, want a skipped line", out)
	}
	b, _ := os.ReadFile(keep) //nolint:gosec // test temp path
	if string(b) != "mine" {
		t.Fatal("existing index.html was overwritten")
	}
	for _, f := range []string{"404.html", "_redirects", "_headers", "app.yaml"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("%s not written: %v", f, err)
		}
	}
	yml, _ := os.ReadFile(filepath.Join(dir, "app.yaml")) //nolint:gosec // test temp path
	if !strings.Contains(string(yml), "type: static") || !strings.Contains(string(yml), "my-site") {
		t.Errorf("app.yaml = %q", yml)
	}
}

func TestPagesInitFiles_UnknownStarter(t *testing.T) {
	if _, err := pagesInitFiles("x", "nope"); err == nil {
		t.Fatal("want an error for an unknown starter")
	}
	for _, s := range pagesStarterNames() {
		files, err := pagesInitFiles("a<b", s)
		if err != nil || strings.Contains(files["index.html"], "a<b") {
			t.Errorf("starter %s: err=%v or unescaped name", s, err)
		}
	}
}

func TestRun_Pages_DeployURL(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"name":"site","deploy_id":"d1"}`))
	}))
	defer srv.Close()
	out, _ := runCLIExpectOK(t, []string{"pages", "deploy", "--url", "https://example.com/dl/Site.zip", "--api-url", srv.URL})
	if got["source_url"] != "https://example.com/dl/Site.zip" || got["name"] != "site" || !strings.Contains(out, "d1") {
		t.Fatalf("body = %v out = %q", got, out)
	}
}

func TestRedactPagesURL(t *testing.T) {
	if got := redactPagesURL("https://u:p@example.com/a.zip?sig=secret#f"); got != "https://example.com/a.zip" {
		t.Fatalf("got %q", got)
	}
}

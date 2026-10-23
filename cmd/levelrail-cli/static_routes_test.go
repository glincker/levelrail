package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func staticRoutesServer(t *testing.T, paths *[]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*paths = append(*paths, r.Method+" "+r.URL.Path)
		switch {
		case strings.HasSuffix(r.URL.Path, "/test"):
			_, _ = w.Write([]byte(`{"kind":"redirect","status":301,"location":"/new","rule":"_redirects:1","trace":["_redirects:1: redirect 301 to /new"],"headers":[],"remove_headers":[]}`))
		case strings.HasSuffix(r.URL.Path, "/lint"):
			var req struct{ Redirects string }
			_ = json.NewDecoder(r.Body).Decode(&req)
			if strings.Contains(req.Redirects, "bad") {
				_, _ = w.Write([]byte(`{"version":1,"rules":[],"headers":[],"options":{},"problems":[{"source":"_redirects","line":1,"severity":"error","message":"missing the destination"}]}`))
				return
			}
			_, _ = w.Write([]byte(`{"version":1,"rules":[],"headers":[],"options":{},"problems":[]}`))
		default:
			_, _ = w.Write([]byte(`{"version":1,"published":true,"rules":[{"source":"_redirects:1","from":"/old","to":"/new","status":301,"force":true}],"headers":[{"source":"_headers:1","path":"/*","set":[{"name":"X-A","value":"b"}]}],"options":{"spa":true},"problems":[]}`))
		}
	}))
}

func TestRun_StaticSites_RoutesShowTestLint(t *testing.T) {
	var paths []string
	srv := staticRoutesServer(t, &paths)
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"static-sites", "routes", "show", "--api-url", srv.URL, "docs"})
	for _, want := range []string{"/old", "/new", "301!", "X-A", "spa=true"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("show output lacks %q:\n%s", want, stdout)
		}
	}
	stdout, _ = runCLIExpectOK(t, []string{"static-sites", "routes", "test", "--api-url", srv.URL, "docs", "/old"})
	if !strings.Contains(stdout, "redirect to /new (rule _redirects:1)") {
		t.Errorf("test output = %q", stdout)
	}

	dir := t.TempDir()
	good, bad := filepath.Join(dir, "good"), filepath.Join(dir, "bad")
	_ = os.WriteFile(good, []byte("/a /b\n"), 0o600)
	_ = os.WriteFile(bad, []byte("bad\n"), 0o600)
	runCLIExpectOK(t, []string{"static-sites", "routes", "lint", "--api-url", srv.URL, "--redirects", good, "docs"})
	var out, errBuf bytes.Buffer
	code := run("levelrail-cli-test", []string{"static-sites", "routes", "lint", "--api-url", srv.URL, "--redirects", bad, "docs"}, &out, &errBuf, envMap())
	if code != exitValidation || !strings.Contains(out.String(), "missing the destination") {
		t.Errorf("lint with problems: exit=%d out=%q", code, out.String())
	}
	want := []string{"GET /api/v1/apps/docs/static-routes", "POST /api/v1/apps/docs/static-routes/test", "POST /api/v1/apps/docs/static-routes/lint"}
	if len(paths) < 3 || paths[0] != want[0] || paths[1] != want[1] || paths[2] != want[2] {
		t.Errorf("paths = %v", paths)
	}
}

func TestRun_StaticSites_RoutesLintNeedsAFile(t *testing.T) {
	var out, errBuf bytes.Buffer
	if code := run("levelrail-cli-test", []string{"static-sites", "routes", "lint", "docs"}, &out, &errBuf, envMap()); code != exitUsage {
		t.Errorf("exit = %d, want usage", code)
	}
}

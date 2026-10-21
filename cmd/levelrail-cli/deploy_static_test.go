package main

import (
	"archive/zip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func writeSiteZip(t *testing.T, name string, files map[string]string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	f, err := os.Create(p) //nolint:gosec // test temp path
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for n, body := range files {
		w, err := zw.Create(n)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(body))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestStaticAppName(t *testing.T) {
	tests := []struct{ in, want string }{
		{"/tmp/My Site.zip", "my-site"},
		{"/tmp/portfolio_v2.ZIP", "portfolio-v2"},
		{"./dist", "dist"},
	}
	for _, tc := range tests {
		if got := staticAppName(tc.in); got != tc.want {
			t.Errorf("staticAppName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestDeployStatic_DryRunZip(t *testing.T) {
	zp := writeSiteZip(t, "blog.zip", map[string]string{"index.html": "hi", "a.css": "b"})
	code, out, errOut := runDeployCLI([]string{"deploy", zp, "--dry-run", "--api-url", "http://127.0.0.1:1"}, nil)
	if code != exitOK || !strings.Contains(out, "index.html") || !strings.Contains(errOut, `static site "blog"`) {
		t.Fatalf("exit = %d, stdout = %s, stderr = %s", code, out, errOut)
	}
}

func TestDeployStatic_Rejections(t *testing.T) {
	bad := filepath.Join(t.TempDir(), "bad.zip")
	if err := os.WriteFile(bad, []byte("nope"), 0o600); err != nil {
		t.Fatal(err)
	}
	txt := filepath.Join(t.TempDir(), "a.txt")
	if err := os.WriteFile(txt, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"corrupt zip", []string{"deploy", bad, "--yes"}, "not a readable zip"},
		{"plain file with --static", []string{"deploy", txt, "--static", "--yes"}, "not a folder or a .zip"},
		{"missing path", []string{"deploy", filepath.Join(t.TempDir(), "nope"), "--static", "--yes"}, "cannot read"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			code, _, errOut := runDeployCLI(append(tc.args, "--api-url", "http://127.0.0.1:1"), nil)
			if code == exitOK || !strings.Contains(errOut, tc.want) {
				t.Fatalf("exit = %d, stderr = %s, want %q", code, errOut, tc.want)
			}
		})
	}
}

func TestDeployStatic_SizeCap(t *testing.T) {
	zp := writeSiteZip(t, "big.zip", map[string]string{"index.html": strings.Repeat("x", 4000)})
	code, _, errOut := runDeployCLI([]string{"deploy", zp, "--yes", "--api-url", "http://127.0.0.1:1"}, map[string]string{envUploadMaxBytes: "100"})
	if code == exitOK || !strings.Contains(errOut, "upload limit") {
		t.Fatalf("exit = %d, stderr = %s", code, errOut)
	}
}

func TestDeployStatic_FullFlowZipAndFolder(t *testing.T) {
	var mu sync.Mutex
	var created, built map[string]any
	var contentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/apps/") && !strings.Contains(r.URL.Path, "/deploys"):
			if created == nil {
				http.Error(w, `{"error":"app not found"}`, http.StatusNotFound)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"name": created["name"], "fallback_url": "https://site.example.test"})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/apps":
			created = nil
			_ = json.NewDecoder(r.Body).Decode(&created)
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(created)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/uploads"):
			contentType = r.Header.Get("Content-Type")
			_, _ = io.Copy(io.Discard, r.Body)
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"hash": strings.Repeat("cd", 32), "file_count": 2, "has_index": true})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/builds"):
			built = nil
			_ = json.NewDecoder(r.Body).Decode(&built)
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"id":"dep_1"}`))
		case strings.HasSuffix(r.URL.Path, "/deploys/dep_1/logs"):
			w.Header().Set("Content-Type", "text/event-stream")
		case strings.HasSuffix(r.URL.Path, "/deploys/dep_1"):
			_, _ = w.Write([]byte(`{"id":"dep_1","outcome":"healthy"}`))
		default:
			http.Error(w, `{"error":"unexpected `+r.Method+" "+r.URL.Path+`"}`, http.StatusNotFound)
		}
	}))
	defer srv.Close()

	check := func(t *testing.T, args []string, wantType string) {
		t.Helper()
		created, built, contentType = nil, nil, ""
		code, out, errOut := runDeployCLI(append(args, "--yes", "--api-url", srv.URL, "--token", "tok"), nil)
		if code != exitOK || strings.TrimSpace(out) != "https://site.example.test" {
			t.Fatalf("exit = %d, stdout = %q, stderr = %s", code, out, errOut)
		}
		if created["port"] != float64(staticSitePort) {
			t.Errorf("created app = %v, want port %d", created, staticSitePort)
		}
		build, _ := built["build"].(map[string]any)
		if built["upload_hash"] != strings.Repeat("cd", 32) || build["type"] != "static" {
			t.Errorf("build request = %v, want a static upload build", built)
		}
		if contentType != wantType {
			t.Errorf("upload content type = %q, want %q", contentType, wantType)
		}
	}
	t.Run("zip", func(t *testing.T) {
		check(t, []string{"deploy", writeSiteZip(t, "docs.zip", map[string]string{"index.html": "hi", "a.css": "b"})}, "application/zip")
	})
	t.Run("folder with --static", func(t *testing.T) {
		check(t, []string{"deploy", writeDeployDir(t, map[string]string{"index.html": "hi", "a.css": "b"}), "--static"}, "application/gzip")
	})
}

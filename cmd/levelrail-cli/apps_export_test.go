package main

import (
	"archive/tar"
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fakeExportServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/apps/web/export" || r.URL.Query().Get("format") != "compose" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		var buf bytes.Buffer
		tw := tar.NewWriter(&buf)
		for _, f := range []struct{ n, c string }{{".env.example", "A=1\n"}, {"README.md", "hi\n"}, {"docker-compose.yml", "services: {}\n"}} {
			_ = tw.WriteHeader(&tar.Header{Name: f.n, Mode: 0o644, Size: int64(len(f.c))})
			_, _ = tw.Write([]byte(f.c))
		}
		_ = tw.Close()
		_, _ = w.Write(buf.Bytes())
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestAppsExportWritesFiles(t *testing.T) {
	srv := fakeExportServer(t)
	dir := filepath.Join(t.TempDir(), "out")
	stdout, _ := runCLIExpectOK(t, []string{"apps", "export", "--format", "compose", "--out", dir, "web", "--api-url", srv.URL})
	if !strings.Contains(stdout, "docker compose up") {
		t.Fatalf("stdout = %q", stdout)
	}
	b, err := os.ReadFile(filepath.Join(dir, "docker-compose.yml")) //nolint:gosec // test temp dir
	if err != nil || !strings.Contains(string(b), "services") {
		t.Fatalf("compose file: %v %q", err, b)
	}
}

func TestAppsExportStdoutTar(t *testing.T) {
	srv := fakeExportServer(t)
	stdout, _ := runCLIExpectOK(t, []string{"apps", "export", "--out", "-", "web", "--api-url", srv.URL})
	if _, err := tar.NewReader(strings.NewReader(stdout)).Next(); err != nil {
		t.Fatalf("stdout is not a tar: %v", err)
	}
}

func TestExtractExportRejectsUnknownEntry(t *testing.T) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	_ = tw.WriteHeader(&tar.Header{Name: "../evil", Mode: 0o644, Size: 1})
	_, _ = tw.Write([]byte("x"))
	_ = tw.Close()
	if _, err := extractExport(buf.Bytes(), t.TempDir()); err == nil {
		t.Fatal("want error")
	}
}

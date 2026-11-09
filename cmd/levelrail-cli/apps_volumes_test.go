package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRun_AppsVolumes_Get(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(appResource{
			Name:    "web",
			Volumes: []appVolumeResource{{Name: "data", ContainerPath: "/data"}},
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "volumes", "get", "web", "--api-url", srv.URL})

	if gotPath != "/api/v1/apps/web" {
		t.Errorf("path = %s, want /api/v1/apps/web", gotPath)
	}
	if !strings.Contains(stdout, "data -> /data") {
		t.Errorf("stdout = %q, want the volume listed", stdout)
	}
}

func TestRun_AppsVolumes_Get_None(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(appResource{Name: "web"})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "volumes", "get", "web", "--api-url", srv.URL})
	if !strings.Contains(stdout, "volumes: none") {
		t.Errorf("stdout = %q, want \"volumes: none\"", stdout)
	}
}

func TestRun_AppsVolumes_Attach(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody setAppVolumesRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(appResource{Name: "web"})
			return
		}
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_ = json.NewEncoder(w).Encode(setAppVolumesResponse{Name: "web", Volumes: gotBody.Volumes})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{
		"apps", "volumes", "attach", "web",
		"--name", "data", "--path", "/data",
		"--api-url", srv.URL,
	})

	if gotMethod != http.MethodPut || gotPath != "/api/v1/apps/web/volumes" {
		t.Errorf("method/path = %s %s, want PUT /api/v1/apps/web/volumes", gotMethod, gotPath)
	}
	if len(gotBody.Volumes) != 1 || gotBody.Volumes[0] != (appVolumeResource{Name: "data", ContainerPath: "/data"}) {
		t.Errorf("request volumes = %+v, want one data volume at /data", gotBody.Volumes)
	}
	if !strings.Contains(stdout, "data -> /data") {
		t.Errorf("stdout = %q, want the resulting volume list printed", stdout)
	}
}

func TestRun_AppsVolumes_Attach_AlreadyExists(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(appResource{
			Name:    "web",
			Volumes: []appVolumeResource{{Name: "data", ContainerPath: "/data"}},
		})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{
		"apps", "volumes", "attach", "web",
		"--name", "data", "--path", "/other",
		"--api-url", srv.URL,
	}, &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d, stderr = %q", got, exitValidation, stderr.String())
	}
	if !strings.Contains(stderr.String(), "already exists") {
		t.Errorf("stderr = %q, want an already-exists error", stderr.String())
	}
}

func TestRun_AppsVolumes_Attach_MissingFlags(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "volumes", "attach", "web", "--api-url", "http://unused"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "requires --name and --path") {
		t.Errorf("stderr = %q, want a missing-flag usage error", stderr.String())
	}
}

func TestRun_AppsVolumes_Detach(t *testing.T) {
	var gotBody setAppVolumesRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(appResource{
				Name: "web",
				Volumes: []appVolumeResource{
					{Name: "data", ContainerPath: "/data"},
					{Name: "cache", ContainerPath: "/cache"},
				},
			})
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_ = json.NewEncoder(w).Encode(setAppVolumesResponse{Name: "web", Volumes: gotBody.Volumes})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "volumes", "detach", "web", "--name", "data", "--api-url", srv.URL})

	if len(gotBody.Volumes) != 1 || gotBody.Volumes[0].Name != "cache" {
		t.Errorf("request volumes = %+v, want only cache remaining", gotBody.Volumes)
	}
	if strings.Contains(stdout, "data ->") || !strings.Contains(stdout, "cache -> /cache") {
		t.Errorf("stdout = %q, want data gone and cache still listed", stdout)
	}
}

func TestRun_AppsVolumes_Detach_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(appResource{Name: "web"})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "volumes", "detach", "web", "--name", "data", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d, stderr = %q", got, exitValidation, stderr.String())
	}
	if !strings.Contains(stderr.String(), "not found") {
		t.Errorf("stderr = %q, want a not-found error", stderr.String())
	}
}

func TestRun_AppsVolumes_Help(t *testing.T) {
	stdout, _ := runCLIExpectOK(t, []string{"apps", "volumes", "-h"})
	if !strings.Contains(stdout, "apps volumes attach") {
		t.Errorf("stdout = %q, want usage text", stdout)
	}
}

func TestRun_AppsVolumes_UnknownSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "volumes", "bogus"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "unknown apps volumes subcommand") {
		t.Errorf("stderr = %q, want an unknown-subcommand error", stderr.String())
	}
}

func exportTestArchive(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	_ = tw.WriteHeader(&tar.Header{Name: "a", Mode: 0o644, Size: 2, Typeflag: tar.TypeReg})
	_, _ = tw.Write([]byte("hi"))
	_ = tw.Close()
	_ = gz.Close()
	return buf.Bytes()
}

func TestRun_VolumesExport_TruncatedStreamFails(t *testing.T) {
	full := exportTestArchive(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(full[:len(full)/2])
	}))
	defer srv.Close()

	file := filepath.Join(t.TempDir(), "data.tar.gz")
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"volumes", "export", "web", "data", "--out", file, "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got == exitOK || !strings.Contains(stderr.String(), "export truncated") {
		t.Fatalf("exit = %d stderr = %q, want a non-zero exit and export truncated", got, stderr.String())
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatal("a truncated export must not leave a finished-looking file")
	}
}

func TestRun_VolumesExportImport(t *testing.T) {
	archive := exportTestArchive(t)
	var gotPath, gotQuery string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		if r.Method == http.MethodGet {
			_, _ = w.Write(archive)
			return
		}
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"entries":2,"bytes":1024}`))
	}))
	defer srv.Close()

	file := filepath.Join(t.TempDir(), "data.tar.gz")
	stdout, _ := runCLIExpectOK(t, []string{"volumes", "export", "web", "data", "--out", file, "--api-url", srv.URL})
	if gotPath != "/api/v1/apps/web/volumes/data/export" || !strings.Contains(stdout, fmt.Sprintf("%d bytes", len(archive))) {
		t.Fatalf("export path=%s stdout=%q", gotPath, stdout)
	}
	b, err := os.ReadFile(file) //nolint:gosec // path built from t.TempDir()
	if err != nil || !bytes.Equal(b, archive) {
		t.Fatalf("file = %q %v", b, err)
	}

	stdout, _ = runCLIExpectOK(t, []string{"apps", "volumes", "import", "web", "data", "--in", file, "--force", "--api-url", srv.URL})
	if gotPath != "/api/v1/apps/web/volumes/data/import" || gotQuery != "force=true" || !bytes.Equal(gotBody, archive) {
		t.Fatalf("import path=%s query=%s body=%q", gotPath, gotQuery, gotBody)
	}
	if !strings.Contains(stdout, "imported 2 entries") {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestRun_AppsVolumes_VolumeUsageSizeHistory(t *testing.T) {
	var gotPath, gotFrom string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotFrom = r.URL.Path, r.URL.Query().Get("from")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `[{"at":"2026-11-08T10:00:00Z","used_bytes":1024}]`)
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "volumes", "get", "web", "--size-history", "data", "--from", "2026-11-08T00:00:00Z", "--api-url", srv.URL})
	if gotPath != "/api/v1/apps/web/volumes/data/size-history" || gotFrom == "" {
		t.Errorf("path %s from %q", gotPath, gotFrom)
	}
	if !strings.Contains(stdout, "1024 bytes") {
		t.Errorf("stdout = %q", stdout)
	}
}

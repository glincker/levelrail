package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

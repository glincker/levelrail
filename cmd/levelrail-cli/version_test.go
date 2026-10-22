package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_Version_UpToDate(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		latest := "v1.2.3"
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(updatesResource{
			CurrentVersion:  "v1.2.3",
			LatestVersion:   &latest,
			UpdateAvailable: false,
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"version", "--server", "--api-url", srv.URL})
	if gotMethod != http.MethodGet || gotPath != "/api/v1/updates" {
		t.Errorf("request = %s %s, want GET /api/v1/updates", gotMethod, gotPath)
	}
	if !strings.Contains(stdout, "running version:    v1.2.3") {
		t.Errorf("stdout = %q, want the running version line", stdout)
	}
	if !strings.Contains(stdout, "up to date") {
		t.Errorf("stdout = %q, want the up to date line", stdout)
	}
}

func TestRun_Version_UpdateAvailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		latest := "v1.3.0"
		url := "https://github.com/glincker/levelrail/releases/tag/v1.3.0"
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(updatesResource{
			CurrentVersion:  "v1.2.3",
			LatestVersion:   &latest,
			UpdateAvailable: true,
			ReleaseURL:      &url,
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"version", "--server", "--api-url", srv.URL})
	if !strings.Contains(stdout, "update available:   yes") {
		t.Errorf("stdout = %q, want the update available line", stdout)
	}
	if !strings.Contains(stdout, "https://github.com/glincker/levelrail/releases/tag/v1.3.0") {
		t.Errorf("stdout = %q, want the release url line", stdout)
	}
}

func TestRun_Version_NoReleasesYet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(updatesResource{CurrentVersion: "dev"})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"version", "--server", "--api-url", srv.URL})
	if !strings.Contains(stdout, "none published yet") {
		t.Errorf("stdout = %q, want the no releases line", stdout)
	}
}

func TestRun_Version_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(updatesResource{CurrentVersion: "dev"})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"version", "--server", "--api-url", srv.URL, "--json"})
	if !strings.Contains(stdout, `"current_version": "dev"`) {
		t.Errorf("stdout = %q, want version info as JSON", stdout)
	}
}

func TestRun_Version_ServerError(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusInternalServerError, `{"error":"boom"}`)

	stderr := runCLIExpectAPIError(t, []string{"version", "--server", "--api-url", srv.URL})
	if !strings.Contains(stderr, "boom") {
		t.Errorf("stderr = %q, want the server's error message", stderr)
	}
}

// The unreachable address proves the default path makes no network call.
func TestRun_VersionOffline(t *testing.T) {
	const unreachable = "http://127.0.0.1:1"
	tests := []struct {
		name       string
		args       []string
		wantOK     bool
		wantOut    string
		notWantOut string
		wantErr    string
	}{
		{
			name:    "prints cli version with no server",
			args:    []string{"version", "--api-url", unreachable},
			wantOK:  true,
			wantOut: "cli version:",
		},
		{
			name:       "json holds cli info and no server block",
			args:       []string{"version", "--json", "--api-url", unreachable},
			wantOK:     true,
			wantOut:    `"version"`,
			notWantOut: `"server"`,
		},
		{
			name:    "server flag fails cleanly when unreachable",
			args:    []string{"version", "--server", "--api-url", unreachable},
			wantOK:  false,
			wantErr: "could not reach the server at " + unreachable,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			got := run("levelrail-cli-test", tt.args, &stdout, &stderr, envMap())
			if (got == exitOK) != tt.wantOK {
				t.Fatalf("exit = %d, wantOK = %v (stderr=%q)", got, tt.wantOK, stderr.String())
			}
			if tt.wantOut != "" && !strings.Contains(stdout.String(), tt.wantOut) {
				t.Errorf("stdout = %q, want it to contain %q", stdout.String(), tt.wantOut)
			}
			if tt.notWantOut != "" && strings.Contains(stdout.String(), tt.notWantOut) {
				t.Errorf("stdout = %q, must not contain %q", stdout.String(), tt.notWantOut)
			}
			if tt.wantErr != "" && !strings.Contains(stderr.String(), tt.wantErr) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tt.wantErr)
			}
		})
	}
}

func TestRun_Version_Help(t *testing.T) {
	_, stderr := runCLIExpectOK(t, []string{"version", "-h"})
	if !strings.Contains(stderr, "version") {
		t.Errorf("stderr = %q, want usage text", stderr)
	}
}

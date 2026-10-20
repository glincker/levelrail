package main

import (
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

	stdout, _ := runCLIExpectOK(t, []string{"version", "--api-url", srv.URL})
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

	stdout, _ := runCLIExpectOK(t, []string{"version", "--api-url", srv.URL})
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

	stdout, _ := runCLIExpectOK(t, []string{"version", "--api-url", srv.URL})
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

	stdout, _ := runCLIExpectOK(t, []string{"version", "--api-url", srv.URL, "--json"})
	if !strings.Contains(stdout, `"current_version": "dev"`) {
		t.Errorf("stdout = %q, want version info as JSON", stdout)
	}
}

func TestRun_Version_ServerError(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusInternalServerError, `{"error":"boom"}`)

	stderr := runCLIExpectAPIError(t, []string{"version", "--api-url", srv.URL})
	if !strings.Contains(stderr, "boom") {
		t.Errorf("stderr = %q, want the server's error message", stderr)
	}
}

func TestRun_Version_Help(t *testing.T) {
	_, stderr := runCLIExpectOK(t, []string{"version", "-h"})
	if !strings.Contains(stderr, "version") {
		t.Errorf("stderr = %q, want usage text", stderr)
	}
}

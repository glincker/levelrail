package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_Changelog_Human(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(changelogResource{
			CurrentVersion: "v1.2.3",
			Entries: []changelogEntryResource{
				{Version: "1.2.3", Date: "2026-10-01", Bullets: []string{"added the changelog panel"}},
			},
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"changelog", "--api-url", srv.URL})
	if gotMethod != http.MethodGet || gotPath != "/api/v1/changelog" {
		t.Errorf("request = %s %s, want GET /api/v1/changelog", gotMethod, gotPath)
	}
	if !strings.Contains(stdout, "1.2.3 (2026-10-01)") {
		t.Errorf("stdout = %q, want the version/date line", stdout)
	}
	if !strings.Contains(stdout, "added the changelog panel") {
		t.Errorf("stdout = %q, want the bullet", stdout)
	}
}

func TestRun_Changelog_LimitFlag(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(changelogResource{CurrentVersion: "dev"})
	}))
	defer srv.Close()

	runCLIExpectOK(t, []string{"changelog", "--api-url", srv.URL, "--limit", "3"})
	if gotQuery != "limit=3" {
		t.Errorf("query = %q, want limit=3", gotQuery)
	}
}

func TestRun_Changelog_NoEntries(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(changelogResource{CurrentVersion: "dev"})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"changelog", "--api-url", srv.URL})
	if !strings.Contains(stdout, "no changelog entries available") {
		t.Errorf("stdout = %q, want the empty-state line", stdout)
	}
}

func TestRun_Changelog_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(changelogResource{CurrentVersion: "dev"})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"changelog", "--api-url", srv.URL, "--json"})
	if !strings.Contains(stdout, `"current_version": "dev"`) {
		t.Errorf("stdout = %q, want changelog info as JSON", stdout)
	}
}

func TestRun_Changelog_ServerError(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusInternalServerError, `{"error":"boom"}`)

	stderr := runCLIExpectAPIError(t, []string{"changelog", "--api-url", srv.URL})
	if !strings.Contains(stderr, "boom") {
		t.Errorf("stderr = %q, want the server's error message", stderr)
	}
}

func TestRun_Changelog_Help(t *testing.T) {
	_, stderr := runCLIExpectOK(t, []string{"changelog", "-h"})
	if !strings.Contains(stderr, "changelog") {
		t.Errorf("stderr = %q, want usage text", stderr)
	}
}

package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_RestoreListsAndBuild(t *testing.T) {
	var gotMethod, gotPath, gotRepo, gotRef string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotRepo, gotRef = body["repo_url"], body["ref"]
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/build/detect":
			_, _ = w.Write([]byte(`{"provider":"node","framework_name":"Next.js","detected":true}`))
		case "/api/v1/git/branches":
			_, _ = w.Write([]byte(`{"branches":["dev","main"]}`))
		case "/api/v1/databases/main/pitr-restores":
			_, _ = w.Write([]byte(`[{"id":"pr_1","base_backup_history_id":"bb_1","target_timestamp":"2026-09-01T00:00:00Z","status":"succeeded","started_at":"2026-09-01T01:00:00Z"}]`))
		default:
			_, _ = w.Write([]byte(`[{"id":"rsh_1","backup_history_id":"bkh_1","status":"failed","error":"disk full","started_at":"2026-09-01T01:00:00Z"}]`))
		}
	}))
	defer srv.Close()

	tests := []struct {
		name       string
		args       []string
		wantMethod string
		wantPath   string
		wantOut    string
		wantRepo   string
		wantRef    string
	}{
		{"backups restores", []string{"backups", "restores", "main"}, "GET", "/api/v1/databases/main/restores", "disk full", "", ""},
		{"volume restores", []string{"app-volume-backups", "restores", "web", "data"}, "GET", "/api/v1/apps/web/volumes/data/restores", "rsh_1", "", ""},
		{"pitr restores", []string{"pitr", "restores", "main"}, "GET", "/api/v1/databases/main/pitr-restores", "bb_1", "", ""},
		{"build detect", []string{"build", "detect", "--repo-url", "https://example.com/a/b", "--ref", "v1"}, "POST", "/api/v1/build/detect", "detected: Next.js", "https://example.com/a/b", "v1"},
		{"build branches", []string{"build", "branches", "--repo-url", "https://example.com/a/b"}, "POST", "/api/v1/git/branches", "dev\nmain", "https://example.com/a/b", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			args := append(append([]string{}, tt.args...), "--api-url", srv.URL)
			if got := run("levelrail-cli-test", args, &stdout, &stderr, envMap()); got != exitOK {
				t.Fatalf("exit = %d (stdout=%q stderr=%q)", got, stdout.String(), stderr.String())
			}
			if gotMethod != tt.wantMethod || gotPath != tt.wantPath || gotRepo != tt.wantRepo || gotRef != tt.wantRef {
				t.Errorf("request = %s %s repo=%q ref=%q", gotMethod, gotPath, gotRepo, gotRef)
			}
			if !strings.Contains(stdout.String(), tt.wantOut) {
				t.Errorf("stdout = %q, want %q", stdout.String(), tt.wantOut)
			}
		})
	}
}

func TestRun_RestoreListsAndBuild_JSONAndQuery(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"rsh_1","backup_history_id":"bkh_1","status":"succeeded","started_at":"t"}]`))
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	if got := run("p", []string{"backups", "restores", "main", "--json", "--api-url", srv.URL}, &stdout, &stderr, envMap()); got != exitOK {
		t.Fatalf("exit = %d (%q)", got, stderr.String())
	}
	var decoded []restoreHistoryResource
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil || len(decoded) != 1 {
		t.Fatalf("decode: %v (%q)", err, stdout.String())
	}

	stdout.Reset()
	if got := run("p", []string{"backups", "restores", "main", "--query", "[0].id", "--json", "--api-url", srv.URL}, &stdout, &stderr, envMap()); got != exitOK || !strings.Contains(stdout.String(), "rsh_1") {
		t.Fatalf("query exit = %d stdout=%q", got, stdout.String())
	}
}

func TestRun_RestoreListsAndBuild_Usage(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want int
	}{
		{"backups restores no arg", []string{"backups", "restores"}, exitUsage},
		{"volume restores one arg", []string{"app-volume-backups", "restores", "web"}, exitUsage},
		{"pitr restores no arg", []string{"pitr", "restores"}, exitUsage},
		{"build no verb", []string{"build"}, exitUsage},
		{"build unknown verb", []string{"build", "nope"}, exitUsage},
		{"build detect no repo", []string{"build", "detect"}, exitValidation},
		{"build branches positional", []string{"build", "branches", "x", "--repo-url", "u"}, exitUsage},
		{"build help", []string{"build", "-h"}, exitOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if got := run("p", tt.args, &stdout, &stderr, envMap()); got != tt.want {
				t.Errorf("exit = %d, want %d (stderr=%q)", got, tt.want, stderr.String())
			}
		})
	}
}

func TestRun_RestoreLists_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"database not found"}`))
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	if got := run("p", []string{"backups", "restores", "nope", "--api-url", srv.URL}, &stdout, &stderr, envMap()); got != exitAPIError {
		t.Errorf("exit = %d, want %d", got, exitAPIError)
	}
}

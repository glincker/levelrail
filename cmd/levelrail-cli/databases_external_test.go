package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_DatabasesConnect_SendsPasswordFromStdinOnly(t *testing.T) {
	var gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"name":"legacy","engine":"postgres","host":"pg","port":5432,"tls_mode":"prefer","has_password":true,"external":true}`))
	}))
	defer srv.Close()

	var stdout, stderr strings.Builder
	args := []string{"--name", "legacy", "--engine", "postgres", "--host", "pg", "--user", "app", "--password-stdin", "--api-url", srv.URL, "--json"}
	code := runDatabasesConnect("levelrail-cli-test", args, strings.NewReader("s3cret\n"), &stdout, &stderr, envMap())
	if code != exitOK {
		t.Fatalf("exit = %d stderr = %s", code, stderr.String())
	}
	if gotPath != "/api/v1/external-databases" || gotBody["password"] != "s3cret" || gotBody["host"] != "pg" {
		t.Errorf("request = %s %v", gotPath, gotBody)
	}
	if strings.Contains(stdout.String(), "s3cret") {
		t.Errorf("stdout leaks the password: %s", stdout.String())
	}
}

func TestRun_DatabasesConnect_RequiresEngineAndHost(t *testing.T) {
	var stdout, stderr strings.Builder
	code := runDatabasesConnect("levelrail-cli-test", []string{"--name", "x"}, strings.NewReader(""), &stdout, &stderr, envMap())
	if code != exitUsage {
		t.Fatalf("exit = %d", code)
	}
}

func TestRun_DatabasesDelete_FallsBackToExternalRecord(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		if strings.HasPrefix(r.URL.Path, "/api/v1/databases/") {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"database not found"}`))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"databases", "delete", "legacy", "--api-url", srv.URL})
	if len(paths) != 2 || paths[1] != "DELETE /api/v1/external-databases/legacy" {
		t.Errorf("paths = %v", paths)
	}
	if !strings.Contains(stdout, "deleted") {
		t.Errorf("stdout = %q", stdout)
	}
}

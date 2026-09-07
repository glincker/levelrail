package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestRun_DatabasesInitScriptsList(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]databaseInitScriptResource{
			{ID: "dis_1", DatabaseName: "mydb", Filename: "01-init.sql", UpdatedAt: "2026-09-06T00:00:00Z"},
		})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"databases", "init-scripts", "list", "mydb", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotPath != "/api/v1/databases/mydb/init-scripts" {
		t.Errorf("path = %q, want /api/v1/databases/mydb/init-scripts", gotPath)
	}
	if !strings.Contains(stdout.String(), "01-init.sql") {
		t.Errorf("stdout = %q, want the filename", stdout.String())
	}
}

func TestRun_DatabasesInitScriptsCreate_FromFile(t *testing.T) {
	scriptPath := t.TempDir() + "/01-init.sql"
	if err := os.WriteFile(scriptPath, []byte("CREATE EXTENSION IF NOT EXISTS vector;"), 0o600); err != nil {
		t.Fatalf("seed script file: %v", err)
	}

	var gotPath, gotMethod string
	var gotBody setDatabaseInitScriptRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(databaseInitScriptResource{ID: "dis_1", DatabaseName: "mydb", Filename: gotBody.Filename, Content: gotBody.Content})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{
		"databases", "init-scripts", "create", "mydb",
		"--filename", "01-init.sql", "--file", scriptPath, "--api-url", srv.URL,
	}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotMethod != http.MethodPost || gotPath != "/api/v1/databases/mydb/init-scripts" {
		t.Errorf("request = %s %s, want POST /api/v1/databases/mydb/init-scripts", gotMethod, gotPath)
	}
	if gotBody.Filename != "01-init.sql" || gotBody.Content != "CREATE EXTENSION IF NOT EXISTS vector;" {
		t.Errorf("request body = %+v, want the file's own content", gotBody)
	}
	if !strings.Contains(stdout.String(), "dis_1") {
		t.Errorf("stdout = %q, want the created script's id", stdout.String())
	}
}

func TestRun_DatabasesInitScriptsCreate_FromStdin(t *testing.T) {
	var gotBody setDatabaseInitScriptRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(databaseInitScriptResource{ID: "dis_1"})
	}))
	defer srv.Close()

	stdin := strings.NewReader("CREATE ROLE readonly;")
	var stdout, stderr bytes.Buffer
	got := runDatabasesInitScriptsCreate("levelrail-cli-test", []string{
		"mydb", "--filename", "01-role.sql", "--file", "-", "--api-url", srv.URL,
	}, &stdout, &stderr, envMap(), stdin)
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotBody.Content != "CREATE ROLE readonly;" {
		t.Errorf("request body content = %q, want stdin's own content", gotBody.Content)
	}
}

func TestRun_DatabasesInitScriptsCreate_MissingFilename(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"databases", "init-scripts", "create", "mydb", "--file", "x.sql"}, &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitValidation, stderr.String())
	}
	if !strings.Contains(stderr.String(), "--filename is required") {
		t.Errorf("stderr = %q, want a missing --filename error", stderr.String())
	}
}

func TestRun_DatabasesInitScriptsCreate_MissingFile(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"databases", "init-scripts", "create", "mydb", "--filename", "01-init.sql"}, &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitValidation, stderr.String())
	}
	if !strings.Contains(stderr.String(), "--file is required") {
		t.Errorf("stderr = %q, want a missing --file error", stderr.String())
	}
}

func TestRun_DatabasesInitScriptsUpdate_CallsAPI(t *testing.T) {
	scriptPath := t.TempDir() + "/updated.sql"
	if err := os.WriteFile(scriptPath, []byte("CREATE EXTENSION IF NOT EXISTS postgis;"), 0o600); err != nil {
		t.Fatalf("seed script file: %v", err)
	}

	var gotPath, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(databaseInitScriptResource{ID: "dis_1", Filename: "updated.sql"})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{
		"databases", "init-scripts", "update", "mydb", "dis_1",
		"--filename", "updated.sql", "--file", scriptPath, "--api-url", srv.URL,
	}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotMethod != http.MethodPut || gotPath != "/api/v1/databases/mydb/init-scripts/dis_1" {
		t.Errorf("request = %s %s, want PUT /api/v1/databases/mydb/init-scripts/dis_1", gotMethod, gotPath)
	}
}

func TestRun_DatabasesInitScriptsUpdate_MissingArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"databases", "init-scripts", "update", "mydb"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
}

func TestRun_DatabasesInitScriptsDelete_CallsAPI(t *testing.T) {
	var gotPath, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"databases", "init-scripts", "delete", "mydb", "dis_1", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotMethod != http.MethodDelete || gotPath != "/api/v1/databases/mydb/init-scripts/dis_1" {
		t.Errorf("request = %s %s, want DELETE /api/v1/databases/mydb/init-scripts/dis_1", gotMethod, gotPath)
	}
	if !strings.Contains(stderr.String(), "deleted") {
		t.Errorf("stderr = %q, want a deletion confirmation", stderr.String())
	}
}

func TestRun_DatabasesInitScripts_NoSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"databases", "init-scripts"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
}

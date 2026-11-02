package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestRun_DatabasesBranchCreate_CallsAPI(t *testing.T) {
	var gotPath string
	var gotBody apiclient.CreateDatabaseBranchRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(apiclient.DatabaseBranchResource{ID: "clr_1", Name: gotBody.Name, BranchedFrom: "main", Status: "running"})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"databases", "branch", "create", "main", "main-try", "--from-backup", "bkh_1", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d (stdout=%q stderr=%q)", got, stdout.String(), stderr.String())
	}
	if gotPath != "/api/v1/databases/main/branches" || gotBody.Name != "main-try" || gotBody.BackupID != "bkh_1" {
		t.Errorf("path = %q body = %+v", gotPath, gotBody)
	}
}

func TestRun_DatabasesBranchCreate_NeedsExactlyOneSource(t *testing.T) {
	for _, extra := range [][]string{nil, {"--from-backup", "b", "--at", "2026-01-01T00:00:00Z"}} {
		var stdout, stderr bytes.Buffer
		args := append([]string{"databases", "branch", "create", "main", "x", "--api-url", "http://127.0.0.1:1"}, extra...)
		if got := run("levelrail-cli-test", args, &stdout, &stderr, envMap()); got == exitOK {
			t.Fatalf("args %v: exit = ok, want failure", extra)
		}
		if !strings.Contains(stderr.String()+stdout.String(), "exactly one") {
			t.Errorf("args %v: output = %q", extra, stderr.String()+stdout.String())
		}
	}
}

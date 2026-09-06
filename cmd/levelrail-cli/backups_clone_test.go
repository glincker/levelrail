package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_BackupsClone_CallsAPI(t *testing.T) {
	var gotPath string
	var gotBody cloneNowRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(cloneNowResource{
			SourceDatabaseName: "main", NewDatabaseName: gotBody.NewName,
			TargetID: "bkt_1", BackupHistoryID: "bkh_1", CloneRestoreID: "clr_1",
		})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"backups", "clone", "main", "--new-name", "main-staging", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotPath != "/api/v1/databases/main/clone" {
		t.Errorf("path = %q, want /api/v1/databases/main/clone", gotPath)
	}
	if gotBody.NewName != "main-staging" {
		t.Errorf("request body = %+v, want new_name=main-staging", gotBody)
	}
	if gotBody.TargetID != "" {
		t.Errorf("request body TargetID = %q, want empty when --target is omitted", gotBody.TargetID)
	}
	if !strings.Contains(stdout.String(), "clr_1") || !strings.Contains(stdout.String(), "bkh_1") {
		t.Errorf("stdout = %q, want the started backup and clone-restore ids", stdout.String())
	}
}

func TestRun_BackupsClone_WithExplicitTarget(t *testing.T) {
	var gotBody cloneNowRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(cloneNowResource{})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{
		"backups", "clone", "main", "--new-name", "main-staging", "--target", "bkt_other", "--api-url", srv.URL,
	}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotBody.TargetID != "bkt_other" {
		t.Errorf("request body TargetID = %q, want bkt_other", gotBody.TargetID)
	}
}

func TestRun_BackupsClone_MissingNewName(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"backups", "clone", "main"}, &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitValidation, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "--new-name is required") {
		t.Errorf("stderr = %q, want a missing --new-name error", stderr.String())
	}
}

func TestRun_BackupsClone_NoName(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"backups", "clone"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
}

func TestRun_BackupsClone_JSONOutput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(cloneNowResource{SourceDatabaseName: "main", NewDatabaseName: "main-staging", BackupHistoryID: "bkh_1", CloneRestoreID: "clr_1"})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"backups", "clone", "main", "--new-name", "main-staging", "--json", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	var got2 cloneNowResource
	if err := json.Unmarshal(stdout.Bytes(), &got2); err != nil {
		t.Fatalf("decode stdout as JSON: %v (stdout=%q)", err, stdout.String())
	}
	if got2.NewDatabaseName != "main-staging" {
		t.Errorf("decoded stdout = %+v, want new_database_name=main-staging", got2)
	}
}

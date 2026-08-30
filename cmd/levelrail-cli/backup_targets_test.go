package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newBackupTargetEchoServer starts a test server that decodes a create or
// update request into gotBody and responds with a backup target resource
// built from the request plus id, recording the request path and method
// when the caller passes non-nil pointers.
func newBackupTargetEchoServer(t *testing.T, id string, gotPath, gotMethod *string) (*httptest.Server, *createBackupTargetRequest) {
	t.Helper()
	gotBody := &createBackupTargetRequest{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if gotPath != nil {
			*gotPath = r.URL.Path
		}
		if gotMethod != nil {
			*gotMethod = r.Method
		}
		if err := json.NewDecoder(r.Body).Decode(gotBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
		}
		_ = json.NewEncoder(w).Encode(backupTargetResource{
			ID: id, Name: gotBody.Name, Provider: gotBody.Provider, Endpoint: gotBody.Endpoint, Region: gotBody.Region, Bucket: gotBody.Bucket,
		})
	}))
	t.Cleanup(srv.Close)
	return srv, gotBody
}

func TestRun_BackupTargetsList(t *testing.T) {
	var gotPath string
	srv := newListEchoServer(t, &gotPath, []backupTargetResource{
		{ID: "bkt_1", Name: "primary", Provider: "aws", Bucket: "backups", CreatedAt: "2026-08-14T00:00:00Z"},
	})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"backup-targets", "list", "--api-url", srv.URL})
	if gotPath != "/api/v1/backup-targets" {
		t.Errorf("path = %q, want /api/v1/backup-targets", gotPath)
	}
	if !strings.Contains(stdout, "bkt_1") {
		t.Errorf("stdout = %q, want the target id listed", stdout)
	}
}

func TestRun_BackupTargetsList_Empty(t *testing.T) {
	var gotPath string
	srv := newListEchoServer(t, &gotPath, []backupTargetResource{})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"backup-targets", "list", "--api-url", srv.URL})
	if !strings.Contains(stdout, "no backup targets") {
		t.Errorf("stdout = %q, want the empty-list message", stdout)
	}
}

func TestRun_BackupTargetsGet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/backup-targets/bkt_1" {
			t.Errorf("path = %q, want /api/v1/backup-targets/bkt_1", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(backupTargetResource{ID: "bkt_1", Name: "primary", Provider: "aws", Bucket: "backups"})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"backup-targets", "get", "bkt_1", "--api-url", srv.URL})
	if !strings.Contains(stdout, "primary") || !strings.Contains(stdout, "backups") {
		t.Errorf("stdout = %q, want name and bucket shown", stdout)
	}
}

func TestRun_BackupTargetsGet_MissingID(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"backup-targets", "get"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "requires exactly one") {
		t.Errorf("stderr = %q, want a missing-id usage error", stderr.String())
	}
}

func TestRun_BackupTargetsCreate(t *testing.T) {
	var gotPath, gotMethod string
	srv, gotBody := newBackupTargetEchoServer(t, "bkt_2", &gotPath, &gotMethod)

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{
		"backup-targets", "create", "--name", "primary", "--provider", "aws", "--bucket", "backups",
		"--access-key-id", "AKID", "--secret-access-key", "topsecret", "--api-url", srv.URL,
	}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotPath != "/api/v1/backup-targets" || gotMethod != http.MethodPost {
		t.Errorf("request = %s %s, want POST /api/v1/backup-targets", gotMethod, gotPath)
	}
	if gotBody.AccessKeyID != "AKID" || gotBody.SecretAccessKey != "topsecret" {
		t.Errorf("request body = %+v, want the given credentials", gotBody)
	}
	if strings.Contains(stdout.String(), "AKID") || strings.Contains(stdout.String(), "topsecret") {
		t.Errorf("stdout = %q, credentials must never be printed back", stdout.String())
	}
}

func TestRun_BackupTargetsCreate_MissingRequiredFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "missing name", args: []string{"--provider", "aws", "--bucket", "b", "--access-key-id", "a", "--secret-access-key", "s"}, want: "--name is required"},
		{name: "missing provider", args: []string{"--name", "n", "--bucket", "b", "--access-key-id", "a", "--secret-access-key", "s"}, want: "--provider is required"},
		{name: "missing bucket", args: []string{"--name", "n", "--provider", "aws", "--access-key-id", "a", "--secret-access-key", "s"}, want: "--bucket is required"},
		{name: "missing access key id", args: []string{"--name", "n", "--provider", "aws", "--bucket", "b", "--secret-access-key", "s"}, want: "--access-key-id is required"},
		{name: "missing secret access key", args: []string{"--name", "n", "--provider", "aws", "--bucket", "b", "--access-key-id", "a"}, want: "--secret-access-key is required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			got := run("levelrail-cli-test", append([]string{"backup-targets", "create"}, tt.args...), &stdout, &stderr, envMap())
			if got != exitValidation {
				t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitValidation, stderr.String())
			}
			if !strings.Contains(stderr.String(), tt.want) {
				t.Errorf("stderr = %q, want %q", stderr.String(), tt.want)
			}
		})
	}
}

func TestRun_BackupTargetsUpdate(t *testing.T) {
	var gotPath, gotMethod string
	srv, gotBody := newBackupTargetEchoServer(t, "bkt_1", &gotPath, &gotMethod)

	stdout, _ := runCLIExpectOK(t, []string{
		"backup-targets", "update", "bkt_1", "--name", "renamed", "--provider", "r2",
		"--endpoint", "https://x.r2.cloudflarestorage.com", "--bucket", "new-bucket", "--api-url", srv.URL,
	})
	if gotPath != "/api/v1/backup-targets/bkt_1" || gotMethod != http.MethodPut {
		t.Errorf("request = %s %s, want PUT /api/v1/backup-targets/bkt_1", gotMethod, gotPath)
	}
	if gotBody.Name != "renamed" || gotBody.Bucket != "new-bucket" {
		t.Errorf("request body = %+v, want the updated fields", gotBody)
	}
	if gotBody.AccessKeyID != "" || gotBody.SecretAccessKey != "" {
		t.Errorf("request body = %+v, want no credentials when none were given", gotBody)
	}
	if !strings.Contains(stdout, "renamed") {
		t.Errorf("stdout = %q, want confirmation naming the updated target", stdout)
	}
}

func TestRun_BackupTargetsUpdate_RotatesCredentials(t *testing.T) {
	srv, gotBody := newBackupTargetEchoServer(t, "bkt_1", nil, nil)

	runCLIExpectOK(t, []string{
		"backup-targets", "update", "bkt_1", "--name", "primary", "--provider", "aws", "--bucket", "backups",
		"--access-key-id", "AKID2", "--secret-access-key", "rotated", "--api-url", srv.URL,
	})
	if gotBody.AccessKeyID != "AKID2" || gotBody.SecretAccessKey != "rotated" {
		t.Errorf("request body = %+v, want the rotated credentials", gotBody)
	}
}

func TestRun_BackupTargetsUpdate_PartialCredentialsRejected(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{
		"backup-targets", "update", "bkt_1", "--name", "n", "--provider", "aws", "--bucket", "b", "--access-key-id", "only-one",
	}, &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitValidation, stderr.String())
	}
	if !strings.Contains(stderr.String(), "must be set together") {
		t.Errorf("stderr = %q, want a paired-credentials validation error", stderr.String())
	}
}

func TestRun_BackupTargetsDelete(t *testing.T) {
	srv, gotPath, gotMethod := newNoContentEchoServer(t)
	defer srv.Close()

	runCLIExpectOK(t, []string{"backup-targets", "delete", "bkt_1", "--api-url", srv.URL})
	if *gotMethod != http.MethodDelete || *gotPath != "/api/v1/backup-targets/bkt_1" {
		t.Errorf("request = %s %s, want DELETE /api/v1/backup-targets/bkt_1", *gotMethod, *gotPath)
	}
}

func TestRun_BackupTargetsDelete_Conflict(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusConflict, `{"error":"backup target still has backup history; delete is blocked until that's addressed"}`)

	stderr := runCLIExpectAPIError(t, []string{"backup-targets", "delete", "bkt_1", "--api-url", srv.URL})
	if !strings.Contains(stderr, "backup history") {
		t.Errorf("stderr = %q, want the server's conflict message", stderr)
	}
}

func TestRun_BackupTargets_Help(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"backup-targets", "-h"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d", got, exitOK)
	}
	if !strings.Contains(stdout.String(), "backup-targets list") || !strings.Contains(stdout.String(), "backup-targets update") {
		t.Errorf("stdout = %q, want usage text listing list and update", stdout.String())
	}
}

func TestRun_BackupTargets_UnknownSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"backup-targets", "bogus"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "unknown backup-targets subcommand") {
		t.Errorf("stderr = %q, want an unknown-subcommand error", stderr.String())
	}
}

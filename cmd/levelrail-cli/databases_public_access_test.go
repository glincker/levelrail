package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_DatabasesPublicAccessSet(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody setDatabasePublicAccessRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(databasePublicAccessResource{
			DatabaseName: "db", PubliclyAccessible: true, PublicPort: 20001, BindAddress: gotBody.BindAddress,
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"databases", "public-access", "set", "db", "--bind-address", "public", "--api-url", srv.URL})
	if gotMethod != http.MethodPut || gotPath != "/api/v1/databases/db/public-access" {
		t.Errorf("request = %s %s, want PUT /api/v1/databases/db/public-access", gotMethod, gotPath)
	}
	if gotBody.BindAddress != "public" {
		t.Errorf("request body BindAddress = %q, want %q", gotBody.BindAddress, "public")
	}
	if !strings.Contains(stdout, `publicly accessible on port 20001, bound to "public"`) {
		t.Errorf("stdout = %q, want a confirmation with port and bind address", stdout)
	}
}

func TestRun_DatabasesPublicAccessSet_DefaultBindAddressOmittedFromRequest(t *testing.T) {
	var gotBody setDatabasePublicAccessRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(databasePublicAccessResource{DatabaseName: "db", PubliclyAccessible: true, PublicPort: 20001, BindAddress: "private"})
	}))
	defer srv.Close()

	runCLIExpectOK(t, []string{"databases", "public-access", "set", "db", "--api-url", srv.URL})
	if gotBody.BindAddress != "" {
		t.Errorf("request body BindAddress = %q, want empty (let the control plane apply its own default)", gotBody.BindAddress)
	}
}

func TestRun_DatabasesPublicAccessSet_NoName(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"databases", "public-access", "set"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitUsage, stderr.String())
	}
	if !strings.Contains(stderr.String(), "requires exactly one") {
		t.Errorf("stderr = %q, want a missing database name usage error", stderr.String())
	}
}

func TestRun_DatabasesPublicAccessClear(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"databases", "public-access", "clear", "db", "--api-url", srv.URL})
	if gotMethod != http.MethodDelete || gotPath != "/api/v1/databases/db/public-access" {
		t.Errorf("request = %s %s, want DELETE /api/v1/databases/db/public-access", gotMethod, gotPath)
	}
	if !strings.Contains(stdout, `public access removed for database "db"`) {
		t.Errorf("stdout = %q, want a removal confirmation", stdout)
	}
}

func TestRun_DatabasesPublicAccessClear_NoName(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"databases", "public-access", "clear"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitUsage, stderr.String())
	}
	if !strings.Contains(stderr.String(), "requires exactly one") {
		t.Errorf("stderr = %q, want a missing database name usage error", stderr.String())
	}
}

func TestRun_DatabasesPublicAccess_UnknownSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"databases", "public-access", "bogus"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitUsage, stderr.String())
	}
	if !strings.Contains(stderr.String(), "unknown databases public-access subcommand") {
		t.Errorf("stderr = %q, want an unknown-subcommand usage error", stderr.String())
	}
}

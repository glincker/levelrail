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

func TestPRBranchHelpers(t *testing.T) {
	if n, err := prBranchName(42); err != nil || n != "pr-42" {
		t.Fatalf("name = %q, %v", n, err)
	}
	if _, err := prBranchName(0); err == nil {
		t.Fatal("pr 0 accepted")
	}
	for _, ok := range []string{"7d", "12h", "never"} {
		if validatePRTTL(ok) != nil {
			t.Errorf("ttl %q rejected", ok)
		}
	}
	for _, bad := range []string{"", "0d", "7", "-1d", "7d;rm", "1w"} {
		if validatePRTTL(bad) == nil {
			t.Errorf("ttl %q accepted", bad)
		}
	}
	if m, err := validatePRMode("full"); err != nil || m != "full" {
		t.Errorf("full: %q %v", m, err)
	}
	if _, err := validatePRMode("anonymized"); err == nil {
		t.Error("anonymized accepted")
	}
	if _, err := validatePRMode("x"); err == nil {
		t.Error("bad mode accepted")
	}
}

func TestRun_DatabasesBranchPR_CreateThenCloseIdempotent(t *testing.T) {
	exists := false
	var created apiclient.CreateLiveDatabaseBranchRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/databases/main/branches/pr-5":
			if !exists {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"error":"not found"}`))
				return
			}
			_ = json.NewEncoder(w).Encode(apiclient.DatabaseBranchNode{Name: "pr-5", Parent: "main", Mode: "schema", State: "ready"})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/databases/main/branches":
			_ = json.NewDecoder(r.Body).Decode(&created)
			exists = true
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(apiclient.DatabaseBranchNode{Name: "pr-5", Parent: "main", Mode: "schema", State: "ready"})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/databases/pr-5":
			_, _ = w.Write([]byte(`{"name":"pr-5","engine":"postgres"}`))
		case r.Method == http.MethodDelete:
			exists = false
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	args := []string{"databases", "branch", "pr", "main", "--pr", "5", "--json", "--api-url", srv.URL}
	if got := run("levelrail-cli-test", args, &stdout, &stderr, envMap()); got != exitOK {
		t.Fatalf("pr exit = %d (%q %q)", got, stdout.String(), stderr.String())
	}
	if created.Name != "pr-5" || created.Mode != "schema" || created.TTL != "7d" || created.Protected {
		t.Errorf("create body = %+v", created)
	}
	if strings.Contains(stdout.String(), "password") || !strings.Contains(stdout.String(), `"created": true`) {
		t.Errorf("output = %q", stdout.String())
	}
	stdout.Reset()
	if got := run("levelrail-cli-test", args, &stdout, &stderr, envMap()); got != exitOK || !strings.Contains(stdout.String(), `"created": false`) {
		t.Fatalf("rerun exit = %d out=%q", got, stdout.String())
	}
	closeArgs := []string{"databases", "branch", "pr-close", "main", "--pr", "5", "--yes", "--api-url", srv.URL}
	for i := 0; i < 2; i++ {
		stdout.Reset()
		if got := run("levelrail-cli-test", closeArgs, &stdout, &stderr, envMap()); got != exitOK {
			t.Fatalf("close %d exit = %d (%q %q)", i, got, stdout.String(), stderr.String())
		}
	}
	if !strings.Contains(stdout.String(), "not found") {
		t.Errorf("second close output = %q", stdout.String())
	}
}

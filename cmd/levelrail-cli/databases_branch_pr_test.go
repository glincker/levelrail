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
	if n, err := prBranchName("app-db", 42); err != nil || n != "app-db-pr-42" {
		t.Fatalf("name = %q, %v", n, err)
	}
	if _, err := prBranchName("app-db", 0); err == nil {
		t.Fatal("pr 0 accepted")
	}
	if _, err := prBranchName("App_DB", 1); err == nil {
		t.Fatal("invalid db accepted")
	}
	a, _ := prBranchName("app-db", 7)
	b, _ := prBranchName("other-db", 7)
	if a == b {
		t.Fatal("same PR number collided across databases")
	}
	long1 := "a" + strings.Repeat("b", 70) + "1"
	long2 := "a" + strings.Repeat("b", 70) + "2"
	n1, _ := prBranchName(long1, 12)
	n1b, _ := prBranchName(long1, 12)
	n2, _ := prBranchName(long2, 12)
	if len(n1) > 63 || !strings.HasSuffix(n1, "-pr-12") || n1 != n1b || n1 == n2 {
		t.Fatalf("long names: %q %q", n1, n2)
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
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/databases/main/branches/main-pr-5":
			if !exists {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"error":"not found"}`))
				return
			}
			_ = json.NewEncoder(w).Encode(apiclient.DatabaseBranchNode{Name: "main-pr-5", Parent: "main", Mode: "schema", State: "ready"})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/databases/main/branches":
			_ = json.NewDecoder(r.Body).Decode(&created)
			exists = true
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(apiclient.DatabaseBranchNode{Name: "main-pr-5", Parent: "main", Mode: "schema", State: "ready"})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/databases/main-pr-5":
			_, _ = w.Write([]byte(`{"name":"main-pr-5","engine":"postgres"}`))
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
	if created.Name != "main-pr-5" || created.Mode != "schema" || created.TTL != "7d" || created.Protected {
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

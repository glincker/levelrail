package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/glincker/levelrail/internal/apiclient"
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
				_, _ = w.Write([]byte(`{"error":"branch not found"}`))
				return
			}
			_ = json.NewEncoder(w).Encode(apiclient.DatabaseBranchNode{Name: "main-pr-5", Parent: "main", Mode: "schema", State: "ready"})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/databases/main/branches":
			_ = json.NewDecoder(r.Body).Decode(&created)
			exists = true
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(apiclient.DatabaseBranchNode{Name: "main-pr-5", Parent: "main", Mode: "schema", State: "ready"})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/databases/main":
			_, _ = w.Write([]byte(`{"name":"main","engine":"postgres"}`))
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

func TestPRNumberAndVerdicts(t *testing.T) {
	for _, bad := range []string{"", "0", "010", "0x10", "-1", "1e3", "12345678901"} {
		if _, err := parsePRNumber(bad); err == nil {
			t.Errorf("pr %q accepted", bad)
		}
	}
	if n, err := parsePRNumber("42"); err != nil || n != 42 {
		t.Fatalf("42 = %d %v", n, err)
	}
	node := func(parent, state, mode string) apiclient.DatabaseBranchNode {
		return apiclient.DatabaseBranchNode{Name: "main-pr-1", Parent: parent, State: state, Mode: mode}
	}
	if reuse, _, err := prExistingVerdict(node("main", "ready", "schema"), "main", "schema"); !reuse || err != nil {
		t.Errorf("same branch not reused: %v", err)
	}
	if _, rec, err := prExistingVerdict(node("main", "failed", "schema"), "main", "schema"); !rec || err != nil {
		t.Errorf("failed branch not recreated: %v", err)
	}
	for _, n := range []apiclient.DatabaseBranchNode{node("other", "ready", "schema"), node("main", "promoted", "schema"), node("main", "deleting", "schema"), node("main", "ready", "full")} {
		if _, _, err := prExistingVerdict(n, "main", "schema"); err == nil {
			t.Errorf("verdict accepted %+v", n)
		}
	}
}

func TestRun_DatabasesBranchPRClose_Guards(t *testing.T) {
	branch := apiclient.DatabaseBranchNode{Name: "main-pr-3", Parent: "main", State: "ready", Protected: true}
	var deleted bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/databases/main":
			_, _ = w.Write([]byte(`{"name":"main","engine":"postgres"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/databases/main/branches/main-pr-3":
			_ = json.NewEncoder(w).Encode(branch)
		case r.Method == http.MethodPatch:
			branch.Protected = false
			_ = json.NewEncoder(w).Encode(branch)
		case r.Method == http.MethodDelete:
			deleted = true
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"database not found"}`))
		}
	}))
	defer srv.Close()
	closeArgs := func(db string, extra ...string) []string {
		return append([]string{"databases", "branch", "pr-close", db, "--pr", "3", "--yes", "--api-url", srv.URL}, extra...)
	}
	var out, errb bytes.Buffer
	if got := run("levelrail-cli-test", closeArgs("nope"), &out, &errb, envMap()); got == exitOK {
		t.Error("missing parent database reported success")
	}
	if got := run("levelrail-cli-test", closeArgs("main"), &out, &errb, envMap()); got == exitOK || deleted {
		t.Errorf("protected branch deleted without --force (exit %d)", got)
	}
	if got := run("levelrail-cli-test", closeArgs("main", "--force"), &out, &errb, envMap()); got != exitOK || !deleted {
		t.Errorf("force close exit = %d deleted=%v (%s)", got, deleted, errb.String())
	}
	branch.State = "promoted"
	branch.Protected = false
	deleted = false
	if got := run("levelrail-cli-test", closeArgs("main"), &out, &errb, envMap()); got == exitOK || deleted {
		t.Error("promoted branch was deleted")
	}
}

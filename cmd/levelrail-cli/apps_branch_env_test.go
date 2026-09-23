package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_AppsBranchEnv_Set(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody struct {
		BranchPattern string `json:"branch_pattern"`
		Key           string `json:"key"`
		Value         string `json:"value"`
		Secret        bool   `json:"secret"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(appBranchEnvOverride{
			ID: "benv_1", BranchPattern: gotBody.BranchPattern, Key: gotBody.Key, Value: gotBody.Value, Secret: gotBody.Secret,
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "branch-env", "set", "web", "SHARED", "--branch", "release/*", "--value", "release-value", "--api-url", srv.URL})

	if gotMethod != http.MethodPost || gotPath != "/api/v1/apps/web/branch-env" {
		t.Errorf("method/path = %s %s, want POST /api/v1/apps/web/branch-env", gotMethod, gotPath)
	}
	if gotBody.BranchPattern != "release/*" || gotBody.Key != "SHARED" || gotBody.Value != "release-value" || gotBody.Secret {
		t.Errorf("request body = %+v, want branch=release/* key=SHARED value=release-value secret=false", gotBody)
	}
	if !strings.Contains(stdout, "value=release-value") {
		t.Errorf("stdout = %q, want a value line", stdout)
	}
}

func TestRun_AppsBranchEnv_Set_Secret(t *testing.T) {
	var gotBody struct {
		Secret bool   `json:"secret"`
		Value  string `json:"value"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(appBranchEnvOverride{ID: "benv_2", BranchPattern: "main", Key: "API_KEY", Secret: true})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "branch-env", "set", "web", "API_KEY", "--branch", "main", "--value", "sk-abc", "--secret", "--api-url", srv.URL})

	if !gotBody.Secret || gotBody.Value != "sk-abc" {
		t.Errorf("request body = %+v, want secret=true value=sk-abc", gotBody)
	}
	if strings.Contains(stdout, "sk-abc") {
		t.Errorf("stdout = %q, must never echo a secret value back", stdout)
	}
	if !strings.Contains(stdout, "secret=true") {
		t.Errorf("stdout = %q, want a secret confirmation with no value", stdout)
	}
}

func TestRun_AppsBranchEnv_Set_MissingFlags(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "branch-env", "set", "web", "KEY", "--value", "v", "--api-url", "http://unused"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "requires --branch") {
		t.Errorf("stderr = %q, want a missing --branch usage error", stderr.String())
	}
}

func TestRun_AppsBranchEnv_Set_MissingValue(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "branch-env", "set", "web", "KEY", "--branch", "main", "--api-url", "http://unused"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "requires --value") {
		t.Errorf("stderr = %q, want a missing --value usage error", stderr.String())
	}
}

func TestRun_AppsBranchEnv_Set_APIError(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusNotFound, `{"error":"app not found"}`)

	stderr := runCLIExpectAPIError(t, []string{"apps", "branch-env", "set", "web", "KEY", "--branch", "main", "--value", "v", "--api-url", srv.URL})
	if !strings.Contains(stderr, "app not found") {
		t.Errorf("stderr = %q, want the server's error", stderr)
	}
}

func TestRun_AppsBranchEnv_List(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]appBranchEnvOverride{
			{ID: "benv_1", BranchPattern: "release/*", Key: "FEATURE_FLAG", Value: "on"},
			{ID: "benv_2", BranchPattern: "main", Key: "API_KEY", Secret: true},
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "branch-env", "list", "web", "--api-url", srv.URL})

	if gotMethod != http.MethodGet || gotPath != "/api/v1/apps/web/branch-env" {
		t.Errorf("method/path = %s %s, want GET /api/v1/apps/web/branch-env", gotMethod, gotPath)
	}
	if !strings.Contains(stdout, "FEATURE_FLAG") || !strings.Contains(stdout, "release/*") {
		t.Errorf("stdout = %q, want the plain override listed", stdout)
	}
	if !strings.Contains(stdout, "(hidden)") {
		t.Errorf("stdout = %q, want the secret override's value hidden", stdout)
	}
}

func TestRun_AppsBranchEnv_List_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]appBranchEnvOverride{})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "branch-env", "list", "web", "--api-url", srv.URL})
	if !strings.Contains(stdout, "no branch env overrides") {
		t.Errorf("stdout = %q, want the empty-list message", stdout)
	}
}

func TestRun_AppsBranchEnv_Clear(t *testing.T) {
	srv, gotPath, gotMethod := newNoContentEchoServer(t)
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "branch-env", "clear", "web", "benv_1", "--api-url", srv.URL})

	if *gotMethod != http.MethodDelete || *gotPath != "/api/v1/apps/web/branch-env/benv_1" {
		t.Errorf("method/path = %s %s, want DELETE /api/v1/apps/web/branch-env/benv_1", *gotMethod, *gotPath)
	}
	if !strings.Contains(stdout, `"cleared": true`) && !strings.Contains(stdout, "cleared for app") {
		t.Errorf("stdout = %q, want a clear confirmation", stdout)
	}
}

func TestRun_AppsBranchEnv_Help(t *testing.T) {
	stdout, _ := runCLIExpectOK(t, []string{"apps", "branch-env", "-h"})
	if !strings.Contains(stdout, "apps branch-env set") {
		t.Errorf("stdout = %q, want usage text", stdout)
	}
}

func TestRun_AppsBranchEnv_UnknownSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "branch-env", "bogus"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "unknown apps branch-env subcommand") {
		t.Errorf("stderr = %q, want an unknown-subcommand error", stderr.String())
	}
}

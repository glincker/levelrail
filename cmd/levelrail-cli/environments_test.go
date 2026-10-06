package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/experimental"
)

func enableGlobalEnvironments(t *testing.T) {
	t.Helper()
	experimental.Set(experimental.GlobalEnvironments)
	t.Cleanup(experimental.Reset)
}

func TestRun_EnvironmentsList(t *testing.T) {
	enableGlobalEnvironments(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/environments" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode([]apiclient.GlobalEnvironmentResource{{ID: "env_production", Name: "Production", Kind: "production", Scope: "global", Protected: true, AppCount: 2}})
	}))
	defer srv.Close()
	stdout, _ := runCLIExpectOK(t, []string{"environments", "list", "--api-url", srv.URL})
	if !strings.Contains(stdout, "env_production") || !strings.Contains(stdout, "production") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestRun_EnvironmentsCreateUpdateDelete(t *testing.T) {
	enableGlobalEnvironments(t)
	var method, path, query string
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path, query = r.Method, r.URL.Path, r.URL.RawQuery
		body = map[string]any{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(apiclient.EnvironmentResource{ID: "env_x", Name: "QA", Kind: "test"})
	}))
	defer srv.Close()

	runCLIExpectOK(t, []string{"environments", "create", "--name", "QA", "--kind", "test", "--protected", "--api-url", srv.URL})
	if method != http.MethodPost || path != "/api/v1/environments" || body["kind"] != "test" || body["protected"] != true {
		t.Errorf("create sent %s %s %v", method, path, body)
	}

	runCLIExpectOK(t, []string{"environments", "update", "env_x", "--kind", "uat", "--protected=false", "--api-url", srv.URL})
	if method != http.MethodPatch || path != "/api/v1/environments/env_x" || body["kind"] != "uat" {
		t.Errorf("update sent %s %s %v", method, path, body)
	}
	if v, present := body["protected"]; !present || v != false {
		t.Errorf("explicit --protected=false must be sent, body = %v", body)
	}
	if _, present := body["name"]; present {
		t.Errorf("unset --name must not be sent, body = %v", body)
	}

	runCLIExpectOK(t, []string{"environments", "delete", "env_x", "--move-to", "env_dev", "--api-url", srv.URL})
	if method != http.MethodDelete || path != "/api/v1/environments/env_x" || query != "move_to=env_dev" {
		t.Errorf("delete sent %s %s?%s", method, path, query)
	}
}

func TestRun_EnvironmentsUpdate_NeedsAField(t *testing.T) {
	enableGlobalEnvironments(t)
	var out, errb bytes.Buffer
	if got := run("levelrail-cli-test", []string{"environments", "update", "env_x", "--api-url", "http://127.0.0.1:1"}, &out, &errb, envMap()); got == exitOK {
		t.Fatalf("expected failure, stderr = %q", errb.String())
	}
}

func TestRun_MoveEnv(t *testing.T) {
	enableGlobalEnvironments(t)
	for _, noun := range []string{"apps", "databases"} {
		t.Run(noun, func(t *testing.T) {
			var gotPath string
			var gotBody apiclient.MoveEnvironmentRequest
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.Method + " " + r.URL.Path
				_ = json.NewDecoder(r.Body).Decode(&gotBody)
				_ = json.NewEncoder(w).Encode(apiclient.MoveEnvironmentResult{Name: "x"})
			}))
			defer srv.Close()
			stdout, _ := runCLIExpectOK(t, []string{noun, "move-env", "thing", "env_dev", "--api-url", srv.URL})
			if gotPath != "PUT /api/v1/"+noun+"/thing/environment" || gotBody.EnvironmentID != "env_dev" || gotBody.Confirm {
				t.Errorf("sent %s %+v", gotPath, gotBody)
			}
			if !strings.Contains(stdout, "moved") {
				t.Errorf("stdout = %q", stdout)
			}
		})
	}
}

func TestRunMoveEnv_ProtectedPromptsThenReportsPendingApproval(t *testing.T) {
	enableGlobalEnvironments(t)
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req apiclient.MoveEnvironmentRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		calls++
		w.Header().Set("Content-Type", "application/json")
		if !req.Confirm {
			w.WriteHeader(http.StatusConflict)
			_, _ = io.WriteString(w, `{"error":"environment \"Production\" is protected; set confirm: true to proceed"}`)
			return
		}
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(apiclient.MoveEnvironmentResult{PendingApproval: &apiclient.DeployApprovalResource{ID: "dap_1"}})
	}))
	defer srv.Close()

	var out, errb bytes.Buffer
	code := runMoveEnv("levelrail-cli-test", "apps", []string{"web", "env_production", "--api-url", srv.URL}, &out, &errb, envMap(), strings.NewReader("yes\n"), func(ctx context.Context, c *Client, name string, req apiclient.MoveEnvironmentRequest) (apiclient.MoveEnvironmentResult, error) {
		return c.MoveAppEnvironment(ctx, name, req)
	})
	if code != exitOK || calls != 2 || !strings.Contains(out.String(), "pending approval") || !strings.Contains(out.String(), "dap_1") {
		t.Fatalf("code=%d calls=%d out=%q err=%q", code, calls, out.String(), errb.String())
	}

	calls = 0
	out.Reset()
	errb.Reset()
	code = runMoveEnv("levelrail-cli-test", "apps", []string{"web", "env_production", "--api-url", srv.URL}, &out, &errb, envMap(), strings.NewReader("no\n"), func(ctx context.Context, c *Client, name string, req apiclient.MoveEnvironmentRequest) (apiclient.MoveEnvironmentResult, error) {
		return c.MoveAppEnvironment(ctx, name, req)
	})
	if code == exitOK || calls != 1 {
		t.Fatalf("declined prompt must fail without confirming: code=%d calls=%d", code, calls)
	}
}

func TestRun_AppsAndDatabasesListEnvironmentFilter(t *testing.T) {
	enableGlobalEnvironments(t)
	var queries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queries = append(queries, r.URL.Path+"?"+r.URL.RawQuery)
		_, _ = io.WriteString(w, `[]`)
	}))
	defer srv.Close()
	runCLIExpectOK(t, []string{"apps", "list", "--environment", "env_dev", "--api-url", srv.URL})
	runCLIExpectOK(t, []string{"apps", "list", "--environment", "env_dev", "--max-items", "5", "--api-url", srv.URL})
	runCLIExpectOK(t, []string{"databases", "list", "--environment", "env_dev", "--api-url", srv.URL})
	want := []string{"/api/v1/apps?environment=env_dev", "/api/v1/apps?environment=env_dev&limit=5", "/api/v1/databases?environment=env_dev"}
	if strings.Join(queries, "|") != strings.Join(want, "|") {
		t.Errorf("queries = %v, want %v", queries, want)
	}
}

func TestEnvironmentsCommandsGatedByFlag(t *testing.T) {
	t.Cleanup(experimental.Reset)
	experimental.Set()
	for _, args := range [][]string{{"environments", "list"}, {"apps", "move-env", "a", "b"}, {"databases", "move-env", "a", "b"}} {
		var out, errb bytes.Buffer
		if code := run("levelrail-cli-test", args, &out, &errb, envMap()); code != exitUsage || !strings.Contains(errb.String(), "global-environments") {
			t.Errorf("%v with flag off: code=%d stderr=%q", args, code, errb.String())
		}
	}
	if strings.Contains(rootUsage("x"), "move-env") || strings.Contains(rootUsage("x"), "environments list|create") {
		t.Error("usage must hide gated commands when the flag is off")
	}
	experimental.Set(experimental.GlobalEnvironments)
	if !strings.Contains(rootUsage("x"), "environments list|create") {
		t.Error("usage must show the commands when the flag is on")
	}
}

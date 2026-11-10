package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestServiceTemplatesValidate_PrintsFieldErrorsAndExitsOne(t *testing.T) {
	var gotPath string
	var gotBody apiclient.ValidateTemplateRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_ = json.NewEncoder(w).Encode(apiclient.ValidateTemplateResult{Valid: false, ID: "x", Errors: []string{"slogan: required"}})
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "template.yaml"), []byte("id: x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte("services: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errBuf bytes.Buffer
	code := run("levelrail-cli-test", []string{"templates", "service", "validate", "-f", dir, "--api-url", srv.URL}, &out, &errBuf, envMap())
	if code != exitValidation {
		t.Fatalf("exit = %d, want %d (stderr=%q)", code, exitValidation, errBuf.String())
	}
	if gotPath != "/api/v1/service-templates/validate" || gotBody.TemplateYAML != "id: x\n" {
		t.Errorf("path=%s body=%+v", gotPath, gotBody)
	}
	if !strings.Contains(out.String(), "slogan: required") {
		t.Errorf("stdout = %q, want the field error", out.String())
	}
}

func TestServiceTemplatesDeploy_PlanThenYes(t *testing.T) {
	var posts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			posts++
			_ = json.NewEncoder(w).Encode(apiclient.ComposeStackDeploy{AppID: "redis-abc12"})
			return
		}
		_ = json.NewEncoder(w).Encode(apiclient.ServiceTemplateDetail{ID: "redis", Name: "Redis"})
	}))
	t.Cleanup(srv.Close)

	stdout, _ := runCLIExpectOK(t, []string{"templates", "service", "deploy", "redis", "--api-url", srv.URL})
	if posts != 0 || !strings.Contains(stdout, "preview only") {
		t.Fatalf("plan must not post: posts=%d out=%q", posts, stdout)
	}
	stdout, _ = runCLIExpectOK(t, []string{"templates", "service", "deploy", "redis", "--yes", "--api-url", srv.URL})
	if posts != 1 || !strings.Contains(stdout, "redis-abc12") {
		t.Fatalf("apply: posts=%d out=%q", posts, stdout)
	}
}

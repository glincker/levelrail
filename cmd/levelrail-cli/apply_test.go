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
	"github.com/GLINCKER/levelrail/internal/iac"
)

type iacFake struct {
	plan     iac.Plan
	applied  bool
	issues   []iac.Issue
	requests []apiclient.IaCRequest
	export   iac.ExportResult
}

func (f *iacFake) handler(t *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/apply/plan", r.Method == http.MethodPost && r.URL.Path == "/api/v1/apply":
			var req apiclient.IaCRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Errorf("decode: %v", err)
			}
			f.requests = append(f.requests, req)
			if len(f.issues) > 0 {
				w.WriteHeader(http.StatusUnprocessableEntity)
				_ = json.NewEncoder(w).Encode(map[string]any{"issues": f.issues})
				return
			}
			if r.URL.Path == "/api/v1/apply/plan" {
				_ = json.NewEncoder(w).Encode(map[string]any{"plan": f.plan})
				return
			}
			f.applied = true
			res := iac.ApplyResult{Plan: f.plan, Applied: f.plan.Summary.Create + f.plan.Summary.Update}
			for _, c := range f.plan.Changes {
				res.Results = append(res.Results, iac.ItemResult{Change: c, Status: iac.StatusApplied})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"result": res})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/export":
			_ = json.NewEncoder(w).Encode(f.export)
		default:
			http.NotFound(w, r)
		}
	})
}

func pendingPlan() iac.Plan {
	return iac.Plan{
		Changes: []iac.Change{
			{Kind: iac.KindApp, Name: "web", Action: iac.ActionUpdate, File: "infra/web.yaml", Line: 3,
				Fields: []iac.FieldChange{{Path: "image", Op: iac.OpChange, Old: "a:1", New: "a:2"}, {Path: "env.LOG", Op: iac.OpAdd, New: "(hidden)"}}},
			{Kind: iac.KindTag, Name: "x", Action: iac.ActionNoop},
		},
		Summary: iac.Summary{Update: 1, Noop: 1}, Hash: "abc123",
	}
}

func writeInfra(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "web.yaml"), []byte("version: 1\nkind: Tag\nmetadata: {name: x}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "b.yml"), []byte("version: 1\nkind: Tag\nmetadata: {name: y}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("ignore me"), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func runIaC(args []string, env map[string]string) (code int, stdout, stderr string) {
	var out, errBuf bytes.Buffer
	lookup := func(k string) (string, bool) { v, ok := env[k]; return v, ok }
	code = run("cli", args, &out, &errBuf, lookup)
	return code, out.String(), errBuf.String()
}

func TestApply_DryRunExitCodes(t *testing.T) {
	f := &iacFake{plan: pendingPlan()}
	srv := httptest.NewServer(f.handler(t))
	defer srv.Close()
	dir := writeInfra(t)

	code, out, _ := runIaC([]string{"apply", "-f", dir, "--dry-run", "--exit-code", "--api-url", srv.URL}, nil)
	if code != iacExitPending {
		t.Fatalf("exit = %d, want %d", code, iacExitPending)
	}
	for _, want := range []string{"1 to update", "~ App/web", "~ image: a:1 -> a:2", "+ env.LOG: (hidden)", "infra/web.yaml:3"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Tag/x") {
		t.Errorf("unchanged items should not be listed:\n%s", out)
	}
	if f.applied {
		t.Fatal("dry run applied")
	}
	if code, _, _ := runIaC([]string{"apply", "-f", dir, "--dry-run", "--api-url", srv.URL}, nil); code != exitOK {
		t.Fatalf("dry run without --exit-code = %d", code)
	}
	f.plan = iac.Plan{Summary: iac.Summary{Noop: 2}}
	if code, _, _ := runIaC([]string{"apply", "-f", dir, "--dry-run", "--exit-code", "--api-url", srv.URL}, nil); code != exitOK {
		t.Fatalf("no changes = %d", code)
	}
	f.plan = pendingPlan()
	if code, _, _ := runIaC([]string{"diff", "-f", dir, "--api-url", srv.URL}, nil); code != iacExitPending {
		t.Fatalf("diff with drift = %d", code)
	}
}

func TestApply_SendsFilesSecretsAndVarsAndAppliesWithYes(t *testing.T) {
	f := &iacFake{plan: pendingPlan()}
	srv := httptest.NewServer(f.handler(t))
	defer srv.Close()
	dir := writeInfra(t)
	if err := os.WriteFile(filepath.Join(dir, "vars.yaml"), []byte("version: 1\nkind: Project\nmetadata: {name: p}\nspec:\n  env: {REGION: \"${{ env.REGION }}\"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	secretFile := filepath.Join(t.TempDir(), "pw")
	if err := os.WriteFile(secretFile, []byte("from-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	code, out, stderr := runIaC([]string{"apply", "-f", dir, "--yes", "--source", "ci", "--project", "p", "--no-deploy",
		"--secret", "API_KEY=env:CI_KEY", "--secret", "web/DB_PASSWORD=file:" + secretFile, "--api-url", srv.URL}, map[string]string{"CI_KEY": "s3cret", "REGION": "eu"})
	if code != exitOK {
		t.Fatalf("exit = %d stderr = %s", code, stderr)
	}
	if !f.applied || len(f.requests) != 2 {
		t.Fatalf("applied = %v requests = %d", f.applied, len(f.requests))
	}
	req := f.requests[1]
	if req.ExpectedPlanHash != "abc123" || req.Source != "ci" || req.Project != "p" || !req.NoDeploy {
		t.Fatalf("request = %+v", req)
	}
	if req.Secrets["API_KEY"] != "s3cret" || req.Secrets["web/DB_PASSWORD"] != "from-file" || req.Vars["REGION"] != "eu" {
		t.Fatalf("secrets/vars = %v %v", req.Secrets, req.Vars)
	}
	if len(req.Files) != 3 || req.Files[0].Name != filepath.ToSlash(filepath.Join(dir, "sub", "b.yml")) {
		t.Fatalf("files = %+v", req.Files)
	}
	if strings.Contains(out+stderr, "s3cret") || strings.Contains(out+stderr, "from-file") {
		t.Fatalf("secret value printed: %s %s", out, stderr)
	}
	if !strings.Contains(out, "1 applied, 0 failed, 0 skipped") {
		t.Fatalf("output:\n%s", out)
	}
}

func TestApply_NeedsConfirmationWithoutTerminal(t *testing.T) {
	f := &iacFake{plan: pendingPlan()}
	srv := httptest.NewServer(f.handler(t))
	defer srv.Close()
	code, _, stderr := runIaC([]string{"apply", "-f", writeInfra(t), "--api-url", srv.URL}, nil)
	if code != iacExitError || !strings.Contains(stderr, "--yes") || f.applied {
		t.Fatalf("code = %d applied = %v stderr = %s", code, f.applied, stderr)
	}
}

func TestApply_ValidationIssuesAreLineNumbered(t *testing.T) {
	f := &iacFake{issues: []iac.Issue{{File: "infra/web.yaml", Line: 14, Path: "spec.service.prot", Message: "additional properties 'prot' not allowed"}}}
	srv := httptest.NewServer(f.handler(t))
	defer srv.Close()
	code, _, stderr := runIaC([]string{"apply", "-f", writeInfra(t), "--yes", "--api-url", srv.URL}, nil)
	if code != iacExitError || !strings.Contains(stderr, "infra/web.yaml:14: additional properties 'prot' not allowed") {
		t.Fatalf("code = %d stderr = %s", code, stderr)
	}
	if f.applied {
		t.Fatal("applied despite issues")
	}
}

func TestApply_PlanErrorsApplyNothing(t *testing.T) {
	f := &iacFake{plan: iac.Plan{
		Changes: []iac.Change{{Kind: iac.KindApp, Name: "web", Action: iac.ActionError, Reason: "project \"ghost\" does not exist", Denied: false}, {Kind: iac.KindTag, Name: "t", Action: iac.ActionCreate}},
		Summary: iac.Summary{Error: 1, Create: 1},
	}}
	srv := httptest.NewServer(f.handler(t))
	defer srv.Close()
	code, out, _ := runIaC([]string{"apply", "-f", writeInfra(t), "--yes", "--api-url", srv.URL}, nil)
	if code != iacExitError || f.applied || !strings.Contains(out, "! App/web") || !strings.Contains(out, "ghost") {
		t.Fatalf("code = %d applied = %v out = %s", code, f.applied, out)
	}
}

func TestApply_UsageErrors(t *testing.T) {
	for name, args := range map[string][]string{
		"no files":             {"apply", "--yes"},
		"prune without source": {"apply", "-f", "x", "--prune"},
		"bad secret source":    {"apply", "-f", "x", "--secret", "A=plaintext"},
	} {
		if code, _, _ := runIaC(args, nil); code != iacExitError {
			t.Errorf("%s: exit = %d", name, code)
		}
	}
}

func TestApply_UnsetSecretEnvIsAnError(t *testing.T) {
	code, _, stderr := runIaC([]string{"apply", "-f", writeInfra(t), "--secret", "A=env:NOPE"}, nil)
	if code != iacExitError || !strings.Contains(stderr, "NOPE") {
		t.Fatalf("code = %d stderr = %s", code, stderr)
	}
}

func TestExport_WritesFilesAndWarns(t *testing.T) {
	f := &iacFake{export: iac.ExportResult{
		Files:    []iac.ExportFile{{Name: "app-web.yaml", Kind: iac.KindApp, Content: "version: 1\n"}, {Name: "tag-x.yaml", Kind: iac.KindTag, Content: "version: 1\n"}},
		Warnings: []string{"app web: volumes are not exported"},
	}}
	srv := httptest.NewServer(f.handler(t))
	defer srv.Close()
	dir := filepath.Join(t.TempDir(), "out")

	code, _, stderr := runIaC([]string{"export", "--project", "shop", "-o", dir, "--api-url", srv.URL}, nil)
	if code != exitOK || !strings.Contains(stderr, "warning: app web: volumes") {
		t.Fatalf("code = %d stderr = %s", code, stderr)
	}
	b, err := os.ReadFile(filepath.Join(dir, "app-web.yaml")) //nolint:gosec // reads the test's own temp file
	if err != nil || string(b) != "version: 1\n" {
		t.Fatalf("file: %v %q", err, b)
	}
	code, out, _ := runIaC([]string{"export", "--api-url", srv.URL}, nil)
	if code != exitOK || !strings.Contains(out, "---\n") {
		t.Fatalf("stdout export = %d %q", code, out)
	}
}

func TestExport_RefusesPathTraversalNames(t *testing.T) {
	err := writeExport(t.TempDir(), iac.ExportResult{Files: []iac.ExportFile{{Name: "../evil.yaml", Content: "x"}}})
	if err == nil {
		t.Fatal("expected a refusal")
	}
}

func TestReadIaCFilesFromStdin(t *testing.T) {
	old := stdinSource
	defer func() { stdinSource = old }()
	stdinSource = strings.NewReader("version: 1\n")
	files, err := readIaCFiles([]string{"-"})
	if err != nil || len(files) != 1 || files[0].Name != "stdin" {
		t.Fatalf("files = %+v err = %v", files, err)
	}
}

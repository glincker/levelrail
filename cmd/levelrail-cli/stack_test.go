package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/iac"
)

type stackFake struct {
	t         *testing.T
	plan      iac.Plan
	status    string // revision status returned by apply and rollback
	conflict  string // when set, apply answers 409 with this message
	requests  map[string]apiclient.StackRequest
	calls     []string
	rawBodies []string
}

func (f *stackFake) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		f.calls = append(f.calls, r.Method+" "+r.URL.RequestURI())
		body, _ := io.ReadAll(r.Body)
		f.rawBodies = append(f.rawBodies, string(body))
		var req apiclient.StackRequest
		_ = json.Unmarshal(body, &req)
		if f.requests == nil {
			f.requests = map[string]apiclient.StackRequest{}
		}
		f.requests[r.URL.Path] = req
		rev := map[string]any{"number": 4, "status": f.status, "plan_hash": "h1", "actor": "ci", "created_at": "2026-10-25T10:00:00Z"}
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/plan"):
			_ = json.NewEncoder(w).Encode(map[string]any{"stack": "shop", "stack_exists": false, "plan": f.plan})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/apply"):
			if f.conflict != "" {
				w.WriteHeader(http.StatusConflict)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": f.conflict})
				return
			}
			res := iac.ApplyResult{Plan: f.plan, Applied: 1}
			if f.status != "applied" {
				res.Failed = 1
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"stack": "shop", "revision": rev, "result": res})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/rollback"):
			rev["rollback_of"] = 2
			_ = json.NewEncoder(w).Encode(map[string]any{"stack": "shop", "revision": rev, "result": iac.ApplyResult{Applied: 1}})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/revisions"):
			_ = json.NewEncoder(w).Encode(map[string]any{"stack": "shop", "pinned_revision": 3, "revisions": []any{
				map[string]any{"number": 4, "status": "applied", "actor": "ci", "git_sha": "0123456789abcdef", "created_at": "2026-10-25T10:00:00Z"},
				map[string]any{"number": 3, "status": "superseded", "actor": "ana", "rollback_of": 1, "created_at": "2026-10-24T09:30:00Z"},
			}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/stacks":
			_ = json.NewEncoder(w).Encode(map[string]any{"stacks": []any{map[string]any{"name": "shop", "project": "p", "latest_revision": 4, "latest_status": "applied"}}})
		default:
			http.NotFound(w, r)
		}
	})
}

func stackTestDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	stack := "version: 1\nkind: Stack\nmetadata: {name: shop}\nspec:\n  project: shopproj\n  prune: true\n"
	res := "version: 1\nkind: Tag\nmetadata: {name: x}\n"
	if err := os.WriteFile(filepath.Join(dir, "stack.yaml"), []byte(stack), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "apps.yaml"), []byte(res+"---\n"+strings.ReplaceAll(res, "x", "y")), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestStackPlan_StripsStackDocAndUsesItsDefaults(t *testing.T) {
	f := &stackFake{t: t, plan: pendingPlan()}
	srv := httptest.NewServer(f.handler())
	defer srv.Close()
	dir := stackTestDir(t)

	code, out, _ := runIaC([]string{"stack", "plan", "shop", "-f", dir, "--exit-code", "--api-url", srv.URL}, nil)
	if code != iacExitPending {
		t.Fatalf("exit = %d, want %d\n%s", code, iacExitPending, out)
	}
	for _, want := range []string{"Stack shop (new", "Plan: 0 to create, 1 to update", "~  update  App/web", "~ image: a:1 -> a:2", "Plan hash: abc123"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	req := f.requests["/api/v1/stacks/shop/plan"]
	if req.Project != "shopproj" || !req.Prune {
		t.Errorf("defaults not taken from the Stack document: %+v", req)
	}
	for _, file := range req.Files {
		if strings.Contains(file.Content, "kind: Stack") {
			t.Errorf("Stack document was sent in %s", file.Name)
		}
	}
	if len(req.Files) != 1 || !strings.Contains(req.Files[0].Content, "name: y") || !strings.Contains(req.Files[0].Content, "name: x") {
		t.Errorf("resource documents were lost: %+v", req.Files)
	}
	if code, _, _ := runIaC([]string{"stack", "plan", "shop", "-f", dir, "--api-url", srv.URL}, nil); code != exitOK {
		t.Fatalf("plan without --exit-code = %d", code)
	}
	if code, _, e := runIaC([]string{"stack", "plan", "other", "-f", dir, "--api-url", srv.URL}, nil); code == exitOK || !strings.Contains(e, `named "shop"`) {
		t.Fatalf("name mismatch not rejected: %d %s", code, e)
	}
	_, jsonOut, _ := runIaC([]string{"stack", "plan", "shop", "-f", dir, "--json", "--api-url", srv.URL}, nil)
	var parsed map[string]any
	if err := json.Unmarshal([]byte(jsonOut), &parsed); err != nil || parsed["stack"] != "shop" {
		t.Errorf("--json is not the server body: %v\n%s", err, jsonOut)
	}
}

func TestStackApply_SendsSecretsGitAndRedacts(t *testing.T) {
	f := &stackFake{t: t, plan: pendingPlan(), status: "applied"}
	srv := httptest.NewServer(f.handler())
	defer srv.Close()
	dir := stackTestDir(t)
	env := map[string]string{
		"DBPW": "hunter2-secret", "GITHUB_REPOSITORY": "acme/shop", "GITHUB_REF": "refs/heads/main",
		"GITHUB_SHA": "deadbeef", "GITHUB_RUN_ID": "77", "GITHUB_SERVER_URL": "https://github.example",
	}
	code, out, errOut := runIaC([]string{"stack", "apply", "shop", "-f", dir, "--yes", "--secret", "DB_PASSWORD=DBPW", "--api-url", srv.URL}, env)
	if code != exitOK {
		t.Fatalf("exit = %d\n%s%s", code, out, errOut)
	}
	req := f.requests["/api/v1/stacks/shop/apply"]
	if req.Secrets["DB_PASSWORD"] != "hunter2-secret" {
		t.Errorf("secret value not sent: %+v", req.Secrets)
	}
	if req.GitRepo != "acme/shop" || req.GitRef != "refs/heads/main" || req.GitSHA != "deadbeef" || req.CIRunURL != "https://github.example/acme/shop/actions/runs/77" {
		t.Errorf("git metadata: %+v", req)
	}
	if req.IdempotencyKey == "" {
		t.Error("no idempotency key")
	}
	if !strings.Contains(out, "revision 4 applied") {
		t.Errorf("output: %s", out)
	}
	if strings.Contains(out+errOut, "hunter2-secret") {
		t.Errorf("secret value was printed:\n%s%s", out, errOut)
	}
}

func TestStackApply_ServerMessageNeverEchoesSecret(t *testing.T) {
	f := &stackFake{t: t, plan: pendingPlan(), conflict: "boom hunter2-secret boom"}
	srv := httptest.NewServer(f.handler())
	defer srv.Close()
	code, out, errOut := runIaC([]string{"stack", "apply", "shop", "-f", stackTestDir(t), "--yes", "--secret", "DB_PASSWORD=DBPW", "--api-url", srv.URL}, map[string]string{"DBPW": "hunter2-secret"})
	if code == exitOK || strings.Contains(out+errOut, "hunter2-secret") || !strings.Contains(errOut, "boom ***") {
		t.Fatalf("code %d, output not redacted:\n%s%s", code, out, errOut)
	}
}

func TestStackApply_HashMismatchIs409(t *testing.T) {
	f := &stackFake{t: t, plan: pendingPlan(), conflict: "plan hash changed since the plan was made"}
	srv := httptest.NewServer(f.handler())
	defer srv.Close()
	code, _, errOut := runIaC([]string{"stack", "apply", "shop", "-f", stackTestDir(t), "--yes", "--expected-plan-hash", "old", "--api-url", srv.URL}, nil)
	if code == exitOK || !strings.Contains(errOut, "409") || !strings.Contains(errOut, "plan hash changed") {
		t.Fatalf("code %d, stderr %q", code, errOut)
	}
	if got := f.requests["/api/v1/stacks/shop/apply"].ExpectedPlanHash; got != "old" {
		t.Errorf("expected hash sent = %q", got)
	}
}

func TestStackApply_FailedAndPartialRevisionsExitNonZero(t *testing.T) {
	for _, status := range []string{"failed", "partial"} {
		f := &stackFake{t: t, plan: pendingPlan(), status: status}
		srv := httptest.NewServer(f.handler())
		code, out, errOut := runIaC([]string{"stack", "apply", "shop", "-f", stackTestDir(t), "--yes", "--api-url", srv.URL}, nil)
		srv.Close()
		if code != iacExitError {
			t.Errorf("%s: exit = %d, want %d\n%s", status, code, iacExitError, out)
		}
		if !strings.Contains(errOut, "ended "+status) {
			t.Errorf("%s: stderr %q", status, errOut)
		}
	}
}

func TestStackApply_WithoutYesNeedsTerminalAfterPlan(t *testing.T) {
	f := &stackFake{t: t, plan: pendingPlan(), status: "applied"}
	srv := httptest.NewServer(f.handler())
	defer srv.Close()
	code, _, errOut := runIaC([]string{"stack", "apply", "shop", "-f", stackTestDir(t), "--api-url", srv.URL}, nil)
	if code == exitOK || !strings.Contains(errOut, "--yes") {
		t.Fatalf("code %d, stderr %q", code, errOut)
	}
	for _, c := range f.calls {
		if strings.HasSuffix(c, "/apply") {
			t.Fatal("applied without confirmation")
		}
	}
}

func TestStackHistory(t *testing.T) {
	f := &stackFake{t: t}
	srv := httptest.NewServer(f.handler())
	defer srv.Close()
	code, out, _ := runIaC([]string{"stack", "history", "shop", "--limit", "5", "--api-url", srv.URL}, nil)
	if code != exitOK {
		t.Fatalf("exit %d", code)
	}
	for _, want := range []string{"REV", "0123456789abcdef"[:8], "rollback of 1 pinned", "2026-10-24 09:30"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if f.calls[0] != "GET /api/v1/stacks/shop/revisions?limit=5" {
		t.Errorf("call = %s", f.calls[0])
	}
	if code, out, _ := runIaC([]string{"stack", "list", "--api-url", srv.URL}, nil); code != exitOK || !strings.Contains(out, "shop") {
		t.Errorf("list: %d %s", code, out)
	}
}

func TestStackRollback_WarnsAndRunsAsNewRevision(t *testing.T) {
	f := &stackFake{t: t, status: "applied"}
	srv := httptest.NewServer(f.handler())
	defer srv.Close()
	code, out, errOut := runIaC([]string{"stack", "rollback", "shop", "2", "--yes", "--api-url", srv.URL}, nil)
	if code != exitOK {
		t.Fatalf("exit %d\n%s%s", code, out, errOut)
	}
	if !strings.Contains(errOut, "NOT rolled back") {
		t.Errorf("no warning: %q", errOut)
	}
	if !strings.Contains(out, "revision 4 rolled back") || !strings.Contains(out, "restores revision 2") {
		t.Errorf("output: %s", out)
	}
	if f.calls[0] != "POST /api/v1/stacks/shop/revisions/2/rollback" {
		t.Errorf("call = %s", f.calls[0])
	}
	f.status = "failed"
	if code, _, _ := runIaC([]string{"stack", "rollback", "shop", "2", "--yes", "--api-url", srv.URL}, nil); code != iacExitError {
		t.Errorf("failed rollback exit = %d", code)
	}
	if code, _, _ := runIaC([]string{"stack", "rollback", "shop", "zero", "--yes", "--api-url", srv.URL}, nil); code == exitOK {
		t.Error("bad revision accepted")
	}
}

func TestStripStackDocs_LeavesPlainFilesUntouched(t *testing.T) {
	mixed := apiclient.IaCFile{Name: "m.yaml", Content: "kind: Tag\nmetadata: {name: m}\n---\nkind: Stack\nmetadata: {name: s}\nspec: {project: p}\n"}
	if out, doc, err := stripStackDocs([]apiclient.IaCFile{mixed}); err != nil || doc == nil || len(out) != 1 || strings.Contains(out[0].Content, "Stack") || !strings.Contains(out[0].Content, "name: m") {
		t.Fatalf("mixed file: out=%+v doc=%+v err=%v", out, doc, err)
	}
	in := []apiclient.IaCFile{{Name: "a.yaml", Content: "kind: Tag\n"}, {Name: "s.yaml", Content: "kind: Stack\nmetadata: {name: s}\nspec: {project: p}\n"}}
	out, doc, err := stripStackDocs(in)
	if err != nil || doc == nil || doc.Spec.Project != "p" || len(out) != 1 || out[0].Content != "kind: Tag\n" {
		t.Fatalf("out=%+v doc=%+v err=%v", out, doc, err)
	}
}

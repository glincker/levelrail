package main

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/iac"
)

func composeStackDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	app := "version: 1\nkind: App\nmetadata: {name: shop}\nspec:\n  compose:\n    path: compose.yaml\n"
	if err := os.WriteFile(filepath.Join(dir, "app.yaml"), []byte(app), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte("services:\n  web:\n    image: nginx:1.27\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestStackPlan_SendsComposeAttachment(t *testing.T) {
	f := &stackFake{t: t, plan: pendingPlan()}
	srv := newStackServer(f)
	defer srv.Close()
	dir := composeStackDir(t)

	if code, out, _ := runIaC([]string{"stack", "plan", "shop", "-f", dir, "--api-url", srv.URL}, nil); code != exitOK {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	req := f.requests["/api/v1/stacks/shop/plan"]
	if !strings.Contains(req.Attachments["compose.yaml"], "nginx:1.27") {
		t.Fatalf("attachments = %+v", req.Attachments)
	}
	for _, file := range req.Files {
		if strings.Contains(file.Content, "nginx:1.27") {
			t.Errorf("compose body left among the files: %s", file.Name)
		}
	}
}

func TestStackPlan_RefusesSymlinkedCompose(t *testing.T) {
	f := &stackFake{t: t, plan: pendingPlan()}
	srv := newStackServer(f)
	defer srv.Close()
	dir := composeStackDir(t)
	outside := filepath.Join(t.TempDir(), "secret.yaml")
	if err := os.WriteFile(outside, []byte("services: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "compose.yaml")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "compose.yaml")); err != nil {
		t.Skip("symlinks unavailable")
	}
	code, _, _ := runIaC([]string{"stack", "plan", "shop", "-f", dir, "--api-url", srv.URL}, nil)
	if code == exitOK || len(f.calls) != 0 {
		t.Fatalf("symlink not refused: code %d, calls %v", code, f.calls)
	}
}

func TestStackConfirmDestroy_CheckedAndSent(t *testing.T) {
	f := &stackFake{t: t, plan: pendingPlan(), status: "applied"}
	srv := newStackServer(f)
	defer srv.Close()
	dir := stackTestDir(t)

	if code, _, e := runIaC([]string{"stack", "plan", "shop", "-f", dir, "--confirm-destroy", "other", "--api-url", srv.URL}, nil); code == exitOK || !strings.Contains(e, `--confirm-destroy must equal the stack name "shop"`) {
		t.Fatalf("mismatch not refused: %d %s", code, e)
	}
	if code, _, e := runIaC([]string{"stack", "apply", "shop", "-f", dir, "--yes", "--confirm-destroy", "other", "--api-url", srv.URL}, nil); code == exitOK || !strings.Contains(e, "must equal the stack name") {
		t.Fatalf("apply mismatch not refused: %d %s", code, e)
	}
	if len(f.calls) != 0 {
		t.Fatalf("a refused flag still called the server: %v", f.calls)
	}
	if code, _, _ := runIaC([]string{"stack", "plan", "shop", "-f", dir, "--confirm-destroy", "shop", "--api-url", srv.URL}, nil); code != exitOK {
		t.Fatalf("plan exit = %d", code)
	}
	if got := f.requests["/api/v1/stacks/shop/plan"].ConfirmDestroy; got != "shop" {
		t.Errorf("plan confirm_destroy = %q", got)
	}
	if code, _, _ := runIaC([]string{"stack", "apply", "shop", "-f", dir, "--yes", "--confirm-destroy", "shop", "--api-url", srv.URL}, nil); code != exitOK {
		t.Fatalf("apply exit = %d", code)
	}
	if got := f.requests["/api/v1/stacks/shop/apply"].ConfirmDestroy; got != "shop" {
		t.Errorf("apply confirm_destroy = %q", got)
	}
	// --yes alone must not confirm a destroy.
	if code, _, _ := runIaC([]string{"stack", "apply", "shop", "-f", dir, "--yes", "--api-url", srv.URL}, nil); code != exitOK {
		t.Fatalf("apply exit = %d", code)
	}
	if got := f.requests["/api/v1/stacks/shop/apply"].ConfirmDestroy; got != "" {
		t.Errorf("--yes implied confirm_destroy = %q", got)
	}
}

func TestStackPlan_HintsConfirmDestroy(t *testing.T) {
	plan := pendingPlan()
	plan.Changes = append(plan.Changes, iac.Change{Kind: iac.KindApp, Name: "old", Action: iac.ActionDelete, Code: iac.CodeDestroyNotConfirmed, Reason: "destroy not confirmed"})
	plan.Summary.Error = 1
	f := &stackFake{t: t, plan: plan}
	srv := newStackServer(f)
	defer srv.Close()
	dir := stackTestDir(t)

	_, out, _ := runIaC([]string{"stack", "plan", "shop", "-f", dir, "--api-url", srv.URL}, nil)
	if !strings.Contains(out, "--confirm-destroy shop") || !strings.Contains(out, "stack apply shop -f "+dir) {
		t.Errorf("hint missing:\n%s", out)
	}
	_, js, _ := runIaC([]string{"stack", "plan", "shop", "-f", dir, "--json", "--api-url", srv.URL}, nil)
	if strings.Contains(js, "hint:") {
		t.Errorf("--json output carries the hint:\n%s", js)
	}
}

func newStackServer(f *stackFake) *httptest.Server { return httptest.NewServer(f.handler()) }

package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestLinkedAppResolvesFromLinkAndSpec(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(sub, 0o750); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	var out, errb bytes.Buffer
	if code := runLink("lr", []string{"shop"}, &out, &errb, noEnv); code != exitOK {
		t.Fatalf("link: %d %s", code, errb.String())
	}
	t.Chdir(sub)
	if n, _ := linkedAppName(); n != "shop" {
		t.Fatalf("walk up found %q", n)
	}
	t.Chdir(t.TempDir())
	if err := os.WriteFile("app.yaml", []byte("version: 1\nservices:\n  web:\n    port: 80\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if n, src := linkedAppName(); n != "web" || src != "app.yaml" {
		t.Fatalf("app.yaml gave %q from %q", n, src)
	}
}

func TestSharedDryRunSendsNoMutation(t *testing.T) {
	var writes atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writes.Add(1)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	t.Chdir(t.TempDir())
	if err := os.MkdirAll(".levelrail", 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(appLinkFile, []byte("shop\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	code := run("lr", []string{"apps", "delete", "--dry-run", "--api-url", srv.URL, "--token", "t", "--yes"}, &out, &errb, noEnv)
	dryRunRequested = false
	if code != exitOK || writes.Load() != 0 || !strings.Contains(errb.String(), "dry-run: would DELETE") {
		t.Fatalf("code=%d writes=%d stderr=%q", code, writes.Load(), errb.String())
	}
}

func TestJSONFieldSelection(t *testing.T) {
	got := pickFields([]any{map[string]any{"name": "a", "status": "up", "x": 1}}, []string{"name", "status"})
	m := got.([]any)[0].(map[string]any)
	if len(m) != 2 || m["name"] != "a" || m["status"] != "up" {
		t.Fatalf("picked %v", m)
	}
}

func TestCommandSearchFindsRollback(t *testing.T) {
	var out bytes.Buffer
	runCommandSearch("lr", []string{"roll", "back"}, &out)
	if !strings.Contains(out.String(), "rollback") {
		t.Fatalf("no rollback in %q", out.String())
	}
}

func TestCompletionInstallWritesScript(t *testing.T) {
	home := t.TempDir()
	env := func(k string) (string, bool) {
		if k == "HOME" {
			return home, true
		}
		return "", false
	}
	t.Setenv("HOME", home)
	var out, errb bytes.Buffer
	if code := runCompletionInstall("lr", []string{"--shell", "fish"}, &out, &errb, env); code != exitOK {
		t.Fatalf("code %d %s", code, errb.String())
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "fish", "completions", "lr.fish")); err != nil {
		t.Fatal(err)
	}
}

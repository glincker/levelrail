package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMCPAddWritesEnvRefOnly(t *testing.T) {
	home := t.TempDir()
	env := func(k string) (string, bool) {
		if k == "HOME" {
			return home, true
		}
		return "", false
	}
	var out, errb bytes.Buffer
	if code := runMCP("levelrail-cli", []string{"add", "windsurf", "--yes"}, &out, &errb, env); code != exitOK {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	raw, err := os.ReadFile(filepath.Join(home, ".codeium", "windsurf", "mcp_config.json")) //nolint:gosec // test path
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "${env:APP_API_TOKEN}") || !strings.Contains(out.String(), "+") {
		t.Fatalf("config %s diff %s", raw, out.String())
	}
	if code := runMCP("levelrail-cli", []string{"add", "codex", "--dry-run"}, &out, &errb, env); code != exitOK || !strings.Contains(out.String(), "env_vars") {
		t.Fatalf("codex dry run: %d %s", code, out.String())
	}
}

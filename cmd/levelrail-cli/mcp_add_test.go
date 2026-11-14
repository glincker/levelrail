package main

import (
	"bytes"
	"encoding/json"
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

func TestMCPAddJSON(t *testing.T) {
	home := t.TempDir()
	env := func(k string) (string, bool) {
		if k == "HOME" {
			return home, true
		}
		if k == "APP_API_TOKEN" {
			return "canary-token-value", true
		}
		return "", false
	}
	var out, errb bytes.Buffer
	if code := runMCP("levelrail-cli", []string{"add", "windsurf", "--json", "--dry-run"}, &out, &errb, env); code != exitOK {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	var res mcpAddResult
	if err := json.Unmarshal(out.Bytes(), &res); err != nil || res.Client != "windsurf" || !res.Changed || !res.DryRun || res.DiffSummary == "" {
		t.Fatalf("dry run json %s: %v", out.String(), err)
	}
	if _, err := os.Stat(res.ConfigPath); err == nil {
		t.Fatal("dry run wrote the file")
	}
	out.Reset()
	if code := runMCP("levelrail-cli", []string{"add", "windsurf", "--json"}, &out, &errb, env); code != exitUsage {
		t.Fatalf("json without --yes must refuse, got %d", code)
	}
	var e jsonError
	if json.Unmarshal(out.Bytes(), &e) != nil || e.Code != errCodeValidation {
		t.Fatalf("error json %s", out.String())
	}
	out.Reset()
	if code := runMCP("levelrail-cli", []string{"add", "windsurf", "--json", "--yes"}, &out, &errb, env); code != exitOK {
		t.Fatalf("exit %d", code)
	}
	if strings.Contains(out.String()+errb.String(), "canary-token-value") {
		t.Fatal("token leaked")
	}
	if _, err := os.Stat(res.ConfigPath); err != nil {
		t.Fatal(err)
	}
}

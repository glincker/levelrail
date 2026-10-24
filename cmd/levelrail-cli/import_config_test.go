package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runConfigCmd(t *testing.T, env map[string]string, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	lookup := func(k string) (string, bool) { v, ok := env[k]; return v, ok }
	code := runImportConfig("levelrail-cli", args, &out, &errb, lookup)
	return code, out.String(), errb.String()
}

func writeConfigDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for n, c := range files {
		if err := os.WriteFile(filepath.Join(dir, n), []byte(c), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestImportConfigPrintsSpecAndPortsFromEnv(t *testing.T) {
	dir := writeConfigDir(t, map[string]string{"Procfile": "web: node server.js\n"})
	code, out, _ := runConfigCmd(t, nil, dir)
	if code != exitOK || !strings.Contains(out, "8080") {
		t.Fatalf("code=%d out=%q", code, out)
	}
	code, out, _ = runConfigCmd(t, map[string]string{envImportWebPort: "9090"}, dir)
	if code != exitOK || !strings.Contains(out, "9090") || strings.Contains(out, "8080") {
		t.Fatalf("code=%d out=%q", code, out)
	}
}

func TestImportConfigBadPortEnvAndEmptyDir(t *testing.T) {
	dir := writeConfigDir(t, map[string]string{"Procfile": "web: x\n"})
	if code, _, _ := runConfigCmd(t, map[string]string{envImportWebPort: "nope"}, dir); code != exitValidation {
		t.Fatalf("bad port code=%d", code)
	}
	if code, _, _ := runConfigCmd(t, nil, t.TempDir()); code != exitValidation {
		t.Fatalf("empty dir code=%d", code)
	}
}

func TestImportConfigStrictOnlyFailsOnBlockers(t *testing.T) {
	dir := writeConfigDir(t, map[string]string{
		"vercel.json": `{"framework":"vite","crons":[{"path":"/a","schedule":"0 * * * *"}]}`,
	})
	code, out, errOut := runConfigCmd(t, nil, dir)
	if code != exitOK || !strings.Contains(errOut, "BLOCKER") || !strings.Contains(errOut, "fix:") {
		t.Fatalf("lenient code=%d err=%q", code, errOut)
	}
	if !strings.Contains(out, "version") {
		t.Fatalf("spec missing: %q", out)
	}
	if code, _, _ := runConfigCmd(t, nil, "--strict", dir); code != exitValidation {
		t.Fatalf("strict code=%d", code)
	}
}

func TestImportConfigJSONAndOut(t *testing.T) {
	dir := writeConfigDir(t, map[string]string{"Procfile": "web: x\n"})
	code, out, _ := runConfigCmd(t, nil, "--json", dir)
	var got importConfigOutput
	if code != exitOK || json.Unmarshal([]byte(out), &got) != nil || got.Spec == "" || got.Findings == nil {
		t.Fatalf("code=%d out=%q", code, out)
	}
	target := filepath.Join(t.TempDir(), "app.yaml")
	code, out, _ = runConfigCmd(t, nil, "--out", target, dir)
	b, err := os.ReadFile(target) //nolint:gosec // a path inside the test temp dir
	if code != exitOK || out != "" || err != nil || !strings.Contains(string(b), "version") {
		t.Fatalf("code=%d out=%q err=%v", code, out, err)
	}
}

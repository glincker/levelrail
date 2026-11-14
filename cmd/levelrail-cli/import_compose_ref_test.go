package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const canary = "CANARY-s3cr3t-value-91"

func composeRefFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("docker-compose.yml", `services:
  web:
    image: nginx:1.27
    ports: ["8080:80"]
    environment:
      API_TOKEN: `+canary+`
    env_file: [.env, ../outside.env]
  worker:
    image: acme/worker:2
    profiles: [jobs]
  api:
    build: ./api
`)
	write("svc/compose.yaml", "services:\n  web:\n    image: caddy:2\n")
	write("node_modules/x/compose.yml", "services:\n  bad:\n    image: bad:1\n")
	return dir
}

func runComposeRefCmd(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := runImportComposeRef("levelrail-cli", args, &out, &errb, func(string) (string, bool) { return "", false })
	return code, out.String(), errb.String()
}

func TestImportComposeRefDryRunEmitsValidDocuments(t *testing.T) {
	dir := composeRefFixture(t)
	code, out, errs := runComposeRefCmd(t, dir, "--stack", "shop")
	if code != exitOK {
		t.Fatalf("code %d: %s", code, errs)
	}
	for _, want := range []string{"name: web\n", "name: worker\n", "name: svc-compose-web\n", "API_TOKEN", "has a build context", "compose profiles", "was refused"} {
		if !strings.Contains(out, want) {
			t.Errorf("output misses %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, canary) || strings.Contains(out, "name: bad") {
		t.Errorf("output leaks a value or scanned node_modules:\n%s", out)
	}
	if !strings.Contains(out, "Dry run") {
		t.Errorf("missing dry run line")
	}
}

func TestImportComposeRefWriteRefusesOverwrite(t *testing.T) {
	dir := composeRefFixture(t)
	dst := filepath.Join(t.TempDir(), "stack")
	if code, _, errs := runComposeRefCmd(t, dir, "--stack", "shop", "--write", dst); code != exitOK {
		t.Fatalf("write: %d %s", code, errs)
	}
	files := readEmitted(t, dst)
	validateEmittedDir(t, files)
	for n, c := range files {
		if strings.Contains(c, canary) {
			t.Errorf("%s leaks the canary", n)
		}
	}
	code, _, errs := runComposeRefCmd(t, dir, "--stack", "shop", "--write", dst)
	if code != exitValidation || !strings.Contains(errs, "--force") {
		t.Errorf("second write should refuse without --force: %d %s", code, errs)
	}
}

func TestImportComposeRefDryRunWritesNothing(t *testing.T) {
	dir := composeRefFixture(t)
	before, _ := os.ReadDir(dir)
	if code, _, errs := runComposeRefCmd(t, dir, "--stack", "shop"); code != exitOK {
		t.Fatal(errs)
	}
	after, _ := os.ReadDir(dir)
	if len(before) != len(after) {
		t.Errorf("dry run changed %s", dir)
	}
}

func TestImportComposeRefSkipsSymlinkOutsideRoot(t *testing.T) {
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "compose.yml"), []byte("services:\n  leak:\n    image: x:1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "compose.yml"), []byte("services:\n  ok:\n    image: y:1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "compose.yml"), filepath.Join(dir, "compose.prod.yml")); err != nil {
		t.Skip("symlinks unavailable")
	}
	code, out, errs := runComposeRefCmd(t, dir, "--stack", "s")
	if code != exitOK {
		t.Fatal(errs)
	}
	if strings.Contains(out, "name: leak") || !strings.Contains(out, "symlink, not followed") {
		t.Errorf("symlink handling wrong:\n%s", out)
	}
}

func TestImportComposeRefNoComposeFiles(t *testing.T) {
	if code, _, _ := runComposeRefCmd(t, t.TempDir()); code != exitValidation {
		t.Errorf("want validation exit, got %d", code)
	}
}

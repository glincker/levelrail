package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPagesImportDryRunThenWriteBacksUp(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "site.toml"), []byte("[[redirects]]\nfrom = \"/old\"\nto = \"/new\"\nstatus = 301\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	if code := runPagesImport("levelrail", []string{dir}, &out, &errb); code != exitOK {
		t.Fatalf("dry run exit %d: %s", code, errb.String())
	}
	if !strings.Contains(out.String(), "/old /new 301") || !strings.Contains(out.String(), "Dry run") {
		t.Fatalf("dry run output: %s", out.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "_redirects")); err == nil {
		t.Fatal("dry run wrote _redirects")
	}
	// Existing hand-written file must survive as .bak.
	if err := os.WriteFile(filepath.Join(dir, "_redirects"), []byte("/keep /me 302\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if code := runPagesImport("levelrail", []string{dir, "--write"}, &out, &errb); code != exitOK {
		t.Fatalf("write exit %d: %s", code, errb.String())
	}
	bak, err := os.ReadFile(filepath.Join(dir, "_redirects.bak")) //nolint:gosec // test temp dir
	if err != nil || string(bak) != "/keep /me 302\n" {
		t.Fatalf("backup missing or wrong: %q %v", bak, err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "_redirects")) //nolint:gosec // test temp dir
	if !strings.Contains(string(got), "/old /new 301") || !strings.Contains(out.String(), "backed up") {
		t.Fatalf("written=%q out=%s", got, out.String())
	}
}

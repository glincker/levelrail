package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImportVercelDryRunHidesValues(t *testing.T) {
	dir := t.TempDir()
	must := func(n, c string) {
		if err := os.WriteFile(filepath.Join(dir, n), []byte(c), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	must("vercel.json", `{"regions":["iad1"],"redirects":[{"source":"/old","destination":"/new","permanent":true}]}`)
	must("package.json", `{"scripts":{"build":"next build"},"dependencies":{"next":"15"}}`)
	vars := filepath.Join(t.TempDir(), "prod.vars")
	if err := os.WriteFile(vars, []byte("NEXT_PUBLIC_API=topsecretvalue\nDB_URL=postgres://x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	code := runImportVercel("levelrail-cli", []string{"--dry-run", "--env-file", vars, "--domain", "www.example.com", dir},
		strings.NewReader(""), &out, &errb, func(string) (string, bool) { return "", false })
	if code != exitOK {
		t.Fatalf("code %d stderr %s out %s", code, errb.String(), out.String())
	}
	got := out.String()
	for _, want := range []string{"Step 7 of 7", "NEXT_PUBLIC_API", "build time", "example.com", "never changes DNS"} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "topsecretvalue") || strings.Contains(got, "postgres://x") {
		t.Fatal("env values leaked")
	}
}

func TestImportVercelRefusesSecretStorePath(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	f := filepath.Join(t.TempDir(), "secrets.vars")
	if err := os.WriteFile(f, []byte("A=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	code := runImportVercel("levelrail-cli", []string{"--dry-run", "--env-file", f, dir}, strings.NewReader(""), &out, &errb, func(string) (string, bool) { return "", false })
	if code != exitUsage || !strings.Contains(errb.String(), "allow-secret-path") {
		t.Fatalf("code %d err %s", code, errb.String())
	}
}

func TestImportVercelDryRunUploadListIsShort(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"scripts":{"build":"next build"},"dependencies":{"next":"15"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 25; i++ {
		if err := os.WriteFile(filepath.Join(dir, "f"+strings.Repeat("a", i)+".txt"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var out, errb bytes.Buffer
	code := runImportVercel("levelrail-cli", []string{"--dry-run", dir}, strings.NewReader(""), &out, &errb, func(string) (string, bool) { return "", false })
	if code != exitOK {
		t.Fatalf("code %d stderr %s", code, errb.String())
	}
	got := out.String()
	if n := strings.Count(got, ".txt\n"); n > 10 {
		t.Errorf("%d file lines, want at most 10:\n%s", n, got)
	}
	if !strings.Contains(got, "... and 16 more (run levelrail-cli deploy <dir> --dry-run for the full list)") {
		t.Errorf("missing trailing hint:\n%s", got)
	}
}

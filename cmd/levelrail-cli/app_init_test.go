package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppsInitWritesValidSpecAndRefusesOverwrite(t *testing.T) {
	file := filepath.Join(t.TempDir(), "app.yaml")
	var out, errb bytes.Buffer
	args := []string{"--file", file, "--name", "api", "--port", "8080", "--database", "postgres"}
	if code := runAppsInit("levelrail-cli", args, &out, &errb, nil); code != exitOK {
		t.Fatalf("code %d: %s", code, errb.String())
	}
	data, err := os.ReadFile(file) //nolint:gosec // test-owned temp path
	if err != nil {
		t.Fatal(err)
	}
	if _, err := validateAppFile(data); err != nil {
		t.Fatalf("scaffold does not validate: %v", err)
	}
	out.Reset()
	errb.Reset()
	if code := runAppsInit("levelrail-cli", []string{"--file", file}, &out, &errb, nil); code == exitOK {
		t.Fatal("second run must refuse to overwrite")
	}
	if !strings.Contains(errb.String(), "--force") {
		t.Fatalf("want --force hint, got %q", errb.String())
	}
	if code := runAppsInit("levelrail-cli", []string{"--file", file, "--force", "--json"}, &out, &errb, nil); code != exitOK {
		t.Fatalf("force: %d %s", code, errb.String())
	}
}

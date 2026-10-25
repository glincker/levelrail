package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStackTemplatesLists(t *testing.T) {
	var out, errb bytes.Buffer
	if code := stackTemplates("levelrail-cli", nil, &out, &errb); code != exitOK {
		t.Fatalf("code %d: %s", code, errb.String())
	}
	for _, n := range []string{"web-app-postgres", "worker-queue", "static-site-cdn", "agent-app", "preview-env"} {
		if !strings.Contains(out.String(), n) {
			t.Fatalf("missing %s in %q", n, out.String())
		}
	}
}

func TestStackInitWritesAndRefusesNonEmpty(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "shop")
	var out, errb bytes.Buffer
	args := []string{"web-app-postgres", "--dir", dir, "--set", "name=shop", "--set", "domain=shop.example.com"}
	if code := stackInit("levelrail-cli", args, &out, &errb); code != exitOK {
		t.Fatalf("code %d: %s", code, errb.String())
	}
	data, err := os.ReadFile(filepath.Join(dir, "domain.yaml")) //nolint:gosec // test reads its own temp dir
	if err != nil || !strings.Contains(string(data), "shop.example.com") {
		t.Fatalf("domain.yaml: %v %q", err, data)
	}
	errb.Reset()
	if code := stackInit("levelrail-cli", args, &out, &errb); code == exitOK || !strings.Contains(errb.String(), "not empty") {
		t.Fatalf("want refusal, code %d: %s", code, errb.String())
	}
	if code := stackInit("levelrail-cli", append(args, "--force"), &out, &errb); code != exitOK {
		t.Fatalf("force: code %d: %s", code, errb.String())
	}
}

func TestStackInitRejectsUnknownTemplateAndVar(t *testing.T) {
	var out, errb bytes.Buffer
	if code := stackInit("levelrail-cli", []string{"nope", "--dir", t.TempDir()}, &out, &errb); code == exitOK {
		t.Fatal("want error for unknown template")
	}
	if code := stackInit("levelrail-cli", []string{"agent-app", "--dir", t.TempDir(), "--set", "bad=1"}, &out, &errb); code == exitOK {
		t.Fatal("want error for unknown variable")
	}
}

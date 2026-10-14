package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSafeWrapsPlaceholders(t *testing.T) {
	cases := []struct{ in, want string }{
		{"plain text", "plain text"},
		{"open https://<host>/cb.", "open `https://<host>/cb`."},
		{"already `<code>` here", "already `<code>` here"},
		{"endpoint {region} host", "endpoint `{region}` host"},
	}
	for _, c := range cases {
		if got := safe(c.in); got != c.want {
			t.Errorf("safe(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestUIStep(t *testing.T) {
	got := uiStep("Settings > Backup targets (/settings/backup-targets), then add a target")
	want := "In the dashboard, open **Settings > Backup targets** (`/settings/backup-targets`), then add a target."
	if got != want {
		t.Errorf("uiStep = %q, want %q", got, want)
	}
}

func TestGeneratedDocsAreCurrent(t *testing.T) {
	root := filepath.Join("..", "..")
	if _, err := os.Stat(filepath.Join(root, catalogPath)); err != nil {
		t.Skip("catalog not found")
	}
	out, err := build(root)
	if err != nil {
		t.Fatal(err)
	}
	for p, body := range out {
		cur, err := os.ReadFile(filepath.Join(root, p)) //nolint:gosec // test reads repo docs
		if err != nil || string(cur) != body {
			t.Errorf("%s is stale: run go run ./scripts/gen-integration-docs", p)
		}
	}
}

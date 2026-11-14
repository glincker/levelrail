package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeRefFixture(t *testing.T, dir, rel, body string) {
	t.Helper()
	p := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func referenceFixture(t *testing.T) string {
	dir := t.TempDir()
	writeRefFixture(t, dir, "shop/compose.yaml", `services:
  web:
    image: nginx:1.27
    ports: ["8080:80"]
    environment:
      TITLE: ${SHOP_TITLE}
      DB_PASSWORD: ${DB_PASSWORD}
      MODE: ${MODE:-prod}
  worker:
    image: acme/worker:2
    profiles: [jobs]
`)
	writeRefFixture(t, dir, "docker-compose.yml", "services:\n  api:\n    build: ./api\n    env_file: [.env]\n")
	writeRefFixture(t, dir, "bind/compose.yaml", "services:\n  db:\n    image: postgres:16\n    volumes: [\"./data:/var/lib/postgresql/data\"]\n")
	return dir
}

func TestImportComposeRefReferenceDryRun(t *testing.T) {
	dir := referenceFixture(t)
	code, out, errs := runComposeRefCmd(t, dir, "--reference", "--stack", "shop", "--profiles", "jobs")
	if code != exitOK {
		t.Fatalf("code %d: %s", code, errs)
	}
	for _, want := range []string{"path: shop/compose.yaml", "- jobs", "SHOP_TITLE: ${{ env.SHOP_TITLE }}", "DB_PASSWORD: ${{ secrets.DB_PASSWORD }}", "compose_unsupported_key", "compose_rejected", "docker-compose.yml is not emitted", "Dry run"} {
		if !strings.Contains(out, want) {
			t.Errorf("output misses %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "MODE:") {
		t.Errorf("a variable with a default must not be declared:\n%s", out)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 3 {
		t.Errorf("dry run wrote files: %v", entries)
	}
}

func TestImportComposeRefReferenceWriteKeepsComposeAndRefusesOverwrite(t *testing.T) {
	dir := referenceFixture(t)
	for _, f := range []string{"docker-compose.yml", "bind"} {
		if err := os.RemoveAll(filepath.Join(dir, f)); err != nil {
			t.Fatal(err)
		}
	}
	before, _ := os.ReadFile(filepath.Join(dir, "shop/compose.yaml")) //nolint:gosec // test temp dir
	code, out, errs := runComposeRefCmd(t, dir, "--reference", "--stack", "shop", "--write", dir, "--json")
	if code != exitOK {
		t.Fatalf("code %d: %s %s", code, out, errs)
	}
	var res composeRefWriteResult
	if err := json.Unmarshal([]byte(out), &res); err != nil || len(res.Files) != 3 {
		t.Fatalf("json: %v %s", err, out)
	}
	after, _ := os.ReadFile(filepath.Join(dir, "shop/compose.yaml")) //nolint:gosec // test temp dir
	if string(before) != string(after) {
		t.Fatal("the compose file was rewritten")
	}
	apps, _ := os.ReadFile(filepath.Join(dir, "apps.yaml")) //nolint:gosec // test temp dir
	if !strings.Contains(string(apps), "path: shop/compose.yaml") || strings.Contains(string(apps), "image:") {
		t.Fatalf("apps.yaml:\n%s", apps)
	}
	if code, _, errs := runComposeRefCmd(t, dir, "--reference", "--stack", "shop", "--write", dir); code == exitOK || !strings.Contains(errs, "--force") {
		t.Fatalf("overwrite must be refused: %d %s", code, errs)
	}
}

func TestImportComposeRefReferenceRejectsParentPaths(t *testing.T) {
	dir := referenceFixture(t)
	other := t.TempDir()
	code, _, errs := runComposeRefCmd(t, dir, "--reference", "--write", other)
	if code == exitOK || !strings.Contains(errs, "outside the document directory") {
		t.Fatalf("code %d: %s", code, errs)
	}
}

func TestImportComposeRefReferenceNoSecretCanary(t *testing.T) {
	dir := t.TempDir()
	writeRefFixture(t, dir, "compose.yaml", "services:\n  web:\n    image: nginx:1\n    environment:\n      API_TOKEN: ${API_TOKEN}\n")
	writeRefFixture(t, dir, ".env", "API_TOKEN="+canary+"\n")
	code, out, errs := runComposeRefCmd(t, dir, "--reference", "--stack", "x")
	if code != exitOK || strings.Contains(out+errs, canary) {
		t.Fatalf("code %d, canary leaked:\n%s%s", code, out, errs)
	}
}

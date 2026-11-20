package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const deployYMLFixture = `service: shop
image: registry.example.com/shop:1.4.0
servers:
  web:
    hosts: [10.0.0.1]
  jobs:
    hosts: [10.0.0.2]
    cmd: bin/jobs
proxy:
  host: shop.example.com
  ssl: true
  app_port: 3000
  healthcheck:
    path: /up
registry:
  username: ci
  password:
    - REGISTRY_PASSWORD
env:
  clear:
    RAILS_ENV: production
  secret:
    - SECRET_KEY_BASE
accessories:
  cache:
    image: redis:7
    host: 10.0.0.3
    port: "6379:6379"
`

func runStackFile(t *testing.T, sub string, args ...string) (string, string, int) {
	t.Helper()
	var out, errb bytes.Buffer
	code := runImportStackFile("levelrail-cli", sub, args, &out, &errb, func(string) (string, bool) { return "", false })
	return out.String(), errb.String(), code
}

func writeStackFixture(t *testing.T, name, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestImportRailwayDryRun(t *testing.T) {
	body := "[deploy]\nstartCommand = \"node server.js\"\nhealthcheckPath = \"/up\"\n"
	p := writeStackFixture(t, "railway.toml", body)
	out, errs, code := runStackFile(t, "railway", p, "--image-repo", "registry.example.com/team")
	if code != exitOK {
		t.Fatalf("exit %d: %s %s", code, out, errs)
	}
	if !strings.Contains(out, "Dry run") || !strings.Contains(out, "/up") {
		t.Fatalf("output:\n%s", out)
	}
}

func TestImportDeployYMLDryRun(t *testing.T) {
	p := writeStackFixture(t, "deploy.yml", deployYMLFixture)
	out, _, code := runStackFile(t, "deploy-yml", p)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, out)
	}
	for _, want := range []string{"shop.example.com", "SECRET_KEY_BASE", "image: redis:7", "Dry run: nothing was written"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q", want)
		}
	}
	for _, bad := range []string{"REGISTRY_PASSWORD", "password"} {
		if strings.Contains(out, bad) {
			t.Errorf("output leaks %q", bad)
		}
	}
}

func TestImportDeployYMLUnsupportedIsLoud(t *testing.T) {
	p := writeStackFixture(t, "deploy.yml", "service: a\nimage: x:1\ntraefik:\n  options: {}\n")
	_, errOut, code := runStackFile(t, "deploy-yml", p)
	if code == exitOK || !strings.Contains(errOut, "traefik") {
		t.Fatalf("want loud error naming traefik, got %d %q", code, errOut)
	}
	p = writeStackFixture(t, "deploy.yml", "service: a\nimage: x:1\nenv:\n  clear:\n    A: <%= ENV['A'] %>\n")
	if _, errOut, code = runStackFile(t, "deploy-yml", p); code == exitOK || !strings.Contains(errOut, "ERB") {
		t.Fatalf("want ERB error, got %d %q", code, errOut)
	}
}

func TestImportDeployYMLWriteNeedsForce(t *testing.T) {
	p := writeStackFixture(t, "deploy.yml", deployYMLFixture)
	dir := filepath.Join(t.TempDir(), "out")
	if _, _, code := runStackFile(t, "deploy-yml", p, "--write", dir); code != exitOK {
		t.Fatalf("first write exit %d", code)
	}
	if _, _, code := runStackFile(t, "deploy-yml", p, "--write", dir); code == exitOK {
		t.Fatal("second write without --force must fail")
	}
}

func TestImportPortainerStackNeverReadsValues(t *testing.T) {
	export := `{"Name":"blog","Env":[{"name":"DB_PASS","value":"hunter2hunter2"}],"StackFileContent":"services:\n  web:\n    image: ghost:5\n    ports:\n      - \"8080:2368\"\n    environment:\n      url: https://blog.example.com\n      MAIL_TOKEN: abc123abc123\n"}`
	p := writeStackFixture(t, "stack.json", export)
	out, _, code := runStackFile(t, "portainer", p)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, out)
	}
	for _, bad := range []string{"hunter2", "abc123abc123"} {
		if strings.Contains(out, bad) {
			t.Errorf("output leaks %q", bad)
		}
	}
	for _, want := range []string{"ghost:5", "MAIL_TOKEN", "DB_PASS"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q", want)
		}
	}
}

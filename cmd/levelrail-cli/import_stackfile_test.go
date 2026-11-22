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

const renderBlueprintFixture = `services:
  - type: web
    name: api
    runtime: docker
    image:
      url: ghcr.io/acme/api:1.2
    healthCheckPath: /up
    numInstances: 2
    envVars:
      - key: DATABASE_URL
        fromDatabase:
          name: appdb
          property: connectionString
      - key: SESSION_SECRET
        generateValue: true
      - key: STRIPE_KEY
        sync: false
      - key: LOG_LEVEL
        value: info
  - type: cron
    name: nightly
    runtime: docker
    image:
      url: ghcr.io/acme/api:1.2
    schedule: "0 3 * * *"
    dockerCommand: ./bin/cleanup
  - type: keyvalue
    name: cache
databases:
  - name: appdb
    postgresMajorVersion: "16"
`

func TestImportRenderBlueprintDryRun(t *testing.T) {
	p := writeStackFixture(t, "render.yaml", renderBlueprintFixture)
	out, errs, code := runStackFile(t, "render", p)
	if code != exitOK {
		t.Fatalf("exit %d: %s %s", code, out, errs)
	}
	for _, want := range []string{"ghcr.io/acme/api:1.2", "0 3 * * *", "engine: redis", "engine: postgres", "SESSION_SECRET", "STRIPE_KEY", "Dry run"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

const doAppSpecFixture = `name: shop
domains:
  - domain: shop.example.com
services:
  - name: web
    http_port: 3000
    instance_count: 2
    run_command: bin/serve
    image:
      registry_type: DOCR
      repository: shop
      tag: v3
    health_check:
      http_path: /up
    envs:
      - key: LOG_LEVEL
        value: info
      - key: API_TOKEN
        value: EV[1:topsecretvalue]
        type: SECRET
      - key: DATABASE_URL
        value: ${db.DATABASE_URL}
workers:
  - name: queue
    image:
      repository: shop
      tag: v3
    run_command: bin/queue
databases:
  - name: db
    engine: PG
    version: "16"
`

func TestImportDOAppSpecDryRun(t *testing.T) {
	p := writeStackFixture(t, "app.yaml", doAppSpecFixture)
	out, errs, code := runStackFile(t, "do-app", p)
	if code != exitOK {
		t.Fatalf("exit %d: %s %s", code, out, errs)
	}
	for _, want := range []string{"shop:v3", "shop.example.com", "API_TOKEN", "engine: postgres", "Dry run"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "topsecretvalue") {
		t.Error("output leaks a secret value")
	}
}

const cloudRunFixture = `apiVersion: serving.knative.dev/v1
kind: Service
metadata:
  name: api
spec:
  template:
    metadata:
      annotations:
        autoscaling.knative.dev/minScale: "2"
        autoscaling.knative.dev/maxScale: "10"
    spec:
      containers:
      - image: us-docker.pkg.dev/acme/svc/api:1.2
        ports:
        - containerPort: 9000
        env:
        - name: LOG_LEVEL
          value: info
        - name: DB_PASS
          valueFrom:
            secretKeyRef:
              name: db-pass
              key: latest
        resources:
          limits:
            cpu: 1000m
            memory: 512Mi
`

func TestImportCloudRunServiceDryRun(t *testing.T) {
	p := writeStackFixture(t, "service.yaml", cloudRunFixture)
	out, errs, code := runStackFile(t, "cloud-run", p)
	if code != exitOK {
		t.Fatalf("exit %d: %s %s", code, out, errs)
	}
	for _, want := range []string{"us-docker.pkg.dev/acme/svc/api:1.2", "9000", "DB_PASS", "512Mi", "maxScale", "Dry run"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if _, _, code := runStackFile(t, "cloud-run", writeStackFixture(t, "x.yaml", "kind: Job\n")); code == exitOK {
		t.Fatal("non-Service must fail")
	}
}

func TestImportNetlifyDryRunEmitsStaticApp(t *testing.T) {
	body := `[build]
  command = "npm run build"
  publish = "dist"
  base = "web"
  [build.environment]
    NODE_VERSION = "20"
    API_SECRET = "hunter2hunter2"

[[redirects]]
  from = "/old/*"
  to = "/new/:splat"
  status = 301
  force = true

[[redirects]]
  from = "/eu"
  to = "/eu/home"
  conditions = {Country = ["DE"]}

[[headers]]
  for = "/*"
  [headers.values]
    X-Frame-Options = "DENY"
`
	p := writeStackFixture(t, "netlify.toml", body)
	out, errs, code := runStackFile(t, "netlify", p)
	if code != exitOK {
		t.Fatalf("exit %d: %s %s", code, out, errs)
	}
	for _, want := range []string{"type: static", "publish: dist", "baseDirectory: web", "NODE_VERSION", "/old/*", "X-Frame-Options", "conditions", "looks like a secret"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "hunter2") || strings.Contains(out, "/eu/home") {
		t.Errorf("secret or skipped rule leaked:\n%s", out)
	}
}

func TestImportFlyDryRun(t *testing.T) {
	body := "app = \"shop-api\"\n[build]\n  image = \"ghcr.io/acme/shop:1.4\"\n[http_service]\n  internal_port = 8080\n"
	p := writeStackFixture(t, "fly.toml", body)
	out, errs, code := runStackFile(t, "fly", p)
	if code != exitOK {
		t.Fatalf("exit %d: %s %s", code, out, errs)
	}
	for _, want := range []string{"shop-api", "ghcr.io/acme/shop:1.4", "port: 8080"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}

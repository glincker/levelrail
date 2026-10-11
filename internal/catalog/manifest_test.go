package catalog

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/GLINCKER/levelrail/internal/compose"
)

const testManifest = `id: demo
name: Demo
description: A demo stack.
category: Testing
documentation_url: https://example.com
recommended_memory_bytes: 1024
env:
  - key: ADMIN_PASSWORD
    generate: password
    length: 24
  - key: ADMIN_EMAIL
    required: true
  - key: TZ
    default: Etc/UTC
domains:
  - service: web
    port: 80
healthcheck:
  service: web
  port: 80
  path: /
`

const testCompose = `services:
  web:
    image: nginx:alpine
    ports: ["80"]
    environment:
      PASSWORD: ${ADMIN_PASSWORD}
      EMAIL: ${ADMIN_EMAIL}
      TZ: ${TZ}
`

func demoFS(manifest, composeBody string) fstest.MapFS {
	return fstest.MapFS{
		"m/demo/template.yaml": {Data: []byte(manifest)},
		"m/demo/compose.yaml":  {Data: []byte(composeBody)},
	}
}

func TestLoadManifests_Valid(t *testing.T) {
	ms, err := LoadManifests(demoFS(testManifest, testCompose), "m")
	if err != nil {
		t.Fatalf("LoadManifests() error = %v", err)
	}
	if len(ms) != 1 || ms[0].ID != "demo" || len(ms[0].Env) != 3 {
		t.Fatalf("got %+v", ms)
	}
}

func TestLoadManifests_Invalid(t *testing.T) {
	tests := []struct {
		name     string
		manifest string
		compose  string
		wantErr  string
	}{
		{"undeclared placeholder", testManifest, testCompose + "      EXTRA: ${NOPE}\n", "${NOPE}"},
		{"unused prompt", strings.Replace(testManifest, "  - key: TZ\n    default: Etc/UTC\n", "  - key: TZ\n    default: Etc/UTC\n  - key: UNUSED\n    default: x\n", 1), testCompose, "UNUSED is never used"},
		{"bad generate kind", strings.Replace(testManifest, "generate: password", "generate: uuid", 1), testCompose, "generate \"uuid\""},
		{"id mismatch", strings.Replace(testManifest, "id: demo", "id: other", 1), testCompose, "must match its directory"},
		{"domain on unknown service", strings.Replace(testManifest, "  - service: web\n    port: 80\nhealthcheck", "  - service: api\n    port: 80\nhealthcheck", 1), testCompose, "\"api\" is not a service"},
		{"privileged compose", testManifest, testCompose + "    privileged: true\n", "privileged"},
		{"missing name", strings.Replace(testManifest, "name: Demo\n", "", 1), testCompose, "missing name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadManifests(demoFS(tt.manifest, tt.compose), "m")
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("LoadManifests() error = %v, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestManifestRender(t *testing.T) {
	ms, err := LoadManifests(demoFS(testManifest, testCompose), "m")
	if err != nil {
		t.Fatalf("LoadManifests() error = %v", err)
	}
	m := ms[0]

	if _, err := m.Render(InstallInput{}, false); err == nil || !strings.Contains(err.Error(), "ADMIN_EMAIL") {
		t.Fatalf("Render() without required value error = %v", err)
	}
	if _, err := m.Render(InstallInput{Env: map[string]string{"BOGUS": "x"}}, true); err == nil {
		t.Fatal("Render() accepted an unknown setting")
	}
	if _, err := m.Render(InstallInput{Domains: map[string]string{"web": "not a host"}}, true); err == nil {
		t.Fatal("Render() accepted an invalid hostname")
	}

	injected := "a\"\n  evil:\n    image: x"
	body, err := m.Render(InstallInput{Env: map[string]string{"ADMIN_EMAIL": injected}, Domains: map[string]string{"web": "demo.example.com"}}, false)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	f, err := compose.Parse([]byte(body))
	if err != nil {
		t.Fatalf("rendered compose does not parse: %v\n%s", err, body)
	}
	if len(f.Services) != 1 {
		t.Fatalf("a value injected %d services, want 1:\n%s", len(f.Services), body)
	}
	env := f.Services["web"].Environment
	if env["EMAIL"] != injected || env["TZ"] != "Etc/UTC" || env["PASSWORD"] != "${SERVICE_PASSWORD_24_ADMIN_PASSWORD}" {
		t.Errorf("env = %v", env)
	}
	if f.Domains["web"] != "demo.example.com" {
		t.Errorf("domains = %v", f.Domains)
	}

	tpl, err := m.Template()
	if err != nil {
		t.Fatalf("Template() error = %v", err)
	}
	if !strings.Contains(tpl.Compose, "${SERVICE_PROMPT_ADMIN_EMAIL}") || tpl.Manifest == nil {
		t.Errorf("default render should leave the required prompt unresolved:\n%s", tpl.Compose)
	}
}

func TestEmbeddedManifests(t *testing.T) {
	ms, err := LoadManifests(manifestFS, manifestDir)
	if err != nil {
		t.Fatalf("LoadManifests(embedded) error = %v", err)
	}
	if len(ms) < 5 {
		t.Fatalf("got %d embedded manifests, want at least 5", len(ms))
	}
	for _, m := range ms {
		if m.Healthcheck == nil || len(m.Domains) == 0 {
			t.Errorf("%s: every shipped manifest needs a healthcheck and a domain prompt", m.ID)
		}
		if tpl, ok := TemplateByID(m.ID); !ok || tpl.Manifest == nil {
			t.Errorf("%s: not served from the catalog as a manifest entry", m.ID)
		}
	}
}

package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type secretsImportFake struct {
	mu      sync.Mutex
	env     map[string]string
	secrets map[string]string
}

func newSecretsImportFake(t *testing.T) (*httptest.Server, *secretsImportFake) {
	t.Helper()
	f := &secretsImportFake{env: map[string]string{}, secrets: map[string]string{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		body, _ := io.ReadAll(r.Body)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/apps/web":
			_ = json.NewEncoder(w).Encode(map[string]any{"name": "web", "image": "img", "port": 80, "env": f.env})
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/apps/web":
			var app struct {
				Env map[string]string `json:"env"`
			}
			_ = json.Unmarshal(body, &app)
			f.env = app.Env
			_, _ = w.Write([]byte(`{"name":"web"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/apps/web/secrets":
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/api/v1/apps/web/secrets/"):
			var req struct {
				Value string `json:"value"`
			}
			_ = json.Unmarshal(body, &req)
			f.secrets[strings.TrimPrefix(r.URL.Path, "/api/v1/apps/web/secrets/")] = req.Value
			_, _ = w.Write([]byte(`{}`))
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv, f
}

const secretsImportSample = "STRIPE_SECRET_KEY=sk-value-aaaa1111\nDATABASE_URL=postgres://u:pw-hunter2@db:5432/app\nPORT=8080\nLOG_LEVEL=debug\n"

func secretsImportRun(t *testing.T, extra ...string) (*secretsImportFake, string) {
	t.Helper()
	srv, f := newSecretsImportFake(t)
	file := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(file, []byte(secretsImportSample), 0o600); err != nil {
		t.Fatal(err)
	}
	args := append([]string{"apps", "env", "import", "web", "--file", file, "--api-url", srv.URL}, extra...)
	stdout, stderr := runCLIExpectOK(t, args)
	return f, stdout + stderr
}

func assertSecretsImportNoValues(t *testing.T, out string) {
	t.Helper()
	for _, v := range []string{"sk-value-aaaa1111", "hunter2", "8080", "debug"} {
		if strings.Contains(out, v) {
			t.Fatalf("output leaks value %q: %s", v, out)
		}
	}
}

func TestEnvImportRoutesSensitiveKeysToSecrets(t *testing.T) {
	f, out := secretsImportRun(t)
	if f.secrets["STRIPE_SECRET_KEY"] == "" || f.secrets["DATABASE_URL"] == "" || len(f.secrets) != 2 {
		t.Fatalf("secrets = %v", f.secrets)
	}
	if f.env["PORT"] != "8080" || f.env["LOG_LEVEL"] != "debug" || len(f.env) != 2 {
		t.Fatalf("env = %v", f.env)
	}
	if !strings.Contains(out, "STRIPE_SECRET_KEY (to secrets)") || !strings.Contains(out, "PORT (new)") {
		t.Fatalf("output = %s", out)
	}
	assertSecretsImportNoValues(t, out)
}

func TestEnvImportAllSecretsAndAllPlainOverride(t *testing.T) {
	f, out := secretsImportRun(t, "--all-secrets")
	if len(f.secrets) != 4 || len(f.env) != 0 {
		t.Fatalf("all-secrets: secrets=%v env=%v", f.secrets, f.env)
	}
	assertSecretsImportNoValues(t, out)

	f, out = secretsImportRun(t, "--all-plain")
	if len(f.secrets) != 0 || len(f.env) != 4 {
		t.Fatalf("all-plain: secrets=%v env=%v", f.secrets, f.env)
	}
	assertSecretsImportNoValues(t, out)
}

func TestEnvImportSecretPatternAndDryRun(t *testing.T) {
	f, _ := secretsImportRun(t, "--secret-pattern", "log_*")
	if f.secrets["LOG_LEVEL"] != "debug" {
		t.Fatalf("pattern key not in secrets: %v", f.secrets)
	}
	f, out := secretsImportRun(t, "--dry-run")
	if len(f.secrets) != 0 || len(f.env) != 0 || !strings.Contains(out, "DATABASE_URL (to secrets)") {
		t.Fatalf("dry run wrote or hid keys: secrets=%v env=%v out=%s", f.secrets, f.env, out)
	}
	assertSecretsImportNoValues(t, out)
}

func TestRouteEnvImport_SkipsPublicEmptyAndExistingSecrets(t *testing.T) {
	entries := []envFileEntry{{"NEXT_PUBLIC_API_KEY", "pk_x"}, {"EMPTY_TOKEN", ""}, {"HAVE_TOKEN", "v"}, {"APP_TOKEN", "v"}}
	plain, secret := routeEnvImport(entries, []string{"HAVE_TOKEN"}, envRouteAuto, nil)
	if len(plain) != 3 || len(secret) != 1 || secret[0].Key != "APP_TOKEN" {
		t.Fatalf("plain=%v secret=%v", plain, secret)
	}
}

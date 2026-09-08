package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFormatEnvValue(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"plain", "production", "production"},
		{"url", "postgres://localhost/app", "postgres://localhost/app"},
		{"space", "hello world", `"hello world"`},
		{"leading space", " value", `" value"`},
		{"trailing space", "value ", `"value "`},
		{"hash", "a#b", `"a#b"`},
		{"double quote only", `say "hi"`, `'say "hi"'`},
		{"single quote only", "it's fine", `"it's fine"`},
		{"both quote types", `say "it's"`, `"say \"it's\""`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatEnvValue(tt.in); got != tt.want {
				t.Errorf("formatEnvValue(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestRenderEnvFile(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		secrets []secretKeyResource
		want    string
	}{
		{
			name: "plain only",
			env:  map[string]string{"LOG_LEVEL": "info"},
			want: "LOG_LEVEL=info\n",
		},
		{
			name:    "secret only",
			env:     nil,
			secrets: []secretKeyResource{{Key: "API_KEY", Locked: true}},
			want: "# API_KEY is a secret; its value is never returned by the API.\n" +
				"# Set it with: levelrail apps secrets set web API_KEY <value>\n" +
				"# API_KEY=\n",
		},
		{
			name:    "plain and secret sorted",
			env:     map[string]string{"B_VAR": "two", "A_VAR": "one"},
			secrets: []secretKeyResource{{Key: "Z_SECRET", Locked: false}, {Key: "A_SECRET", Locked: true}},
			want: "A_VAR=one\n" +
				"B_VAR=two\n" +
				"\n" +
				"# A_SECRET is a secret; its value is never returned by the API.\n" +
				"# Set it with: levelrail apps secrets set web A_SECRET <value>\n" +
				"# A_SECRET=\n" +
				"# Z_SECRET is a secret; its value is never returned by the API.\n" +
				"# Set it with: levelrail apps secrets set web Z_SECRET <value>\n" +
				"# Z_SECRET=\n",
		},
		{
			name: "empty",
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := renderEnvFile("levelrail", "web", tt.env, tt.secrets)
			if got != tt.want {
				t.Errorf("renderEnvFile() = %q, want %q", got, tt.want)
			}
		})
	}
}

// envPullTestServer fakes GET /api/v1/apps/{name} and
// /api/v1/apps/{name}/secrets, the two existing endpoints "apps env
// pull" reads from.
func envPullTestServer(t *testing.T, env map[string]string, secrets []secretKeyResource) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/apps/web":
			_ = json.NewEncoder(w).Encode(appResource{Name: "web", Image: "nginx:latest", Port: 80, Env: env})
		case "/api/v1/apps/web/secrets":
			_ = json.NewEncoder(w).Encode(secrets)
		default:
			t.Fatalf("unexpected request path %s", r.URL.Path)
		}
	}))
}

func TestRun_AppsEnvPull_WritesFile(t *testing.T) {
	srv := envPullTestServer(t,
		map[string]string{"LOG_LEVEL": "info"},
		[]secretKeyResource{{Key: "API_KEY", Locked: true}},
	)
	defer srv.Close()

	dir := t.TempDir()
	file := filepath.Join(dir, ".env")

	stdout, _ := runCLIExpectOK(t, []string{"apps", "env", "pull", "web", file, "--api-url", srv.URL})
	if !strings.Contains(stdout, "wrote 1 env var(s) and 1 secret placeholder(s)") {
		t.Errorf("stdout = %q, want a written-count summary", stdout)
	}

	data, err := os.ReadFile(file) //nolint:gosec // file is a t.TempDir() path this test itself constructed, not external input
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "LOG_LEVEL=info\n") {
		t.Errorf("content = %q, want the plain env var written", content)
	}
	if !strings.Contains(content, "# API_KEY is a secret") {
		t.Errorf("content = %q, want a commented secret placeholder", content)
	}
	if strings.Contains(content, "API_KEY=s3cr3t") {
		t.Errorf("content = %q, must never contain a secret value", content)
	}
}

func TestRun_AppsEnvPull_DefaultsToDotEnv(t *testing.T) {
	srv := envPullTestServer(t, map[string]string{"FOO": "bar"}, nil)
	defer srv.Close()

	dir := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(oldwd) }()

	runCLIExpectOK(t, []string{"apps", "env", "pull", "web", "--api-url", srv.URL})

	if _, err := os.Stat(filepath.Join(dir, ".env")); err != nil {
		t.Errorf("expected .env to be created in cwd: %v", err)
	}
}

func TestRun_AppsEnvPull_RefusesOverwriteWithoutForce(t *testing.T) {
	srv := envPullTestServer(t, map[string]string{"FOO": "bar"}, nil)
	defer srv.Close()

	dir := t.TempDir()
	file := filepath.Join(dir, ".env")
	if err := os.WriteFile(file, []byte("EXISTING=1\n"), 0o600); err != nil {
		t.Fatalf("seed existing file: %v", err)
	}

	var stdout, stderr strings.Builder
	got := run("levelrail-cli-test", []string{"apps", "env", "pull", "web", file, "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitUsage, stderr.String())
	}
	if !strings.Contains(stderr.String(), "already exists") {
		t.Errorf("stderr = %q, want an already-exists error", stderr.String())
	}

	data, err := os.ReadFile(file) //nolint:gosec // file is a t.TempDir() path this test itself constructed, not external input
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(data) != "EXISTING=1\n" {
		t.Errorf("file content = %q, want it untouched", string(data))
	}
}

func TestRun_AppsEnvPull_ForceOverwrites(t *testing.T) {
	srv := envPullTestServer(t, map[string]string{"FOO": "bar"}, nil)
	defer srv.Close()

	dir := t.TempDir()
	file := filepath.Join(dir, ".env")
	if err := os.WriteFile(file, []byte("EXISTING=1\n"), 0o600); err != nil {
		t.Fatalf("seed existing file: %v", err)
	}

	runCLIExpectOK(t, []string{"apps", "env", "pull", "web", file, "--force", "--api-url", srv.URL})

	data, err := os.ReadFile(file) //nolint:gosec // file is a t.TempDir() path this test itself constructed, not external input
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if !strings.Contains(string(data), "FOO=bar") {
		t.Errorf("file content = %q, want it overwritten with the pulled env", string(data))
	}
}

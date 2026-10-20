package main

import (
	"bytes"
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

func writeDeployDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func runDeployCLI(args []string, env map[string]string) (code int, stdout, stderr string) {
	var o, e bytes.Buffer
	code = run("levelrail-cli-test", args, &o, &e, func(k string) (string, bool) { v, ok := env[k]; return v, ok })
	return code, o.String(), e.String()
}

var appDirFiles = map[string]string{
	"Dockerfile":        "FROM nginx\nEXPOSE 8080\n",
	"index.html":        "<h1>hi</h1>",
	".env":              "SECRET=1",
	".gitignore":        "out/\n",
	"out/bundle.js":     "x",
	"node_modules/a.js": "x",
}

func TestWantsUploadDeploy(t *testing.T) {
	tests := []struct {
		args []string
		want bool
	}{
		{nil, true},
		{[]string{"./app", "--dry-run"}, true},
		{[]string{"web", "--image", "nginx:1"}, false},
		{[]string{"web", "--latest"}, false},
		{[]string{"web", "--commit=abc"}, false},
		{[]string{"status", "web"}, false},
	}
	for _, tc := range tests {
		if got := wantsUploadDeploy(tc.args); got != tc.want {
			t.Errorf("wantsUploadDeploy(%v) = %v, want %v", tc.args, got, tc.want)
		}
	}
}

func TestDeployUpload_DryRunListsFilesOnly(t *testing.T) {
	dir := writeDeployDir(t, appDirFiles)
	code, out, errOut := runDeployCLI([]string{"deploy", dir, "--dry-run", "--api-url", "http://127.0.0.1:1"}, nil)
	if code != exitOK {
		t.Fatalf("exit = %d, stderr = %s", code, errOut)
	}
	if got := strings.Fields(out); strings.Join(got, ",") != ".gitignore,Dockerfile,index.html" {
		t.Fatalf("stdout files = %v, want .gitignore, Dockerfile, index.html only", got)
	}
	for _, want := range []string{"3 files", "largest files:", "dry run"} {
		if !strings.Contains(errOut, want) {
			t.Errorf("stderr missing %q:\n%s", want, errOut)
		}
	}
	if strings.Contains(out+errOut, "SECRET") || strings.Contains(out, "node_modules") {
		t.Error("ignored content leaked into output")
	}
}

func TestDeployUpload_DryRunInclude(t *testing.T) {
	dir := writeDeployDir(t, appDirFiles)
	_, out, _ := runDeployCLI([]string{"deploy", dir, "--dry-run", "--include", ".env", "--api-url", "http://127.0.0.1:1"}, nil)
	if !strings.Contains(out, ".env") {
		t.Fatalf("--include .env not honored: %q", out)
	}
}

func TestDeployUpload_SizeCap(t *testing.T) {
	dir := writeDeployDir(t, map[string]string{"Dockerfile": "FROM nginx\nEXPOSE 80\n", "big.bin": strings.Repeat("x", 4096)})
	code, _, errOut := runDeployCLI([]string{"deploy", dir, "--dry-run", "--api-url", "http://127.0.0.1:1"}, map[string]string{envUploadMaxBytes: "1000"})
	if code == exitOK || !strings.Contains(errOut, envUploadMaxBytes) {
		t.Fatalf("exit = %d, stderr = %s, want a size cap error naming %s", code, errOut, envUploadMaxBytes)
	}
}

func TestDeployUpload_NotADirectory(t *testing.T) {
	code, _, errOut := runDeployCLI([]string{"deploy", filepath.Join(t.TempDir(), "missing"), "--api-url", "http://127.0.0.1:1"}, nil)
	if code == exitOK || !strings.Contains(errOut, "--image") {
		t.Fatalf("exit = %d, stderr = %s", code, errOut)
	}
}

func TestDeployUpload_FullFlow(t *testing.T) {
	dir := writeDeployDir(t, appDirFiles)
	var mu sync.Mutex
	var created, built map[string]any
	var uploaded []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/apps/"+filepath.Base(dir):
			if created == nil {
				http.Error(w, `{"error":"app not found"}`, http.StatusNotFound)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"name": created["name"], "fallback_url": "https://app.example.test"})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/apps":
			_ = json.NewDecoder(r.Body).Decode(&created)
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(created)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/uploads"):
			uploaded, _ = io.ReadAll(r.Body)
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"hash": strings.Repeat("ab", 32), "size": len(uploaded), "file_count": 3})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/builds"):
			_ = json.NewDecoder(r.Body).Decode(&built)
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"id":"dep_1"}`))
		case strings.HasSuffix(r.URL.Path, "/deploys/dep_1/logs"):
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"line\":\"step 1/2 FROM nginx\",\"stream\":\"stdout\"}\n\n"))
		case strings.HasSuffix(r.URL.Path, "/deploys/dep_1"):
			_, _ = w.Write([]byte(`{"id":"dep_1","outcome":"healthy"}`))
		default:
			http.Error(w, `{"error":"unexpected `+r.Method+" "+r.URL.Path+`"}`, http.StatusNotFound)
		}
	}))
	defer srv.Close()

	code, out, errOut := runDeployCLI([]string{"deploy", dir, "--yes", "--env", "A=1", "--api-url", srv.URL, "--token", "tok"}, nil)
	if code != exitOK {
		t.Fatalf("exit = %d, stderr = %s", code, errOut)
	}
	if strings.TrimSpace(out) != "https://app.example.test" {
		t.Errorf("stdout = %q, want only the app URL; stderr %s", out, errOut)
	}
	for _, want := range []string{"3 files", "uploaded", "step 1/2 FROM nginx", "healthy"} {
		if !strings.Contains(errOut, want) {
			t.Errorf("stderr missing %q:\n%s", want, errOut)
		}
	}
	if created["port"] != float64(8080) || created["env"].(map[string]any)["A"] != "1" {
		t.Errorf("created app = %v, want port 8080 inferred from EXPOSE and env A=1", created)
	}
	if built["upload_hash"] != strings.Repeat("ab", 32) || built["repo_url"] != "" {
		t.Errorf("build request = %v", built)
	}
	if len(uploaded) == 0 || bytes.Contains(uploaded, []byte("SECRET")) {
		t.Error("archive empty or contains the ignored .env content")
	}
}

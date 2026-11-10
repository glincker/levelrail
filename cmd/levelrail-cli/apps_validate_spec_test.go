package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func specServer(t *testing.T, hits *int, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*hits++
		if r.URL.Path == "/api/v1/apps/web/validate-spec" {
			var req map[string]string
			if r.Method != http.MethodPost || json.NewDecoder(r.Body).Decode(&req) != nil || req["yaml"] == "" {
				http.Error(w, `{"error":"bad"}`, http.StatusBadRequest)
				return
			}
		} else if r.Method != http.MethodGet || r.URL.Path != "/api/v1/apps/web/listening-ports" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
}

func runSpec(t *testing.T, srv *httptest.Server, args ...string) (int, string) {
	t.Helper()
	var out, errb bytes.Buffer
	full := append(args, "--api-url", srv.URL, "--token", "t")
	code := run("cli", full, &out, &errb, envMap())
	return code, out.String()
}

func TestAppsValidateSpec(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "app.yaml")
	if err := os.WriteFile(file, []byte("name: web\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	hits := 0
	ok := specServer(t, &hits, `{"valid":true,"issues":[],"services":2}`)
	defer ok.Close()
	if code, out := runSpec(t, ok, "apps", "validate-spec", "web", "--file", file); code != exitOK || !strings.Contains(out, "valid (2 services)") {
		t.Fatalf("valid: code %d out %q", code, out)
	}

	bad := specServer(t, &hits, `{"valid":false,"issues":[{"path":"services.0.image","line":3,"message":"required"},{"path":"name","line":1,"message":"too long"}]}`)
	defer bad.Close()
	code, out := runSpec(t, bad, "apps", "validate-spec", "web", "--file", file)
	if code != exitValidation || !strings.Contains(out, "services.0.image: required") || !strings.Contains(out, "name: too long") {
		t.Fatalf("invalid: code %d out %q", code, out)
	}

	big := filepath.Join(dir, "big.yaml")
	if err := os.WriteFile(big, bytes.Repeat([]byte("a"), validateSpecMaxBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	hits = 0
	if code, _ := runSpec(t, ok, "apps", "validate-spec", "web", "--file", big); code != exitValidation || hits != 0 {
		t.Fatalf("oversize: code %d hits %d", code, hits)
	}
	if code, _ := runSpec(t, ok, "apps", "validate-spec", "web", "--file", dir); code != exitValidation || hits != 0 {
		t.Fatalf("directory: code %d hits %d", code, hits)
	}
}

func TestAppsListeningPorts(t *testing.T) {
	hits := 0
	srv := specServer(t, &hits, `{"app":"web","port":8080,"probed":true,"listening":[3000,8080],"verdict":"listening"}`)
	defer srv.Close()
	code, out := runSpec(t, srv, "apps", "listening-ports", "web")
	if code != exitOK || !strings.Contains(out, "CONFIGURED") || !strings.Contains(out, "8080") || !strings.Contains(out, "3000,8080") {
		t.Fatalf("table: code %d out %q", code, out)
	}
}

package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestRun_AppsCreateFileSavesRollout(t *testing.T) {
	file := filepath.Join(t.TempDir(), "app.yaml")
	yaml := "version: 1\nservices:\n  web:\n    build:\n      type: image\n      image: web:v1\n    port: 8080\n    replicas: 3\n    strategy: rolling\n    rollout:\n      max_surge: \"1\"\n      min_available: \"2\"\n"
	if err := os.WriteFile(file, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	var gotPath string
	var gotBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == "/api/v1/apps/web/rollout" {
			gotPath = r.URL.Path
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(appResource{Name: "web", Image: "web:v1", Port: 8080})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "create", "--file", file, "--api-url", srv.URL, "--json"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d (stdout=%q stderr=%q)", got, stdout.String(), stderr.String())
	}
	if gotPath == "" || gotBody["max_surge"] != "1" || gotBody["min_available"] != "2" {
		t.Errorf("rollout PUT path=%q body=%v, want max_surge 1 and min_available 2", gotPath, gotBody)
	}
}

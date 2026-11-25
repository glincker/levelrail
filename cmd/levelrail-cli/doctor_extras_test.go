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
	"time"
)

func TestDoctor_CLIUpdateLine_FromCacheOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".config", "levelrail-cli-test")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	state := `{"checked_at":"` + time.Now().Add(-5*time.Hour).UTC().Format(time.RFC3339) + `","latest":"v99.0.0"}`
	if err := os.WriteFile(filepath.Join(dir, "update-check.json"), []byte(state), 0o600); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/settings/runtime/") {
			_, _ = w.Write([]byte(`{"key":"build.warm_base_images","value":"node:22,python:3.13"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(systemDoctorResource{OK: true, Checks: []doctorCheckResource{{Code: "docker", Name: "Docker daemon", Status: "ok", Message: "reachable"}}})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"doctor", "--api-url", srv.URL})
	if !strings.Contains(stdout, "latest v99.0.0, checked 5h ago") {
		t.Errorf("stdout = %q, want the cached CLI update line", stdout)
	}
	if !strings.Contains(stdout, "2 images configured") || !strings.Contains(stdout, "remote build nodes") {
		t.Errorf("stdout = %q, want the base image warming line", stdout)
	}

	var out, errb bytes.Buffer
	if got := run("levelrail-cli-test", []string{"doctor", "--json", "--api-url", srv.URL}, &out, &errb, envMap()); got != exitOK {
		t.Fatalf("exit = %d", got)
	}
	var js struct {
		OK  bool `json:"ok"`
		CLI struct {
			Latest string `json:"latest"`
		} `json:"cli_update"`
		Warm struct {
			Count int `json:"count"`
		} `json:"warm_base_images"`
	}
	if err := json.Unmarshal(out.Bytes(), &js); err != nil || !js.OK || js.CLI.Latest != "v99.0.0" || js.Warm.Count != 2 {
		t.Errorf("json = %s (err %v), want ok plus cli_update and warm_base_images", out.String(), err)
	}
}

func TestDoctor_WarmBaseImages_OffAndUnreadable(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cases := map[string]struct {
		status int
		body   string
		want   string
	}{
		"off":        {http.StatusOK, `{"value":""}`, "Base image warming: off"},
		"unreadable": {http.StatusNotFound, `{"error":"x"}`, ""},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if strings.Contains(r.URL.Path, "/settings/runtime/") {
					w.WriteHeader(tc.status)
					_, _ = w.Write([]byte(tc.body))
					return
				}
				_ = json.NewEncoder(w).Encode(systemDoctorResource{OK: true})
			}))
			defer srv.Close()
			stdout, _ := runCLIExpectOK(t, []string{"doctor", "--api-url", srv.URL})
			has := strings.Contains(stdout, "Base image warming")
			if tc.want == "" && has || tc.want != "" && !strings.Contains(stdout, tc.want) {
				t.Errorf("stdout = %q, want %q", stdout, tc.want)
			}
		})
	}
}

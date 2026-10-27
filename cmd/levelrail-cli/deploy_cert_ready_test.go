package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// certDeployServer serves a healthy upload deploy whose app has a domain; certs is called per /cert/status read.
func certDeployServer(t *testing.T, dir string, certs func() string) *httptest.Server {
	t.Helper()
	var created map[string]any
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/apps/"+filepath.Base(dir):
			if created == nil {
				http.Error(w, `{"error":"app not found"}`, http.StatusNotFound)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"name": created["name"], "domains": []string{"app.1-2-3-4.sslip.io"}})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/apps":
			_ = json.NewDecoder(r.Body).Decode(&created)
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(created)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/uploads"):
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"hash": strings.Repeat("ab", 32), "size": 1, "file_count": 3})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/builds"):
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"id":"dep_1"}`))
		case strings.HasSuffix(r.URL.Path, "/deploys/dep_1/logs"):
			w.Header().Set("Content-Type", "text/event-stream")
		case strings.HasSuffix(r.URL.Path, "/deploys/dep_1"):
			_, _ = w.Write([]byte(`{"id":"dep_1","outcome":"healthy"}`))
		case r.URL.Path == "/api/v1/domains/app.1-2-3-4.sslip.io/cert/status":
			_, _ = w.Write([]byte(certs()))
		default:
			http.Error(w, `{"error":"unexpected"}`, http.StatusNotFound)
		}
	}))
}

const issuedCert = `{"state":"issued"}`

func TestDeployUpload_HealthyButCertPendingSaysSo(t *testing.T) {
	dir := writeDeployDir(t, appDirFiles)
	srv := certDeployServer(t, dir, func() string { return `{"state":"pending"}` })
	defer srv.Close()

	code, out, errOut := runDeployCLI([]string{"deploy", dir, "--yes", "--api-url", srv.URL, "--token", "tok"}, nil)
	if code != exitOK {
		t.Fatalf("exit = %d, stderr = %s", code, errOut)
	}
	if strings.TrimSpace(out) != "https://app.1-2-3-4.sslip.io" {
		t.Errorf("stdout = %q, want only the URL", out)
	}
	if !strings.Contains(errOut, "certificate is being issued") {
		t.Errorf("stderr must qualify the URL while the certificate is pending:\n%s", errOut)
	}
}

func TestDeployUpload_JSONCarriesCertificateState(t *testing.T) {
	dir := writeDeployDir(t, appDirFiles)
	srv := certDeployServer(t, dir, func() string { return `{"state":"pending"}` })
	defer srv.Close()

	_, out, errOut := runDeployCLI([]string{"deploy", dir, "--yes", "--json", "--api-url", srv.URL, "--token", "tok"}, nil)
	var res map[string]any
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s\n%s", err, out, errOut)
	}
	if res["certificate"] != "pending" {
		t.Errorf("certificate = %v, want pending", res["certificate"])
	}
}

func TestDeployUpload_IssuedCertAddsNoHint(t *testing.T) {
	dir := writeDeployDir(t, appDirFiles)
	srv := certDeployServer(t, dir, func() string { return issuedCert })
	defer srv.Close()

	_, _, errOut := runDeployCLI([]string{"deploy", dir, "--yes", "--api-url", srv.URL, "--token", "tok"}, nil)
	if strings.Contains(errOut, "being issued") {
		t.Errorf("no hint expected once issued:\n%s", errOut)
	}
}

func TestDeployUpload_WaitURLReportsReadyOnce(t *testing.T) {
	dir := writeDeployDir(t, appDirFiles)
	var reads atomic.Int32
	srv := certDeployServer(t, dir, func() string {
		if reads.Add(1) < 2 {
			return `{"state":"pending"}`
		}
		return issuedCert
	})
	defer srv.Close()

	code, _, errOut := runDeployCLI([]string{"deploy", dir, "--yes", "--wait-url", "--api-url", srv.URL, "--token", "tok"}, nil)
	if code != exitOK {
		t.Fatalf("exit = %d, stderr = %s", code, errOut)
	}
	if !strings.Contains(errOut, "certificate issued, https://app.1-2-3-4.sslip.io is ready") {
		t.Errorf("stderr lacks the ready line:\n%s", errOut)
	}
}

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

func TestRun_DomainsGoogleCloudDNS(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody updateGoogleCloudDNSRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		gotBody = updateGoogleCloudDNSRequest{}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/test") {
			_ = json.NewEncoder(w).Encode(googleCloudDNSTestResult{OK: true, Zones: []string{"example.com"}})
			return
		}
		_ = json.NewEncoder(w).Encode(googleCloudDNSResource{Enabled: true, ProjectID: "proj-1", ClientEmail: "sa@proj-1.iam.gserviceaccount.com", HasKey: true})
	}))
	defer srv.Close()

	keyFile := filepath.Join(t.TempDir(), "key.json")
	if err := os.WriteFile(keyFile, []byte(`{"project_id":"proj-1"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantMethod string
		wantPath   string
		wantOut    string
	}{
		{"get", []string{"get"}, exitOK, http.MethodGet, "/api/v1/settings/google-cloud-dns", "project_id:   proj-1"},
		{"set", []string{"set", "--key-file", keyFile}, exitOK, http.MethodPut, "/api/v1/settings/google-cloud-dns", "has_key:      true"},
		{"set without key file", []string{"set"}, exitUsage, "", "", ""},
		{"test", []string{"test"}, exitOK, http.MethodPost, "/api/v1/settings/google-cloud-dns/test", "example.com"},
		{"clear", []string{"clear"}, exitOK, http.MethodDelete, "/api/v1/settings/google-cloud-dns", "enabled:      true"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotMethod, gotPath = "", ""
			var stdout, stderr bytes.Buffer
			args := append([]string{"domains", "google-cloud-dns"}, tc.args...)
			args = append(args, "--api-url", srv.URL)
			if got := run("levelrail-cli-test", args, &stdout, &stderr, envMap()); got != tc.wantCode {
				t.Fatalf("exit = %d, want %d (stderr=%q)", got, tc.wantCode, stderr.String())
			}
			if gotMethod != tc.wantMethod || gotPath != tc.wantPath {
				t.Errorf("request = %s %s, want %s %s", gotMethod, gotPath, tc.wantMethod, tc.wantPath)
			}
			if !strings.Contains(stdout.String(), tc.wantOut) {
				t.Errorf("stdout = %q, want containing %q", stdout.String(), tc.wantOut)
			}
			if tc.name == "set" && (!gotBody.Enabled || !strings.Contains(gotBody.ServiceAccountJSON, "proj-1") || strings.HasSuffix(gotBody.ServiceAccountJSON, "\n")) {
				t.Errorf("body = %+v", gotBody)
			}
		})
	}
}

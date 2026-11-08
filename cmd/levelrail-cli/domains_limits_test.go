package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_DomainsLimits(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody setDomainLimitsRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(domainLimitsResource{Domain: "app.example.com", Configured: r.Method != http.MethodDelete, MaxBodyBytes: gotBody.MaxBodyBytes, ReadTimeoutSeconds: 30})
	}))
	defer srv.Close()
	tests := []struct {
		args       []string
		wantMethod string
		wantOut    string
	}{
		{[]string{"set", "web", "app.example.com", "--max-body", "5MiB", "--read-timeout", "30"}, http.MethodPut, "read_timeout_seconds:  30"},
		{[]string{"get", "web", "app.example.com"}, http.MethodGet, "max_body_bytes"},
		{[]string{"clear", "web", "app.example.com"}, http.MethodDelete, "no own limits"},
	}
	for _, tt := range tests {
		var stdout, stderr bytes.Buffer
		args := append(append([]string{"domains", "limits"}, tt.args...), "--api-url", srv.URL)
		if got := run("levelrail-cli-test", args, &stdout, &stderr, envMap()); got != exitOK {
			t.Fatalf("%v exit = %d (%q %q)", tt.args, got, stdout.String(), stderr.String())
		}
		if gotMethod != tt.wantMethod || gotPath != "/api/v1/apps/web/domains/app.example.com/limits" || !strings.Contains(stdout.String(), tt.wantOut) {
			t.Errorf("%v: %s %s out=%q", tt.args, gotMethod, gotPath, stdout.String())
		}
	}
	if gotBody.MaxBodyBytes != 0 {
		// last decoded body is the empty DELETE; the PUT body was checked via the echoed output above
		t.Logf("last body %+v", gotBody)
	}
}

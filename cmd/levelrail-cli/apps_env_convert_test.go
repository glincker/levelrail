package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_AppsEnvConvertToSecret(t *testing.T) {
	var gotPath, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	for _, args := range [][]string{
		{"apps", "env", "convert-to-secret", "web", "API_KEY", "--api-url", srv.URL},
		{"apps", "env", "convert-to-secret", "API_KEY", "--app", "web", "--api-url", srv.URL},
	} {
		gotPath, gotMethod = "", ""
		stdout, _ := runCLIExpectOK(t, args)
		if gotMethod != http.MethodPost || gotPath != "/api/v1/apps/web/env/API_KEY/convert-to-secret" {
			t.Errorf("request = %s %s", gotMethod, gotPath)
		}
		if !strings.Contains(stdout, "API_KEY") || strings.Contains(stdout, "hunter2") {
			t.Errorf("stdout = %q", stdout)
		}
	}
}

func TestRun_AppsSecretsScan(t *testing.T) {
	var gotPath, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"scope":"env","findings":[{"rule_id":"generic-key","source":"env","var":"TOKEN","fingerprint":"fp1"}]}`))
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "secrets", "scan", "web", "--api-url", srv.URL})
	if gotMethod != http.MethodPost || gotPath != "/api/v1/apps/web/secret-scan/run" {
		t.Errorf("request = %s %s", gotMethod, gotPath)
	}
	if !strings.Contains(stdout, "TOKEN") || !strings.Contains(stdout, "fp1") {
		t.Errorf("stdout = %q", stdout)
	}

	runCLIExpectOK(t, []string{"apps", "secrets", "scan", "web", "--deployment", "d1", "--api-url", srv.URL})
	if gotMethod != http.MethodGet || gotPath != "/api/v1/apps/web/deployments/d1/secret-scan" {
		t.Errorf("deployment request = %s %s", gotMethod, gotPath)
	}
}

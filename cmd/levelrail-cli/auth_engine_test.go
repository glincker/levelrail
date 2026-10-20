package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_AuthEngineStatus(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"library_version":"v2.7.0","totp":true,"passkeys":false,"oauth":true}`))
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"auth-engine", "status", "--api-url", srv.URL})
	if gotMethod != http.MethodGet || gotPath != "/api/v1/auth-engine/status" {
		t.Errorf("request = %s %s", gotMethod, gotPath)
	}
	for _, want := range []string{"library version:  v2.7.0", "two-factor (TOTP): available", "passkeys:         unavailable", "oauth sign-in:    available"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout = %q, missing %q", stdout, want)
		}
	}
}

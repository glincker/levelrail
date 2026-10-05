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
		_, _ = w.Write([]byte(`{"mode":"shadow","library_version":"v2.6.0","areas":[],"compared":10,"matched":9,"mismatched":1,"dropped":0,"skipped":0,"errors":0,"mismatches":[{"at":"2026-10-05T10:00:00Z","kind":"abilities","token_id":"tok_1","legacy_abilities":["read","write"],"library_abilities":["read"]}]}`))
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"auth-engine", "status", "--api-url", srv.URL})
	if gotMethod != http.MethodGet || gotPath != "/api/v1/auth-engine/status" {
		t.Errorf("request = %s %s", gotMethod, gotPath)
	}
	for _, want := range []string{"mode:             shadow", "mismatched:       1", "abilities", "tok_1"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout = %q, missing %q", stdout, want)
		}
	}
}

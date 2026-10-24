package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_TokensCleanup(t *testing.T) {
	var gotMethod, gotPath, gotBody, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotMethod, gotPath, gotBody, gotAuth = r.Method, r.URL.Path, strings.TrimSpace(string(b)), r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"deleted":3}`))
	}))
	t.Cleanup(srv.Close)

	tests := []struct {
		name     string
		extra    []string
		wantBody string
		wantOut  string
		wantExit int
	}{
		{"default window", nil, `{"older_than_days":90}`, "removed 3 revoked tokens", exitOK},
		{"explicit days", []string{"--older-than-days", "30"}, `{"older_than_days":30}`, "removed 3", exitOK},
		{"zero needs yes", []string{"--older-than-days", "0"}, "", "", exitValidation},
		{"zero removes all revoked", []string{"--older-than-days", "0", "--yes"}, `{"older_than_days":0,"confirm":true}`, "removed 3", exitOK},
		{"json output", []string{"--json"}, `{"older_than_days":90}`, `"deleted"`, exitOK},
		{"negative days", []string{"--older-than-days", "-1"}, "", "", exitValidation},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotMethod, gotPath, gotBody, gotAuth = "", "", "", ""
			var stdout, stderr bytes.Buffer
			args := append([]string{"tokens", "cleanup", "--token", "root-tok", "--api-url", srv.URL}, tc.extra...)
			if got := run("levelrail-cli-test", args, &stdout, &stderr, envMap()); got != tc.wantExit {
				t.Fatalf("exit = %d, want %d (stderr=%q)", got, tc.wantExit, stderr.String())
			}
			if tc.wantExit != exitOK {
				if gotMethod != "" {
					t.Errorf("a rejected flag still called the server: %s %s", gotMethod, gotPath)
				}
				return
			}
			if gotMethod != http.MethodPost || gotPath != "/api/v1/auth/tokens/cleanup" || gotBody != tc.wantBody || gotAuth != "Bearer root-tok" {
				t.Errorf("request = %s %s %q auth %q", gotMethod, gotPath, gotBody, gotAuth)
			}
			if !strings.Contains(stdout.String(), tc.wantOut) {
				t.Errorf("stdout = %q, want %q", stdout.String(), tc.wantOut)
			}
		})
	}
}

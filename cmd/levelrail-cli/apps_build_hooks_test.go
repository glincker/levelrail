package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_AppsBuildHooks(t *testing.T) {
	var gotMethod, gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		var b bytes.Buffer
		_, _ = b.ReadFrom(r.Body)
		gotBody = strings.TrimSpace(b.String())
		switch r.Method {
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		case http.MethodGet:
			_, _ = w.Write([]byte(`[{"id":"bh_1","name":"cms","trigger_count":2,"last_result":"deployed"}]`))
		default:
			_, _ = w.Write([]byte(`{"id":"bh_1","name":"cms","path":"/api/v1/hooks/deploy/bh_1/tok","token":"tok"}`))
		}
	}))
	t.Cleanup(srv.Close)

	tests := []struct {
		name       string
		args       []string
		wantMethod string
		wantPath   string
		wantBody   string
		wantOut    string
		wantExit   int
	}{
		{"list", []string{"list", "web"}, "GET", "/api/v1/apps/web/build-hooks", "", "deployed", exitOK},
		{"create", []string{"create", "--name", "cms", "--branch", "main", "web"}, "POST", "/api/v1/apps/web/build-hooks", `{"branch":"main","name":"cms"}`, srv.URL + "/api/v1/hooks/deploy/bh_1/tok", exitOK},
		{"rotate", []string{"rotate", "web", "bh_1"}, "POST", "/api/v1/apps/web/build-hooks/bh_1/rotate", "", "shown once", exitOK},
		{"revoke", []string{"revoke", "web", "bh_1"}, "DELETE", "/api/v1/apps/web/build-hooks/bh_1", "", "revoked bh_1", exitOK},
		{"create without name", []string{"create", "web"}, "", "", "", "", exitUsage},
		{"revoke without id", []string{"revoke", "web"}, "", "", "", "", exitUsage},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotMethod, gotPath, gotBody = "", "", ""
			var stdout, stderr bytes.Buffer
			args := append([]string{"apps", "build-hooks"}, tc.args...)
			args = append(args, "--token", "t", "--api-url", srv.URL)
			if got := run("levelrail-cli-test", args, &stdout, &stderr, envMap()); got != tc.wantExit {
				t.Fatalf("exit = %d, want %d (stderr=%q)", got, tc.wantExit, stderr.String())
			}
			if gotMethod != tc.wantMethod || gotPath != tc.wantPath || gotBody != tc.wantBody {
				t.Errorf("request = %s %s %q, want %s %s %q", gotMethod, gotPath, gotBody, tc.wantMethod, tc.wantPath, tc.wantBody)
			}
			if !strings.Contains(stdout.String(), tc.wantOut) {
				t.Errorf("stdout = %q, want %q", stdout.String(), tc.wantOut)
			}
		})
	}
}

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestRun_AppsBranchDeploys(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody struct {
		Mode     string   `json:"mode"`
		Patterns []string `json:"patterns"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		gotBody.Mode, gotBody.Patterns = "", nil
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"settings":{"mode":"patterns","patterns":["dev","qa/*"],"production_branch":"main"},"deploys":[{"branch":"dev","slug":"dev","status":"active","head_sha":"0123456789abcdef","url":"https://web-br-dev.example.com"}]}`))
	}))
	defer srv.Close()

	tests := []struct {
		name       string
		args       []string
		wantMethod string
		wantPath   string
		wantBody   []string
		wantOut    string
	}{
		{"list", []string{"apps", "branch-deploys", "list", "web"}, http.MethodGet, "/api/v1/apps/web/branch-deploys", nil, "https://web-br-dev.example.com"},
		{"set patterns", []string{"apps", "branch-deploys", "set", "web", "--mode", "patterns", "--branch", "dev", "--branch", "qa/*"}, http.MethodPut, "/api/v1/apps/web/branch-deploys", []string{"dev", "qa/*"}, "mode: patterns"},
		{"delete", []string{"apps", "branch-deploys", "delete", "web", "dev"}, http.MethodDelete, "/api/v1/apps/web/branch-deploys/dev", nil, "removed"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stdout, _ := runCLIExpectOK(t, append(tc.args, "--api-url", srv.URL))
			if gotMethod != tc.wantMethod || gotPath != tc.wantPath {
				t.Errorf("request = %s %s, want %s %s", gotMethod, gotPath, tc.wantMethod, tc.wantPath)
			}
			if tc.wantBody != nil && !reflect.DeepEqual(gotBody.Patterns, tc.wantBody) {
				t.Errorf("patterns = %v, want %v", gotBody.Patterns, tc.wantBody)
			}
			if !strings.Contains(stdout, tc.wantOut) {
				t.Errorf("stdout = %q, want it to contain %q", stdout, tc.wantOut)
			}
		})
	}
}

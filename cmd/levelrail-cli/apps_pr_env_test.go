package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_AppsPREnv(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody struct {
		Key    string `json:"key"`
		Value  string `json:"value"`
		Secret bool   `json:"secret"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode([]prPreviewEnvOverride{{ID: "prenv_1", Key: "API_KEY", Secret: true}, {ID: "prenv_2", Key: "FLAG", Value: "on"}})
		default:
			_ = json.NewEncoder(w).Encode(prPreviewEnvOverride{ID: "prenv_1", Key: gotBody.Key, Value: gotBody.Value, Secret: gotBody.Secret})
		}
	}))
	defer srv.Close()

	tests := []struct {
		name       string
		args       []string
		wantMethod string
		wantPath   string
		wantOut    string
		denyOut    string
	}{
		{"set", []string{"apps", "pr-env", "set", "web", "FLAG", "--pr", "42", "--value", "on", "--api-url", srv.URL}, http.MethodPost, "/api/v1/apps/web/previews/42/env", "value=on", ""},
		{"set secret never echoes", []string{"apps", "pr-env", "set", "web", "API_KEY", "--pr", "42", "--value", "sk-abc", "--secret", "--api-url", srv.URL}, http.MethodPost, "/api/v1/apps/web/previews/42/env", "secret=true", "sk-abc"},
		{"list hides secrets", []string{"apps", "pr-env", "list", "web", "--pr", "42", "--api-url", srv.URL}, http.MethodGet, "/api/v1/apps/web/previews/42/env", "(hidden)", ""},
		{"clear", []string{"apps", "pr-env", "clear", "web", "prenv_1", "--pr", "42", "--api-url", srv.URL}, http.MethodDelete, "/api/v1/apps/web/previews/42/env/prenv_1", "cleared", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout, _ := runCLIExpectOK(t, tt.args)
			if gotMethod != tt.wantMethod || gotPath != tt.wantPath {
				t.Errorf("request = %s %s, want %s %s", gotMethod, gotPath, tt.wantMethod, tt.wantPath)
			}
			if !strings.Contains(stdout, tt.wantOut) {
				t.Errorf("stdout = %q, want %q", stdout, tt.wantOut)
			}
			if tt.denyOut != "" && strings.Contains(stdout, tt.denyOut) {
				t.Errorf("stdout = %q leaked %q", stdout, tt.denyOut)
			}
		})
	}
}

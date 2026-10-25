package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_AppsSet_PRCommentCommands(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"enabled":false,"post_pr_comments":false,"pr_comment_commands":true,"pr_comment_commands_forks":false}`))
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "set", "web", "--pr-comment-commands", "on", "--api-url", srv.URL})
	if gotMethod != http.MethodPut || gotPath != "/api/v1/apps/web/preview-settings" {
		t.Errorf("request = %s %s", gotMethod, gotPath)
	}
	if gotBody["pr_comment_commands"] != true {
		t.Errorf("body = %v, want pr_comment_commands true", gotBody)
	}
	if _, present := gotBody["pr_comment_commands_forks"]; present {
		t.Errorf("body = %v, forks must be left unchanged when its flag is absent", gotBody)
	}
	if !strings.Contains(stdout, "comment commands on") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestRun_AppsSet_Validation(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want int
	}{
		{"nothing to set", []string{"apps", "set", "web"}, exitValidation},
		{"bad value", []string{"apps", "set", "web", "--pr-comment-commands", "yes"}, exitUsage},
		{"missing name", []string{"apps", "set", "--pr-comment-commands", "on"}, exitUsage},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if got := run("levelrail-cli-test", tt.args, &stdout, &stderr, envMap()); got != tt.want {
				t.Fatalf("exit = %d, want %d (stderr=%q)", got, tt.want, stderr.String())
			}
		})
	}
}

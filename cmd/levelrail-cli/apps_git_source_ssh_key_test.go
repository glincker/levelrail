package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_AppsGitSourceSSHKeyGenerate(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"public_key":"ssh-ed25519 AAAA","fingerprint":"SHA256:abc","host_fingerprint":"SHA256:host","host_key_pinned":true}`))
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "git-source", "ssh-key", "generate", "web", "--trust-on-first-use", "--api-url", srv.URL})
	if gotMethod != http.MethodPost || gotPath != "/api/v1/apps/web/git-source/ssh-key" || gotBody["trust_on_first_use"] != true {
		t.Errorf("request = %s %s %v", gotMethod, gotPath, gotBody)
	}
	if !strings.Contains(stdout, "SHA256:host (pinned)") || !strings.Contains(stdout, "ssh-ed25519 AAAA") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestRun_AppsGitSourceSSHKeyNeedsAHostKeyChoice(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusInternalServerError, `{"error":"must not be called"}`)
	for _, args := range [][]string{
		{"apps", "git-source", "ssh-key", "generate", "web", "--api-url", srv.URL},
		{"apps", "git-source", "ssh-key", "generate", "web", "--trust-on-first-use", "--host-key", "x", "--api-url", srv.URL},
	} {
		if stderr := runCLIExpectValidationError(t, args); !strings.Contains(stderr, "exactly one of") {
			t.Errorf("%v: stderr = %q", args, stderr)
		}
	}
}

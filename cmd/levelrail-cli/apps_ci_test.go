package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func ciTokenServer(t *testing.T, gotIDToken *string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		*gotIDToken = body["id_token"]
		_ = json.NewEncoder(w).Encode(map[string]any{"token": "lr_deploy_token", "expires_at": "2030-01-01T00:00:00Z"}) //nolint:gosec // fake test token
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestRun_AppsCITokenIsProviderNeutral(t *testing.T) {
	var gotIDToken string
	srv := ciTokenServer(t, &gotIDToken)
	env := map[string]string{"LEVELRAIL_ID_TOKEN": "gitlab.jwt", "MY_JWT": "circle.jwt"} //nolint:gosec // fake test tokens
	lookup := func(k string) (string, bool) { v, ok := env[k]; return v, ok }

	for _, tc := range []struct {
		args  []string
		idTok string
	}{
		{[]string{"apps", "ci", "token", "web", "--provider", "gitlab", "--api-url", srv.URL}, "gitlab.jwt"},
		{[]string{"apps", "ci", "token", "web", "--provider", "circleci", "--id-token-env", "MY_JWT", "--api-url", srv.URL}, "circle.jwt"},
		{[]string{"apps", "github-actions", "token", "web", "--id-token-env", "MY_JWT", "--api-url", srv.URL}, "circle.jwt"},
	} {
		var stdout, stderr bytes.Buffer
		if code := run("levelrail-cli-test", tc.args, &stdout, &stderr, lookup); code != exitOK {
			t.Fatalf("%v exit = %d, stderr = %s", tc.args, code, stderr.String())
		}
		if gotIDToken != tc.idTok || strings.TrimSpace(stdout.String()) != "lr_deploy_token" {
			t.Errorf("%v sent %q, printed %q", tc.args, gotIDToken, stdout.String())
		}
	}
}

func TestRun_AppsCITokenRejectsBadProvider(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run("levelrail-cli-test", []string{"apps", "ci", "token", "web", "--provider", "bitbucket", "--api-url", "http://127.0.0.1:1"}, &stdout, &stderr, envMap())
	if code == exitOK || !strings.Contains(stderr.String()+stdout.String(), "--provider must be") {
		t.Fatalf("exit %d, out %q err %q", code, stdout.String(), stderr.String())
	}
}

func TestRun_GitHubActionsTokenIsMarkedDeprecated(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run("levelrail-cli-test", []string{"apps", "github-actions", "--help"}, &stdout, &stderr, envMap()); code != exitOK {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(stdout.String(), "deprecated alias of") || !strings.Contains(stdout.String(), "apps ci token") {
		t.Errorf("help = %q", stdout.String())
	}
}

package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_AppsCITemplate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/apps/web/ci/templates" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"templates":[{"id":"github","target":".github/workflows/deploy.yml","auth":"oidc","content":"name: deploy\n"},{"id":"jenkins","target":"Jenkinsfile","auth":"token","content":"pipeline {}\n"}],"api_url":"","needs_domain":true,"deployer_abilities":["read","deploy"]}`))
	}))
	t.Cleanup(srv.Close)
	base := []string{"--token", "t", "--api-url", srv.URL}

	var stdout, stderr bytes.Buffer
	if got := run("levelrail-cli-test", append([]string{"apps", "ci", "template", "web"}, base...), &stdout, &stderr, envMap()); got != exitOK || !strings.Contains(stdout.String(), "jenkins") {
		t.Fatalf("list: exit = %d, stdout = %q, stderr = %q", got, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if got := run("levelrail-cli-test", append([]string{"apps", "ci", "template", "web", "jenkins"}, base...), &stdout, &stderr, envMap()); got != exitOK || stdout.String() != "pipeline {}\n" {
		t.Fatalf("print: exit = %d, stdout = %q, stderr = %q", got, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "--abilities read,deploy") {
		t.Errorf("token template must say how to create the deployer token, stderr = %q", stderr.String())
	}
}

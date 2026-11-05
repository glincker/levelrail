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
	stdout.Reset()
	stderr.Reset()
	if got := run("levelrail-cli-test", append([]string{"apps", "ci", "template", "web", "jenkns"}, base...), &stdout, &stderr, envMap()); got != exitUsage || stdout.Len() != 0 || !strings.Contains(stderr.String(), "unknown provider") {
		t.Fatalf("typo: exit = %d, stdout = %q, stderr = %q", got, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "unknown provider") {
		t.Fatal("unknown provider must explain itself")
	}
	stdout.Reset()
	stderr.Reset()
	if got := run("levelrail-cli-test", append([]string{"apps", "ci", "template", "web", "jenkins"}, base...), &stdout, &stderr, envMap()); got != exitOK {
		t.Fatalf("print again: exit = %d", got)
	}
	if !strings.Contains(stderr.String(), "no project, so this token can deploy any app") || strings.Contains(stderr.String(), "expires-in-days") {
		t.Errorf("install-wide hint missing or stale, stderr = %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "--abilities read,deploy") {
		t.Errorf("token template must say how to create the deployer token, stderr = %q", stderr.String())
	}
}

func TestRun_AppsCITemplate_TokenNoteFollowsTheScope(t *testing.T) {
	for _, tc := range []struct {
		name    string
		extra   string
		want    []string
		notWant string
	}{
		{"scoped", `,"deployer_scope":{"team_id":"team_1","project_id":"proj_1","team_role":"developer"}`,
			[]string{"--name ci-web", "--scope-team team_1 --scope-project proj_1 --scope-role developer", "every app in project proj_1"}, "--name ci-deployer"},
		{"unavailable", `,"deployer_scope_unavailable":true`,
			[]string{"--name ci-deployer", "cannot scope tokens to the app's project"}, "--scope-team"},
		{"no project", ``,
			[]string{"--name ci-deployer", "in no project, so this token can deploy any app"}, "--scope-team"},
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"templates":[{"id":"jenkins","target":"Jenkinsfile","auth":"token","content":"pipeline {}\n"}],"api_url":"https://cp.example.test","needs_domain":false,"deployer_abilities":["read","deploy"]` + tc.extra + `}`))
		}))
		var stdout, stderr bytes.Buffer
		args := []string{"apps", "ci", "template", "web", "jenkins", "--token", "t", "--api-url", srv.URL}
		if got := run("levelrail-cli-test", args, &stdout, &stderr, envMap()); got != exitOK {
			t.Fatalf("%s: exit = %d, stderr = %q", tc.name, got, stderr.String())
		}
		srv.Close()
		for _, want := range tc.want {
			if !strings.Contains(stderr.String(), want) {
				t.Errorf("%s: stderr is missing %q:\n%s", tc.name, want, stderr.String())
			}
		}
		if strings.Contains(stderr.String(), tc.notWant) {
			t.Errorf("%s: stderr must not contain %q:\n%s", tc.name, tc.notWant, stderr.String())
		}
	}
}

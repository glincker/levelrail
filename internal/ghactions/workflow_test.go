package ghactions

import (
	"strings"
	"testing"
)

func TestWorkflow(t *testing.T) {
	t.Parallel()
	got := Workflow(WorkflowOptions{
		App: "web", APIURL: "https://cp.example.com", Branch: "trunk", Environment: "staging",
		Paths: []string{"src/**"}, IgnorePaths: []string{"docs/**"}, ActionRef: "org/repo/action@main",
	})
	for _, want := range []string{
		"name: Deploy web", "branches: [trunk]", "paths:\n      - \"src/**\"", "paths-ignore:\n      - \"docs/**\"",
		"id-token: write", "environment: staging", "uses: org/repo/action@main", "oidc: \"true\"",
		"rollback-on-failure: \"true\"", "api-url: https://cp.example.com", "!contains(github.event.head_commit.message, '[skip ci]')",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("workflow missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "secrets.LEVEL") || strings.Contains(got, "api-token") {
		t.Errorf("workflow must not reference a long-lived deploy secret:\n%s", got)
	}
}

func TestWorkflowDefaults(t *testing.T) {
	t.Parallel()
	got := Workflow(WorkflowOptions{App: "web", APIURL: "https://x", ActionRef: "a/b/c@main"})
	if !strings.Contains(got, "branches: [main]") || !strings.Contains(got, "environment: production") {
		t.Fatalf("defaults missing:\n%s", got)
	}
}

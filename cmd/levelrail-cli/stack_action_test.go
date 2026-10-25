package main

import (
	"os"
	"strings"
	"testing"
)

func readActionFile(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile("../../integrations/github-action/stack/" + name) //nolint:gosec // fixed in-repo test path
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestStackAction_StickyCommentOnlyEditsTheActionsBot(t *testing.T) {
	if !strings.Contains(readActionFile(t, "action.yml"), "c.user.login === 'github-actions[bot]'") {
		t.Fatal("sticky comment lookup must require the github-actions[bot] author")
	}
}

func TestStackAction_ExamplePinsTheCLIVersion(t *testing.T) {
	ex := readActionFile(t, "example-workflow.yml")
	if strings.Contains(ex, "version: latest") || strings.Count(ex, "version: v") < 3 {
		t.Fatal("every stack step in the example must pin version to a release tag")
	}
}

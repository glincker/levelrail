package main

import (
	"encoding/json"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestAbsoluteWebhookURL(t *testing.T) {
	cases := []struct{ base, raw, want string }{
		{"https://cp.example.com", "/api/v1/webhooks/github/web", "https://cp.example.com/api/v1/webhooks/github/web"},
		{"https://cp.example.com/", "api/v1/webhooks/github/web", "https://cp.example.com/api/v1/webhooks/github/web"},
		{"http://127.0.0.1:8080", "https://hooks.example.com/x", "https://hooks.example.com/x"},
	}
	for _, tc := range cases {
		if got := absoluteWebhookURL(tc.base, tc.raw); got != tc.want {
			t.Errorf("absoluteWebhookURL(%q, %q) = %q, want %q", tc.base, tc.raw, got, tc.want)
		}
	}
}

func TestDeployAttemptJSONIncludesCommitFields(t *testing.T) {
	b, err := json.Marshal(apiclient.DeployAttemptResource{ID: "dep_1", Branch: "main", Author: "ann", CommitMessage: "fix", Trigger: "git_push"})
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"branch", "author", "commit_message", "trigger"} {
		if m[k] == nil {
			t.Errorf("missing %s in %s", k, b)
		}
	}
}

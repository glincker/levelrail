package webhook

import (
	"errors"
	"net/http"
	"testing"
)

func TestIsIssueCommentEvent(t *testing.T) {
	tests := []struct {
		name   string
		header map[string]string
		want   bool
	}{
		{"github", map[string]string{HeaderGitHubEvent: "issue_comment"}, true},
		{"other github event", map[string]string{HeaderGitHubEvent: "push"}, false},
		{"gitea compat headers", map[string]string{HeaderGitHubEvent: "issue_comment", HeaderGiteaEventType: "issue_comment"}, false},
		{"gitlab", map[string]string{HeaderGitHubEvent: "issue_comment", HeaderGitLabEvent: "Note Hook"}, false},
		{"none", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := http.Header{}
			for k, v := range tt.header {
				h.Set(k, v)
			}
			if got := IsIssueCommentEvent(h); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseIssueCommentEvent(t *testing.T) {
	ev, err := ParseIssueCommentEvent([]byte(`{"action":"created","issue":{"number":5,"pull_request":{"url":"x"}},"comment":{"id":99,"body":"/levelrail status","user":{"login":"octo","type":"User"},"performed_via_github_app":null},"sender":{"type":"User"},"repository":{"full_name":"acme/widgets"}}`))
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if ev.CommentID != 99 || ev.Number != 5 || !ev.IsPullRequest || ev.PerformedViaApp || ev.Commenter != "octo" || ev.RepoFullName != "acme/widgets" {
		t.Errorf("ev = %+v", ev)
	}

	ev, err = ParseIssueCommentEvent([]byte(`{"action":"created","issue":{"number":5},"comment":{"id":1,"body":"x","user":{"login":"a","type":"Bot"},"performed_via_github_app":{"id":3}},"sender":{"type":"Bot"},"repository":{"full_name":"a/b"}}`))
	if err != nil || ev.IsPullRequest || !ev.PerformedViaApp || ev.CommenterType != "Bot" {
		t.Errorf("ev = %+v err = %v", ev, err)
	}

	if _, err := ParseIssueCommentEvent([]byte(`{"action":"created"}`)); !errors.Is(err, ErrIssueCommentFieldsMissing) {
		t.Errorf("err = %v, want ErrIssueCommentFieldsMissing", err)
	}
	if _, err := ParseIssueCommentEvent([]byte(`nope`)); err == nil {
		t.Error("want a decode error")
	}
}

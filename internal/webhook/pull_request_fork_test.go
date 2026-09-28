package webhook

import (
	"fmt"
	"testing"
)

func githubForkBody(headRepo, baseRepo string) []byte {
	head := "null"
	if headRepo != "" {
		head = fmt.Sprintf(`{"full_name": %q}`, headRepo)
	}
	return []byte(fmt.Sprintf(`{"action":"opened","number":7,"pull_request":{
		"head":{"ref":"feat","sha":"abc","repo":%s},
		"base":{"ref":"main","repo":{"full_name":%q}}}}`, head, baseRepo))
}

func gitlabForkBody(source, target string) []byte {
	return []byte(fmt.Sprintf(`{"object_attributes":{"iid":7,"action":"open","source_branch":"feat","target_branch":"main",
		"source":{"path_with_namespace":%q},"target":{"path_with_namespace":%q},"last_commit":{"id":"abc"}}}`, source, target))
}

func bitbucketForkBody(source, target string) []byte {
	return []byte(fmt.Sprintf(`{"pullrequest":{"id":7,
		"source":{"branch":{"name":"feat"},"commit":{"hash":"abc"},"repository":{"full_name":%q}},
		"destination":{"branch":{"name":"main"},"repository":{"full_name":%q}}}}`, source, target))
}

func TestPullRequestEventForkDetection(t *testing.T) {
	tests := []struct {
		name      string
		body      []byte
		header    string
		headerVal string
		wantHead  string
		wantBase  string
		wantFork  bool
	}{
		{"github same repo", githubForkBody("acme/app", "acme/app"), "X-GitHub-Event", "pull_request", "acme/app", "acme/app", false},
		{"github fork", githubForkBody("mallory/app", "acme/app"), "X-GitHub-Event", "pull_request", "mallory/app", "acme/app", true},
		{"github case-insensitive same repo", githubForkBody("Acme/App", "acme/app"), "X-GitHub-Event", "pull_request", "Acme/App", "acme/app", false},
		{"github different base repo", githubForkBody("acme/app", "acme/other"), "X-GitHub-Event", "pull_request", "acme/app", "acme/other", true},
		{"github deleted fork fails closed", githubForkBody("", "acme/app"), "X-GitHub-Event", "pull_request", "", "acme/app", true},
		{"gitea same repo", githubForkBody("acme/app", "acme/app"), "X-Gitea-Event-Type", "pull_request", "acme/app", "acme/app", false},
		{"gitea fork", githubForkBody("mallory/app", "acme/app"), "X-Gitea-Event-Type", "pull_request", "mallory/app", "acme/app", true},
		{"gitlab same project", gitlabForkBody("acme/app", "acme/app"), "X-Gitlab-Event", "Merge Request Hook", "acme/app", "acme/app", false},
		{"gitlab fork", gitlabForkBody("mallory/app", "acme/app"), "X-Gitlab-Event", "Merge Request Hook", "mallory/app", "acme/app", true},
		{"bitbucket same repo", bitbucketForkBody("acme/app", "acme/app"), "X-Event-Key", "pullrequest:created", "acme/app", "acme/app", false},
		{"bitbucket fork", bitbucketForkBody("mallory/app", "acme/app"), "X-Event-Key", "pullrequest:created", "mallory/app", "acme/app", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParsePullRequestEventForProvider(tt.body, headerWith(tt.header, tt.headerVal))
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if got.HeadRepoFullName != tt.wantHead || got.BaseRepoFullName != tt.wantBase {
				t.Errorf("repos = %q -> %q, want %q -> %q", got.HeadRepoFullName, got.BaseRepoFullName, tt.wantHead, tt.wantBase)
			}
			if got.IsFork() != tt.wantFork {
				t.Errorf("IsFork() = %v, want %v", got.IsFork(), tt.wantFork)
			}
		})
	}
}

func TestPullRequestEventWithoutRepoInfoIsAFork(t *testing.T) {
	if !(PullRequestEvent{}).IsFork() {
		t.Fatal("an event naming no repositories must fail closed")
	}
	if !(PullRequestEvent{HeadRepoFullName: "acme/app"}).IsFork() {
		t.Fatal("a missing base repository must fail closed")
	}
}

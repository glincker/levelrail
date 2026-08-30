package webhook

import (
	"net/http"
	"testing"
)

func headerWith(key, value string) http.Header {
	h := http.Header{}
	h.Set(key, value)
	return h
}

func TestIsPullRequestEvent(t *testing.T) {
	tests := []struct {
		name   string
		header http.Header
		want   bool
	}{
		{"github pull_request", headerWith("X-GitHub-Event", "pull_request"), true},
		{"github push", headerWith("X-GitHub-Event", "push"), false},
		{"gitlab merge request", headerWith("X-Gitlab-Event", "Merge Request Hook"), true},
		{"gitlab push", headerWith("X-Gitlab-Event", "Push Hook"), false},
		{"bitbucket created", headerWith("X-Event-Key", "pullrequest:created"), true},
		{"bitbucket push", headerWith("X-Event-Key", "repo:push"), false},
		{"no headers", http.Header{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsPullRequestEvent(tt.header); got != tt.want {
				t.Errorf("IsPullRequestEvent() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParsePullRequestEventForProvider_GitHub(t *testing.T) {
	body := []byte(`{
		"action": "opened",
		"number": 42,
		"pull_request": {"head": {"ref": "feature-x", "sha": "abc123"}, "base": {"ref": "main"}}
	}`)
	got, err := ParsePullRequestEventForProvider(body, headerWith("X-GitHub-Event", "pull_request"))
	if err != nil {
		t.Fatalf("ParsePullRequestEventForProvider() error = %v", err)
	}
	want := PullRequestEvent{Action: PullRequestOpened, Number: 42, HeadRef: "feature-x", HeadSHA: "abc123", BaseRef: "main"}
	if got != want {
		t.Errorf("ParsePullRequestEventForProvider() = %+v, want %+v", got, want)
	}
}

func TestParsePullRequestEventForProvider_GitHubSynchronizeAndClosed(t *testing.T) {
	tests := []struct {
		action string
		want   PullRequestAction
	}{
		{"synchronize", PullRequestSynchronize},
		{"reopened", PullRequestOpened},
		{"closed", PullRequestClosed},
	}
	for _, tt := range tests {
		t.Run(tt.action, func(t *testing.T) {
			body := []byte(`{"action":"` + tt.action + `","number":7,"pull_request":{"head":{"ref":"x","sha":"sha1"},"base":{"ref":"main"}}}`)
			got, err := ParsePullRequestEventForProvider(body, headerWith("X-GitHub-Event", "pull_request"))
			if err != nil {
				t.Fatalf("ParsePullRequestEventForProvider() error = %v", err)
			}
			if got.Action != tt.want {
				t.Errorf("action = %q, want %q", got.Action, tt.want)
			}
		})
	}
}

func TestParsePullRequestEventForProvider_GitHubUnrecognizedAction(t *testing.T) {
	body := []byte(`{"action":"labeled","number":7,"pull_request":{"head":{"ref":"x","sha":"sha1"}}}`)
	_, err := ParsePullRequestEventForProvider(body, headerWith("X-GitHub-Event", "pull_request"))
	if err != ErrPullRequestEventFieldsMissing {
		t.Fatalf("ParsePullRequestEventForProvider() error = %v, want ErrPullRequestEventFieldsMissing", err)
	}
}

func TestParsePullRequestEventForProvider_GitLab(t *testing.T) {
	body := []byte(`{
		"object_attributes": {
			"iid": 9, "action": "open", "source_branch": "feature-y",
			"target_branch": "main", "last_commit": {"id": "def456"}
		}
	}`)
	got, err := ParsePullRequestEventForProvider(body, headerWith("X-Gitlab-Event", "Merge Request Hook"))
	if err != nil {
		t.Fatalf("ParsePullRequestEventForProvider() error = %v", err)
	}
	want := PullRequestEvent{Action: PullRequestOpened, Number: 9, HeadRef: "feature-y", HeadSHA: "def456", BaseRef: "main"}
	if got != want {
		t.Errorf("ParsePullRequestEventForProvider() = %+v, want %+v", got, want)
	}
}

func TestParsePullRequestEventForProvider_Bitbucket(t *testing.T) {
	body := []byte(`{
		"pullrequest": {
			"id": 3,
			"source": {"branch": {"name": "feature-z"}, "commit": {"hash": "ghi789"}},
			"destination": {"branch": {"name": "main"}}
		}
	}`)
	got, err := ParsePullRequestEventForProvider(body, headerWith("X-Event-Key", "pullrequest:created"))
	if err != nil {
		t.Fatalf("ParsePullRequestEventForProvider() error = %v", err)
	}
	want := PullRequestEvent{Action: PullRequestOpened, Number: 3, HeadRef: "feature-z", HeadSHA: "ghi789", BaseRef: "main"}
	if got != want {
		t.Errorf("ParsePullRequestEventForProvider() = %+v, want %+v", got, want)
	}

	closed, err := ParsePullRequestEventForProvider(body, headerWith("X-Event-Key", "pullrequest:fulfilled"))
	if err != nil {
		t.Fatalf("ParsePullRequestEventForProvider() error = %v", err)
	}
	if closed.Action != PullRequestClosed {
		t.Errorf("fulfilled action = %q, want %q", closed.Action, PullRequestClosed)
	}

	rejected, err := ParsePullRequestEventForProvider(body, headerWith("X-Event-Key", "pullrequest:rejected"))
	if err != nil {
		t.Fatalf("ParsePullRequestEventForProvider() error = %v", err)
	}
	if rejected.Action != PullRequestClosed {
		t.Errorf("rejected action = %q, want %q", rejected.Action, PullRequestClosed)
	}
}

func TestParsePullRequestEventForProvider_MalformedJSON(t *testing.T) {
	_, err := ParsePullRequestEventForProvider([]byte("not json"), headerWith("X-GitHub-Event", "pull_request"))
	if err == nil {
		t.Fatal("ParsePullRequestEventForProvider() error = nil, want an error for malformed JSON")
	}
}

func TestParsePullRequestEventForProvider_MissingSHA(t *testing.T) {
	body := []byte(`{"action":"opened","number":1,"pull_request":{"head":{"ref":"x","sha":""}}}`)
	_, err := ParsePullRequestEventForProvider(body, headerWith("X-GitHub-Event", "pull_request"))
	if err != ErrPullRequestEventFieldsMissing {
		t.Fatalf("ParsePullRequestEventForProvider() error = %v, want ErrPullRequestEventFieldsMissing", err)
	}
}

package webhook

import (
	"fmt"
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

// gitlabMergeRequestFixture builds a Merge Request Hook payload shaped
// like GitLab's own documented example
// (https://docs.gitlab.com/user/project/integrations/webhook_events/#merge-request-events),
// trimmed to the fields object_attributes carries in every real
// delivery this package reads plus a representative sample of the
// surrounding fields (user, project, labels, changes) a real payload
// always includes, to make sure the parser tolerates unknown sibling
// fields rather than depending on the payload being pre-trimmed.
func gitlabMergeRequestFixture(action string, iid int, sourceBranch, targetBranch, commitID string) []byte {
	return []byte(fmt.Sprintf(`{
		"object_kind": "merge_request",
		"user": {"name": "Ada Lovelace", "username": "ada"},
		"project": {"id": 123, "name": "widgets", "default_branch": "main"},
		"object_attributes": {
			"id": 555, "iid": %d, "title": "Add widget support",
			"action": %q, "source_branch": %q,
			"target_branch": %q, "state": "opened",
			"merge_status": "unchecked",
			"last_commit": {"id": %q, "message": "add widget", "author": {"name": "Ada Lovelace"}}
		},
		"labels": [],
		"changes": {}
	}`, iid, action, sourceBranch, targetBranch, commitID))
}

// TestParsePullRequestEventForProvider_GitLab covers the "open" action:
// GitLab's Merge Request Hook fires this the moment an MR is created,
// the merge-request equivalent of GitHub's "opened".
func TestParsePullRequestEventForProvider_GitLab(t *testing.T) {
	body := gitlabMergeRequestFixture("open", 9, "feature-y", "main", "def456")
	got, err := ParsePullRequestEventForProvider(body, headerWith("X-Gitlab-Event", "Merge Request Hook"))
	if err != nil {
		t.Fatalf("ParsePullRequestEventForProvider() error = %v", err)
	}
	want := PullRequestEvent{Action: PullRequestOpened, Number: 9, HeadRef: "feature-y", HeadSHA: "def456", BaseRef: "main"}
	if got != want {
		t.Errorf("ParsePullRequestEventForProvider() = %+v, want %+v", got, want)
	}
}

// TestParsePullRequestEventForProvider_GitLabSynchronizeReopenAndClosed
// covers the remaining action values GitLab's own docs list for
// object_attributes.action: "update" (new commits pushed to an
// already-open MR, this package's synchronize equivalent), "reopen",
// "close" (closed without merging), and "merge" (closed via merge).
// Each case asserts Number/HeadRef/HeadSHA/BaseRef, not just Action, so
// a field-path regression in gitlabPullRequestPayload would fail here
// even if the wrong action still happened to normalize correctly.
func TestParsePullRequestEventForProvider_GitLabSynchronizeReopenAndClosed(t *testing.T) {
	tests := []struct {
		action string
		want   PullRequestAction
	}{
		{"update", PullRequestSynchronize},
		{"reopen", PullRequestOpened},
		{"close", PullRequestClosed},
		{"merge", PullRequestClosed},
	}
	for _, tt := range tests {
		t.Run(tt.action, func(t *testing.T) {
			body := gitlabMergeRequestFixture(tt.action, 14, "fix-bug", "develop", "abc999")
			got, err := ParsePullRequestEventForProvider(body, headerWith("X-Gitlab-Event", "Merge Request Hook"))
			if err != nil {
				t.Fatalf("ParsePullRequestEventForProvider() error = %v", err)
			}
			want := PullRequestEvent{Action: tt.want, Number: 14, HeadRef: "fix-bug", HeadSHA: "abc999", BaseRef: "develop"}
			if got != want {
				t.Errorf("ParsePullRequestEventForProvider() = %+v, want %+v", got, want)
			}
		})
	}
}

// TestParsePullRequestEventForProvider_GitLabApprovalActionIgnored
// documents current, deliberate behavior for action values GitLab's
// Merge Request Hook can send that this package has no use for
// ("approved", "unapproved", the events fired by an MR approval, not a
// code change): normalizePullRequestAction has no case for them, so
// they surface as ErrPullRequestEventFieldsMissing exactly like an
// unrecognized GitHub action does (see
// TestParsePullRequestEventForProvider_GitHubUnrecognizedAction), one
// shared table producing one shared "not a lifecycle event this
// package acts on" outcome across all three providers.
func TestParsePullRequestEventForProvider_GitLabApprovalActionIgnored(t *testing.T) {
	body := gitlabMergeRequestFixture("approved", 14, "fix-bug", "develop", "abc999")
	_, err := ParsePullRequestEventForProvider(body, headerWith("X-Gitlab-Event", "Merge Request Hook"))
	if err != ErrPullRequestEventFieldsMissing {
		t.Fatalf("ParsePullRequestEventForProvider() error = %v, want ErrPullRequestEventFieldsMissing", err)
	}
}

// bitbucketPullRequestFixture builds a pullrequest:* payload shaped
// like Bitbucket Cloud's own documented examples
// (https://support.atlassian.com/bitbucket-cloud/docs/event-payloads/#Pull-request-created,
// same page's Updated/Fulfilled/Rejected sections), including the
// surrounding actor/repository fields a real delivery always carries
// alongside the pullrequest object this package actually reads.
func bitbucketPullRequestFixture(id int, sourceBranch, sourceHash, destBranch string) []byte {
	return []byte(fmt.Sprintf(`{
		"actor": {"username": "ada", "display_name": "Ada Lovelace"},
		"repository": {"full_name": "acme/widgets", "name": "widgets"},
		"pullrequest": {
			"id": %d,
			"title": "Add widget support",
			"state": "OPEN",
			"source": {
				"branch": {"name": %q},
				"commit": {"hash": %q}
			},
			"destination": {
				"branch": {"name": %q},
				"commit": {"hash": "0000000000000000000000000000000000000000"}
			}
		}
	}`, id, sourceBranch, sourceHash, destBranch))
}

// TestParsePullRequestEventForProvider_Bitbucket covers
// pullrequest:created, the event Bitbucket Cloud fires when a pull
// request is opened.
func TestParsePullRequestEventForProvider_Bitbucket(t *testing.T) {
	body := bitbucketPullRequestFixture(3, "feature-z", "ghi789", "main")
	got, err := ParsePullRequestEventForProvider(body, headerWith("X-Event-Key", "pullrequest:created"))
	if err != nil {
		t.Fatalf("ParsePullRequestEventForProvider() error = %v", err)
	}
	want := PullRequestEvent{Action: PullRequestOpened, Number: 3, HeadRef: "feature-z", HeadSHA: "ghi789", BaseRef: "main"}
	if got != want {
		t.Errorf("ParsePullRequestEventForProvider() = %+v, want %+v", got, want)
	}
}

// TestParsePullRequestEventForProvider_BitbucketUpdatedFulfilledAndRejected
// covers pullrequest:updated (this package's synchronize equivalent,
// fired on new commits and on metadata changes alike), pullrequest:fulfilled
// (merged), and pullrequest:rejected (declined), each asserting the
// full parsed event, not just Action, against a distinct fixture.
func TestParsePullRequestEventForProvider_BitbucketUpdatedFulfilledAndRejected(t *testing.T) {
	tests := []struct {
		eventKey string
		want     PullRequestAction
	}{
		{"pullrequest:updated", PullRequestSynchronize},
		{"pullrequest:fulfilled", PullRequestClosed},
		{"pullrequest:rejected", PullRequestClosed},
	}
	for _, tt := range tests {
		t.Run(tt.eventKey, func(t *testing.T) {
			body := bitbucketPullRequestFixture(8, "hotfix-y", "jkl012", "release")
			got, err := ParsePullRequestEventForProvider(body, headerWith("X-Event-Key", tt.eventKey))
			if err != nil {
				t.Fatalf("ParsePullRequestEventForProvider() error = %v", err)
			}
			want := PullRequestEvent{Action: tt.want, Number: 8, HeadRef: "hotfix-y", HeadSHA: "jkl012", BaseRef: "release"}
			if got != want {
				t.Errorf("ParsePullRequestEventForProvider() = %+v, want %+v", got, want)
			}
		})
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

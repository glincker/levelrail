package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/giteaapp"
)

// giteaPullRequestBody builds a pull_request event payload carrying
// just the fields parseGiteaPullRequestEvent (internal/webhook) reads,
// mirroring githubPullRequestBody's own shape. The PR number, head SHA,
// and base branch are fixed: every test in this file exercises the
// same PR against the same branch, matching setUpPreviewApp's own
// fixed-name convention for the app itself.
func giteaPullRequestBody(action string) []byte {
	return giteaPullRequestBodyFrom(action, "acme/web")
}

func giteaPullRequestBodyFrom(action, headRepo string) []byte {
	b, _ := json.Marshal(map[string]any{
		"action": action,
		"number": 42,
		"pull_request": map[string]any{
			"head": map[string]any{"ref": "feature-x", "sha": "sha1", "repo": map[string]any{"full_name": headRepo}},
			"base": map[string]any{"ref": "main", "repo": map[string]any{"full_name": "acme/web"}},
		},
	})
	return b
}

// sendGiteaPullRequestWebhook posts body to app "web"'s webhook route
// under Gitea's own auth scheme (X-Hub-Signature-256, the same
// HMAC-SHA256 GitHub uses, sent for GitHub compatibility), mirroring
// sendPullRequestWebhook's own shape for GitHub.
func sendGiteaPullRequestWebhook(rt *Router, secret string, body []byte) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/github/web", bytes.NewReader(body))
	req.Header.Set("X-Gitea-Event-Type", "pull_request")
	req.Header.Set("X-Hub-Signature-256", sign([]byte(secret), body))
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	return rec
}

// setUpPreviewAppWithGiteaNotifications mirrors
// setUpPreviewAppWithGitHubNotifications for Gitea: a connected,
// authorized Gitea OAuth2 application plus the same preview-app fixture.
func setUpPreviewAppWithGiteaNotifications(t *testing.T, postPRComments bool) (rt *Router, secret string, builder *sequencedBuilder, fakeClient *fakeGiteaAppClient) {
	t.Helper()
	gitSourceSecrets := newFakeGitSourceSecrets()
	giteaSecrets := authorizedGiteaSecrets(t)
	fakeClient = &fakeGiteaAppClient{}

	rt, db := newTestRouterWithGiteaAndGitSource(t, giteaSecrets, fakeClient, gitSourceSecrets)
	cookie := loginTestSession(t, rt, db)
	seedGiteaAppConnection(t, db)
	seedApp(t, db, "web")

	created := connectGitSource(t, rt, cookie, `{"repo_url":"https://git.example.com/org/web.git","branch":"main"}`)
	setPreviewEnabled(t, rt, cookie)
	if postPRComments {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/preview-settings", `{"post_pr_comments":true}`))
		if rec.Code != http.StatusOK {
			t.Fatalf("set post_pr_comments: status = %d, body = %s", rec.Code, rec.Body.String())
		}
	}

	builder = &sequencedBuilder{db: db, tag: "levelrail/web-pr-42:sha1"}
	rt.builder = builder
	rt.gitSourceFetch = newFakeGitSourceFetch(&[]fakeGitSourceFetchCall{}, t.TempDir(), new(bool), nil)

	return rt, created.WebhookSecret, builder, fakeClient
}

// TestHandlePullRequestWebhook_Gitea_PostPRComments_PostsPendingThenSuccess
// proves a successful preview deploy posts a pending commit status
// before the build, then a success commit status plus an issue comment
// once it's live, mirroring the GitHub equivalent.
func TestHandlePullRequestWebhook_Gitea_PostPRComments_PostsPendingThenSuccess(t *testing.T) {
	rt, secret, _, fakeClient := setUpPreviewAppWithGiteaNotifications(t, true)

	body := giteaPullRequestBody("opened")
	rec := sendGiteaPullRequestWebhook(rt, secret, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	if len(fakeClient.statusCalls) != 2 {
		t.Fatalf("CreateCommitStatus called %d times, want 2 (pending, then success), got %+v", len(fakeClient.statusCalls), fakeClient.statusCalls)
	}
	if fakeClient.statusCalls[0].state != giteaapp.CommitStatusPending {
		t.Errorf("first status state = %q, want pending", fakeClient.statusCalls[0].state)
	}
	if fakeClient.statusCalls[1].state != giteaapp.CommitStatusSuccess {
		t.Errorf("second status state = %q, want success", fakeClient.statusCalls[1].state)
	}
	for _, call := range fakeClient.statusCalls {
		if call.fullName != "org/web" || call.sha != "sha1" {
			t.Errorf("status call = %+v, want full_name=org/web sha=sha1", call)
		}
	}

	if len(fakeClient.commentCalls) != 1 {
		t.Fatalf("CreateIssueComment called %d times, want 1", len(fakeClient.commentCalls))
	}
	if fakeClient.commentCalls[0].number != 42 {
		t.Errorf("comment PR number = %d, want 42", fakeClient.commentCalls[0].number)
	}
}

// TestHandlePullRequestWebhook_Gitea_PostPRComments_Disabled_NoGiteaCalls
// proves the opt-in gate: with post_pr_comments left off, a preview
// still deploys, but no Gitea API call is ever made.
func TestHandlePullRequestWebhook_Gitea_PostPRComments_Disabled_NoGiteaCalls(t *testing.T) {
	rt, secret, _, fakeClient := setUpPreviewAppWithGiteaNotifications(t, false)

	body := giteaPullRequestBody("opened")
	rec := sendGiteaPullRequestWebhook(rt, secret, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	if len(fakeClient.statusCalls) != 0 {
		t.Errorf("CreateCommitStatus called %d times, want 0 when post_pr_comments is off", len(fakeClient.statusCalls))
	}
	if len(fakeClient.commentCalls) != 0 {
		t.Errorf("CreateIssueComment called %d times, want 0 when post_pr_comments is off", len(fakeClient.commentCalls))
	}
}

// TestHandlePullRequestWebhook_Gitea_PostPRComments_DeployFailed_PostsFailureStatusOnly
// proves a failed deploy posts a failure commit status, and the single status comment ends in the failed state.
func TestHandlePullRequestWebhook_Gitea_PostPRComments_DeployFailed_PostsFailureStatusOnly(t *testing.T) {
	rt, secret, builder, fakeClient := setUpPreviewAppWithGiteaNotifications(t, true)
	builder.errs = []error{context.DeadlineExceeded}

	body := giteaPullRequestBody("opened")
	rec := sendGiteaPullRequestWebhook(rt, secret, body)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}

	if len(fakeClient.statusCalls) != 2 {
		t.Fatalf("CreateCommitStatus called %d times, want 2 (pending, then failure), got %+v", len(fakeClient.statusCalls), fakeClient.statusCalls)
	}
	if fakeClient.statusCalls[1].state != giteaapp.CommitStatusFailure {
		t.Errorf("second status state = %q, want failure", fakeClient.statusCalls[1].state)
	}
	if len(fakeClient.commentCalls) != 1 || !strings.Contains(fakeClient.prComments.comments[0].Body, "Preview environment: failed") {
		t.Errorf("status comment = %+v (created %d), want one comment ending in the failed state", fakeClient.prComments.comments, len(fakeClient.commentCalls))
	}
}

// TestHandlePullRequestWebhook_Gitea_PostPRComments_Teardown_PostsComment
// proves a torn-down preview posts an issue comment noting the
// teardown, mirroring the GitHub equivalent.
func TestHandlePullRequestWebhook_Gitea_PostPRComments_Teardown_PostsComment(t *testing.T) {
	rt, secret, _, fakeClient := setUpPreviewAppWithGiteaNotifications(t, true)

	opened := giteaPullRequestBody("opened")
	if rec := sendGiteaPullRequestWebhook(rt, secret, opened); rec.Code != http.StatusOK {
		t.Fatalf("opened: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	fakeClient.commentCalls = nil

	closed := giteaPullRequestBody("closed")
	rec := sendGiteaPullRequestWebhook(rt, secret, closed)
	if rec.Code != http.StatusOK {
		t.Fatalf("closed: status = %d, body = %s", rec.Code, rec.Body.String())
	}

	if len(fakeClient.commentCalls) != 0 {
		t.Errorf("teardown created %d new comments, want the existing one edited in place", len(fakeClient.commentCalls))
	}
	if n := len(fakeClient.prComments.updates); n == 0 || !strings.Contains(fakeClient.prComments.updates[n-1].Body, "Preview environment: removed") {
		t.Errorf("teardown updates = %+v, want the last edit to say removed", fakeClient.prComments.updates)
	}
}

// TestGiteaFullNameFromURL covers giteaFullNameFromURL's own contract:
// only an https URL on the connected instance's host, shaped like a
// real owner/repo path, resolves.
func TestGiteaFullNameFromURL(t *testing.T) {
	const instanceURL = "https://git.example.com"
	tests := []struct {
		name     string
		repoURL  string
		wantName string
		wantOK   bool
	}{
		{"matching instance", "https://git.example.com/org/web.git", "org/web", true},
		{"no .git suffix", "https://git.example.com/org/web", "org/web", true},
		{"different host", "https://git.other.com/org/web.git", "", false},
		{"http scheme", "http://git.example.com/org/web.git", "", false},
		{"too many segments", "https://git.example.com/org/team/web.git", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotName, gotOK := giteaFullNameFromURL(tt.repoURL, instanceURL)
			if gotName != tt.wantName || gotOK != tt.wantOK {
				t.Errorf("giteaFullNameFromURL(%q) = (%q, %v), want (%q, %v)", tt.repoURL, gotName, gotOK, tt.wantName, tt.wantOK)
			}
		})
	}
}

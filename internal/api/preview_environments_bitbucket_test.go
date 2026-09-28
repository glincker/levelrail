package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/bitbucketapp"
)

// bitbucketPullRequestBody builds a pullrequest:* event payload
// carrying just the fields parseBitbucketPullRequestEvent
// (internal/webhook) reads, mirroring githubPullRequestBody's own
// shape. The PR id, head hash, and destination branch are fixed: every
// test in this file exercises the same PR against the same branch,
// matching setUpPreviewApp's own fixed-name convention for the app
// itself.
func bitbucketPullRequestBody() []byte {
	return bitbucketPullRequestBodyFrom("acme/web")
}

func bitbucketPullRequestBodyFrom(headRepo string) []byte {
	b, _ := json.Marshal(map[string]any{
		"pullrequest": map[string]any{
			"id": 42,
			"source": map[string]any{
				"branch":     map[string]any{"name": "feature-x"},
				"commit":     map[string]any{"hash": "sha1"},
				"repository": map[string]any{"full_name": headRepo},
			},
			"destination": map[string]any{
				"branch":     map[string]any{"name": "main"},
				"repository": map[string]any{"full_name": "acme/web"},
			},
		},
	})
	return b
}

// sendBitbucketPullRequestWebhook posts body to app "web"'s webhook
// route under Bitbucket's own auth scheme (X-Hub-Signature, HMAC-SHA256
// over "sha256=<hex>"), mirroring sendPullRequestWebhook's own shape for
// GitHub.
func sendBitbucketPullRequestWebhook(rt *Router, secret string, eventKey string, body []byte) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/github/web", bytes.NewReader(body))
	req.Header.Set("X-Event-Key", eventKey)
	req.Header.Set("X-Hub-Signature", sign([]byte(secret), body))
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	return rec
}

// setUpPreviewAppWithBitbucketNotifications mirrors
// setUpPreviewAppWithGitHubNotifications for Bitbucket: a connected,
// authorized Bitbucket OAuth consumer plus the same preview-app fixture.
func setUpPreviewAppWithBitbucketNotifications(t *testing.T, postPRComments bool) (rt *Router, secret string, builder *sequencedBuilder, fakeClient *fakeBitbucketAppClient) {
	t.Helper()
	gitSourceSecrets := newFakeGitSourceSecrets()
	bitbucketSecrets := authorizedBitbucketSecrets(t)
	fakeClient = &fakeBitbucketAppClient{}

	rt, db := newTestRouterWithBitbucketAndGitSource(t, bitbucketSecrets, fakeClient, gitSourceSecrets)
	cookie := loginTestSession(t, rt, db)
	seedBitbucketAppConnection(t, db)
	seedApp(t, db, "web")

	created := connectGitSource(t, rt, cookie, `{"repo_url":"https://bitbucket.org/org/web.git","branch":"main"}`)
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

// TestHandlePullRequestWebhook_Bitbucket_PostPRComments_PostsPendingThenSuccess
// proves a successful preview deploy posts an in-progress build status
// before the build, then a successful build status plus a pull request
// comment once it's live, mirroring the GitHub equivalent.
func TestHandlePullRequestWebhook_Bitbucket_PostPRComments_PostsPendingThenSuccess(t *testing.T) {
	rt, secret, _, fakeClient := setUpPreviewAppWithBitbucketNotifications(t, true)

	body := bitbucketPullRequestBody()
	rec := sendBitbucketPullRequestWebhook(rt, secret, "pullrequest:created", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	if len(fakeClient.statusCalls) != 2 {
		t.Fatalf("CreateCommitBuildStatus called %d times, want 2 (in-progress, then successful), got %+v", len(fakeClient.statusCalls), fakeClient.statusCalls)
	}
	if fakeClient.statusCalls[0].state != bitbucketapp.BuildStatusInProgress {
		t.Errorf("first status state = %q, want INPROGRESS", fakeClient.statusCalls[0].state)
	}
	if fakeClient.statusCalls[1].state != bitbucketapp.BuildStatusSuccessful {
		t.Errorf("second status state = %q, want SUCCESSFUL", fakeClient.statusCalls[1].state)
	}
	for _, call := range fakeClient.statusCalls {
		if call.fullName != "org/web" || call.commit != "sha1" {
			t.Errorf("status call = %+v, want full_name=org/web commit=sha1", call)
		}
	}

	if len(fakeClient.commentCalls) != 1 {
		t.Fatalf("CreatePullRequestComment called %d times, want 1", len(fakeClient.commentCalls))
	}
	if fakeClient.commentCalls[0].prID != 42 {
		t.Errorf("comment PR id = %d, want 42", fakeClient.commentCalls[0].prID)
	}
}

// TestHandlePullRequestWebhook_Bitbucket_PostPRComments_Disabled_NoBitbucketCalls
// proves the opt-in gate: with post_pr_comments left off, a preview
// still deploys, but no Bitbucket API call is ever made.
func TestHandlePullRequestWebhook_Bitbucket_PostPRComments_Disabled_NoBitbucketCalls(t *testing.T) {
	rt, secret, _, fakeClient := setUpPreviewAppWithBitbucketNotifications(t, false)

	body := bitbucketPullRequestBody()
	rec := sendBitbucketPullRequestWebhook(rt, secret, "pullrequest:created", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	if len(fakeClient.statusCalls) != 0 {
		t.Errorf("CreateCommitBuildStatus called %d times, want 0 when post_pr_comments is off", len(fakeClient.statusCalls))
	}
	if len(fakeClient.commentCalls) != 0 {
		t.Errorf("CreatePullRequestComment called %d times, want 0 when post_pr_comments is off", len(fakeClient.commentCalls))
	}
}

// TestHandlePullRequestWebhook_Bitbucket_PostPRComments_DeployFailed_PostsFailureStatusOnly
// proves a failed deploy posts a failed build status, and the single status comment ends in the failed state.
func TestHandlePullRequestWebhook_Bitbucket_PostPRComments_DeployFailed_PostsFailureStatusOnly(t *testing.T) {
	rt, secret, builder, fakeClient := setUpPreviewAppWithBitbucketNotifications(t, true)
	builder.errs = []error{context.DeadlineExceeded}

	body := bitbucketPullRequestBody()
	rec := sendBitbucketPullRequestWebhook(rt, secret, "pullrequest:created", body)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}

	if len(fakeClient.statusCalls) != 2 {
		t.Fatalf("CreateCommitBuildStatus called %d times, want 2 (in-progress, then failed), got %+v", len(fakeClient.statusCalls), fakeClient.statusCalls)
	}
	if fakeClient.statusCalls[1].state != bitbucketapp.BuildStatusFailed {
		t.Errorf("second status state = %q, want FAILED", fakeClient.statusCalls[1].state)
	}
	if len(fakeClient.commentCalls) != 1 || !strings.Contains(fakeClient.prComments.comments[0].Body, "Preview environment: failed") {
		t.Errorf("status comment = %+v (created %d), want one comment ending in the failed state", fakeClient.prComments.comments, len(fakeClient.commentCalls))
	}
}

// TestHandlePullRequestWebhook_Bitbucket_PostPRComments_Teardown_PostsComment
// proves a torn-down preview posts a pull request comment noting the
// teardown, mirroring the GitHub equivalent.
func TestHandlePullRequestWebhook_Bitbucket_PostPRComments_Teardown_PostsComment(t *testing.T) {
	rt, secret, _, fakeClient := setUpPreviewAppWithBitbucketNotifications(t, true)

	opened := bitbucketPullRequestBody()
	if rec := sendBitbucketPullRequestWebhook(rt, secret, "pullrequest:created", opened); rec.Code != http.StatusOK {
		t.Fatalf("opened: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	fakeClient.commentCalls = nil

	closed := bitbucketPullRequestBody()
	rec := sendBitbucketPullRequestWebhook(rt, secret, "pullrequest:fulfilled", closed)
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

// TestBitbucketFullNameFromURL covers bitbucketFullNameFromURL's own
// contract: only an https URL on bitbucket.org, shaped like a real
// "workspace/repo_slug" path, resolves.
func TestBitbucketFullNameFromURL(t *testing.T) {
	tests := []struct {
		name     string
		repoURL  string
		wantName string
		wantOK   bool
	}{
		{"bitbucket.org", "https://bitbucket.org/org/web.git", "org/web", true},
		{"no .git suffix", "https://bitbucket.org/org/web", "org/web", true},
		{"case-insensitive host", "https://Bitbucket.org/org/web.git", "org/web", true},
		{"different host", "https://gitlab.example.com/org/web.git", "", false},
		{"http scheme", "http://bitbucket.org/org/web.git", "", false},
		{"too many segments", "https://bitbucket.org/org/team/web.git", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotName, gotOK := bitbucketFullNameFromURL(tt.repoURL)
			if gotName != tt.wantName || gotOK != tt.wantOK {
				t.Errorf("bitbucketFullNameFromURL(%q) = (%q, %v), want (%q, %v)", tt.repoURL, gotName, gotOK, tt.wantName, tt.wantOK)
			}
		})
	}
}

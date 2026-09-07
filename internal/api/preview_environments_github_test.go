package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/githubapp"
)

// setUpPreviewAppWithGitHubNotifications mirrors setUpPreviewApp
// (preview_environments_test.go), plus a connected, installed GitHub
// App: the fixture every test in this file needs to exercise
// notifyPreviewPending/Success/Failure/TornDown against a real (fake)
// GitHubAppClient instead of the nil-githubAppSecrets no-op path
// setUpPreviewApp's own callers exercise.
func setUpPreviewAppWithGitHubNotifications(t *testing.T, postPRComments bool) (rt *Router, secret string, builder *sequencedBuilder, fakeClient *fakeGitHubAppClient) {
	t.Helper()
	gitSourceSecrets := newFakeGitSourceSecrets()
	githubSecrets := newFakeGitHubAppSecrets()
	fakeClient = &fakeGitHubAppClient{mintToken: githubapp.InstallationToken{Token: "ghs_installtoken"}}

	rt, db := newTestRouterWithGitHubAndGitSource(t, githubSecrets, fakeClient, gitSourceSecrets)
	cookie := loginTestSession(t, rt, db)
	seedInstalledGitHubApp(t, db, githubSecrets)
	seedApp(t, db, "web")

	created := connectGitSource(t, rt, cookie, `{"repo_url":"https://github.com/org/web.git","branch":"main"}`)
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

// TestHandlePullRequestWebhook_PostPRComments_PostsPendingThenSuccess
// proves a successful preview deploy posts a pending commit status
// before the build, then a success commit status plus a PR comment once
// it's live, all against the PR's own owner/repo/head SHA.
func TestHandlePullRequestWebhook_PostPRComments_PostsPendingThenSuccess(t *testing.T) {
	rt, secret, _, fakeClient := setUpPreviewAppWithGitHubNotifications(t, true)

	body := githubPullRequestBody("opened", 42, "sha1", "main")
	rec := sendPullRequestWebhook(rt, secret, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	if len(fakeClient.statusCalls) != 2 {
		t.Fatalf("CreateCommitStatus called %d times, want 2 (pending, then success), got %+v", len(fakeClient.statusCalls), fakeClient.statusCalls)
	}
	if fakeClient.statusCalls[0].state != githubapp.CommitStatusPending {
		t.Errorf("first status state = %q, want pending", fakeClient.statusCalls[0].state)
	}
	if fakeClient.statusCalls[1].state != githubapp.CommitStatusSuccess {
		t.Errorf("second status state = %q, want success", fakeClient.statusCalls[1].state)
	}
	for _, call := range fakeClient.statusCalls {
		if call.owner != "org" || call.repo != "web" || call.sha != "sha1" {
			t.Errorf("status call = %+v, want owner=org repo=web sha=sha1", call)
		}
	}

	if len(fakeClient.commentCalls) != 1 {
		t.Fatalf("CreateIssueComment called %d times, want 1", len(fakeClient.commentCalls))
	}
	if fakeClient.commentCalls[0].number != 42 {
		t.Errorf("comment PR number = %d, want 42", fakeClient.commentCalls[0].number)
	}
}

// TestHandlePullRequestWebhook_PostPRComments_Disabled_NoGitHubCalls
// proves the opt-in gate: with post_pr_comments left off (the default),
// a preview still deploys, but no GitHub API call is ever made.
func TestHandlePullRequestWebhook_PostPRComments_Disabled_NoGitHubCalls(t *testing.T) {
	rt, secret, _, fakeClient := setUpPreviewAppWithGitHubNotifications(t, false)

	body := githubPullRequestBody("opened", 42, "sha1", "main")
	rec := sendPullRequestWebhook(rt, secret, body)
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

// TestHandlePullRequestWebhook_PostPRComments_DeployFailed_PostsFailureStatusOnly
// proves a failed deploy posts a failure commit status, and deliberately
// no PR comment (notifyPreviewFailure's own doc comment): only pending
// (before the attempt) and failure (after) commit statuses.
func TestHandlePullRequestWebhook_PostPRComments_DeployFailed_PostsFailureStatusOnly(t *testing.T) {
	rt, secret, builder, fakeClient := setUpPreviewAppWithGitHubNotifications(t, true)
	builder.errs = []error{context.DeadlineExceeded}

	body := githubPullRequestBody("opened", 42, "sha1", "main")
	rec := sendPullRequestWebhook(rt, secret, body)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}

	if len(fakeClient.statusCalls) != 2 {
		t.Fatalf("CreateCommitStatus called %d times, want 2 (pending, then failure), got %+v", len(fakeClient.statusCalls), fakeClient.statusCalls)
	}
	if fakeClient.statusCalls[1].state != githubapp.CommitStatusFailure {
		t.Errorf("second status state = %q, want failure", fakeClient.statusCalls[1].state)
	}
	if len(fakeClient.commentCalls) != 0 {
		t.Errorf("CreateIssueComment called %d times, want 0 on a failed deploy", len(fakeClient.commentCalls))
	}
}

// TestHandlePullRequestWebhook_PostPRComments_Teardown_PostsComment proves
// a torn-down preview posts a PR comment noting the teardown, using the
// same opt-in gate as the deploy-time notifications.
func TestHandlePullRequestWebhook_PostPRComments_Teardown_PostsComment(t *testing.T) {
	rt, secret, _, fakeClient := setUpPreviewAppWithGitHubNotifications(t, true)

	opened := githubPullRequestBody("opened", 42, "sha1", "main")
	if rec := sendPullRequestWebhook(rt, secret, opened); rec.Code != http.StatusOK {
		t.Fatalf("opened: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	fakeClient.commentCalls = nil

	closed := githubPullRequestBody("closed", 42, "sha1", "main")
	rec := sendPullRequestWebhook(rt, secret, closed)
	if rec.Code != http.StatusOK {
		t.Fatalf("closed: status = %d, body = %s", rec.Code, rec.Body.String())
	}

	if len(fakeClient.commentCalls) != 1 {
		t.Fatalf("CreateIssueComment called %d times, want 1 for the teardown notice", len(fakeClient.commentCalls))
	}
	if fakeClient.commentCalls[0].number != 42 {
		t.Errorf("teardown comment PR number = %d, want 42", fakeClient.commentCalls[0].number)
	}
}

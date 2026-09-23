package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/gitlabapp"
)

// gitlabPullRequestBody builds a Merge Request Hook payload carrying
// just the object_attributes fields parseGitLabPullRequestEvent
// (internal/webhook) reads, mirroring githubPullRequestBody's own shape
// for the GitHub equivalent. The IID, head SHA, and target branch are
// fixed: every test in this file exercises the same MR against the
// same branch, matching setUpPreviewApp's own fixed-name convention for
// the app itself.
func gitlabPullRequestBody(action string) []byte {
	b, _ := json.Marshal(map[string]any{
		"object_attributes": map[string]any{
			"iid":           42,
			"action":        action,
			"source_branch": "feature-x",
			"target_branch": "main",
			"last_commit":   map[string]any{"id": "sha1"},
		},
	})
	return b
}

// sendGitLabPullRequestWebhook posts body to app "web"'s webhook route
// under GitLab's own auth scheme (X-Gitlab-Token, a plain secret
// comparison, not a signature), mirroring sendPullRequestWebhook's own
// shape for GitHub.
func sendGitLabPullRequestWebhook(rt *Router, secret string, body []byte) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/github/web", bytes.NewReader(body))
	req.Header.Set("X-Gitlab-Event", "Merge Request Hook")
	req.Header.Set("X-Gitlab-Token", secret)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	return rec
}

// setUpPreviewAppWithGitLabNotifications mirrors
// setUpPreviewAppWithGitHubNotifications for GitLab: a connected,
// authorized GitLab OAuth application plus the same preview-app fixture.
func setUpPreviewAppWithGitLabNotifications(t *testing.T, postPRComments bool) (rt *Router, secret string, builder *sequencedBuilder, fakeClient *fakeGitLabAppClient) {
	t.Helper()
	gitSourceSecrets := newFakeGitSourceSecrets()
	gitlabSecrets := authorizedGitLabSecrets(t)
	fakeClient = &fakeGitLabAppClient{}

	rt, db := newTestRouterWithGitLabAndGitSource(t, gitlabSecrets, fakeClient, gitSourceSecrets)
	cookie := loginTestSession(t, rt, db)
	seedGitLabAppConnection(t, db)
	seedApp(t, db, "web")

	created := connectGitSource(t, rt, cookie, `{"repo_url":"https://gitlab.example.com/org/web.git","branch":"main"}`)
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

// TestHandlePullRequestWebhook_GitLab_PostPRComments_PostsPendingThenSuccess
// proves a successful preview deploy posts a pending commit status
// before the build, then a success commit status plus a merge request
// note once it's live, mirroring the GitHub equivalent
// (TestHandlePullRequestWebhook_PostPRComments_PostsPendingThenSuccess).
func TestHandlePullRequestWebhook_GitLab_PostPRComments_PostsPendingThenSuccess(t *testing.T) {
	rt, secret, _, fakeClient := setUpPreviewAppWithGitLabNotifications(t, true)

	body := gitlabPullRequestBody("open")
	rec := sendGitLabPullRequestWebhook(rt, secret, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	if len(fakeClient.statusCalls) != 2 {
		t.Fatalf("CreateCommitStatus called %d times, want 2 (pending, then success), got %+v", len(fakeClient.statusCalls), fakeClient.statusCalls)
	}
	if fakeClient.statusCalls[0].state != gitlabapp.CommitStatePending {
		t.Errorf("first status state = %q, want pending", fakeClient.statusCalls[0].state)
	}
	if fakeClient.statusCalls[1].state != gitlabapp.CommitStateSuccess {
		t.Errorf("second status state = %q, want success", fakeClient.statusCalls[1].state)
	}
	for _, call := range fakeClient.statusCalls {
		if call.projectPath != "org/web" || call.sha != "sha1" {
			t.Errorf("status call = %+v, want project_path=org/web sha=sha1", call)
		}
	}

	if len(fakeClient.noteCalls) != 1 {
		t.Fatalf("CreateMergeRequestNote called %d times, want 1", len(fakeClient.noteCalls))
	}
	if fakeClient.noteCalls[0].mrIID != 42 {
		t.Errorf("note MR IID = %d, want 42", fakeClient.noteCalls[0].mrIID)
	}
}

// TestHandlePullRequestWebhook_GitLab_PostPRComments_Disabled_NoGitLabCalls
// proves the opt-in gate: with post_pr_comments left off, a preview
// still deploys, but no GitLab API call is ever made.
func TestHandlePullRequestWebhook_GitLab_PostPRComments_Disabled_NoGitLabCalls(t *testing.T) {
	rt, secret, _, fakeClient := setUpPreviewAppWithGitLabNotifications(t, false)

	body := gitlabPullRequestBody("open")
	rec := sendGitLabPullRequestWebhook(rt, secret, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	if len(fakeClient.statusCalls) != 0 {
		t.Errorf("CreateCommitStatus called %d times, want 0 when post_pr_comments is off", len(fakeClient.statusCalls))
	}
	if len(fakeClient.noteCalls) != 0 {
		t.Errorf("CreateMergeRequestNote called %d times, want 0 when post_pr_comments is off", len(fakeClient.noteCalls))
	}
}

// TestHandlePullRequestWebhook_GitLab_PostPRComments_DeployFailed_PostsFailureStatusOnly
// proves a failed deploy posts a failure commit status, and deliberately
// no merge request note, mirroring the GitHub equivalent.
func TestHandlePullRequestWebhook_GitLab_PostPRComments_DeployFailed_PostsFailureStatusOnly(t *testing.T) {
	rt, secret, builder, fakeClient := setUpPreviewAppWithGitLabNotifications(t, true)
	builder.errs = []error{context.DeadlineExceeded}

	body := gitlabPullRequestBody("open")
	rec := sendGitLabPullRequestWebhook(rt, secret, body)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}

	if len(fakeClient.statusCalls) != 2 {
		t.Fatalf("CreateCommitStatus called %d times, want 2 (pending, then failure), got %+v", len(fakeClient.statusCalls), fakeClient.statusCalls)
	}
	if fakeClient.statusCalls[1].state != gitlabapp.CommitStateFailed {
		t.Errorf("second status state = %q, want failed", fakeClient.statusCalls[1].state)
	}
	if len(fakeClient.noteCalls) != 0 {
		t.Errorf("CreateMergeRequestNote called %d times, want 0 on a failed deploy", len(fakeClient.noteCalls))
	}
}

// TestHandlePullRequestWebhook_GitLab_PostPRComments_Teardown_PostsNote
// proves a torn-down preview posts a merge request note noting the
// teardown, mirroring the GitHub equivalent.
func TestHandlePullRequestWebhook_GitLab_PostPRComments_Teardown_PostsNote(t *testing.T) {
	rt, secret, _, fakeClient := setUpPreviewAppWithGitLabNotifications(t, true)

	opened := gitlabPullRequestBody("open")
	if rec := sendGitLabPullRequestWebhook(rt, secret, opened); rec.Code != http.StatusOK {
		t.Fatalf("opened: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	fakeClient.noteCalls = nil

	closed := gitlabPullRequestBody("close")
	rec := sendGitLabPullRequestWebhook(rt, secret, closed)
	if rec.Code != http.StatusOK {
		t.Fatalf("closed: status = %d, body = %s", rec.Code, rec.Body.String())
	}

	if len(fakeClient.noteCalls) != 1 {
		t.Fatalf("CreateMergeRequestNote called %d times, want 1 for the teardown notice", len(fakeClient.noteCalls))
	}
	if fakeClient.noteCalls[0].mrIID != 42 {
		t.Errorf("teardown note MR IID = %d, want 42", fakeClient.noteCalls[0].mrIID)
	}
}

// TestGitlabProjectPathFromURL covers gitlabProjectPathFromURL's own
// contract: only an https URL on the connected instance's host, shaped
// like a real project path, resolves.
func TestGitlabProjectPathFromURL(t *testing.T) {
	const instanceURL = "https://gitlab.example.com"
	tests := []struct {
		name     string
		repoURL  string
		wantPath string
		wantOK   bool
	}{
		{"matching instance", "https://gitlab.example.com/org/web.git", "org/web", true},
		{"subgroup path", "https://gitlab.example.com/org/team/web.git", "org/team/web", true},
		{"no .git suffix", "https://gitlab.example.com/org/web", "org/web", true},
		{"different host", "https://gitlab.other.com/org/web.git", "", false},
		{"http scheme", "http://gitlab.example.com/org/web.git", "", false},
		{"empty path", "https://gitlab.example.com/", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPath, gotOK := gitlabProjectPathFromURL(tt.repoURL, instanceURL)
			if gotPath != tt.wantPath || gotOK != tt.wantOK {
				t.Errorf("gitlabProjectPathFromURL(%q) = (%q, %v), want (%q, %v)", tt.repoURL, gotPath, gotOK, tt.wantPath, tt.wantOK)
			}
		})
	}
}

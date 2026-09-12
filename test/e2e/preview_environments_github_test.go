// TestPreviewEnvironmentGitHub_Live_OpenSynchronizeAndCloseLifecycle is
// this package's live proof of GitHub's own pull_request webhook,
// alongside TestPreviewEnvironmentGitLab_Live_OpenSynchronizeAndCloseLifecycle
// and TestPreviewEnvironmentBitbucket_Live_CreateUpdateAndTeardownLifecycle
// in preview_environments_gitlab_bitbucket_test.go for GitLab and
// Bitbucket. Reuses that file's fixture, helpers, and router unchanged;
// only the payload shape, event header, and signature scheme are
// GitHub-specific.
package e2e

import (
	"fmt"
	"testing"

	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	e2ePreviewGitHubAdminUsername = "e2e-preview-github-admin"
	e2ePreviewGitHubAdminPassword = "e2e-preview-github-correct-horse" //nolint:gosec // test fixture credential, not a real secret
)

func TestPreviewEnvironmentGitHub_Live_OpenSynchronizeAndCloseLifecycle(t *testing.T) {
	const appName = "levelrail-test-e2e-preview-github"
	const prNumber = 314
	f := newLivePreviewFixture(t, appName, prNumber, e2ePreviewGitHubAdminUsername, e2ePreviewGitHubAdminPassword, "preview github opened", "preview github synchronized")
	githubHeaders := func(payload []byte) map[string]string {
		return map[string]string{"X-GitHub-Event": "pull_request", "X-Hub-Signature-256": "sha256=" + signHMAC(f.webhookSecret, payload)}
	}

	// Step 1: opened deploys a fresh preview from commitA.
	openedPayload := githubPullRequestWebhookPayload("opened", prNumber, "feature-preview", "main", f.commitA)
	fireLifecycleStep(t, f, appName, previewLifecycleStep{
		name:           "github opened",
		payload:        openedPayload,
		headers:        githubHeaders(openedPayload),
		wantBodyPhrase: "preview deployed",
	})
	reconcileAndAssertPreviewContent(f.ctx, t, f.runtime, f.previewCtrl, f.previewName, f.tagA, "preview github opened")
	preview := getPreviewAndAssertHeadSHA(t, f, appName, prNumber, "opened", f.commitA)
	if preview.Status != store.PreviewStatusActive {
		t.Errorf("preview Status = %q, want %q", preview.Status, store.PreviewStatusActive)
	}
	if preview.PreviewAppID != f.previewName {
		t.Errorf("preview PreviewAppID = %q, want %q", preview.PreviewAppID, f.previewName)
	}

	// Step 2: synchronize (a new commit pushed to the PR) redeploys the
	// same preview app in place, replacing commitA's container with
	// commitB's.
	syncPayload := githubPullRequestWebhookPayload("synchronize", prNumber, "feature-preview", "main", f.commitB)
	fireLifecycleStep(t, f, appName, previewLifecycleStep{
		name:    "github synchronize",
		payload: syncPayload,
		headers: githubHeaders(syncPayload),
	})
	reconcileAndAssertPreviewContent(f.ctx, t, f.runtime, f.previewCtrl, f.previewName, f.tagB, "preview github synchronized")
	assertContainerAbsent(f.ctx, t, f.runtime, application.ContainerName(f.previewName, f.tagA, ""), "github preview commitA")
	getPreviewAndAssertHeadSHA(t, f, appName, prNumber, "synchronize", f.commitB)

	// Step 3: closed tears the preview down: desired state, the
	// preview_environments row, and the running container are all gone.
	closedPayload := githubPullRequestWebhookPayload("closed", prNumber, "feature-preview", "main", f.commitB)
	fireLifecycleStep(t, f, appName, previewLifecycleStep{
		name:           "github closed",
		payload:        closedPayload,
		headers:        githubHeaders(closedPayload),
		wantBodyPhrase: "torn down",
	})
	assertPreviewTornDown(t, f, appName, prNumber)
}

// githubPullRequestWebhookPayload builds a pull_request event payload
// shaped like GitHub's own documented example
// (https://docs.github.com/en/webhooks/webhook-events-and-payloads#pull_request),
// the same shape internal/webhook/pull_request_test.go's own GitHub
// fixture already verifies parseGitHubPullRequestEvent against,
// reproduced here for the same reason gitlabPreviewWebhookPayload and
// bitbucketPreviewWebhookPayload in the sibling file are.
func githubPullRequestWebhookPayload(action string, number int, headRef, baseRef, headSHA string) []byte {
	return []byte(fmt.Sprintf(`{
		"action": %q,
		"number": %d,
		"pull_request": {
			"head": {"ref": %q, "sha": %q},
			"base": {"ref": %q}
		}
	}`, action, number, headRef, headSHA, baseRef))
}

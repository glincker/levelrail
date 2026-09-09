// TestPreviewEnvironmentGitLab_Live_OpenSynchronizeAndCloseLifecycle and
// TestPreviewEnvironmentBitbucket_Live_CreateUpdateAndTeardownLifecycle
// are this package's live proof for docs/roadmap.md's own preview
// environments bullet: GitLab's Merge Request Hook and Bitbucket's
// pullrequest:* webhook payloads are parsed (internal/webhook/pull_request.go)
// and unit-tested there, and internal/api/preview_environments*_test.go
// already prove handlePullRequestWebhookEvent's own logic against fake
// stores and a fake Builder. Neither proves the operator-facing claim:
// that a real, provider-signed webhook delivery against a real
// *api.Router, with a real git source and a real BuildKit build, actually
// produces a running container, redeploys it on a new commit, and tears
// it down on close. This closes that gap for the two providers
// TestWebhook_Live_PushToRunningContainer (webhook_test.go) doesn't
// cover. Unlike that test, PUT /api/v1/apps/{name}/git-source requires
// repo_url to be http(s) (requireHTTPOrHTTPSScheme), so a plain
// filesystem path (webhook_test.go's own local-filesystem-as-remote
// trick) doesn't clear that guard here; newPreviewGitHTTPServer instead
// serves a real bare repo over git's smart-HTTP protocol via
// git-http-backend, the same throwaway-server pattern go-git's own
// plumbing/transport/http test suite uses to test its HTTP client.
package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cgi" //nolint:gosec // test-only, loopback-only git-http-backend server (newPreviewGitHTTPServer), never exposed to real traffic
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/image"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/brand"
	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	e2ePreviewGitLabAdminUsername    = "e2e-preview-gitlab-admin"
	e2ePreviewGitLabAdminPassword    = "e2e-preview-gitlab-correct-horse" //nolint:gosec // test fixture credential, not a real secret
	e2ePreviewBitbucketAdminUsername = "e2e-preview-bitbucket-admin"
	e2ePreviewBitbucketAdminPassword = "e2e-preview-bitbucket-correct-horse" //nolint:gosec // test fixture credential, not a real secret
)

// livePreviewFixture is the setup both provider lifecycle tests below
// share: a running control plane, a real git repo with two commits
// served over HTTP, a connected+preview-enabled git source, and the
// preview app's own reconciler. Only the per-step webhook payload shape
// and headers genuinely differ between GitLab and Bitbucket, which each
// test still builds and fires itself.
type livePreviewFixture struct {
	ctx           context.Context
	svcStore      *store.DB
	runtime       docker.Runtime
	ts            *httptest.Server
	previewCtrl   *application.Controller
	previewName   string
	tagA, tagB    string
	commitA       string
	commitB       string
	webhookSecret string
}

func newLivePreviewFixture(t *testing.T, appName string, prNumber int, adminUsername, adminPassword, bodyA, bodyB string) *livePreviewFixture {
	t.Helper()
	env := newLiveBuildEnv(t)
	dockerCli, buildClient, runtime := env.DockerCli, env.BuildClient, env.Runtime

	previewName := fmt.Sprintf("%s-pr-%d", appName, prNumber)
	cleanupContainers(context.Background(), t, runtime, previewName)
	t.Cleanup(func() { cleanupContainers(context.Background(), t, runtime, previewName) })

	svcStore := openLiveStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	t.Cleanup(cancel)

	repoDir, commitA, commitB := newPreviewGitRepo(t, bodyA, bodyB)
	repoHTTPURL := newPreviewGitHTTPServer(t, repoDir)

	tagA, tagB := previewName+":"+commitA, previewName+":"+commitB
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = dockerCli.ImageRemove(cleanupCtx, tagA, image.RemoveOptions{Force: true})
		_, _ = dockerCli.ImageRemove(cleanupCtx, tagB, image.RemoveOptions{Force: true})
	})

	if err := svcStore.SaveDesiredService(ctx, store.DesiredService{
		Name: appName, Image: appName + ":source", Port: 8080,
		Health: &store.ServiceHealth{Readiness: &store.ServiceProbe{Path: "/"}},
	}); err != nil {
		t.Fatalf("SaveDesiredService(%q) error = %v", appName, err)
	}

	ts := newPreviewE2ERouter(t, svcStore, buildClient)
	if err := api.BootstrapAdmin(ctx, svcStore, adminUsername, adminPassword); err != nil {
		t.Fatalf("BootstrapAdmin() error = %v", err)
	}
	client := loginE2EClient(t, ts.URL, adminUsername, adminPassword)
	webhookSecret := connectPreviewGitSource(t, client, ts.URL, appName, repoHTTPURL)

	return &livePreviewFixture{
		ctx: ctx, svcStore: svcStore, runtime: runtime, ts: ts,
		previewCtrl:   application.New(previewName, svcStore, runtime),
		previewName:   previewName,
		tagA:          tagA,
		tagB:          tagB,
		commitA:       commitA,
		commitB:       commitB,
		webhookSecret: webhookSecret,
	}
}

// previewLifecycleStep is one webhook delivery in a preview's open (or
// create), update, and close (or fulfilled) lifecycle: only its payload,
// headers, and how it's described in a failure message differ between
// GitLab and Bitbucket.
type previewLifecycleStep struct {
	name           string
	payload        []byte
	headers        map[string]string
	wantBodyPhrase string
}

// fireLifecycleStep delivers step's webhook and asserts the response
// shape both provider lifecycle tests below require: a 200 and,
// optionally, a substring in the response body.
func fireLifecycleStep(t *testing.T, f *livePreviewFixture, appName string, step previewLifecycleStep) string {
	t.Helper()
	status, body := postWebhookPayload(t, f.ts.URL, appName, step.payload, step.headers)
	if status != http.StatusOK {
		t.Fatalf("%s webhook: status = %d, want %d, body = %s", step.name, status, http.StatusOK, body)
	}
	if step.wantBodyPhrase != "" && !strings.Contains(body, step.wantBodyPhrase) {
		t.Errorf("%s webhook body = %q, want it to mention %q", step.name, body, step.wantBodyPhrase)
	}
	return body
}

// getPreviewAndAssertHeadSHA loads appName's preview_environments row
// for prNumber and asserts its HeadSHA, the check both tests repeat
// after their open/create and update steps.
func getPreviewAndAssertHeadSHA(t *testing.T, f *livePreviewFixture, appName string, prNumber int, when, wantSHA string) *store.PreviewEnvironment {
	t.Helper()
	preview, err := f.svcStore.GetPreviewEnvironmentByAppAndPR(f.ctx, appName, prNumber)
	if err != nil {
		t.Fatalf("GetPreviewEnvironmentByAppAndPR() after %s error = %v", when, err)
	}
	if preview.HeadSHA != wantSHA {
		t.Errorf("preview HeadSHA after %s = %q, want %q", when, preview.HeadSHA, wantSHA)
	}
	return preview
}

// assertPreviewTornDown asserts the close/fulfilled step's own
// contract: the preview_environments row and the preview app's desired
// state are both gone. Neither test asserts the running container is
// stopped too; see the doc comment at each test's own close/fulfilled
// step for why.
func assertPreviewTornDown(t *testing.T, f *livePreviewFixture, appName string, prNumber int) {
	t.Helper()
	if _, err := f.svcStore.GetPreviewEnvironmentByAppAndPR(f.ctx, appName, prNumber); !errors.Is(err, store.ErrPreviewEnvironmentNotFound) {
		t.Fatalf("GetPreviewEnvironmentByAppAndPR() after teardown error = %v, want ErrPreviewEnvironmentNotFound", err)
	}
	if _, err := f.svcStore.GetDesiredService(f.ctx, f.previewName); !errors.Is(err, store.ErrServiceNotFound) {
		t.Fatalf("GetDesiredService(%q) after teardown error = %v, want ErrServiceNotFound", f.previewName, err)
	}
}

func TestPreviewEnvironmentGitLab_Live_OpenSynchronizeAndCloseLifecycle(t *testing.T) {
	const appName = "levelrail-test-e2e-preview-gitlab"
	const prNumber = 501
	f := newLivePreviewFixture(t, appName, prNumber, e2ePreviewGitLabAdminUsername, e2ePreviewGitLabAdminPassword, "preview gitlab open", "preview gitlab updated")
	gitlabHeaders := map[string]string{"X-Gitlab-Event": "Merge Request Hook", "X-Gitlab-Token": f.webhookSecret}

	// Step 1: opening the MR deploys a fresh preview from commitA.
	fireLifecycleStep(t, f, appName, previewLifecycleStep{
		name:           "gitlab open",
		payload:        gitlabPreviewWebhookPayload("open", prNumber, "feature-preview", "main", f.commitA),
		headers:        gitlabHeaders,
		wantBodyPhrase: "preview deployed",
	})
	reconcileAndAssertPreviewContent(f.ctx, t, f.runtime, f.previewCtrl, f.previewName, f.tagA, "preview gitlab open")
	preview := getPreviewAndAssertHeadSHA(t, f, appName, prNumber, "open", f.commitA)
	if preview.Status != store.PreviewStatusActive {
		t.Errorf("preview Status = %q, want %q", preview.Status, store.PreviewStatusActive)
	}
	if preview.PreviewAppID != f.previewName {
		t.Errorf("preview PreviewAppID = %q, want %q", preview.PreviewAppID, f.previewName)
	}

	// Step 2: a new commit on the MR (GitLab's "update" action, this
	// package's synchronize equivalent) redeploys the same preview app
	// in place, replacing commitA's container with commitB's.
	fireLifecycleStep(t, f, appName, previewLifecycleStep{
		name:    "gitlab update",
		payload: gitlabPreviewWebhookPayload("update", prNumber, "feature-preview", "main", f.commitB),
		headers: gitlabHeaders,
	})
	reconcileAndAssertPreviewContent(f.ctx, t, f.runtime, f.previewCtrl, f.previewName, f.tagB, "preview gitlab updated")
	assertContainerAbsent(f.ctx, t, f.runtime, application.ContainerName(f.previewName, f.tagA, ""), "gitlab preview commitA")
	getPreviewAndAssertHeadSHA(t, f, appName, prNumber, "update", f.commitB)

	// Step 3: closing the MR tears the preview down: its desired state
	// and preview_environments row are both deleted. Stopping the
	// now-orphaned container itself is a documented pre-existing gap
	// (see teardownPreviewApp's own doc comment in
	// internal/api/preview_environments.go), so this deliberately does
	// not assert the container is gone.
	fireLifecycleStep(t, f, appName, previewLifecycleStep{
		name:           "gitlab close",
		payload:        gitlabPreviewWebhookPayload("close", prNumber, "feature-preview", "main", f.commitB),
		headers:        gitlabHeaders,
		wantBodyPhrase: "torn down",
	})
	assertPreviewTornDown(t, f, appName, prNumber)
}

func TestPreviewEnvironmentBitbucket_Live_CreateUpdateAndTeardownLifecycle(t *testing.T) {
	const appName = "levelrail-test-e2e-preview-bitbucket"
	const prNumber = 777
	f := newLivePreviewFixture(t, appName, prNumber, e2ePreviewBitbucketAdminUsername, e2ePreviewBitbucketAdminPassword, "preview bitbucket created", "preview bitbucket updated")
	bitbucketHeaders := func(eventKey string, payload []byte) map[string]string {
		return map[string]string{"X-Event-Key": eventKey, "X-Hub-Signature": "sha256=" + signHMAC(f.webhookSecret, payload)}
	}

	// Step 1: pullrequest:created deploys a fresh preview from commitA.
	createdPayload := bitbucketPreviewWebhookPayload(prNumber, "feature-preview", f.commitA, "main")
	fireLifecycleStep(t, f, appName, previewLifecycleStep{
		name:           "bitbucket created",
		payload:        createdPayload,
		headers:        bitbucketHeaders("pullrequest:created", createdPayload),
		wantBodyPhrase: "preview deployed",
	})
	reconcileAndAssertPreviewContent(f.ctx, t, f.runtime, f.previewCtrl, f.previewName, f.tagA, "preview bitbucket created")
	preview := getPreviewAndAssertHeadSHA(t, f, appName, prNumber, "create", f.commitA)
	if preview.Status != store.PreviewStatusActive {
		t.Errorf("preview Status = %q, want %q", preview.Status, store.PreviewStatusActive)
	}

	// Step 2: pullrequest:updated (fired on new commits, this package's
	// synchronize equivalent) redeploys the same preview app from commitB.
	updatedPayload := bitbucketPreviewWebhookPayload(prNumber, "feature-preview", f.commitB, "main")
	fireLifecycleStep(t, f, appName, previewLifecycleStep{
		name:    "bitbucket updated",
		payload: updatedPayload,
		headers: bitbucketHeaders("pullrequest:updated", updatedPayload),
	})
	reconcileAndAssertPreviewContent(f.ctx, t, f.runtime, f.previewCtrl, f.previewName, f.tagB, "preview bitbucket updated")
	assertContainerAbsent(f.ctx, t, f.runtime, application.ContainerName(f.previewName, f.tagA, ""), "bitbucket preview commitA")
	getPreviewAndAssertHeadSHA(t, f, appName, prNumber, "update", f.commitB)

	// Step 3: pullrequest:fulfilled (merged) tears the preview down, the
	// same desired-state/preview-record deletion GitLab's close covers
	// above; see that test's own comment for why the container itself is
	// deliberately not asserted absent here.
	fulfilledPayload := bitbucketPreviewWebhookPayload(prNumber, "feature-preview", f.commitB, "main")
	fireLifecycleStep(t, f, appName, previewLifecycleStep{
		name:           "bitbucket fulfilled",
		payload:        fulfilledPayload,
		headers:        bitbucketHeaders("pullrequest:fulfilled", fulfilledPayload),
		wantBodyPhrase: "torn down",
	})
	assertPreviewTornDown(t, f, appName, prNumber)
}

// newPreviewE2ERouter builds a real *api.Router wired with a real
// secrets.Manager (git source webhook secrets/tokens) and a real
// *deploy.Pipeline (git-source webhook builds), the two options
// TestProtectedEnvironment_Live_DeployBlockedThenAllowed and
// TestMasterKeyRotation_Live_SecretStillResolvesAfterRotation already
// establish separately for their own narrower needs, combined here since
// a preview deploy needs both at once, and starts a real HTTP server for
// it.
func newPreviewE2ERouter(t *testing.T, svcStore *store.DB, buildClient *build.Client) *httptest.Server {
	t.Helper()
	masterKey, err := secrets.GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() error = %v", err)
	}
	secretsManager := secrets.NewManager(svcStore, masterKey)
	pipeline := deploy.New(buildClient, svcStore)

	logger := discardTestLogger()
	b := &brand.Brand{Name: "E2E Test Platform", BinaryName: "e2e-test-platform"}
	router := api.NewRouter(logger, b, svcStore, api.WithGitSourceSecrets(secretsManager), api.WithBuilder(pipeline))
	return newE2ETestServer(t, router)
}

// connectPreviewGitSource connects appName's git source at repoDir
// (branch "main") and opts it into preview environments, the real
// PUT .../git-source then PUT .../preview-settings flow an operator
// would use, returning the generated webhook secret every subsequent
// webhook call in this test needs to authenticate with.
func connectPreviewGitSource(t *testing.T, client *http.Client, baseURL, appName, repoDir string) string {
	t.Helper()
	status, body := requestJSON(t, client, http.MethodPut, baseURL+"/api/v1/apps/"+appName+"/git-source",
		`{"repo_url":"`+repoDir+`","branch":"main","build_type":"dockerfile"}`)
	if status != http.StatusCreated {
		t.Fatalf("connect git source: status = %d, want %d, body = %s", status, http.StatusCreated, body)
	}
	var resp struct {
		WebhookSecret string `json:"webhook_secret"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decode git source response %s: %v", body, err)
	}
	if resp.WebhookSecret == "" {
		t.Fatalf("git source response has no webhook_secret: %s", body)
	}

	status, body = requestJSON(t, client, http.MethodPut, baseURL+"/api/v1/apps/"+appName+"/preview-settings", `{"enabled":true}`)
	if status != http.StatusOK {
		t.Fatalf("enable previews: status = %d, want %d, body = %s", status, http.StatusOK, body)
	}
	return resp.WebhookSecret
}

// newPreviewGitHTTPServer serves workDir's history as a real bare repo
// over git's smart-HTTP protocol, so PUT .../git-source's own
// requireHTTPOrHTTPSScheme guard sees a genuine http:// repo_url, not a
// filesystem path. git-http-backend (via net/http/cgi, GIT_HTTP_EXPORT_ALL
// so no per-repo export marker is needed) is the exact throwaway-server
// pattern go-git's own plumbing/transport/http test suite
// (BaseSuite.SetUpTest) uses for the identical reason: go-git's client
// package has no server-side counterpart of its own. Skips cleanly, not
// a failure, when git or git-http-backend aren't on this machine, the
// same shape newLiveBuildEnv already uses for a missing Docker/BuildKit.
func newPreviewGitHTTPServer(t *testing.T, workDir string) (baseRepoURL string) {
	t.Helper()

	execPathOut, err := exec.Command("git", "--exec-path").Output() //nolint:noctx // test helper, one-shot lookup
	if err != nil {
		t.Skipf("git not available: %v", err)
	}
	backendPath := filepath.Join(strings.TrimSpace(string(execPathOut)), "git-http-backend")
	if _, err := os.Stat(backendPath); err != nil {
		t.Skipf("git-http-backend not available at %q: %v", backendPath, err)
	}

	base := t.TempDir()
	const repoName = "preview.git"
	if _, err := git.PlainClone(filepath.Join(base, repoName), true, &git.CloneOptions{URL: workDir}); err != nil {
		t.Fatalf("PlainClone(bare) error = %v", err)
	}

	ts := httptest.NewServer(&cgi.Handler{
		Path: backendPath,
		Env:  []string{"GIT_HTTP_EXPORT_ALL=true", "GIT_PROJECT_ROOT=" + base},
	})
	t.Cleanup(ts.Close)

	return ts.URL + "/" + repoName
}

// newPreviewGitRepo creates a real local git repo (go-git PlainInit) with
// two commits, each building a busybox+httpd image that serves its own
// distinctive body, so a preview's real HTTP response can prove which
// commit it was actually built from. Fetched over real HTTP by
// newPreviewGitHTTPServer above, not cloned directly from this
// filesystem path.
func newPreviewGitRepo(t *testing.T, bodyA, bodyB string) (repoDir, commitA, commitB string) {
	t.Helper()
	repoDir = t.TempDir()
	repo, err := git.PlainInit(repoDir, false)
	if err != nil {
		t.Fatalf("PlainInit() error = %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree() error = %v", err)
	}
	commitA = writePreviewCommit(t, wt, repoDir, bodyA)
	commitB = writePreviewCommit(t, wt, repoDir, bodyB)
	return repoDir, commitA, commitB
}

func writePreviewCommit(t *testing.T, wt *git.Worktree, repoDir, body string) string {
	t.Helper()
	dockerfile := "FROM busybox:1.36\n" +
		"WORKDIR /srv\n" +
		"RUN echo \"" + body + "\" > index.html\n" +
		"EXPOSE 8080\n" +
		"CMD [\"busybox\", \"httpd\", \"-f\", \"-p\", \"8080\", \"-h\", \"/srv\"]\n"
	if err := writeFile(repoDir, "Dockerfile", dockerfile); err != nil {
		t.Fatalf("write Dockerfile: %v", err)
	}
	if _, err := wt.Add("Dockerfile"); err != nil {
		t.Fatalf("Add Dockerfile: %v", err)
	}
	sig := &object.Signature{Name: "Levelrail Test", Email: "test@example.invalid", When: time.Now()}
	commitHash, err := wt.Commit(body, &git.CommitOptions{Author: sig})
	if err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	return commitHash.String()
}

// reconcileAndAssertPreviewContent reconciles serviceName through ctrl
// and asserts, directly against runtime and a real HTTP request (not
// the returned Result alone), that exactly one running container exists
// for wantTag and serves wantBodyFragment, the same independent-
// verification rigor TestWebhook_Live_PushToRunningContainer applies.
func reconcileAndAssertPreviewContent(ctx context.Context, t *testing.T, runtime docker.Runtime, ctrl *application.Controller, serviceName, wantTag, wantBodyFragment string) {
	t.Helper()
	result, err := ctrl.Reconcile(ctx)
	if err != nil {
		t.Fatalf("Controller.Reconcile() for %q error = %v, result = %+v", serviceName, err, result)
	}
	if len(result.Conditions) == 0 || result.Conditions[0].Status != "True" {
		t.Fatalf("Controller.Reconcile() for %q result = %+v, want a True Ready condition", serviceName, result)
	}

	containers, err := runtime.ListByPrefix(ctx, serviceName+"-")
	if err != nil {
		t.Fatalf("ListByPrefix() error = %v", err)
	}
	if len(containers) != 1 {
		t.Fatalf("expected exactly 1 container for %q, got %d: %+v", serviceName, len(containers), containers)
	}
	if !containers[0].Running || containers[0].Image != wantTag || len(containers[0].Ports) == 0 {
		t.Fatalf("container %+v is not a running %q with a published port", containers[0], wantTag)
	}

	client := &http.Client{Timeout: 5 * time.Second}
	addr := "http://127.0.0.1:" + strconv.Itoa(containers[0].Ports[0].HostPort) + "/"
	body := getBodyWithRetry(t, client, addr)
	if !strings.Contains(body, wantBodyFragment) {
		t.Errorf("preview response body = %q, want it to contain %q", body, wantBodyFragment)
	}
}

// postWebhookPayload POSTs payload to appName's real git-push/preview
// webhook route with headers set verbatim, the low-level primitive both
// live preview tests in this file use to drive a real, provider-shaped
// delivery (unlike postJSON/requestJSON, this needs to set provider
// event/signature headers no other live test in this package needs).
func postWebhookPayload(t *testing.T, baseURL, appName string, payload []byte, headers map[string]string) (status int, body string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, baseURL+"/api/v1/webhooks/github/"+appName, bytes.NewReader(payload)) //nolint:noctx // test helper, url is loopback-only
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := (&http.Client{Timeout: e2eHTTPTimeout}).Do(req)
	if err != nil {
		t.Fatalf("POST webhook: %v", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			t.Errorf("closing response body: %v", closeErr)
		}
	}()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading webhook response body: %v", err)
	}
	return resp.StatusCode, string(b)
}

// gitlabPreviewWebhookPayload builds a Merge Request Hook payload shaped
// like GitLab's own documented example
// (https://docs.gitlab.com/user/project/integrations/webhook_events/#merge-request-events),
// the same shape internal/webhook/pull_request_test.go's
// gitlabMergeRequestFixture already verifies parseGitLabPullRequestEvent
// against, reproduced here in this package since that helper is
// unexported to internal/webhook's own tests.
func gitlabPreviewWebhookPayload(action string, iid int, sourceBranch, targetBranch, commitID string) []byte {
	return []byte(fmt.Sprintf(`{
		"object_kind": "merge_request",
		"user": {"name": "Preview E2E", "username": "preview-e2e"},
		"project": {"id": 4242, "name": "preview-e2e", "default_branch": %q},
		"object_attributes": {
			"id": 9001, "iid": %d, "title": "preview e2e",
			"action": %q, "source_branch": %q, "target_branch": %q,
			"state": "opened", "merge_status": "unchecked",
			"last_commit": {"id": %q, "message": "preview e2e commit"}
		},
		"labels": [],
		"changes": {}
	}`, targetBranch, iid, action, sourceBranch, targetBranch, commitID))
}

// bitbucketPreviewWebhookPayload builds a pullrequest:* payload shaped
// like Bitbucket Cloud's own documented examples
// (https://support.atlassian.com/bitbucket-cloud/docs/event-payloads/#Pull-request-created),
// the same shape internal/webhook/pull_request_test.go's
// bitbucketPullRequestFixture already verifies parseBitbucketPullRequestEvent
// against, reproduced here for the same reason gitlabPreviewWebhookPayload
// above is.
func bitbucketPreviewWebhookPayload(id int, sourceBranch, sourceHash, destBranch string) []byte {
	return []byte(fmt.Sprintf(`{
		"actor": {"username": "preview-e2e", "display_name": "Preview E2E"},
		"repository": {"full_name": "acme/preview-e2e", "name": "preview-e2e"},
		"pullrequest": {
			"id": %d,
			"title": "preview e2e",
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

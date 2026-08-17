package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

// sign mirrors internal/webhook/webhook_test.go's own sign helper
// exactly: GitHub's X-Hub-Signature-256 scheme, duplicated here rather
// than shared since Go test helpers aren't exported across packages, the
// same reasoning fakeBuilder's own doc comment already gives for its own
// duplication of internal/webhook's fakeDeployer.
func sign(secret []byte, body []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// pushBody builds a push event payload for ref, always at commit sha1:
// every test in this file that cares about a different commit
// (TestHandleGitPushWebhook_CorrectSignature_TriggersDeploy and its
// siblings) only ever varies ref/branch matching, never the commit
// itself, so this stays single-purpose rather than taking a parameter
// nothing here would ever vary.
func pushBody(ref string) []byte {
	b, _ := json.Marshal(struct {
		Ref   string `json:"ref"`
		After string `json:"after"`
	}{Ref: ref, After: "sha1"})
	return b
}

// fakeGitSourceFetchCall records one gitSourceFetchFunc call's
// arguments, the multi-app counterpart to builds_test.go's own
// fakeFetchCall, with the one extra field (token) that distinguishes
// this seam.
type fakeGitSourceFetchCall struct {
	repoURL, sha, token string
}

func newFakeGitSourceFetch(calls *[]fakeGitSourceFetchCall, dir string, cleanupCalled *bool, err error) gitSourceFetchFunc {
	return func(_ context.Context, repoURL, sha, token string) (string, func(), error) {
		*calls = append(*calls, fakeGitSourceFetchCall{repoURL: repoURL, sha: sha, token: token})
		if err != nil {
			return "", nil, err
		}
		return dir, func() { *cleanupCalled = true }, nil
	}
}

// connectGitSource drives the real PUT .../git-source handler (rather
// than writing store/secrets rows directly) so these tests exercise the
// actual connect flow a webhook depends on, returning the generated
// webhook secret. Always against app "web": every test in this file
// seeds exactly that one app name, the same fixed-name convention
// seedApp's own callers across this package already use.
func connectGitSource(t *testing.T, rt *Router, cookie *http.Cookie, body string) gitSourceResource {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/git-source", body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("connect git source: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got gitSourceResource
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode connect response: %v", err)
	}
	return got
}

func TestHandleGitPushWebhook_NoGitSource(t *testing.T) {
	rt, db := newTestRouterWithGitSourceSecrets(t, newFakeGitSourceSecrets())
	seedApp(t, db, "web")

	body := pushBody("refs/heads/main")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/github/web", strings.NewReader(string(body)))
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleGitPushWebhook_NotConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithGitSourceSecrets
	seedApp(t, db, "web")

	body := pushBody("refs/heads/main")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/github/web", strings.NewReader(string(body)))
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}
}

func TestHandleGitPushWebhook_IsUnauthenticated(t *testing.T) {
	// No session cookie, no Authorization header at all: proves this
	// route is not wrapped in requireAbility (router.go's own comment on
	// why), unlike every /api/v1/apps/... route this package otherwise
	// registers. A 404 (no git source connected, checked before any auth
	// question is even relevant) is proof enough that the request wasn't
	// bounced at 401 first.
	rt, db := newTestRouter(t)
	seedApp(t, db, "web")

	body := pushBody("refs/heads/main")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/github/web", strings.NewReader(string(body)))
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code == http.StatusUnauthorized {
		t.Fatalf("status = %d, want anything but 401: the git-push webhook route must never require session/token auth", rec.Code)
	}
}

func TestHandleGitPushWebhook_WrongSecret_Rejected(t *testing.T) {
	secrets := newFakeGitSourceSecrets()
	rt, db := newTestRouterWithGitSourceSecrets(t, secrets)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	connectGitSource(t, rt, cookie, `{"repo_url":"https://github.com/org/web.git","branch":"main"}`)

	var fetchCalls []fakeGitSourceFetchCall
	rt.gitSourceFetch = newFakeGitSourceFetch(&fetchCalls, t.TempDir(), new(bool), nil)
	rt.builder = &fakeBuilder{tag: "levelrail/web:sha1"}

	body := pushBody("refs/heads/main")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/github/web", strings.NewReader(string(body)))
	req.Header.Set("X-Hub-Signature-256", sign([]byte("wrong-secret"), body))
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if len(fetchCalls) != 0 {
		t.Errorf("fetch called %d times, want 0 when signature verification fails", len(fetchCalls))
	}
}

// TestHandleGitPushWebhook_WrongApp_SecretDoesNotCrossOver is a
// regression test against a future refactor silently breaking the
// per-app secret isolation handleGitPushWebhook relies on: it resolves
// secretsKey from the URL path's {name}, so app A's own secret must
// never validate a signature against app B's webhook route, even
// though both apps are configured and both secrets are real, connected
// secrets, not placeholders.
func TestHandleGitPushWebhook_WrongApp_SecretDoesNotCrossOver(t *testing.T) {
	secrets := newFakeGitSourceSecrets()
	rt, db := newTestRouterWithGitSourceSecrets(t, secrets)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	seedApp(t, db, "other")
	connectGitSource(t, rt, cookie, `{"repo_url":"https://github.com/org/web.git","branch":"main"}`)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/other/git-source",
		`{"repo_url":"https://github.com/org/other.git","branch":"main"}`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("connect other's git source: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var otherResource gitSourceResource
	if err := json.NewDecoder(rec.Body).Decode(&otherResource); err != nil {
		t.Fatalf("decode other's connect response: %v", err)
	}

	var fetchCalls []fakeGitSourceFetchCall
	rt.gitSourceFetch = newFakeGitSourceFetch(&fetchCalls, t.TempDir(), new(bool), nil)
	rt.builder = &fakeBuilder{tag: "levelrail/other:sha1"}

	body := pushBody("refs/heads/main")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/github/other", strings.NewReader(string(body)))
	req.Header.Set("X-Hub-Signature-256", sign([]byte(otherResource.WebhookSecret), body))
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("other's own secret against its own route: status = %d, want 200", rec.Code)
	}
	if len(fetchCalls) != 1 {
		t.Fatalf("fetch called %d times with other's own secret, want 1", len(fetchCalls))
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/github/other", strings.NewReader(string(body)))
	req.Header.Set("X-Hub-Signature-256", sign([]byte("web-app-secret-guess"), body))
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong secret against other's route: status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandleGitPushWebhook_CorrectSignature_TriggersDeploy(t *testing.T) {
	secrets := newFakeGitSourceSecrets()
	rt, db := newTestRouterWithGitSourceSecrets(t, secrets)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	created := connectGitSource(t, rt, cookie, `{"repo_url":"https://github.com/org/web.git","branch":"main","build_type":"dockerfile"}`)

	var fetchCalls []fakeGitSourceFetchCall
	rt.gitSourceFetch = newFakeGitSourceFetch(&fetchCalls, t.TempDir(), new(bool), nil)
	fb := &fakeBuilder{tag: "web:sha1"}
	rt.builder = fb

	body := pushBody("refs/heads/main")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/github/web", strings.NewReader(string(body)))
	req.Header.Set("X-Hub-Signature-256", sign([]byte(created.WebhookSecret), body))
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if fb.calls != 1 {
		t.Fatalf("builder called %d times, want 1", fb.calls)
	}
	if fb.lastReq.ServiceName != "web" || fb.lastReq.CommitSHA != "sha1" || fb.lastReq.ImageRepo != "web" {
		t.Errorf("deploy request = %+v, want ServiceName=web CommitSHA=sha1 ImageRepo=web", fb.lastReq)
	}
	if len(fetchCalls) != 1 || fetchCalls[0].repoURL != "https://github.com/org/web.git" || fetchCalls[0].sha != "sha1" {
		t.Errorf("fetch calls = %+v, want one call for the repo at sha1", fetchCalls)
	}
	if fetchCalls[0].token != "" {
		t.Errorf("fetch token = %q, want empty: no token was configured", fetchCalls[0].token)
	}

	attempts, err := db.ListDeployAttempts(context.Background(), "web")
	if err != nil {
		t.Fatalf("ListDeployAttempts() error = %v", err)
	}
	if len(attempts) != 1 || attempts[0].Source != store.DeployAttemptSourceWebhook {
		t.Errorf("deploy attempts = %+v, want exactly one with Source=%q", attempts, store.DeployAttemptSourceWebhook)
	}
}

// TestHandleGitPushWebhook_GitLabTokenHeader_TriggersDeploy proves the
// exact same webhook route accepts GitLab's own X-Gitlab-Token scheme
// (a plain secret comparison, not an HMAC signature), the receiver
// gitlab_app_projects.go's CreateProjectWebhook call registers a
// project hook against.
func TestHandleGitPushWebhook_GitLabTokenHeader_TriggersDeploy(t *testing.T) {
	secrets := newFakeGitSourceSecrets()
	rt, db := newTestRouterWithGitSourceSecrets(t, secrets)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	created := connectGitSource(t, rt, cookie, `{"repo_url":"https://gitlab.example.com/org/web.git","branch":"main","build_type":"dockerfile"}`)

	var fetchCalls []fakeGitSourceFetchCall
	rt.gitSourceFetch = newFakeGitSourceFetch(&fetchCalls, t.TempDir(), new(bool), nil)
	fb := &fakeBuilder{tag: "web:sha1"}
	rt.builder = fb

	body := pushBody("refs/heads/main")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/github/web", strings.NewReader(string(body)))
	req.Header.Set("X-Gitlab-Token", created.WebhookSecret)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if fb.calls != 1 {
		t.Fatalf("builder called %d times, want 1", fb.calls)
	}
}

// TestHandleGitPushWebhook_GitLabTokenHeader_WrongToken_Rejected proves
// a wrong X-Gitlab-Token is rejected the same way a wrong
// X-Hub-Signature-256 already is (TestHandleGitPushWebhook_WrongSecret_Rejected).
func TestHandleGitPushWebhook_GitLabTokenHeader_WrongToken_Rejected(t *testing.T) {
	secrets := newFakeGitSourceSecrets()
	rt, db := newTestRouterWithGitSourceSecrets(t, secrets)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	connectGitSource(t, rt, cookie, `{"repo_url":"https://gitlab.example.com/org/web.git","branch":"main"}`)
	rt.builder = &fakeBuilder{tag: "web:sha1"}

	body := pushBody("refs/heads/main")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/github/web", strings.NewReader(string(body)))
	req.Header.Set("X-Gitlab-Token", "not-the-real-secret")
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
}

func TestHandleGitPushWebhook_PrivateRepo_PassesToken(t *testing.T) {
	secrets := newFakeGitSourceSecrets()
	rt, db := newTestRouterWithGitSourceSecrets(t, secrets)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	created := connectGitSource(t, rt, cookie, `{"repo_url":"https://github.com/org/web.git","branch":"main","token":"ghp_supersecret"}`)

	var fetchCalls []fakeGitSourceFetchCall
	rt.gitSourceFetch = newFakeGitSourceFetch(&fetchCalls, t.TempDir(), new(bool), nil)
	rt.builder = &fakeBuilder{tag: "web:sha1"}

	body := pushBody("refs/heads/main")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/github/web", strings.NewReader(string(body)))
	req.Header.Set("X-Hub-Signature-256", sign([]byte(created.WebhookSecret), body))
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if len(fetchCalls) != 1 || fetchCalls[0].token != "ghp_supersecret" {
		t.Fatalf("fetch calls = %+v, want the deploy token passed through", fetchCalls)
	}
}

func TestHandleGitPushWebhook_WrongBranch_AcceptedButNotTriggered(t *testing.T) {
	secrets := newFakeGitSourceSecrets()
	rt, db := newTestRouterWithGitSourceSecrets(t, secrets)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	created := connectGitSource(t, rt, cookie, `{"repo_url":"https://github.com/org/web.git","branch":"main"}`)

	var fetchCalls []fakeGitSourceFetchCall
	rt.gitSourceFetch = newFakeGitSourceFetch(&fetchCalls, t.TempDir(), new(bool), nil)
	fb := &fakeBuilder{tag: "web:sha1"}
	rt.builder = fb

	body := pushBody("refs/heads/feature-x")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/github/web", strings.NewReader(string(body)))
	req.Header.Set("X-Hub-Signature-256", sign([]byte(created.WebhookSecret), body))
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if fb.calls != 0 {
		t.Errorf("builder called %d times, want 0 for a non-target branch", fb.calls)
	}
	if len(fetchCalls) != 0 {
		t.Errorf("fetch called %d times, want 0 for a non-target branch", len(fetchCalls))
	}
}

func TestHandleGitPushWebhook_MalformedPayload(t *testing.T) {
	secrets := newFakeGitSourceSecrets()
	rt, db := newTestRouterWithGitSourceSecrets(t, secrets)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	created := connectGitSource(t, rt, cookie, `{"repo_url":"https://github.com/org/web.git"}`)

	body := []byte("not json at all")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/github/web", strings.NewReader(string(body)))
	req.Header.Set("X-Hub-Signature-256", sign([]byte(created.WebhookSecret), body))
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleGitPushWebhook_NoBuilderConfigured(t *testing.T) {
	secrets := newFakeGitSourceSecrets()
	rt, db := newTestRouterWithGitSourceSecrets(t, secrets)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	created := connectGitSource(t, rt, cookie, `{"repo_url":"https://github.com/org/web.git","branch":"main"}`)
	// No WithBuilder: rt.builder stays nil.

	body := pushBody("refs/heads/main")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/github/web", strings.NewReader(string(body)))
	req.Header.Set("X-Hub-Signature-256", sign([]byte(created.WebhookSecret), body))
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}
}

func TestHandleGitPushWebhook_FetchFailure_ReturnsServerErrorNotLeaky(t *testing.T) {
	secrets := newFakeGitSourceSecrets()
	rt, db := newTestRouterWithGitSourceSecrets(t, secrets)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	created := connectGitSource(t, rt, cookie, `{"repo_url":"https://github.com/org/web.git","branch":"main"}`)

	var fetchCalls []fakeGitSourceFetchCall
	rt.gitSourceFetch = newFakeGitSourceFetch(&fetchCalls, "", new(bool), errors.New("clone failed: connection refused"))
	fb := &fakeBuilder{tag: "web:sha1"}
	rt.builder = fb

	body := pushBody("refs/heads/main")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/github/web", strings.NewReader(string(body)))
	req.Header.Set("X-Hub-Signature-256", sign([]byte(created.WebhookSecret), body))
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if strings.Contains(rec.Body.String(), "connection refused") {
		t.Errorf("response body = %s, must not leak internal error detail", rec.Body.String())
	}
	if fb.calls != 0 {
		t.Errorf("builder called %d times, want 0 when fetch fails", fb.calls)
	}
}

func TestHandleGitPushWebhook_AppDeleted_ReturnsNotFound(t *testing.T) {
	secrets := newFakeGitSourceSecrets()
	rt, db := newTestRouterWithGitSourceSecrets(t, secrets)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	created := connectGitSource(t, rt, cookie, `{"repo_url":"https://github.com/org/web.git","branch":"main"}`)

	if err := db.DeleteDesiredService(context.Background(), "web"); err != nil {
		t.Fatalf("DeleteDesiredService() error = %v", err)
	}

	fb := &fakeBuilder{tag: "web:sha1"}
	rt.builder = fb

	body := pushBody("refs/heads/main")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/github/web", strings.NewReader(string(body)))
	req.Header.Set("X-Hub-Signature-256", sign([]byte(created.WebhookSecret), body))
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	if fb.calls != 0 {
		t.Errorf("builder called %d times, want 0", fb.calls)
	}
}

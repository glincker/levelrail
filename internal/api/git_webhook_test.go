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

	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/spec"
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

// newConnectedGitSourceRouter is the fixed router+seed+connect preamble
// every trigger-mode test in this file repeats before it gets to what
// it actually tests: a router, app "web" seeded, and its git source
// connected via connectBody.
func newConnectedGitSourceRouter(t *testing.T, connectBody string) (*Router, gitSourceResource) {
	t.Helper()
	rt, db := newTestRouterWithGitSourceSecrets(t, newFakeGitSourceSecrets())
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	return rt, connectGitSource(t, rt, cookie, connectBody)
}

// postGitWebhook POSTs body to app "web"'s webhook route with sigHeader
// set to sig, plus any extraHeaders (the event-type headers the
// receiver switches on), and returns the recorder rather than asserting
// a status: this file's trigger-mode tests expect different codes.
func postGitWebhook(t *testing.T, rt *Router, sigHeader, sig string, body []byte, extraHeaders map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/github/web", strings.NewReader(string(body)))
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}
	req.Header.Set(sigHeader, sig)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	return rec
}

func requireStatusOK(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
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

// bitbucketPushBody builds a Bitbucket-shaped repo:push payload for
// branch, always at commit "bb-sha1": the same fixed-commit convention
// pushBody's own doc comment establishes.
func bitbucketPushBody(branch string) []byte {
	b, _ := json.Marshal(map[string]any{
		"push": map[string]any{
			"changes": []map[string]any{
				{"new": map[string]any{"type": "branch", "name": branch, "target": map[string]any{"hash": "bb-sha1"}}},
			},
		},
	})
	return b
}

// TestHandleGitPushWebhook_BitbucketSignatureHeader_TriggersDeploy
// proves the same webhook route also accepts Bitbucket's own payload
// shape and X-Hub-Signature header (HMAC-SHA256 like GitHub's
// X-Hub-Signature-256, just without the "-256" suffix), selected by
// X-Event-Key: repo:push (webhook.ParsePushEventForProvider's own doc
// comment).
func TestHandleGitPushWebhook_BitbucketSignatureHeader_TriggersDeploy(t *testing.T) {
	secrets := newFakeGitSourceSecrets()
	rt, db := newTestRouterWithGitSourceSecrets(t, secrets)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	created := connectGitSource(t, rt, cookie, `{"repo_url":"https://bitbucket.org/org/web.git","branch":"main","build_type":"dockerfile"}`)

	var fetchCalls []fakeGitSourceFetchCall
	rt.gitSourceFetch = newFakeGitSourceFetch(&fetchCalls, t.TempDir(), new(bool), nil)
	fb := &fakeBuilder{tag: "web:bb-sha1"}
	rt.builder = fb

	body := bitbucketPushBody("main")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/github/web", strings.NewReader(string(body)))
	req.Header.Set("X-Event-Key", "repo:push")
	req.Header.Set("X-Hub-Signature", sign([]byte(created.WebhookSecret), body))
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if fb.calls != 1 {
		t.Fatalf("builder called %d times, want 1", fb.calls)
	}
	if fb.lastReq.CommitSHA != "bb-sha1" {
		t.Errorf("deploy request CommitSHA = %q, want bb-sha1", fb.lastReq.CommitSHA)
	}
}

// TestHandleGitPushWebhook_BitbucketWrongSignature_Rejected proves a
// wrong X-Hub-Signature is rejected the same way a wrong
// X-Hub-Signature-256 already is.
func TestHandleGitPushWebhook_BitbucketWrongSignature_Rejected(t *testing.T) {
	secrets := newFakeGitSourceSecrets()
	rt, db := newTestRouterWithGitSourceSecrets(t, secrets)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	connectGitSource(t, rt, cookie, `{"repo_url":"https://bitbucket.org/org/web.git","branch":"main"}`)
	rt.builder = &fakeBuilder{tag: "web:bb-sha1"}

	body := bitbucketPushBody("main")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/github/web", strings.NewReader(string(body)))
	req.Header.Set("X-Event-Key", "repo:push")
	req.Header.Set("X-Hub-Signature", sign([]byte("not-the-real-secret"), body))
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
}

// TestVerifyGitPushWebhookAuth covers the full header matrix
// verifyGitPushWebhookAuth branches on, including the case none of the
// three known headers are present, which must fail closed rather than
// pass by default.
func TestVerifyGitPushWebhookAuth(t *testing.T) {
	secret := "s3cr3t"
	body := []byte(`{"ref":"refs/heads/main","after":"sha1"}`)

	tests := []struct {
		name    string
		headers map[string]string
		want    bool
	}{
		{
			name:    "github valid signature",
			headers: map[string]string{"X-Hub-Signature-256": sign([]byte(secret), body)},
			want:    true,
		},
		{
			name:    "github wrong signature",
			headers: map[string]string{"X-Hub-Signature-256": sign([]byte("wrong"), body)},
			want:    false,
		},
		{
			name:    "gitlab valid token",
			headers: map[string]string{"X-Gitlab-Token": secret},
			want:    true,
		},
		{
			name:    "gitlab wrong token",
			headers: map[string]string{"X-Gitlab-Token": "wrong"},
			want:    false,
		},
		{
			name:    "bitbucket valid signature",
			headers: map[string]string{"X-Hub-Signature": sign([]byte(secret), body)},
			want:    true,
		},
		{
			name:    "bitbucket wrong signature",
			headers: map[string]string{"X-Hub-Signature": sign([]byte("wrong"), body)},
			want:    false,
		},
		{
			name:    "no known header present",
			headers: map[string]string{},
			want:    false,
		},
		{
			name:    "gitlab token takes precedence when multiple headers present",
			headers: map[string]string{"X-Gitlab-Token": secret, "X-Hub-Signature": sign([]byte("wrong"), body)},
			want:    true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := http.Header{}
			for k, v := range tt.headers {
				h.Set(k, v)
			}
			if got := verifyGitPushWebhookAuth(secret, body, h); got != tt.want {
				t.Errorf("verifyGitPushWebhookAuth() = %v, want %v", got, tt.want)
			}
		})
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

func TestHandleGitPushWebhook_AdditionalServices_FansOut(t *testing.T) {
	secrets := newFakeGitSourceSecrets()
	rt, db := newTestRouterWithGitSourceSecrets(t, secrets)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	seedApp(t, db, "worker")
	created := connectGitSource(t, rt, cookie, `{"repo_url":"https://github.com/org/web.git","branch":"main","build_type":"dockerfile",
		"additional_services":{"worker":{"build_type":"dockerfile","build_path":"./worker/Dockerfile"}}}`)

	rt.gitSourceFetch = newFakeGitSourceFetch(&[]fakeGitSourceFetchCall{}, t.TempDir(), new(bool), nil)
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
	if fb.calls != 2 {
		t.Fatalf("builder called %d times, want 2 (primary + additional)", fb.calls)
	}

	for _, name := range []string{"web", "worker"} {
		attempts, err := db.ListDeployAttempts(context.Background(), name)
		if err != nil {
			t.Fatalf("ListDeployAttempts(%q) error = %v", name, err)
		}
		if len(attempts) != 1 || attempts[0].Source != store.DeployAttemptSourceWebhook {
			t.Errorf("deploy attempts for %q = %+v, want exactly one with Source=%q", name, attempts, store.DeployAttemptSourceWebhook)
		}
	}
}

func TestHandleGitPushWebhook_AdditionalServices_MissingSibling_PartialFailure(t *testing.T) {
	secrets := newFakeGitSourceSecrets()
	rt, db := newTestRouterWithGitSourceSecrets(t, secrets)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	created := connectGitSource(t, rt, cookie, `{"repo_url":"https://github.com/org/web.git","branch":"main","build_type":"dockerfile",
		"additional_services":{"ghost":{"build_type":"dockerfile"}}}`)

	rt.gitSourceFetch = newFakeGitSourceFetch(&[]fakeGitSourceFetchCall{}, t.TempDir(), new(bool), nil)
	fb := &fakeBuilder{tag: "web:sha1"}
	rt.builder = fb

	body := pushBody("refs/heads/main")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/github/web", strings.NewReader(string(body)))
	req.Header.Set("X-Hub-Signature-256", sign([]byte(created.WebhookSecret), body))
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusMultiStatus {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusMultiStatus, rec.Body.String())
	}
	if fb.calls != 1 {
		t.Fatalf("builder called %d times, want 1: only the primary service should deploy", fb.calls)
	}

	attempts, err := db.ListDeployAttempts(context.Background(), "web")
	if err != nil {
		t.Fatalf("ListDeployAttempts() error = %v", err)
	}
	if len(attempts) != 1 {
		t.Errorf("deploy attempts for web = %+v, want exactly one: the missing sibling must not block the primary deploy", attempts)
	}
}

// servicesSpecBody is a two-key app.yaml-style services: map, the same
// shape multiDeployBody (apps_multi_test.go) uses for the manual/API
// deploy-spec path, so a git source's own persisted version exercises
// an identical request shape.
func servicesSpecBody() string {
	return `{"repo_url":"https://github.com/org/web.git","branch":"main","services":{
		"web":{"build":{"type":"dockerfile","path":"./web/Dockerfile"},"port":3000},
		"worker":{"build":{"type":"dockerfile","path":"./worker/Dockerfile"}}
	}}`
}

// TestHandleGitPushWebhook_ServicesSpec_RoutesThroughDeploySpec proves a
// git source with a persisted services: map fans a push out through
// Builder.DeploySpec (the same method handleDeploySpec calls), not
// through the single Deploy call the AdditionalServices path uses: this
// is the unification docs/roadmap.md's own "the two paths aren't
// unified yet" gap named.
func TestHandleGitPushWebhook_ServicesSpec_RoutesThroughDeploySpec(t *testing.T) {
	secrets := newFakeGitSourceSecrets()
	rt, db := newTestRouterWithGitSourceSecrets(t, secrets)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	created := connectGitSource(t, rt, cookie, servicesSpecBody())

	rt.gitSourceFetch = newFakeGitSourceFetch(&[]fakeGitSourceFetchCall{}, t.TempDir(), new(bool), nil)
	fb := &fakeBuilder{tag: "img:sha1"}
	rt.builder = fb

	body := pushBody("refs/heads/main")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/github/web", strings.NewReader(string(body)))
	req.Header.Set("X-Hub-Signature-256", sign([]byte(created.WebhookSecret), body))
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	if fb.calls != 0 {
		t.Errorf("single-service Deploy called %d times, want 0: a services: map must route through DeploySpec instead", fb.calls)
	}
	if fb.multiCalls != 1 {
		t.Fatalf("DeploySpec called %d times, want 1", fb.multiCalls)
	}
	if fb.lastMultiReq.AppName != "web" {
		t.Errorf("AppName = %q, want %q (existing.AppID falls back to the git source's own service name)", fb.lastMultiReq.AppName, "web")
	}
	if fb.lastMultiReq.CommitSHA != "sha1" {
		t.Errorf("CommitSHA = %q, want %q", fb.lastMultiReq.CommitSHA, "sha1")
	}
	if len(fb.lastMultiReq.Services) != 2 {
		t.Errorf("Services = %+v, want 2 keys", fb.lastMultiReq.Services)
	}
}

// TestHandleGitPushWebhook_ServicesSpec_PartialFailure_OneServiceFailing
// covers the required case for this dispatch: a fan-out where
// one service key's own build fails must still deploy every other key
// (not silently drop it), must not retry or double-trigger the failed
// key, and must surface the partial failure as 207, mirroring
// TestHandleDeploySpec_PartialFailure_StillReturns2xxWithPerServiceErrors's
// own contract for the manual/API path this now shares.
func TestHandleGitPushWebhook_ServicesSpec_PartialFailure_OneServiceFailing(t *testing.T) {
	secrets := newFakeGitSourceSecrets()
	rt, db := newTestRouterWithGitSourceSecrets(t, secrets)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	created := connectGitSource(t, rt, cookie, servicesSpecBody())

	rt.gitSourceFetch = newFakeGitSourceFetch(&[]fakeGitSourceFetchCall{}, t.TempDir(), new(bool), nil)
	fb := &fakeBuilder{
		multiOutcomes: []deploy.ServiceOutcome{
			{ServiceKey: "web", ServiceName: "web-web", Image: "img:sha1"},
			{ServiceKey: "worker", ServiceName: "web-worker", Err: errors.New("worker build failed")},
		},
	}
	rt.builder = fb

	body := pushBody("refs/heads/main")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/github/web", strings.NewReader(string(body)))
	req.Header.Set("X-Hub-Signature-256", sign([]byte(created.WebhookSecret), body))
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusMultiStatus {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusMultiStatus, rec.Body.String())
	}
	if fb.multiCalls != 1 {
		t.Fatalf("DeploySpec called %d times, want exactly 1: a partial failure must not be retried or double-triggered", fb.multiCalls)
	}
	if len(fb.lastMultiReq.Services) != 2 {
		t.Errorf("Services = %+v, want both keys still submitted in one call, not dropped for the failing one", fb.lastMultiReq.Services)
	}
}

// releaseBody builds a GitHub release event payload for action and
// tagName, the same fixed-shape convention pushBody's own doc comment
// establishes for push events.
func releaseBody(action string) []byte {
	b, _ := json.Marshal(map[string]any{
		"action": action,
		"release": map[string]any{
			"tag_name": "v1.2.3",
		},
	})
	return b
}

// TestGitSourceTriggerMatchesPush covers gitSourceTriggerMatchesPush's
// full decision table: push mode only matches the configured branch
// (unchanged pre-trigger-mode behavior), release mode only matches a
// tag ref regardless of branch, and an empty (pre-migration) TriggerMode
// behaves exactly like an explicit "push".
func TestGitSourceTriggerMatchesPush(t *testing.T) {
	tests := []struct {
		name string
		gs   store.GitSource
		ref  string
		want bool
	}{
		{name: "push mode matches configured branch", gs: store.GitSource{Branch: "main", TriggerMode: "push"}, ref: "refs/heads/main", want: true},
		{name: "push mode ignores other branch", gs: store.GitSource{Branch: "main", TriggerMode: "push"}, ref: "refs/heads/feature-x", want: false},
		{name: "push mode ignores a tag push", gs: store.GitSource{Branch: "main", TriggerMode: "push"}, ref: "refs/tags/v1.0.0", want: false},
		{name: "empty trigger mode defaults to push behavior", gs: store.GitSource{Branch: "main", TriggerMode: ""}, ref: "refs/heads/main", want: true},
		{name: "release mode matches a tag push", gs: store.GitSource{Branch: "main", TriggerMode: "release"}, ref: "refs/tags/v1.0.0", want: true},
		{name: "release mode ignores the configured branch", gs: store.GitSource{Branch: "main", TriggerMode: "release"}, ref: "refs/heads/main", want: false},
		{name: "release mode ignores any other branch", gs: store.GitSource{Branch: "main", TriggerMode: "release"}, ref: "refs/heads/feature-x", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, msg := gitSourceTriggerMatchesPush(tt.gs, tt.ref)
			if got != tt.want {
				t.Errorf("gitSourceTriggerMatchesPush(%+v, %q) = %v, want %v", tt.gs, tt.ref, got, tt.want)
			}
			if !got && msg == "" {
				t.Errorf("gitSourceTriggerMatchesPush(%+v, %q) returned no ignored-response message", tt.gs, tt.ref)
			}
			if got && msg != "" {
				t.Errorf("gitSourceTriggerMatchesPush(%+v, %q) returned a message %q for a triggering match", tt.gs, tt.ref, msg)
			}
		})
	}
}

// TestParseGitHubReleaseEvent covers parseGitHubReleaseEvent's own
// decode contract: a missing action is rejected, tag_name is optional
// at parse time (processGitHubReleaseWebhookEvent only requires it once
// action is actually "published").
func TestParseGitHubReleaseEvent(t *testing.T) {
	got, err := parseGitHubReleaseEvent(releaseBody("published"))
	if err != nil {
		t.Fatalf("parseGitHubReleaseEvent() error = %v", err)
	}
	if got.Action != "published" || got.TagName != "v1.2.3" {
		t.Errorf("parseGitHubReleaseEvent() = %+v, want Action=published TagName=v1.2.3", got)
	}

	if _, err := parseGitHubReleaseEvent([]byte("not json")); err == nil {
		t.Error("parseGitHubReleaseEvent(malformed) error = nil, want an error")
	}
	if _, err := parseGitHubReleaseEvent([]byte(`{"release":{"tag_name":"v1.0.0"}}`)); err == nil {
		t.Error("parseGitHubReleaseEvent(missing action) error = nil, want an error")
	}
}

func TestDockerSafeTag(t *testing.T) {
	if got := dockerSafeTag("release/v1.2.3"); got != "release-v1.2.3" {
		t.Errorf("dockerSafeTag(%q) = %q, want release-v1.2.3", "release/v1.2.3", got)
	}
	if got := dockerSafeTag("v1.2.3"); got != "v1.2.3" {
		t.Errorf("dockerSafeTag(%q) = %q, want v1.2.3 unchanged", "v1.2.3", got)
	}
}

// TestHandleGitPushWebhook_ReleaseMode_BranchPush_Ignored proves a git
// source configured for trigger_mode "release" does not deploy on an
// ordinary push to its own configured branch, the core behavior change
// this trigger mode exists for.
func TestHandleGitPushWebhook_ReleaseMode_BranchPush_Ignored(t *testing.T) {
	rt, created := newConnectedGitSourceRouter(t, `{"repo_url":"https://github.com/org/web.git","branch":"main","trigger_mode":"release"}`)

	var fetchCalls []fakeGitSourceFetchCall
	rt.gitSourceFetch = newFakeGitSourceFetch(&fetchCalls, t.TempDir(), new(bool), nil)
	fb := &fakeBuilder{tag: "web:sha1"}
	rt.builder = fb

	body := pushBody("refs/heads/main")
	rec := postGitWebhook(t, rt, "X-Hub-Signature-256", sign([]byte(created.WebhookSecret), body), body, nil)
	requireStatusOK(t, rec)
	if fb.calls != 0 {
		t.Errorf("builder called %d times, want 0: release mode must ignore a branch push", fb.calls)
	}
	if len(fetchCalls) != 0 {
		t.Errorf("fetch called %d times, want 0: release mode must ignore a branch push", len(fetchCalls))
	}
}

// TestHandleGitPushWebhook_ReleaseMode_TagPush_TriggersDeploy proves a
// git source configured for trigger_mode "release" deploys on a tag ref
// push, using the tag push's own real commit SHA (ev.After) exactly as
// an ordinary push would: no special checkout handling is needed here,
// unlike the github release-event path.
func TestHandleGitPushWebhook_ReleaseMode_TagPush_TriggersDeploy(t *testing.T) {
	rt, created := newConnectedGitSourceRouter(t, `{"repo_url":"https://github.com/org/web.git","branch":"main","trigger_mode":"release"}`)

	var fetchCalls []fakeGitSourceFetchCall
	rt.gitSourceFetch = newFakeGitSourceFetch(&fetchCalls, t.TempDir(), new(bool), nil)
	fb := &fakeBuilder{tag: "web:sha1"}
	rt.builder = fb

	body := pushBody("refs/tags/v1.0.0")
	rec := postGitWebhook(t, rt, "X-Hub-Signature-256", sign([]byte(created.WebhookSecret), body), body, nil)
	requireStatusOK(t, rec)
	if fb.calls != 1 {
		t.Fatalf("builder called %d times, want 1", fb.calls)
	}
	if fb.lastReq.CommitSHA != "sha1" {
		t.Errorf("CommitSHA = %q, want sha1: a tag push's own ev.After", fb.lastReq.CommitSHA)
	}
	if len(fetchCalls) != 1 || fetchCalls[0].sha != "sha1" {
		t.Errorf("fetch calls = %+v, want one call checking out sha1", fetchCalls)
	}
}

// TestHandleGitPushWebhook_PushMode_TagPush_Ignored proves the default
// "push" trigger mode still ignores a tag push, the same shape
// TestHandleGitPushWebhook_WrongBranch_AcceptedButNotTriggered already
// proves for a wrong branch: a tag ref never equals
// webhook.Config.TargetRef()'s "refs/heads/<branch>".
func TestHandleGitPushWebhook_PushMode_TagPush_Ignored(t *testing.T) {
	rt, created := newConnectedGitSourceRouter(t, `{"repo_url":"https://github.com/org/web.git","branch":"main"}`)

	fb := &fakeBuilder{tag: "web:sha1"}
	rt.builder = fb

	body := pushBody("refs/tags/v1.0.0")
	rec := postGitWebhook(t, rt, "X-Hub-Signature-256", sign([]byte(created.WebhookSecret), body), body, nil)
	requireStatusOK(t, rec)
	if fb.calls != 0 {
		t.Errorf("builder called %d times, want 0: default push mode must ignore a tag push", fb.calls)
	}
}

// TestHandleGitPushWebhook_GitHubReleasePublished_TriggersDeploy proves
// a github "release" event with action "published" deploys when trigger
// mode is "release", checking out the tag's own ref (no commit SHA
// exists in the release payload) and tagging the built image with the
// (sanitized) release tag name.
func TestHandleGitPushWebhook_GitHubReleasePublished_TriggersDeploy(t *testing.T) {
	rt, created := newConnectedGitSourceRouter(t, `{"repo_url":"https://github.com/org/web.git","branch":"main","trigger_mode":"release"}`)

	var fetchCalls []fakeGitSourceFetchCall
	rt.gitSourceFetch = newFakeGitSourceFetch(&fetchCalls, t.TempDir(), new(bool), nil)
	fb := &fakeBuilder{tag: "web:v1.2.3"}
	rt.builder = fb

	body := releaseBody("published")
	rec := postGitWebhook(t, rt, "X-Hub-Signature-256", sign([]byte(created.WebhookSecret), body), body, map[string]string{"X-GitHub-Event": "release"})
	requireStatusOK(t, rec)
	if fb.calls != 1 {
		t.Fatalf("builder called %d times, want 1", fb.calls)
	}
	if fb.lastReq.CommitSHA != "v1.2.3" {
		t.Errorf("CommitSHA = %q, want v1.2.3 (the release tag name)", fb.lastReq.CommitSHA)
	}
	if len(fetchCalls) != 1 || fetchCalls[0].sha != "refs/tags/v1.2.3" {
		t.Errorf("fetch calls = %+v, want one call checking out refs/tags/v1.2.3", fetchCalls)
	}
}

// TestHandleGitPushWebhook_GitHubReleaseNotPublished_Ignored proves a
// release event action other than "published" (created, edited, a
// draft being saved, etc.) never deploys, even with trigger_mode
// "release".
func TestHandleGitPushWebhook_GitHubReleaseNotPublished_Ignored(t *testing.T) {
	rt, created := newConnectedGitSourceRouter(t, `{"repo_url":"https://github.com/org/web.git","branch":"main","trigger_mode":"release"}`)

	fb := &fakeBuilder{tag: "web:v1.2.3"}
	rt.builder = fb

	body := releaseBody("created")
	rec := postGitWebhook(t, rt, "X-Hub-Signature-256", sign([]byte(created.WebhookSecret), body), body, map[string]string{"X-GitHub-Event": "release"})
	requireStatusOK(t, rec)
	if fb.calls != 0 {
		t.Errorf("builder called %d times, want 0: only a \"published\" action deploys", fb.calls)
	}
}

// TestHandleGitPushWebhook_GitHubReleaseEvent_PushModeIgnored proves a
// github release event against a git source still on the default "push"
// trigger mode is accepted (200) and ignored, not treated as a
// malformed push payload (400): before trigger modes existed, this
// event type fell through into webhook.ParsePushEventForProvider and
// failed to parse.
func TestHandleGitPushWebhook_GitHubReleaseEvent_PushModeIgnored(t *testing.T) {
	rt, created := newConnectedGitSourceRouter(t, `{"repo_url":"https://github.com/org/web.git","branch":"main"}`)

	fb := &fakeBuilder{tag: "web:v1.2.3"}
	rt.builder = fb

	body := releaseBody("published")
	rec := postGitWebhook(t, rt, "X-Hub-Signature-256", sign([]byte(created.WebhookSecret), body), body, map[string]string{"X-GitHub-Event": "release"})
	requireStatusOK(t, rec)
	if fb.calls != 0 {
		t.Errorf("builder called %d times, want 0: default push mode must ignore a release event", fb.calls)
	}
}

// TestHandleGitPushWebhook_ReleaseMode_BitbucketTagPush_KnownGap is a
// regression/documentation test for the gap docs/git-integrations.md
// spells out: ParseBitbucketPushEvent (internal/webhook/bitbucket_push.go)
// always synthesizes "refs/heads/<name>" regardless of whether Bitbucket's
// own payload marked the change as a tag ("type":"tag"), so a Bitbucket
// tag push can never match gitSourceTriggerMatchesPush's "refs/tags/"
// check today. This fails closed (never triggers), not open, so it's a
// documented missing feature, not a security bug, but this test exists
// so a future fix to Bitbucket tag support has to consciously update it
// rather than silently leaving stale documentation behind.
func TestHandleGitPushWebhook_ReleaseMode_BitbucketTagPush_KnownGap(t *testing.T) {
	rt, created := newConnectedGitSourceRouter(t, `{"repo_url":"https://bitbucket.org/org/web.git","branch":"main","trigger_mode":"release"}`)

	fb := &fakeBuilder{tag: "web:bb-sha1"}
	rt.builder = fb

	body, _ := json.Marshal(map[string]any{
		"push": map[string]any{
			"changes": []map[string]any{
				{"new": map[string]any{"type": "tag", "name": "v1.0.0", "target": map[string]any{"hash": "bb-sha1"}}},
			},
		},
	})
	rec := postGitWebhook(t, rt, "X-Hub-Signature", sign([]byte(created.WebhookSecret), body), body, map[string]string{"X-Event-Key": "repo:push"})
	requireStatusOK(t, rec)
	if fb.calls != 0 {
		t.Errorf("builder called %d times, want 0 (known gap): a bitbucket tag push is indistinguishable from a same-named branch push today", fb.calls)
	}
}

// TestHandleGitPushWebhook_ServicesSpec_TakesPriorityOverAdditionalServices
// cannot happen through the connect API (validateGitSourceServices
// rejects setting both), but a git source created before that
// validation existed, or written directly to the store, could still
// carry both. This proves handleGitPushWebhook's own routing decision
// (len(gs.Services) > 0) is services-first regardless, so such a row
// deploys once, not twice.
func TestHandleGitPushWebhook_ServicesSpec_TakesPriorityOverAdditionalServices(t *testing.T) {
	secrets := newFakeGitSourceSecrets()
	rt, db := newTestRouterWithGitSourceSecrets(t, secrets)
	seedApp(t, db, "web")
	seedApp(t, db, "worker")

	webhookSecretKey := store.GitSourceSecretsKey("web")
	if err := secrets.SetValue(context.Background(), webhookSecretKey, gitSourceSecretKey, "shh"); err != nil {
		t.Fatalf("seed webhook secret: %v", err)
	}
	if err := db.SaveGitSource(context.Background(), store.GitSource{
		ServiceName: "web", RepoURL: "https://github.com/org/web.git", Branch: "main", BuildType: "dockerfile",
		AdditionalServices: map[string]store.GitSourceBuild{"worker": {BuildType: "dockerfile"}},
		Services:           map[string]spec.Service{"web": {Build: spec.Build{Type: spec.BuildDockerfile}}},
	}); err != nil {
		t.Fatalf("seed git source: %v", err)
	}

	rt.gitSourceFetch = newFakeGitSourceFetch(&[]fakeGitSourceFetchCall{}, t.TempDir(), new(bool), nil)
	fb := &fakeBuilder{tag: "img:sha1"}
	rt.builder = fb

	body := pushBody("refs/heads/main")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/github/web", strings.NewReader(string(body)))
	req.Header.Set("X-Hub-Signature-256", sign([]byte("shh"), body))
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if fb.multiCalls != 1 || fb.calls != 0 {
		t.Errorf("multiCalls = %d, calls = %d, want DeploySpec called once and single Deploy never called", fb.multiCalls, fb.calls)
	}
}

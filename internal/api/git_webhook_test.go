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
	"github.com/GLINCKER/levelrail/internal/githubapp"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

// sign duplicates internal/webhook's helper, since test helpers are not shared across packages.
func sign(secret []byte, body []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// pushBody always uses one commit, since no test here varies the commit.
func pushBody(ref string) []byte {
	b, _ := json.Marshal(struct {
		Ref   string `json:"ref"`
		After string `json:"after"`
	}{Ref: ref, After: "sha1"})
	return b
}

// fakeGitSourceFetchCall adds the token field to the fake fetch record.
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

// connectGitSource uses the real PUT .../git-source route, so tests exercise the actual connect flow.
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

// newConnectedGitSourceRouter sets up app web with its git source connected.
func newConnectedGitSourceRouter(t *testing.T, connectBody string) (*Router, gitSourceResource) {
	t.Helper()
	rt, db := newTestRouterWithGitSourceSecrets(t, newFakeGitSourceSecrets())
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	return rt, connectGitSource(t, rt, cookie, connectBody)
}

// postGitWebhook returns the recorder, since the trigger-mode tests expect different status codes.
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
	// The route is not behind requireAbility, so a request with no auth must reach the handler.
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

// TestHandleGitPushWebhook_WrongApp_SecretDoesNotCrossOver guards per-app secret isolation on the webhook route.
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

// TestHandleGitPushWebhook_GitLabTokenHeader_TriggersDeploy checks GitLab's plain X-Gitlab-Token is accepted on the same route.
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

// TestHandleGitPushWebhook_BitbucketSignatureHeader_TriggersDeploy checks Bitbucket's X-Hub-Signature is accepted on the same route.
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

// TestHandleGitPushWebhook_GiteaSignatureHeader_TriggersDeploy checks a real Gitea delivery is accepted on the same route.
func TestHandleGitPushWebhook_GiteaSignatureHeader_TriggersDeploy(t *testing.T) {
	secrets := newFakeGitSourceSecrets()
	rt, db := newTestRouterWithGitSourceSecrets(t, secrets)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	created := connectGitSource(t, rt, cookie, `{"repo_url":"https://git.example.com/org/web.git","branch":"main","build_type":"dockerfile"}`)

	var fetchCalls []fakeGitSourceFetchCall
	rt.gitSourceFetch = newFakeGitSourceFetch(&fetchCalls, t.TempDir(), new(bool), nil)
	fb := &fakeBuilder{tag: "web:gitea-sha1"}
	rt.builder = fb

	body := []byte(`{"ref":"refs/heads/main","after":"gitea-sha1"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/github/web", strings.NewReader(string(body)))
	req.Header.Set("X-Gitea-Event-Type", "push")
	req.Header.Set("X-Hub-Signature-256", sign([]byte(created.WebhookSecret), body))
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if fb.calls != 1 {
		t.Fatalf("builder called %d times, want 1", fb.calls)
	}
	if fb.lastReq.CommitSHA != "gitea-sha1" {
		t.Errorf("deploy request CommitSHA = %q, want gitea-sha1", fb.lastReq.CommitSHA)
	}
}

// TestDetectWebhookProviderAndEvent checks precedence when a delivery carries more than one discriminator header.
func TestDetectWebhookProviderAndEvent(t *testing.T) {
	tests := []struct {
		name          string
		headers       map[string]string
		wantProvider  string
		wantEventType string
	}{
		{"github", map[string]string{"X-GitHub-Event": "push"}, "github", "push"},
		{"gitlab", map[string]string{"X-Gitlab-Event": "Push Hook"}, "gitlab", "Push Hook"},
		{"bitbucket", map[string]string{"X-Event-Key": "repo:push"}, "bitbucket", "repo:push"},
		{"gitea", map[string]string{"X-Gitea-Event-Type": "push"}, "gitea", "push"},
		{
			name:          "gitea takes precedence over its own github-compat signature header alone",
			headers:       map[string]string{"X-Gitea-Event-Type": "push", "X-Hub-Signature-256": "sha256=irrelevant"},
			wantProvider:  "gitea",
			wantEventType: "push",
		},
		{"unknown", map[string]string{}, "unknown", "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := http.Header{}
			for k, v := range tt.headers {
				h.Set(k, v)
			}
			provider, eventType, _ := detectWebhookProviderAndEvent(h)
			if provider != tt.wantProvider || eventType != tt.wantEventType {
				t.Errorf("detectWebhookProviderAndEvent() = (%q, %q), want (%q, %q)", provider, eventType, tt.wantProvider, tt.wantEventType)
			}
		})
	}
}

// TestVerifyGitPushWebhookAuth checks that a request with none of the known headers fails closed.
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
			name: "gitea valid signature (falls into the github branch via its own X-Hub-Signature-256)",
			// Gitea also sends a bare-hex X-Gitea-Signature this package
			// never reads; only its GitHub-compatible header matters here.
			headers: map[string]string{"X-Hub-Signature-256": sign([]byte(secret), body), "X-Gitea-Signature": "unused-bare-hex"},
			want:    true,
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
	attempts, err := db.ListDeployAttempts(context.Background(), "web")
	if err != nil {
		t.Fatalf("ListDeployAttempts() error = %v", err)
	}
	if len(attempts) != 1 {
		t.Fatalf("deploy attempts = %d, want 1 failed attempt recorded for the fetch failure", len(attempts))
	}
	if attempts[0].Status != store.DeployAttemptStatusFailed || !strings.Contains(attempts[0].Error, "connection refused") {
		t.Errorf("attempt = status %q error %q, want failed with the fetch error", attempts[0].Status, attempts[0].Error)
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

// servicesSpecBody uses the same shape as the manual deploy-spec path, so git and API requests match.
func servicesSpecBody() string {
	return `{"repo_url":"https://github.com/org/web.git","branch":"main","services":{
		"web":{"build":{"type":"dockerfile","path":"./web/Dockerfile"},"port":3000},
		"worker":{"build":{"type":"dockerfile","path":"./worker/Dockerfile"}}
	}}`
}

// TestHandleGitPushWebhook_ServicesSpec_RoutesThroughDeploySpec checks a services map fans out through DeploySpec.
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

// TestHandleGitPushWebhook_ServicesSpec_PartialFailure_OneServiceFailing checks other services still deploy and the result is 207.
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

// TestGitSourceTriggerMatchesPush covers the trigger decision table, including an empty pre-migration mode acting as push.
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

// TestParseGitHubReleaseEvent checks a missing action is rejected, while tag_name is only required once published.
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

// TestHandleGitPushWebhook_ReleaseMode_BranchPush_Ignored checks a release-mode source ignores ordinary branch pushes.
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

// TestHandleGitPushWebhook_ReleaseMode_TagPush_TriggersDeploy checks a tag push deploys using its own commit SHA.
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

// TestHandleGitPushWebhook_PushMode_TagPush_Ignored checks push mode ignores tag pushes.
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

// TestHandleGitPushWebhook_GitHubPing_Returns200 guards against GitHub's ping event being rejected as a malformed push.
func TestHandleGitPushWebhook_GitHubPing_Returns200(t *testing.T) {
	rt, created := newConnectedGitSourceRouter(t, `{"repo_url":"https://github.com/org/web.git","branch":"main"}`)

	fb := &fakeBuilder{tag: "web:v1.2.3"}
	rt.builder = fb

	body := []byte(`{"zen":"Non-blocking is better than blocking.","hook_id":1}`)
	rec := postGitWebhook(t, rt, "X-Hub-Signature-256", sign([]byte(created.WebhookSecret), body), body, map[string]string{"X-GitHub-Event": "ping"})
	requireStatusOK(t, rec)
	if fb.calls != 0 {
		t.Errorf("builder called %d times, want 0: a ping event must never deploy", fb.calls)
	}
}

// TestHandleGitPushWebhook_GitHubReleasePublished_TriggersDeploy checks a published release deploys its tag ref.
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

// TestHandleGitPushWebhook_GitHubReleaseNotPublished_Ignored checks only the published action deploys.
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

// TestHandleGitPushWebhook_GitHubReleaseEvent_PushModeIgnored checks a release event on a push-mode source is accepted and ignored, not a 400.
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

// TestHandleGitPushWebhook_ReleaseMode_BitbucketTagPush_KnownGap documents that Bitbucket tag pushes never match; it fails closed, so a fix must update it.
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

// TestHandleGitPushWebhook_ServicesSpec_TakesPriorityOverAdditionalServices checks a row with both fields deploys once, services first.
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

// newGitHubAppGitSourceRouter adds a connected GitHub App whose installation token is ghs_installtoken.
func newGitHubAppGitSourceRouter(t *testing.T) (*Router, gitSourceResource) {
	t.Helper()
	appSecrets := newFakeGitHubAppSecrets()
	db := openTestDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db, WithGitSourceSecrets(newFakeGitSourceSecrets()), WithGitHubAppSecrets(appSecrets))
	rt.githubAppClient = &fakeGitHubAppClient{mintToken: githubapp.InstallationToken{Token: "ghs_installtoken"}}
	seedInstalledGitHubApp(t, db, appSecrets)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	return rt, connectGitSource(t, rt, cookie, `{"repo_url":"https://github.com/octocat/web.git","branch":"main","build_type":"dockerfile"}`)
}

// postSignedPush sends a push to ref with the connected source's real
// webhook secret.
func postSignedPush(t *testing.T, rt *Router, created gitSourceResource, ref string) *httptest.ResponseRecorder {
	t.Helper()
	body := pushBody(ref)
	return postGitWebhook(t, rt, "X-Hub-Signature-256", sign([]byte(created.WebhookSecret), body), body, nil)
}

// TestHandleGitPushWebhook_PrivateRepo_FallsBackToGitHubAppToken checks a private repo clones with the App token when no deploy token is stored.
func TestHandleGitPushWebhook_PrivateRepo_FallsBackToGitHubAppToken(t *testing.T) {
	rt, created := newGitHubAppGitSourceRouter(t)
	var fetchCalls []fakeGitSourceFetchCall
	rt.gitSourceFetch = newFakeGitSourceFetch(&fetchCalls, t.TempDir(), new(bool), nil)
	rt.builder = &fakeBuilder{tag: "web:sha1"}

	requireStatusOK(t, postSignedPush(t, rt, created, "refs/heads/main"))
	if len(fetchCalls) != 1 || fetchCalls[0].token != "ghs_installtoken" {
		t.Fatalf("fetch calls = %+v, want one call with the minted installation token", fetchCalls)
	}
}

// TestHandleGitPushWebhook_IgnoredBranch_StoresNote proves a push to a
// branch the app does not watch is a 200 whose reason is persisted on the
// delivery row, so the Source page and CLI can say why nothing deployed.
func TestHandleGitPushWebhook_IgnoredBranch_StoresNote(t *testing.T) {
	rt, created := newGitHubAppGitSourceRouter(t)
	fb := &fakeBuilder{tag: "web:sha1"}
	rt.builder = fb

	requireStatusOK(t, postSignedPush(t, rt, created, "refs/heads/develop"))
	if fb.calls != 0 {
		t.Fatalf("builder called %d times, want 0 for an ignored branch", fb.calls)
	}
	deliveries, err := rt.webhookDeliveries.(*store.DB).ListWebhookDeliveries(context.Background(), "web", 10, nil)
	if err != nil || len(deliveries) != 1 {
		t.Fatalf("ListWebhookDeliveries() = %d rows, err = %v, want 1", len(deliveries), err)
	}
	if got := deliveries[0]; got.Error != "" || !strings.Contains(got.Note, "refs/heads/develop") || !strings.Contains(got.Note, "refs/heads/main") {
		t.Errorf("delivery error = %q note = %q, want no error and a note naming both refs", got.Error, got.Note)
	}
}

// TestResolveGitSourceDeployToken_FallsBackToGitHubAppToken covers the
// shared resolver scheduled deploys and preview environments call, not
// just the push path.
func TestResolveGitSourceDeployToken_FallsBackToGitHubAppToken(t *testing.T) {
	rt, _ := newGitHubAppGitSourceRouter(t)
	got, err := rt.resolveGitSourceDeployToken(context.Background(), "web")
	if err != nil || got != "ghs_installtoken" {
		t.Fatalf("resolveGitSourceDeployToken() = %q, %v, want the installation token", got, err)
	}
}

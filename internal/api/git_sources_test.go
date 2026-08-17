package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeGitSourceSecrets is a hand-written fake for GitSourceSecrets, the
// same pattern fakeSecretSetter/fakeBackupSecretsSetter already
// establish, extended with Resolve/Exists (this interface, unlike those
// two, is genuinely read back within this package: see GitSourceSecrets'
// own doc comment).
type fakeGitSourceSecrets struct {
	mu     sync.Mutex
	values map[string]map[string]string

	setErr, resolveErr, existsErr error
	setCalls                      int
}

func newFakeGitSourceSecrets() *fakeGitSourceSecrets {
	return &fakeGitSourceSecrets{values: map[string]map[string]string{}}
}

func (f *fakeGitSourceSecrets) SetValue(_ context.Context, serviceName, envKey, plaintext string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.setCalls++
	if f.setErr != nil {
		return f.setErr
	}
	if f.values[serviceName] == nil {
		f.values[serviceName] = map[string]string{}
	}
	f.values[serviceName][envKey] = plaintext
	return nil
}

func (f *fakeGitSourceSecrets) Resolve(_ context.Context, serviceName, envKey string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.resolveErr != nil {
		return "", f.resolveErr
	}
	v, ok := f.values[serviceName][envKey]
	if !ok {
		return "", errors.New("fakeGitSourceSecrets: value not found")
	}
	return v, nil
}

func (f *fakeGitSourceSecrets) Exists(_ context.Context, serviceName, envKey string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.existsErr != nil {
		return false, f.existsErr
	}
	_, ok := f.values[serviceName][envKey]
	return ok, nil
}

func newTestRouterWithGitSourceSecrets(t *testing.T, secrets GitSourceSecrets) (*Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	return NewRouter(discardLogger(), testBrand(), db, WithGitSourceSecrets(secrets)), db
}

func TestHandleSetGitSource_NotConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithGitSourceSecrets
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/git-source", `{"repo_url":"https://github.com/org/web.git"}`))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}
}

func TestHandleSetGitSource_AppNotFound(t *testing.T) {
	rt, db := newTestRouterWithGitSourceSecrets(t, newFakeGitSourceSecrets())
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/ghost/git-source", `{"repo_url":"https://github.com/org/web.git"}`))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleSetGitSource_MissingRepoURL(t *testing.T) {
	rt, db := newTestRouterWithGitSourceSecrets(t, newFakeGitSourceSecrets())
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/git-source", `{}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleSetGitSource_RejectsNonHTTPScheme(t *testing.T) {
	rt, db := newTestRouterWithGitSourceSecrets(t, newFakeGitSourceSecrets())
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/git-source", `{"repo_url":"file:///etc/passwd"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleSetGitSource_RejectsComposeBuildType(t *testing.T) {
	rt, db := newTestRouterWithGitSourceSecrets(t, newFakeGitSourceSecrets())
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/git-source", `{"repo_url":"https://github.com/org/web.git","build_type":"compose"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

// TestHandleSetGitSource_RejectsImageBuildType proves a persistent git
// source cannot be pointed at build.type: image: a pinned image has no
// build to redeploy on the next push, so this must fail loudly rather
// than silently connect a webhook that would never do anything useful.
func TestHandleSetGitSource_RejectsImageBuildType(t *testing.T) {
	rt, db := newTestRouterWithGitSourceSecrets(t, newFakeGitSourceSecrets())
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/git-source", `{"repo_url":"https://github.com/org/web.git","build_type":"image"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "/deploys") {
		t.Errorf("body = %s, want it to point callers at POST .../deploys", rec.Body.String())
	}
}

func TestHandleSetGitSource_Create_Success(t *testing.T) {
	secrets := newFakeGitSourceSecrets()
	rt, db := newTestRouterWithGitSourceSecrets(t, secrets)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/git-source",
		`{"repo_url":"https://github.com/org/web.git","branch":"main","build_type":"dockerfile"}`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var got gitSourceResource
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.RepoURL != "https://github.com/org/web.git" || got.Branch != "main" || got.BuildType != "dockerfile" {
		t.Errorf("response = %+v, want fields to match the request", got)
	}
	if got.HasToken {
		t.Errorf("HasToken = true, want false: no token was set")
	}
	if got.WebhookURL != "/api/v1/webhooks/github/web" {
		t.Errorf("WebhookURL = %q, want /api/v1/webhooks/github/web", got.WebhookURL)
	}
	if got.WebhookSecret == "" {
		t.Errorf("WebhookSecret = %q, want a non-empty generated secret on the create response", got.WebhookSecret)
	}

	stored, err := db.GetGitSource(context.Background(), "web")
	if err != nil {
		t.Fatalf("GetGitSource() error = %v", err)
	}
	if stored.RepoURL != got.RepoURL || stored.Branch != got.Branch {
		t.Errorf("stored = %+v, want it to match the response", stored)
	}

	resolved, err := secrets.Resolve(context.Background(), store.GitSourceSecretsKey("web"), gitSourceSecretKey)
	if err != nil {
		t.Fatalf("Resolve(webhook_secret) error = %v", err)
	}
	if resolved != got.WebhookSecret {
		t.Errorf("stored webhook secret = %q, want it to match the response's WebhookSecret %q", resolved, got.WebhookSecret)
	}
}

func TestHandleSetGitSource_Create_WithToken(t *testing.T) {
	secrets := newFakeGitSourceSecrets()
	rt, db := newTestRouterWithGitSourceSecrets(t, secrets)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/git-source",
		`{"repo_url":"https://github.com/org/web.git","token":"ghp_supersecret"}`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	if strings.Contains(rec.Body.String(), "ghp_supersecret") {
		t.Errorf("response body = %s, the deploy token must never be echoed back", rec.Body.String())
	}

	var got gitSourceResource
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !got.HasToken {
		t.Errorf("HasToken = false, want true: a token was set")
	}

	resolved, err := secrets.Resolve(context.Background(), store.GitSourceSecretsKey("web"), gitSourceTokenKey)
	if err != nil {
		t.Fatalf("Resolve(deploy_token) error = %v", err)
	}
	if resolved != "ghp_supersecret" {
		t.Errorf("stored token = %q, want ghp_supersecret: the credential must round-trip through internal/secrets unchanged", resolved)
	}
}

func TestHandleSetGitSource_Update_DoesNotReturnOrRotateSecret(t *testing.T) {
	secrets := newFakeGitSourceSecrets()
	rt, db := newTestRouterWithGitSourceSecrets(t, secrets)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")

	createRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(createRec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/git-source", `{"repo_url":"https://github.com/org/web.git","branch":"main"}`))
	var created gitSourceResource
	if err := json.NewDecoder(createRec.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.WebhookSecret == "" {
		t.Fatalf("create response had no webhook secret")
	}

	updateRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(updateRec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/git-source", `{"repo_url":"https://github.com/org/web.git","branch":"release"}`))
	if updateRec.Code != http.StatusOK {
		t.Fatalf("update status = %d, want %d, body = %s", updateRec.Code, http.StatusOK, updateRec.Body.String())
	}
	var updated gitSourceResource
	if err := json.NewDecoder(updateRec.Body).Decode(&updated); err != nil {
		t.Fatalf("decode update response: %v", err)
	}
	if updated.WebhookSecret != "" {
		t.Errorf("WebhookSecret = %q on an update, want empty: it must only ever appear once, on create", updated.WebhookSecret)
	}
	if updated.Branch != "release" {
		t.Errorf("Branch = %q, want release", updated.Branch)
	}

	resolved, err := secrets.Resolve(context.Background(), store.GitSourceSecretsKey("web"), gitSourceSecretKey)
	if err != nil {
		t.Fatalf("Resolve(webhook_secret) error = %v", err)
	}
	if resolved != created.WebhookSecret {
		t.Errorf("webhook secret changed across an unrelated update: got %q, want it to stay %q", resolved, created.WebhookSecret)
	}
}

func TestHandleGetGitSource_NotFound(t *testing.T) {
	rt, db := newTestRouterWithGitSourceSecrets(t, newFakeGitSourceSecrets())
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/git-source", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleGetGitSource_NeverReturnsSecretOrToken(t *testing.T) {
	secrets := newFakeGitSourceSecrets()
	rt, db := newTestRouterWithGitSourceSecrets(t, secrets)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")

	createRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(createRec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/git-source", `{"repo_url":"https://github.com/org/web.git","token":"ghp_supersecret"}`))
	var created gitSourceResource
	if err := json.NewDecoder(createRec.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	getRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(getRec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/git-source", ""))
	if getRec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", getRec.Code, http.StatusOK)
	}
	body := getRec.Body.String()
	if strings.Contains(body, "ghp_supersecret") || strings.Contains(body, created.WebhookSecret) {
		t.Fatalf("GET response body = %s, must never contain the deploy token or webhook secret", body)
	}
	var got gitSourceResource
	if err := json.NewDecoder(strings.NewReader(body)).Decode(&got); err != nil {
		t.Fatalf("decode get response: %v", err)
	}
	if !got.HasToken {
		t.Errorf("HasToken = false, want true")
	}
	if got.WebhookSecret != "" {
		t.Errorf("WebhookSecret = %q, want empty on GET", got.WebhookSecret)
	}
}

func TestHandleDeleteGitSource(t *testing.T) {
	secrets := newFakeGitSourceSecrets()
	rt, db := newTestRouterWithGitSourceSecrets(t, secrets)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")

	rt.Handler().ServeHTTP(httptest.NewRecorder(), authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/git-source", `{"repo_url":"https://github.com/org/web.git"}`))

	delRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(delRec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/web/git-source", ""))
	if delRec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want %d", delRec.Code, http.StatusNoContent)
	}

	getRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(getRec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/git-source", ""))
	if getRec.Code != http.StatusNotFound {
		t.Fatalf("get after delete status = %d, want %d", getRec.Code, http.StatusNotFound)
	}
}

func TestHandleDeleteGitSource_NotFound(t *testing.T) {
	rt, db := newTestRouterWithGitSourceSecrets(t, newFakeGitSourceSecrets())
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/ghost/git-source", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestGitSourceRoutes_RequireAuth(t *testing.T) {
	rt, _ := newTestRouterWithGitSourceSecrets(t, newFakeGitSourceSecrets())

	routes := []struct{ method, target string }{
		{http.MethodGet, "/api/v1/apps/web/git-source"},
		{http.MethodPut, "/api/v1/apps/web/git-source"},
		{http.MethodDelete, "/api/v1/apps/web/git-source"},
	}
	for _, r := range routes {
		t.Run(r.method+" "+r.target, func(t *testing.T) {
			req := httptest.NewRequest(r.method, r.target, nil)
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
			}
		})
	}
}

func TestNormalizeGitSourceBuildType(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{name: "empty defaults to dockerfile", in: "", want: "dockerfile"},
		{name: "dockerfile", in: "dockerfile", want: "dockerfile"},
		{name: "railpack", in: "railpack", want: "railpack"},
		{name: "static", in: "static", want: "static"},
		{name: "compose rejected", in: "compose", wantErr: true},
		{name: "image rejected", in: "image", wantErr: true},
		{name: "unrecognized rejected", in: "nixpacks", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeGitSourceBuildType(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("normalizeGitSourceBuildType(%q) error = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
			if err == nil && got != tt.want {
				t.Errorf("normalizeGitSourceBuildType(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestRequireHTTPOrHTTPSScheme(t *testing.T) {
	tests := []struct {
		name    string
		repoURL string
		wantErr bool
	}{
		{name: "https ok", repoURL: "https://github.com/org/web.git"},
		{name: "http ok", repoURL: "http://example.com/web.git"},
		{name: "file rejected", repoURL: "file:///etc/passwd", wantErr: true},
		{name: "ssh rejected", repoURL: "ssh://git@github.com/org/web.git", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := requireHTTPOrHTTPSScheme(tt.repoURL)
			if (err != nil) != tt.wantErr {
				t.Errorf("requireHTTPOrHTTPSScheme(%q) error = %v, wantErr %v", tt.repoURL, err, tt.wantErr)
			}
		})
	}
}

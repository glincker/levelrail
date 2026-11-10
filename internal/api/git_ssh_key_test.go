package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/gitsshkey"
	"github.com/GLINCKER/levelrail/internal/sourcecache"
	"github.com/GLINCKER/levelrail/internal/store"
)

// sshKeyFakeSecrets adds the delete the real secrets store has.
type sshKeyFakeSecrets struct{ *fakeGitSourceSecrets }

func (f sshKeyFakeSecrets) DeleteValue(_ context.Context, service, envKey string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.values[service], envKey)
	return nil
}

func TestGitSSHKeyLifecycleNeverReturnsPrivateKey(t *testing.T) {
	secrets := sshKeyFakeSecrets{newFakeGitSourceSecrets()}
	rt, db := newTestRouterWithGitSourceSecrets(t, secrets)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	if err := db.SaveGitSource(context.Background(), store.GitSource{
		ServiceName: "web", RepoURL: "git@git.example.com:acme/web.git", Branch: "main", BuildType: "dockerfile",
	}); err != nil {
		t.Fatal(err)
	}
	host, err := gitsshkey.Generate()
	if err != nil {
		t.Fatal(err)
	}
	do := func(method, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, "/api/v1/apps/web/git-source/ssh-key", body))
		return rec
	}

	if rec := do(http.MethodGet, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("GET before set = %d", rec.Code)
	}
	if rec := do(http.MethodPost, `{}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("POST without a host key = %d", rec.Code)
	}
	badPin := strings.NewReplacer("\n", `\n`).Replace(host.PublicKey + "\nnot a key")
	if rec := do(http.MethodPost, `{"host_key":"`+badPin+`"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("POST with an invalid second pin line = %d", rec.Code)
	}

	post := do(http.MethodPost, `{"host_key":"`+host.PublicKey+`"}`)
	if post.Code != http.StatusOK {
		t.Fatalf("POST = %d: %s", post.Code, post.Body)
	}
	secrets.mu.Lock()
	priv := secrets.values[store.GitSourceSecretsKey("web")][gitSSHPrivateKey]
	secrets.mu.Unlock()
	if !strings.Contains(priv, "PRIVATE KEY") {
		t.Fatal("private key was not stored")
	}
	get := do(http.MethodGet, "")
	if get.Code != http.StatusOK || !strings.Contains(get.Body.String(), `"host_key_pinned":true`) {
		t.Fatalf("GET = %d: %s", get.Code, get.Body)
	}
	for name, body := range map[string]string{"POST": post.Body.String(), "GET": get.Body.String()} {
		if strings.Contains(body, "PRIVATE") || strings.Contains(body, strings.TrimSpace(priv)) || strings.Contains(body, "private_key") {
			t.Fatalf("%s response leaks the private key: %s", name, body)
		}
	}

	if rec := do(http.MethodDelete, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE = %d", rec.Code)
	}
	if rec := do(http.MethodGet, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("GET after delete = %d", rec.Code)
	}
}

func TestWithSSHDeployKeyFailsClosedWhenKeyUnusable(t *testing.T) {
	secrets := newFakeGitSourceSecrets()
	rt, _ := newTestRouterWithGitSourceSecrets(t, secrets)
	key := store.GitSourceSecretsKey("web")
	_ = secrets.SetValue(context.Background(), key, gitSSHPrivateKey, "not a key")
	_ = secrets.SetValue(context.Background(), key, gitSSHHostKey, "not a host key")

	ctx := sourcecacheScope("app:web")
	got := rt.withSSHDeployKey(ctx, "git@git.example.com:acme/web.git")
	if got == ctx {
		t.Fatal("an unusable stored key must poison the context, not pass it through")
	}
}

func sourcecacheScope(scope string) context.Context {
	return sourcecache.WithScope(context.Background(), scope)
}

func TestSSHTriggerRefusal(t *testing.T) {
	secrets := sshKeyFakeSecrets{newFakeGitSourceSecrets()}
	rt, db := newTestRouterWithGitSourceSecrets(t, secrets)
	ctx := context.Background()
	seedApp(t, db, "web")
	const repo = "ssh://git@git.example.com/acme/web.git"
	if msg := rt.sshTriggerRefusal(ctx, "web", repo); !strings.Contains(msg, "connected git source") {
		t.Fatalf("no git source: %q", msg)
	}
	if err := db.SaveGitSource(ctx, store.GitSource{ServiceName: "web", RepoURL: repo, Branch: "main", BuildType: "dockerfile"}); err != nil {
		t.Fatal(err)
	}
	if msg := rt.sshTriggerRefusal(ctx, "web", "ssh://git@other.example.com/acme/web.git"); !strings.Contains(msg, "connected git source") {
		t.Fatalf("different repo: %q", msg)
	}
	if msg := rt.sshTriggerRefusal(ctx, "web", repo); !strings.Contains(msg, "deploy key") {
		t.Fatalf("no key: %q", msg)
	}
	if err := secrets.SetValue(ctx, store.GitSourceSecretsKey("web"), gitSSHPrivateKey, "k"); err != nil {
		t.Fatal(err)
	}
	if msg := rt.sshTriggerRefusal(ctx, "web", repo); msg != "" {
		t.Fatalf("key stored and repo matches, got %q", msg)
	}
}

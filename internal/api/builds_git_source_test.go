package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/githubapp"
	"github.com/GLINCKER/levelrail/internal/store"
)

func newBuildGitSourceRouter(t *testing.T) (*Router, *store.DB, *http.Cookie, *fakeGitHubAppClient, *fakeGitSourceSecrets) {
	t.Helper()
	fakeSecrets := newFakeGitHubAppSecrets()
	fake := &fakeGitHubAppClient{mintToken: githubapp.InstallationToken{Token: "ghs"}}
	rt, db := newTestRouterWithGitHubApp(t, fakeSecrets, fake)
	gitSecrets := newFakeGitSourceSecrets()
	rt.gitSourceSecrets = gitSecrets
	rt.builder = newFakeBuilder("levelrail/web:abc123", nil)
	rt.fetch = newFakeFetch(t.TempDir(), nil).fetch
	cookie := loginTestSession(t, rt, db)
	seedInstalledGitHubApp(t, db, fakeSecrets)
	seedWebApp(t, db)
	ctx := context.Background()
	s, _ := db.GetIngressSettings(ctx)
	s.PrimaryDomain = "deploy.example.com"
	if err := db.UpdateIngressSettings(ctx, s); err != nil {
		t.Fatal(err)
	}
	return rt, db, cookie, fake, gitSecrets
}

// postBuildRetrying waits out the previous async build, which holds the app's deploy slot briefly.
func postBuildRetrying(t *testing.T, rt *Router, cookie *http.Cookie, body string) triggerBuildResponse {
	t.Helper()
	for range 100 {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/builds", body))
		if rec.Code == http.StatusAccepted {
			var resp triggerBuildResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatal(err)
			}
			return resp
		}
		if rec.Code != http.StatusConflict {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("build never accepted")
	return triggerBuildResponse{}
}

func TestTriggerBuildConnectsGitSourceAndRegistersWebhookOnce(t *testing.T) {
	rt, db, cookie, fake, _ := newBuildGitSourceRouter(t)
	body := `{"repo_url":"https://github.com/acme/web.git","ref":"main","build":{"type":"railpack","base_directory":"apps/web"}}`

	resp := postBuildRetrying(t, rt, cookie, body)
	if !strings.Contains(resp.GitSourceNote, "registered its push webhook") {
		t.Fatalf("note = %q", resp.GitSourceNote)
	}
	gs, err := db.GetGitSource(context.Background(), "web")
	if err != nil {
		t.Fatalf("git source not written: %v", err)
	}
	if gs.Branch != "main" || gs.BuildType != "railpack" || gs.BaseDirectory != "apps/web" || gs.TriggerMode != "push" {
		t.Fatalf("source = %+v", gs)
	}
	if fake.createHookCount != 1 {
		t.Fatalf("hook registrations = %d, want 1", fake.createHookCount)
	}

	// An idempotent second trigger neither rewrites the source nor registers again.
	resp = postBuildRetrying(t, rt, cookie, body)
	if resp.GitSourceNote != "" || fake.createHookCount != 1 {
		t.Fatalf("second trigger note %q, hooks %d", resp.GitSourceNote, fake.createHookCount)
	}
	if got := getGitSourceResource(t, rt, cookie, "web"); got.WebhookRegistration == nil || got.WebhookRegistration.State != store.GitWebhookRegistered {
		t.Fatalf("registration = %+v", got.WebhookRegistration)
	}
}

func TestTriggerBuildKeepsDifferentExistingGitSource(t *testing.T) {
	rt, db, cookie, fake, _ := newBuildGitSourceRouter(t)
	if err := db.SaveGitSource(context.Background(), store.GitSource{ServiceName: "web", RepoURL: "https://github.com/acme/other.git", Branch: "dev", BuildType: "dockerfile"}); err != nil {
		t.Fatal(err)
	}
	resp := postBuildRetrying(t, rt, cookie, `{"repo_url":"https://github.com/acme/web.git","ref":"main"}`)
	if !strings.Contains(resp.GitSourceNote, "kept the app's existing git source") {
		t.Fatalf("note = %q", resp.GitSourceNote)
	}
	gs, _ := db.GetGitSource(context.Background(), "web")
	if gs.RepoURL != "https://github.com/acme/other.git" || gs.Branch != "dev" || fake.createHookCount != 0 {
		t.Fatalf("source = %+v, hooks %d", gs, fake.createHookCount)
	}
}

func TestTriggerBuildSSHConnectsSourceThenNeedsDeployKey(t *testing.T) {
	rt, db, cookie, fake, gitSecrets := newBuildGitSourceRouter(t)
	body := `{"repo_url":"ssh://git@github.com/acme/web.git","ref":"main"}`

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/builds", body))
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "no deploy key") {
		t.Fatalf("status %d body %s, want the deploy key refusal", rec.Code, rec.Body.String())
	}
	if _, err := db.GetGitSource(context.Background(), "web"); err != nil {
		t.Fatalf("ssh source not connected: %v", err)
	}
	hooksAfterConnect := fake.createHookCount

	if err := gitSecrets.SetValue(context.Background(), store.GitSourceSecretsKey("web"), gitSSHPrivateKey, "key"); err != nil {
		t.Fatal(err)
	}
	postBuildRetrying(t, rt, cookie, body)
	if fake.createHookCount != hooksAfterConnect {
		t.Fatalf("webhook registered again for an existing source")
	}
}

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/glincker/levelrail/internal/githubapp"
	"github.com/glincker/levelrail/internal/store"
)

// hookListingFake adds hook listing to the GitHub fake and remembers created hooks.
type hookListingFake struct {
	*fakeGitHubAppClient
	hooks   []githubapp.RepoWebhook
	listErr error
}

func (f *hookListingFake) ListRepoWebhooks(_ context.Context, _, _, _, _ string) ([]githubapp.RepoWebhook, error) {
	return f.hooks, f.listErr
}

func (f *hookListingFake) CreateRepoWebhook(ctx context.Context, i, t, o, r, hookURL, secret string) error {
	if err := f.fakeGitHubAppClient.CreateRepoWebhook(ctx, i, t, o, r, hookURL, secret); err != nil {
		return err
	}
	var h githubapp.RepoWebhook
	h.Config.URL = hookURL
	f.hooks = append(f.hooks, h)
	return nil
}

func repairSetup(t *testing.T, fake *hookListingFake) (*Router, *http.Cookie) {
	t.Helper()
	fakeSecrets := newFakeGitHubAppSecrets()
	rt, db := newTestRouterWithGitHubApp(t, fakeSecrets, fake.fakeGitHubAppClient)
	rt.githubAppClient = fake
	secrets := newFakeGitSourceSecrets()
	_ = secrets.SetValue(context.Background(), store.GitSourceSecretsKey("web"), gitSourceSecretKey, "whsecret")
	rt.gitSourceSecrets = secrets
	cookie := loginTestSession(t, rt, db)
	seedInstalledGitHubApp(t, db, fakeSecrets)
	seedWebApp(t, db)
	ctx := context.Background()
	s, _ := db.GetIngressSettings(ctx)
	s.PrimaryDomain = "deploy.example.com"
	if err := db.UpdateIngressSettings(ctx, s); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveGitSource(ctx, store.GitSource{ServiceName: "web", RepoURL: "https://github.com/acme/web.git", Branch: "main", BuildType: "dockerfile"}); err != nil {
		t.Fatal(err)
	}
	return rt, cookie
}

func registerHook(t *testing.T, rt *Router, cookie *http.Cookie) gitSourceResource {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/git-source/webhook/register", ""))
	var res gitSourceResource
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("register = %d %s (%v)", rec.Code, rec.Body.String(), err)
	}
	return res
}

func TestWebhookRepairTwiceLeavesOneHook(t *testing.T) {
	fake := &hookListingFake{fakeGitHubAppClient: &fakeGitHubAppClient{
		mintToken: githubapp.InstallationToken{Token: "ghs"},
		repo:      githubapp.Repo{FullName: "acme/web", Name: "web", OwnerLogin: "acme", DefaultBranch: "main"},
	}}
	rt, cookie := repairSetup(t, fake)
	if got := getGitSourceResource(t, rt, cookie, "web"); got.WebhookRegistration == nil || got.WebhookRegistration.State != "missing" {
		t.Fatalf("before repair = %+v, want missing", got.WebhookRegistration)
	}
	registerHook(t, rt, cookie)
	res := registerHook(t, rt, cookie)
	if len(fake.hooks) != 1 || fake.createHookCount != 1 {
		t.Fatalf("hooks = %d creates = %d, want 1 and 1", len(fake.hooks), fake.createHookCount)
	}
	if res.WebhookRegistration == nil || res.WebhookRegistration.State != store.GitWebhookRegistered {
		t.Fatalf("after repair = %+v", res.WebhookRegistration)
	}
}

func TestWebhookRepairNamesMissingPermission(t *testing.T) {
	fake := &hookListingFake{
		fakeGitHubAppClient: &fakeGitHubAppClient{
			mintToken: githubapp.InstallationToken{Token: "ghs"},
			repo:      githubapp.Repo{FullName: "acme/web", Name: "web", OwnerLogin: "acme", DefaultBranch: "main"},
		},
		listErr: githubapp.ErrPermissionDenied,
	}
	rt, cookie := repairSetup(t, fake)
	res := registerHook(t, rt, cookie)
	r := res.WebhookRegistration
	if r == nil || r.State != store.GitWebhookFailed || r.MissingPermission != hooksPermissionName {
		t.Fatalf("registration = %+v, want failed with %q", r, hooksPermissionName)
	}
	if got := getGitSourceResource(t, rt, cookie, "web").WebhookRegistration; got == nil || got.MissingPermission != hooksPermissionName {
		t.Fatalf("get = %+v, want the missing permission kept", got)
	}
}

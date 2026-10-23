package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/bitbucketserver"
	"github.com/GLINCKER/levelrail/internal/giteaapp"
	"github.com/GLINCKER/levelrail/internal/gitprovider"
	"github.com/GLINCKER/levelrail/internal/repolayout"
)

type fakeBitbucketServerClient struct {
	accounts   map[string]bitbucketserver.Account
	repo       bitbucketserver.Repo
	repos      []bitbucketserver.Repo
	branches   []bitbucketserver.Branch
	hookCalled bool
	hookRepo   string
	hookToken  string
	hookURL    string
	hookSecret string
	deleted    string
}

func (f *fakeBitbucketServerClient) CurrentUser(_ context.Context, _, token string) (bitbucketserver.Account, error) {
	a, ok := f.accounts[token]
	if !ok {
		return bitbucketserver.Account{}, errors.New("401")
	}
	return a, nil
}

func (f *fakeBitbucketServerClient) ListRepos(context.Context, string, string, gitprovider.ListOptions) ([]bitbucketserver.Repo, gitprovider.ListResult, error) {
	return f.repos, gitprovider.ListResult{}, nil
}

func (f *fakeBitbucketServerClient) GetRepo(context.Context, string, string, string) (bitbucketserver.Repo, error) {
	return f.repo, nil
}

func (f *fakeBitbucketServerClient) DefaultBranch(context.Context, string, string, string) (string, error) {
	return f.repo.DefaultBranch, nil
}

func (f *fakeBitbucketServerClient) ListBranches(context.Context, string, string, string) ([]bitbucketserver.Branch, error) {
	return f.branches, nil
}

func (f *fakeBitbucketServerClient) CreateRepoWebhook(_ context.Context, _, token, repo, hookURL, secret string) error {
	f.hookCalled, f.hookToken, f.hookRepo, f.hookURL, f.hookSecret = true, token, repo, hookURL, secret
	return nil
}

func (f *fakeBitbucketServerClient) DeleteRepoWebhook(_ context.Context, _, _, repo, _ string) error {
	f.deleted = repo
	return nil
}

func TestBitbucketServerConnection_CreateWithToken(t *testing.T) {
	fake := &fakeBitbucketServerClient{accounts: map[string]bitbucketserver.Account{"pat-1": {ID: "42", Login: "jane"}}}
	secrets := newFakeGitConnSecrets()
	rt, db := newGitConnTestRouter(t, secrets)
	rt.bitbucketServerClient = fake
	cookie := loginTestSession(t, rt, db)

	tests := []struct {
		name, body string
		want       int
	}{
		{"missing token", `{"provider":"bitbucket_server","instance_url":"https://bitbucket.corp"}`, http.StatusBadRequest},
		{"missing url", `{"provider":"bitbucket_server","token":"pat-1"}`, http.StatusBadRequest},
		{"bad scheme", `{"provider":"bitbucket_server","instance_url":"ftp://bitbucket.corp","token":"pat-1"}`, http.StatusBadRequest},
		{"rejected token", `{"provider":"bitbucket_server","instance_url":"https://bitbucket.corp","token":"nope"}`, http.StatusBadGateway},
		{"ok", `{"provider":"bitbucket_server","instance_url":"https://bitbucket.corp/","token":"pat-1"}`, http.StatusCreated},
		{"same account again", `{"provider":"bitbucket_server","instance_url":"https://bitbucket.corp","token":"pat-1"}`, http.StatusConflict},
	}
	var created gitConnectionResource
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := gcRequest(t, rt, cookie, http.MethodPost, "/api/v1/git-connections", tc.body)
			if rec.Code != tc.want {
				t.Fatalf("status %d, want %d: %s", rec.Code, tc.want, rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), "pat-1") {
				t.Fatalf("response leaks the token: %s", rec.Body.String())
			}
			if tc.want == http.StatusCreated {
				created = decodeConn(t, rec)
			}
		})
	}
	if created.Provider != "bitbucket_server" || !created.Authorized || created.AccountName != "jane" || created.Host != "bitbucket.corp" || created.AuthorizePath != "" {
		t.Fatalf("created = %+v", created)
	}
	if created.Label != "jane @ bitbucket.corp" || !created.IsDefault {
		t.Errorf("label/default = %q/%v", created.Label, created.IsDefault)
	}
	if got := listConns(t, rt, cookie, "bitbucket_server"); len(got) != 1 {
		t.Errorf("connections = %d, want 1 (failed attempts must leave nothing behind)", len(got))
	}
	conn, token, err := rt.gitConnToken(t.Context(), created.ID)
	if err != nil || token != "pat-1" || conn.InstanceURL != "https://bitbucket.corp" {
		t.Fatalf("gitConnToken = %q, %+v, %v: a personal access token must be usable without a refresh", token, conn, err)
	}
}

func TestBitbucketServerConnection_UseAsSourceAndRepoList(t *testing.T) {
	fake := &fakeBitbucketServerClient{
		repo:  bitbucketserver.Repo{FullName: "PROJ/app", CloneURL: "https://bitbucket.corp/scm/proj/app.git", DefaultBranch: "develop"},
		repos: []bitbucketserver.Repo{{FullName: "PROJ/app", Name: "app", CloneURL: "https://bitbucket.corp/scm/proj/app.git", Private: true}},
	}
	secrets := newFakeGitConnSecrets()
	gitSecrets := newFakeGitSourceSecrets()
	rt, db := newTestRouterWithProviderAndGitSource(t, secrets, gitSecrets)
	rt.bitbucketServerClient = fake
	cookie := loginTestSession(t, rt, db)
	setPrimaryDomain(t, db)
	conn := seedGitConn(t, db, secrets, gitProviderBitbucketServer, "bitbucket.corp", "jane", true)
	seedApp(t, db, "web")

	rec := gcRequest(t, rt, cookie, http.MethodPost, "/api/v1/git-connections/"+conn.ID+"/use-as-source", `{"app_name":"web","repo":"PROJ/app"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	if !fake.hookCalled || fake.hookToken != "at-jane" || fake.hookRepo != "PROJ/app" {
		t.Errorf("webhook call = %+v", fake)
	}
	gs, err := db.GetGitSource(t.Context(), "web")
	if err != nil || gs.Branch != "develop" || gs.RepoURL != "https://bitbucket.corp/scm/proj/app.git" {
		t.Fatalf("source = %+v, err %v: the server's default branch must be used", gs, err)
	}

	list := gcRequest(t, rt, cookie, http.MethodGet, "/api/v1/git/repos?provider=bitbucket_server", "")
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "PROJ/app") {
		t.Fatalf("repo list status %d body %s", list.Code, list.Body.String())
	}
}

func TestForgejoConnection_UsesGiteaAPIUnderItsOwnKind(t *testing.T) {
	fake := &fakeGiteaAppClient{
		repos: []giteaapp.Repo{{FullName: "me/site", Name: "site", CloneURL: "https://codeberg.example/me/site.git", DefaultBranch: "main"}},
	}
	secrets := newFakeGitConnSecrets()
	rt, db := newTestRouterWithProviderAndGitSource(t, secrets, newFakeGitSourceSecrets())
	rt.giteaAppClient = fake
	cookie := loginTestSession(t, rt, db)

	rec := gcRequest(t, rt, cookie, http.MethodPost, "/api/v1/git-connections", `{"provider":"forgejo","instance_url":"https://codeberg.example","client_id":"cid","client_secret":"sec"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	c := decodeConn(t, rec)
	if c.Provider != "forgejo" || !strings.HasPrefix(c.AuthorizePath, "/api/v1/forgejo-app/connect?connection_id=") {
		t.Fatalf("connection = %+v", c)
	}
	if got := listConns(t, rt, cookie, "gitea"); len(got) != 0 {
		t.Errorf("forgejo connection leaked into gitea list: %+v", got)
	}
	a, ok := rt.gitAdapter("forgejo")
	if !ok || a.label != "Forgejo" || a.callbackPath != "/api/v1/forgejo-app/callback" || a.patAuth {
		t.Fatalf("adapter = %+v", a)
	}

	seedForgejo := seedGitConn(t, db, secrets, gitProviderForgejo, "codeberg.example", "me", true)
	list := gcRequest(t, rt, cookie, http.MethodGet, "/api/v1/git/repos?provider=forgejo&connection="+seedForgejo.ID, "")
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "me/site") {
		t.Fatalf("repo list status %d body %s", list.Code, list.Body.String())
	}
}

type defaultBranchSource struct {
	fakeLayoutSource
	branch string
	err    error
}

func (d *defaultBranchSource) DefaultBranch(context.Context) (string, error) { return d.branch, d.err }

func TestAnalyzeBranchCandidates(t *testing.T) {
	tests := []struct {
		name string
		src  repolayout.Source
		want []string
	}{
		{"provider answers", &defaultBranchSource{branch: "trunk"}, []string{"trunk"}},
		{"provider errors", &defaultBranchSource{err: errors.New("403")}, []string{"main", "master"}},
		{"provider returns nothing", &defaultBranchSource{branch: " "}, []string{"main", "master"}},
		{"source cannot tell", &fakeLayoutSource{}, []string{"main", "master"}},
	}
	rt := &Router{}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			open := func(context.Context, string, string, string, repolayout.Hosts) (repolayout.Source, error) {
				return tc.src, nil
			}
			got := rt.analyzeBranchCandidates(t.Context(), open, "https://x/y/z", "", repolayout.Hosts{})
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

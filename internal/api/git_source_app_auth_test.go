package api

import (
	"context"
	"testing"

	"github.com/GLINCKER/levelrail/internal/githubapp"
)

const appAuthRepoURL = "https://github.com/acme/widgets.git"

func teamScopedCtx() context.Context {
	return context.WithValue(context.Background(), teamScopeKey{}, &teamScope{roles: map[string]string{"t1": "admin"}})
}

func instanceScopedCtx() context.Context {
	return context.WithValue(context.Background(), teamScopeKey{}, (*teamScope)(nil))
}

func newAppAuthRouter(t *testing.T) *Router {
	t.Helper()
	fakeClient := &fakeGitHubAppClient{mintToken: githubapp.InstallationToken{Token: "ghs_installtoken"}}
	rt, db := newPrivateRepoAuthRouter(t, newFakeBuilder("img:1", nil), newFakeFetch(t.TempDir(), nil), fakeClient)
	rt.gitSources = db
	rt.gitSourceSecrets = newFakeGitSourceSecrets()
	seedWebApp(t, db)
	return rt
}

func TestGitSourceAppAuthFollowsCallerScope(t *testing.T) {
	cases := []struct {
		name string
		ctx  context.Context
		want bool
	}{
		{"team scoped", teamScopedCtx(), false},
		{"instance level", instanceScopedCtx(), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rt := newAppAuthRouter(t)
			res, err := rt.connectGitSource(c.ctx, "web", connectGitSourceParams{RepoURL: appAuthRepoURL, Branch: "main", BuildType: "dockerfile"})
			if err != nil {
				t.Fatal(err)
			}
			if res.Resource.AppAuth != c.want {
				t.Fatalf("response app_auth = %v, want %v", res.Resource.AppAuth, c.want)
			}
			gs, err := rt.gitSources.GetGitSource(context.Background(), "web")
			if err != nil {
				t.Fatal(err)
			}
			if gs.AppAuth != c.want {
				t.Fatalf("stored app_auth = %v, want %v", gs.AppAuth, c.want)
			}
		})
	}
}

func TestTokenForRepoStoredFlowsHonourAppAuth(t *testing.T) {
	for _, appAuth := range []bool{true, false} {
		rt := newAppAuthRouter(t)
		ctx := context.Background()
		if _, err := rt.connectGitSource(ctx, "web", connectGitSourceParams{RepoURL: appAuthRepoURL, Branch: "main", BuildType: "dockerfile"}); err != nil {
			t.Fatal(err)
		}
		if err := rt.gitSources.SetGitSourceAppAuth(ctx, "web", appAuth); err != nil {
			t.Fatal(err)
		}
		got := rt.GitSourceCloneToken(ctx, "web", appAuthRepoURL)
		if (got != "") != appAuth {
			t.Fatalf("app_auth=%v: token = %q", appAuth, got)
		}
	}
}

func TestNewlyGatedPathsGiveTeamScopedCallersNoAppToken(t *testing.T) {
	paths := []string{"release notes", "build settings", "source commits"}
	for _, p := range paths {
		for _, c := range []struct {
			name string
			ctx  context.Context
			want bool
		}{{"team scoped", teamScopedCtx(), false}, {"instance level", instanceScopedCtx(), true}} {
			t.Run(p+" "+c.name, func(t *testing.T) {
				rt := newAppAuthRouter(t)
				got := rt.tokenForRepo(c.ctx, "web", appAuthRepoURL, gitHubAppTokenAllowed(c.ctx))
				if (got != "") != c.want {
					t.Fatalf("token = %q, want present = %v", got, c.want)
				}
			})
		}
	}
}

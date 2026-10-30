package api

import (
	"context"
	"testing"

	"github.com/GLINCKER/levelrail/internal/githubapp"
)

func TestGitHubAppTokenAllowedByCallerScope(t *testing.T) {
	cases := []struct {
		name  string
		ctx   func() context.Context
		allow bool
	}{
		{"instance level", func() context.Context {
			return context.WithValue(context.Background(), teamScopeKey{}, (*teamScope)(nil))
		}, true},
		{"default team member", func() context.Context {
			return context.WithValue(context.Background(), teamScopeKey{}, &teamScope{instance: true})
		}, true},
		{"team scoped", func() context.Context {
			return context.WithValue(context.Background(), teamScopeKey{}, &teamScope{roles: map[string]string{"t1": "admin"}})
		}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctx := c.ctx()
			fakeClient := &fakeGitHubAppClient{mintToken: githubapp.InstallationToken{Token: "ghs_installtoken"}}
			rt, _ := newPrivateRepoAuthRouter(t, newFakeBuilder("img:1", nil), newFakeFetch(t.TempDir(), nil), fakeClient)
			got := rt.tokenForRepo(ctx, "web", "https://github.com/acme/widgets.git", gitHubAppTokenAllowed(ctx))
			if (got != "") != c.allow {
				t.Fatalf("token = %q, want present = %v", got, c.allow)
			}
		})
	}
}

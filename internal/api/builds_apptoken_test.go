package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
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

func TestCloneTokenRequestNamesOneRepoAndReadPermissions(t *testing.T) {
	var gotPath string
	var gotBody struct {
		Repositories []string          `json:"repositories"`
		Permissions  map[string]string `json:"permissions"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"token":"ghs_narrow","expires_at":"2030-01-01T00:00:00Z"}`))
	}))
	t.Cleanup(srv.Close)

	rt, _ := newPrivateRepoAuthRouter(t, newFakeBuilder("img:1", nil), newFakeFetch(t.TempDir(), nil), &fakeGitHubAppClient{})
	rt.githubAppClient = &githubapp.Client{BaseURL: srv.URL, HTTP: srv.Client()}

	got := rt.tokenForRepo(context.Background(), "", "https://github.com/acme/widgets.git", true)
	if got != "ghs_narrow" {
		t.Fatalf("token = %q, want the narrowed token", got)
	}
	if gotPath != "/app/installations/7/access_tokens" {
		t.Errorf("path = %q", gotPath)
	}
	if !reflect.DeepEqual(gotBody.Repositories, []string{"widgets"}) {
		t.Errorf("repositories = %v, want [widgets]", gotBody.Repositories)
	}
	want := map[string]string{"contents": "read", "metadata": "read"}
	if !reflect.DeepEqual(gotBody.Permissions, want) {
		t.Errorf("permissions = %v, want %v", gotBody.Permissions, want)
	}
}

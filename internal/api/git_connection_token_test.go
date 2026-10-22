package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/bitbucketapp"
	"github.com/GLINCKER/levelrail/internal/giteaapp"
	"github.com/GLINCKER/levelrail/internal/gitlabapp"
)

type fakeHost struct {
	wantAuth string
	scopes   string
	noScopes bool
	repos    int
}

func (f fakeHost) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != f.wantAuth {
			http.Error(w, `{"message":"401 Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		p := r.URL.Path
		switch {
		case strings.HasSuffix(p, "/personal_access_tokens/self"):
			if f.noScopes {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write([]byte(`{"name":"levelrail","scopes":[` + f.scopes + `]}`))
		case strings.HasSuffix(p, "/user"):
			_, _ = w.Write([]byte(`{"id":7,"login":"jane","username":"jane","account_id":"acct-7","nickname":"jane"}`))
		case strings.Contains(p, "/projects"):
			_, _ = w.Write([]byte(fakeList(f.repos, `{"id":1,"path_with_namespace":"g/p","name":"p"}`)))
		case strings.HasSuffix(p, "/repositories"):
			_, _ = w.Write([]byte(`{"values":[` + strings.TrimSuffix(strings.TrimPrefix(fakeList(f.repos, `{"full_name":"w/r","name":"r"}`), "["), "]") + `]}`))
		case strings.Contains(p, "repos"):
			_, _ = w.Write([]byte(fakeList(f.repos, `{"full_name":"o/r","name":"r"}`)))
		default:
			http.NotFound(w, r)
		}
	})
}

func fakeList(n int, item string) string {
	items := make([]string, n)
	for i := range items {
		items[i] = item
	}
	return "[" + strings.Join(items, ",") + "]"
}

func TestProbeGitTokenPerHost(t *testing.T) {
	tests := []struct {
		name, provider, token string
		host                  fakeHost
		wantKind              string
		wantScopesKnown       bool
		wantRepos             int
		wantMissing           string
	}{
		{"gitlab full access", "gitlab", "glpat-ok", fakeHost{wantAuth: "Bearer glpat-ok", scopes: `"api","read_user"`, repos: 3}, probeOK, true, 3, ""},
		{"gitlab read only token", "gitlab", "glpat-ro", fakeHost{wantAuth: "Bearer glpat-ro", scopes: `"read_api"`, repos: 3}, probeBadScope, true, 3, "api"},
		{"gitlab too old to report scopes", "gitlab", "glpat-old", fakeHost{wantAuth: "Bearer glpat-old", noScopes: true, repos: 1}, probeOK, false, 1, ""},
		{"gitlab wrong token", "gitlab", "nope", fakeHost{wantAuth: "Bearer glpat-ok", repos: 1}, probeBadToken, false, 0, ""},
		{"gitea with repositories", "gitea", "gt", fakeHost{wantAuth: "Bearer gt", repos: 2}, probeOK, false, 2, ""},
		{"gitea token without repository access", "gitea", "gt", fakeHost{wantAuth: "Bearer gt", repos: 0}, probeOK, false, 0, ""},
		{"forgejo with repositories", "forgejo", "fj", fakeHost{wantAuth: "Bearer fj", repos: 1}, probeOK, false, 1, ""},
		{"forgejo wrong token", "forgejo", "x", fakeHost{wantAuth: "Bearer fj"}, probeBadToken, false, 0, ""},
		{"bitbucket api token pair", "bitbucket", "me@example.com:tok", fakeHost{wantAuth: "Basic bWVAZXhhbXBsZS5jb206dG9r", repos: 4}, probeOK, false, 4, ""},
		{"bitbucket access token", "bitbucket", "bbtok", fakeHost{wantAuth: "Bearer bbtok", repos: 1}, probeOK, false, 1, ""},
		{"bitbucket wrong token", "bitbucket", "bad", fakeHost{wantAuth: "Bearer bbtok"}, probeBadToken, false, 0, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(tc.host.handler())
			defer srv.Close()
			rt, _ := newGitConnTestRouter(t, newFakeGitConnSecrets())
			rt.gitlabAppClient = &gitlabapp.Client{HTTP: srv.Client()}
			rt.giteaAppClient = &giteaapp.Client{HTTP: srv.Client()}
			rt.bitbucketAppClient = &bitbucketapp.Client{HTTP: srv.Client(), APIBaseURL: srv.URL}
			adapter, ok := rt.gitAdapter(tc.provider)
			if !ok {
				t.Fatalf("no adapter for %s", tc.provider)
			}
			got := rt.probeGitInstance(t.Context(), adapter, gitProbeRequest{Provider: tc.provider, InstanceURL: srv.URL, Token: tc.token})
			if got.Kind != tc.wantKind {
				t.Fatalf("kind = %q (%s), want %q", got.Kind, got.Message, tc.wantKind)
			}
			if tc.wantKind == probeBadToken {
				return
			}
			if got.Account != "jane" {
				t.Errorf("account = %q, want jane", got.Account)
			}
			if got.Token == nil {
				t.Fatal("token details missing")
			}
			if got.Token.ScopesKnown != tc.wantScopesKnown || got.Token.RepoCount != tc.wantRepos || got.Token.RepoAccess != (tc.wantRepos > 0) {
				t.Errorf("details = %+v, want scopesKnown=%v repos=%d", *got.Token, tc.wantScopesKnown, tc.wantRepos)
			}
			if tc.wantMissing != "" && strings.Join(got.Token.Missing, ",") != tc.wantMissing {
				t.Errorf("missing = %v, want %s", got.Token.Missing, tc.wantMissing)
			}
			if strings.Contains(got.Message+got.Fix, tc.token) {
				t.Error("result echoes the token")
			}
		})
	}
}

func TestCreateGitConnectionWithTokenStoresEncryptedAndHidesToken(t *testing.T) {
	srv := httptest.NewServer(fakeHost{wantAuth: "Bearer glpat-ok", scopes: `"api"`, repos: 2}.handler())
	defer srv.Close()
	secrets := newFakeGitConnSecrets()
	rt, db := newGitConnTestRouter(t, secrets)
	rt.gitlabAppClient = &gitlabapp.Client{HTTP: srv.Client()}
	cookie := loginTestSession(t, rt, db)

	rec := gcRequest(t, rt, cookie, http.MethodPost, "/api/v1/git-connections", `{"provider":"gitlab","instance_url":"`+srv.URL+`","token":"glpat-ok"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, "glpat-ok") {
		t.Fatal("response echoes the token")
	}
	for _, want := range []string{`"auth_method":"token"`, `"authorized":true`, `"account_name":"jane"`, `"authorize_path":""`} {
		if !strings.Contains(body, want) {
			t.Errorf("response missing %s: %s", want, body)
		}
	}

	rec = gcRequest(t, rt, cookie, http.MethodPost, "/api/v1/git-connections", `{"provider":"gitlab","instance_url":"`+srv.URL+`","token":"glpat-ok"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("second add status %d, want 409: %s", rec.Code, rec.Body.String())
	}

	ro := httptest.NewServer(fakeHost{wantAuth: "Bearer glpat-ro", scopes: `"read_api"`, repos: 1}.handler())
	defer ro.Close()
	rt.gitlabAppClient = &gitlabapp.Client{HTTP: ro.Client()}
	rec = gcRequest(t, rt, cookie, http.MethodPost, "/api/v1/git-connections", `{"provider":"gitlab","instance_url":"`+ro.URL+`","token":"glpat-ro"}`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "api") {
		t.Fatalf("read-only token status %d: %s", rec.Code, rec.Body.String())
	}
}

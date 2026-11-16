package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/GLINCKER/levelrail/internal/giteaapp"
	"github.com/GLINCKER/levelrail/internal/gitlabapp"
)

// accessFakeGitLab adds the token and group calls the access view probes for.
type accessFakeGitLab struct {
	*fakeGitLabAppClient
	oauthInfo gitlabapp.TokenInfo
	infoErr   error
	groups    []string
	more      bool
	calls     atomic.Int32
}

func (f *accessFakeGitLab) TokenInfo(context.Context, string, string) (gitlabapp.TokenInfo, error) {
	f.calls.Add(1)
	return f.oauthInfo, f.infoErr
}

func (f *accessFakeGitLab) OAuthTokenInfo(context.Context, string, string) (gitlabapp.TokenInfo, error) {
	f.calls.Add(1)
	return f.oauthInfo, f.infoErr
}

func (f *accessFakeGitLab) ListGroups(context.Context, string, string, int) ([]string, bool, error) {
	return f.groups, f.more, nil
}

func getAccess(t *testing.T, rt *Router, cookie *http.Cookie, id string) gitAccessResource {
	t.Helper()
	rec := gcRequest(t, rt, cookie, http.MethodGet, "/api/v1/git-connections/"+id+"/access", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	var out gitAccessResource
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return out
}

func capByID(res gitAccessResource, id string) gitAccessCapability {
	for _, c := range res.Capabilities {
		if c.ID == id {
			return c
		}
	}
	return gitAccessCapability{}
}

func TestGitConnectionAccess_NarrowGitLabScopesNeedScope(t *testing.T) {
	fake := &accessFakeGitLab{
		fakeGitLabAppClient: &fakeGitLabAppClient{},
		oauthInfo:           gitlabapp.TokenInfo{Scopes: []string{"read_api"}, ExpiresAt: "2030-01-01T00:00:00Z"},
		groups:              []string{"acme", "acme/platform"}, more: true,
	}
	secrets := newFakeGitConnSecrets()
	rt, db := newGitConnTestRouter(t, secrets)
	rt.gitlabAppClient = fake
	cookie := loginTestSession(t, rt, db)
	c := seedGitConn(t, db, secrets, gitProviderGitLab, "gitlab.com", "ann", true)

	res := getAccess(t, rt, cookie, c.ID)
	if res.AuthMethod != "oauth_app" || !res.ScopesKnown || res.ScopesSource != "provider" || res.ExpiresAt == "" {
		t.Fatalf("unexpected facts: %+v", res)
	}
	if got := capByID(res, "list_repos"); got.State != "allowed" || !got.Verified {
		t.Errorf("list_repos = %+v", got)
	}
	for _, id := range []string{"register_webhook", "commit_status", "mr_comments"} {
		if got := capByID(res, id); got.State != "needs_scope" || got.NeedsScope != "api" {
			t.Errorf("%s = %+v, want needs_scope api", id, got)
		}
	}
	if got := capByID(res, "clone_sync"); got.State != "needs_scope" || got.NeedsScope != "read_repository" || !got.Verified {
		t.Errorf("clone_sync = %+v", got)
	}
	if res.Visible.Total != 2 || !res.Visible.Truncated {
		t.Errorf("visible = %+v", res.Visible)
	}
	if body := mustMarshal(t, res); strings.Contains(body, "at-ann") || strings.Contains(body, "csecret") {
		t.Errorf("response leaks a credential: %s", body)
	}

	getAccess(t, rt, cookie, c.ID)
	if n := fake.calls.Load(); n != 1 {
		t.Errorf("provider called %d times, want 1 (cached)", n)
	}
}

func TestGitConnectionAccess_ProviderFailureKeepsStoredFacts(t *testing.T) {
	fake := &accessFakeGitLab{fakeGitLabAppClient: &fakeGitLabAppClient{}, infoErr: errors.New("gitlab: 503")}
	secrets := newFakeGitConnSecrets()
	rt, db := newGitConnTestRouter(t, secrets)
	rt.gitlabAppClient = fake
	cookie := loginTestSession(t, rt, db)
	c := seedGitConn(t, db, secrets, gitProviderGitLab, "gitlab.com", "ann", true)

	res := getAccess(t, rt, cookie, c.ID)
	if res.LiveCheckErr == "" || res.ID != c.ID || res.AccountName != "ann" || len(res.Capabilities) != 9 {
		t.Fatalf("want stored facts plus live_check_error, got %+v", res)
	}
	if res.ScopesKnown || capByID(res, "register_webhook").Verified {
		t.Errorf("nothing may be verified without a provider report: %+v", res)
	}
}

func TestGitConnectionAccess_GiteaScopesUnknown(t *testing.T) {
	fake := &fakeGiteaAppClient{repos: []giteaapp.Repo{{FullName: "ann/web"}, {FullName: "acme/api"}, {FullName: "acme/db"}}}
	secrets := newFakeGitConnSecrets()
	rt, db := newGitConnTestRouter(t, secrets)
	rt.giteaAppClient = fake
	cookie := loginTestSession(t, rt, db)
	c := seedGitConn(t, db, secrets, gitProviderGitea, "git.example.com", "ann", true)

	res := getAccess(t, rt, cookie, c.ID)
	if res.ScopesKnown || res.Visible.Total != 2 || res.Visible.Names[0] != "acme" {
		t.Fatalf("unexpected: %+v", res)
	}
	if got := capByID(res, "commit_status"); got.State != "allowed" || got.Verified {
		t.Errorf("commit_status = %+v, want inferred allowed", got)
	}
}

func TestGitConnectionAccess_UnknownConnectionIs404(t *testing.T) {
	rt, db := newGitConnTestRouter(t, newFakeGitConnSecrets())
	cookie := loginTestSession(t, rt, db)
	if rec := gcRequest(t, rt, cookie, http.MethodGet, "/api/v1/git-connections/gc_nope/access", ""); rec.Code != http.StatusNotFound {
		t.Errorf("status %d, want 404", rec.Code)
	}
}

func mustMarshal(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

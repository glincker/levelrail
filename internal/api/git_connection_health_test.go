package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/githubapp"
	"github.com/GLINCKER/levelrail/internal/gitlabapp"
	"github.com/GLINCKER/levelrail/internal/gitprovider"
	"github.com/GLINCKER/levelrail/internal/store"
)

// rotatingGitLab spends a refresh token once: reusing it is the invalid_grant
// a real provider answers, which is what breaks an unserialized refresh.
type rotatingGitLab struct {
	*fakeGitLabAppClient
	mu    sync.Mutex
	valid string
	calls atomic.Int32
}

func (r *rotatingGitLab) RefreshToken(_ context.Context, _, _, _, refresh string) (gitlabapp.Tokens, error) {
	r.calls.Add(1)
	time.Sleep(20 * time.Millisecond)
	r.mu.Lock()
	defer r.mu.Unlock()
	if refresh != r.valid {
		return gitlabapp.Tokens{}, &gitprovider.APIError{Prefix: "gitlabapp", API: "gitlab", StatusCode: http.StatusBadRequest, Body: `{"error":"invalid_grant"}`}
	}
	r.valid = "rt-" + strconv.Itoa(int(r.calls.Load()))
	return gitlabapp.Tokens{AccessToken: "at-" + r.valid, RefreshToken: r.valid, ExpiresAt: time.Now().Add(time.Hour)}, nil
}

func expiredGitLabSecrets(t *testing.T, refresh string) *fakeGitLabAppSecrets {
	t.Helper()
	s := newFakeGitLabAppSecrets()
	key := store.GitLabAppSecretsKey()
	mustSetGitLabSecret(t, s, key, gitLabAppClientSecretKey, "csecret")
	mustSetGitLabSecret(t, s, key, gitLabAppAccessTokenKey, "at-old")
	mustSetGitLabSecret(t, s, key, gitLabAppRefreshTokenKey, refresh)
	mustSetGitLabSecret(t, s, key, gitLabAppTokenExpiresKey, strconv.FormatInt(time.Now().Add(-time.Hour).Unix(), 10))
	return s
}

func TestGitLabAccessToken_ConcurrentCallersRefreshOnce(t *testing.T) {
	rot := &rotatingGitLab{fakeGitLabAppClient: &fakeGitLabAppClient{}, valid: "rt-0"}
	rt, db := newTestRouterWithGitLabApp(t, expiredGitLabSecrets(t, "rt-0"), rot)
	seedGitLabAppConnection(t, db)

	const callers = 16
	tokens := make([]string, callers)
	errs := make([]error, callers)
	var wg sync.WaitGroup
	for i := range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, tokens[i], errs[i] = rt.gitlabAccessToken(t.Context())
		}()
	}
	wg.Wait()

	for i := range callers {
		if errs[i] != nil || tokens[i] != "at-rt-1" {
			t.Fatalf("caller %d got token %q, err %v; want the one refreshed token", i, tokens[i], errs[i])
		}
	}
	if rot.calls.Load() != 1 {
		t.Fatalf("refresh token spent %d times, want exactly once", rot.calls.Load())
	}
	if got := rt.gitConnHealth(t.Context(), gitProviderGitLab); got.Status != store.GitConnOK {
		t.Fatalf("health after a good refresh = %+v", got)
	}
}

func TestRefreshGitOAuth_DiscardsResultWhenRefreshTokenChangedUnderneath(t *testing.T) {
	rt, _ := newTestRouter(t)
	var mu sync.Mutex
	state := gitOAuthState{Access: "old", Refresh: "rt-1", ExpiresUnix: time.Now().Add(-time.Hour).Unix()}
	saved := false
	spec := gitOAuthSpec{
		provider: gitProviderGitLab,
		read: func(context.Context) (gitOAuthState, error) {
			mu.Lock()
			defer mu.Unlock()
			return state, nil
		},
		exchange: func(context.Context, string) (gitOAuthTokens, error) {
			mu.Lock()
			defer mu.Unlock()
			// The operator reconnected while the refresh was in flight.
			state = gitOAuthState{Access: "reconnected", Refresh: "rt-2", ExpiresUnix: time.Now().Add(time.Hour).Unix()}
			return gitOAuthTokens{Access: "stale", Refresh: "rt-1b", Expires: time.Now().Add(time.Hour)}, nil
		},
		save: func(context.Context, gitOAuthTokens) error { saved = true; return nil },
	}
	got, err := rt.refreshGitOAuth(t.Context(), spec)
	if err != nil || got != "reconnected" || saved {
		t.Fatalf("got %q, err %v, saved %v; want the reconnected token and no overwrite", got, err, saved)
	}
}

func TestGitLabRevokedGrantIsTypedNotA500(t *testing.T) {
	rot := &rotatingGitLab{fakeGitLabAppClient: &fakeGitLabAppClient{}, valid: "someone-else"}
	rt, db := newTestRouterWithGitLabApp(t, expiredGitLabSecrets(t, "rt-0"), rot)
	cookie := loginTestSession(t, rt, db)
	seedGitLabAppConnection(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/gitlab-app/projects", ""))
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), `"code":"needs_reauthorize"`) {
		t.Fatalf("status = %d body = %s, want 409 needs_reauthorize", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/git-providers", ""))
	if !strings.Contains(rec.Body.String(), `"provider":"gitlab","connected":true`) || !strings.Contains(rec.Body.String(), `"status":"needs_reauthorize"`) {
		t.Fatalf("git-providers = %s, want gitlab needs_reauthorize", rec.Body.String())
	}
}

func TestGitHubRemovedInstallationIsRevoked(t *testing.T) {
	fakeSecrets := newFakeGitHubAppSecrets()
	fake := &fakeGitHubAppClient{}
	rt, db := newTestRouterWithGitHubApp(t, fakeSecrets, fake)
	cookie := loginTestSession(t, rt, db)
	seedInstalledGitHubApp(t, db, fakeSecrets)
	fake.mintErr = &gitprovider.APIError{Prefix: "githubapp", API: "github", StatusCode: http.StatusNotFound, Body: "Not Found"}

	code, _, _, _ := getRepoList(t, rt, cookie, "provider=github")
	if code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", code)
	}
	if got := rt.gitConnHealth(t.Context(), gitProviderGitHub); got.Status != store.GitConnRevoked {
		t.Fatalf("health = %+v, want revoked", got)
	}
}

func TestGitHubUnauthorizedListIsNeedsReauthorizeAndRecovers(t *testing.T) {
	fakeSecrets := newFakeGitHubAppSecrets()
	fake := &fakeGitHubAppClient{
		mintToken: githubapp.InstallationToken{Token: "ghs"},
		reposErr:  &gitprovider.APIError{Prefix: "githubapp", API: "github", StatusCode: http.StatusUnauthorized, Body: "Bad credentials"},
	}
	rt, db := newTestRouterWithGitHubApp(t, fakeSecrets, fake)
	rt.repoCache = newRepoCacheForTest(0)
	cookie := loginTestSession(t, rt, db)
	seedInstalledGitHubApp(t, db, fakeSecrets)

	if code, _, _, _ := getRepoList(t, rt, cookie, "provider=github"); code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", code)
	}
	if got := rt.gitConnHealth(t.Context(), gitProviderGitHub); got.Status != store.GitConnNeedsReauthorize {
		t.Fatalf("health = %+v", got)
	}

	fake.reposErr = nil
	fake.repos = []githubapp.Repo{{FullName: "acme/a", Name: "a", OwnerLogin: "acme"}}
	if code, _, _, _ := getRepoList(t, rt, cookie, "provider=github&refresh=1"); code != http.StatusOK {
		t.Fatalf("status after reconnect = %d", code)
	}
	if got := rt.gitConnHealth(t.Context(), gitProviderGitHub); got.Status != store.GitConnOK {
		t.Fatalf("health after a good listing = %+v, want ok", got)
	}
}

func TestGitLabDisconnectRevokesTokenAtProvider(t *testing.T) {
	fake := &fakeGitLabAppClient{}
	secrets := authorizedGitLabSecrets(t)
	mustSetGitLabSecret(t, secrets, store.GitLabAppSecretsKey(), gitLabAppRefreshTokenKey, "rt-live")
	rt, db := newTestRouterWithGitLabApp(t, secrets, fake)
	cookie := loginTestSession(t, rt, db)
	seedGitLabAppConnection(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/gitlab-app", ""))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"token_revoked":true`) {
		t.Fatalf("status = %d body = %s, want a revoked token", rec.Code, rec.Body.String())
	}
	if fake.revokedToken != "rt-live" {
		t.Fatalf("revoked %q, want the refresh token", fake.revokedToken)
	}
}

func TestGitLabDisconnectStillSucceedsWhenRevocationFails(t *testing.T) {
	fake := &fakeGitLabAppClient{revokeErr: errors.New("gitlab down")}
	rt, db := newTestRouterWithGitLabApp(t, authorizedGitLabSecrets(t), fake)
	cookie := loginTestSession(t, rt, db)
	seedGitLabAppConnection(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/gitlab-app", ""))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"token_revoked":false`) || !strings.Contains(rec.Body.String(), "Revoke it yourself") {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestBitbucketDisconnectSaysItCannotRevoke(t *testing.T) {
	rt, db := newTestRouterWithBitbucketApp(t, newFakeBitbucketAppSecrets(), &fakeBitbucketAppClient{})
	cookie := loginTestSession(t, rt, db)
	seedBitbucketAppConnection(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/bitbucket-app", ""))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"token_revoked":false`) || !strings.Contains(rec.Body.String(), "no API to revoke") {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestDoctorGitConnectionsWarnsWhenAProviderNeedsReconnect(t *testing.T) {
	rt, db := newTestRouterWithGitLabApp(t, authorizedGitLabSecrets(t), &fakeGitLabAppClient{})
	seedGitLabAppConnection(t, db)
	if got := rt.doctorCheckGitConnections(t.Context()); got.Status != doctorStatusOK {
		t.Fatalf("healthy = %+v", got)
	}
	rt.recordGitConn(t.Context(), &gitConnError{Provider: gitProviderGitLab, Status: store.GitConnNeedsReauthorize, Err: errors.New("x")})
	got := rt.doctorCheckGitConnections(t.Context())
	if got.Status != doctorStatusWarn || !strings.Contains(got.Message, "gitlab (needs_reauthorize)") {
		t.Fatalf("check = %+v, want a warning naming gitlab", got)
	}
}

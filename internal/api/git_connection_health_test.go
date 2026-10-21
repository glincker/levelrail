package api

import (
	"context"
	"encoding/json"
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

// expiredGitConn seeds a connection whose access token is already past expiry.
func expiredGitConn(t *testing.T, db *store.DB, secrets GitConnSecrets, host, account, refresh string) store.GitConnection {
	t.Helper()
	c := seedGitConn(t, db, secrets, gitProviderGitLab, host, account, true)
	mustSetGitConnSecret(t, secrets, c.SecretsKey, gitConnAccessTokenKey, "at-old")
	mustSetGitConnSecret(t, secrets, c.SecretsKey, gitConnRefreshTokenKey, refresh)
	mustSetGitConnSecret(t, secrets, c.SecretsKey, gitConnTokenExpiresKey, strconv.FormatInt(time.Now().Add(-time.Hour).Unix(), 10))
	return c
}

func TestGitConnToken_ConcurrentCallersRefreshOnce(t *testing.T) {
	rot := &rotatingGitLab{fakeGitLabAppClient: &fakeGitLabAppClient{}, valid: "rt-0"}
	secrets := newFakeGitConnSecrets()
	rt, db := newGitConnTestRouter(t, secrets)
	rt.gitlabAppClient = rot
	conn := expiredGitConn(t, db, secrets, "gitlab.example.com", "ann", "rt-0")

	const callers = 16
	tokens := make([]string, callers)
	errs := make([]error, callers)
	var wg sync.WaitGroup
	for i := range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, tokens[i], errs[i] = rt.gitConnToken(t.Context(), conn.ID)
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
	if got := rt.gitConnHealth(t.Context(), conn.ID); got.Status != store.GitConnOK {
		t.Fatalf("health after a good refresh = %+v", got)
	}
}

// Two accounts on the same provider refresh independently: one refresh per
// connection, and a refresh stuck on one never blocks the other.
func TestGitConnToken_RefreshIsSerializedPerConnectionNotPerProvider(t *testing.T) {
	gate := make(chan struct{})
	fake := &fakeGitLabAppClient{
		refreshTokens: gitlabapp.Tokens{AccessToken: "at-new", RefreshToken: "rt-new", ExpiresAt: time.Now().Add(time.Hour)},
		refreshGate:   gate,
	}
	secrets := newFakeGitConnSecrets()
	rt, db := newGitConnTestRouter(t, secrets)
	rt.gitlabAppClient = fake
	a := expiredGitConn(t, db, secrets, "gitlab.com", "ann", "rt-a")
	b := expiredGitConn(t, db, secrets, "git.corp.example", "bob", "rt-b")

	var wg sync.WaitGroup
	for _, c := range []store.GitConnection{a, a, b, b} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, _, err := rt.gitConnToken(t.Context(), c.ID); err != nil {
				t.Errorf("token for %s: %v", c.ID, err)
			}
		}()
	}
	deadline := time.Now().Add(5 * time.Second)
	for fake.refreshCount.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if got := fake.refreshCount.Load(); got != 2 {
		t.Fatalf("while both refreshes are in flight, refresh calls = %d, want 2 (one per connection, in parallel)", got)
	}
	close(gate)
	wg.Wait()
	if got := fake.refreshCount.Load(); got != 2 {
		t.Fatalf("total refresh calls = %d, want 2: duplicate callers must reuse the refreshed token", got)
	}
}

func TestRefreshGitOAuth_DiscardsResultWhenRefreshTokenChangedUnderneath(t *testing.T) {
	rt, _ := newTestRouter(t)
	var mu sync.Mutex
	state := gitOAuthState{Access: "old", Refresh: "rt-1", ExpiresUnix: time.Now().Add(-time.Hour).Unix()}
	saved := false
	spec := gitOAuthSpec{
		provider: gitProviderGitLab, key: "gc_test",
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

func TestGitLabRevokedGrantIsTypedNotA500AndScopedToItsConnection(t *testing.T) {
	rot := &rotatingGitLab{fakeGitLabAppClient: &fakeGitLabAppClient{}, valid: "someone-else"}
	secrets := newFakeGitConnSecrets()
	rt, db := newGitConnTestRouter(t, secrets)
	rt.gitlabAppClient = rot
	cookie := loginTestSession(t, rt, db)
	broken := expiredGitConn(t, db, secrets, "gitlab.com", "ann", "rt-0")
	healthy := seedGitConn(t, db, secrets, gitProviderGitLab, "git.corp.example", "bob", true)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/git/repos?provider=gitlab&connection="+broken.ID, ""))
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), `"code":"needs_reauthorize"`) {
		t.Fatalf("status = %d body = %s, want 409 needs_reauthorize", rec.Code, rec.Body.String())
	}
	if got := rt.gitConnHealth(t.Context(), broken.ID); got.Status != store.GitConnNeedsReauthorize {
		t.Fatalf("broken connection health = %+v", got)
	}
	if got := rt.gitConnHealth(t.Context(), healthy.ID); got.Status != store.GitConnOK {
		t.Fatalf("the other account's health = %+v, want ok", got)
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/git-connections?provider=gitlab", ""))
	var conns []gitConnectionResource
	if err := json.Unmarshal(rec.Body.Bytes(), &conns); err != nil || len(conns) != 2 {
		t.Fatalf("connections = %s, err %v", rec.Body.String(), err)
	}
	for _, c := range conns {
		want := store.GitConnOK
		if c.ID == broken.ID {
			want = store.GitConnNeedsReauthorize
		}
		if c.Connection.Status != want {
			t.Errorf("connection %s status = %q, want %q", c.ID, c.Connection.Status, want)
		}
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/git-providers", ""))
	if !strings.Contains(rec.Body.String(), `"provider":"gitlab","connected":true`) || !strings.Contains(rec.Body.String(), `"status":"needs_reauthorize"`) {
		t.Fatalf("git-providers = %s, want gitlab flagged needs_reauthorize", rec.Body.String())
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
	rt.repoCache = newRepoCacheForTest()
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

func TestGitConnectionDisconnectRevokesOnlyItsOwnTokenAtProvider(t *testing.T) {
	fake := &fakeGitLabAppClient{}
	secrets := newFakeGitConnSecrets()
	rt, db := newGitConnTestRouter(t, secrets)
	rt.gitlabAppClient = fake
	cookie := loginTestSession(t, rt, db)
	gone := seedGitConn(t, db, secrets, gitProviderGitLab, "gitlab.com", "ann", true)
	kept := seedGitConn(t, db, secrets, gitProviderGitLab, "git.corp.example", "bob", true)
	mustSetGitConnSecret(t, secrets, gone.SecretsKey, gitConnRefreshTokenKey, "rt-live")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/git-connections/"+gone.ID, ""))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"token_revoked":true`) {
		t.Fatalf("status = %d body = %s, want a revoked token", rec.Code, rec.Body.String())
	}
	if len(fake.revokedTokens) != 1 || fake.revokedTokens[0] != "rt-live" {
		t.Fatalf("revoked %v, want only the disconnected connection's refresh token", fake.revokedTokens)
	}
	if ok, _ := secrets.Exists(t.Context(), kept.SecretsKey, gitConnAccessTokenKey); !ok {
		t.Error("the other connection's token was deleted")
	}
	if ok, _ := secrets.Exists(t.Context(), gone.SecretsKey, gitConnAccessTokenKey); ok {
		t.Error("the disconnected connection's token is still stored")
	}
}

func TestGitConnectionDisconnectStillSucceedsWhenRevocationFails(t *testing.T) {
	fake := &fakeGitLabAppClient{revokeErr: errors.New("gitlab down")}
	secrets := newFakeGitConnSecrets()
	rt, db := newGitConnTestRouter(t, secrets)
	rt.gitlabAppClient = fake
	cookie := loginTestSession(t, rt, db)
	c := seedGitConn(t, db, secrets, gitProviderGitLab, "gitlab.com", "ann", true)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/git-connections/"+c.ID, ""))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"token_revoked":false`) || !strings.Contains(rec.Body.String(), "Revoke it yourself") {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestBitbucketDisconnectSaysItCannotRevoke(t *testing.T) {
	secrets := newFakeGitConnSecrets()
	rt, db := newGitConnTestRouter(t, secrets)
	cookie := loginTestSession(t, rt, db)
	c := seedGitConn(t, db, secrets, gitProviderBitbucket, "bitbucket.org", "wsuser", true)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/git-connections/"+c.ID, ""))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"token_revoked":false`) || !strings.Contains(rec.Body.String(), "no API to revoke") {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestDoctorGitConnectionsWarnsWhenAProviderNeedsReconnect(t *testing.T) {
	secrets := newFakeGitConnSecrets()
	rt, db := newGitConnTestRouter(t, secrets)
	c := seedGitConn(t, db, secrets, gitProviderGitLab, "gitlab.com", "ann", true)
	if got := rt.doctorCheckGitConnections(t.Context()); got.Status != doctorStatusOK {
		t.Fatalf("healthy = %+v", got)
	}
	rt.recordGitConn(t.Context(), &gitConnError{Provider: gitProviderGitLab, Key: c.ID, Status: store.GitConnNeedsReauthorize, Err: errors.New("x")})
	got := rt.doctorCheckGitConnections(t.Context())
	if got.Status != doctorStatusWarn || !strings.Contains(got.Message, "gitlab (needs_reauthorize)") {
		t.Fatalf("check = %+v, want a warning naming gitlab", got)
	}
}

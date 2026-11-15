package api

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/gitlabapp"
	"github.com/GLINCKER/levelrail/internal/store"
)

func pkceFake() *fakeGitLabAppClient {
	return &fakeGitLabAppClient{
		exchangeTokens: gitlabapp.Tokens{AccessToken: "at-1", RefreshToken: "rt-1", ExpiresAt: time.Now().Add(2 * time.Hour)},
	}
}

// startPKCEConnect runs the connect redirect and returns its state and challenge.
func startPKCEConnect(t *testing.T, rt *Router, cookie *http.Cookie, connID string) (state, challenge string) {
	t.Helper()
	start := gcRequest(t, rt, cookie, http.MethodGet, "/api/v1/gitlab-app/connect?connection_id="+connID, "")
	if start.Code != http.StatusFound {
		t.Fatalf("connect: status %d", start.Code)
	}
	loc, err := url.Parse(start.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse location: %v", err)
	}
	q := loc.Query()
	if q.Get("code_challenge_method") != "S256" || len(q.Get("code_challenge")) != 43 {
		t.Fatalf("authorize url %q lacks an S256 challenge", loc.String())
	}
	return q.Get("state"), q.Get("code_challenge")
}

func TestGitConnectPKCE_VerifierMatchesChallengeAndIsConsumedOnce(t *testing.T) {
	fake := pkceFake()
	rt, _, cookie := newOAuthRouter(t, fake)
	c := createGitLabConn(t, rt, cookie, "https://gitlab.com", "cid-a")
	state, challenge := startPKCEConnect(t, rt, cookie, c.ID)
	fake.wantChallenge = challenge

	cb := "/api/v1/gitlab-app/callback?code=code-1&state=" + state
	if rec := gcRequest(t, rt, cookie, http.MethodGet, cb, ""); rec.Code != http.StatusFound {
		t.Fatalf("callback: status %d body %s", rec.Code, rec.Body.String())
	}
	if fake.gotVerifier == "" || gitPKCEChallenge(fake.gotVerifier) != challenge {
		t.Fatalf("exchange verifier does not hash to the authorize challenge")
	}
	// Replaying the same callback must not reach the provider again.
	if rec := gcRequest(t, rt, cookie, http.MethodGet, cb, ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("replayed callback: status %d, want 400", rec.Code)
	}
	if fake.exchangeCalls != 1 {
		t.Errorf("exchange calls = %d, want 1", fake.exchangeCalls)
	}
}

func TestGitConnectPKCE_WrongVerifierFailsAndStoresNoToken(t *testing.T) {
	fake := pkceFake()
	rt, secrets, cookie := newOAuthRouter(t, fake)
	c := createGitLabConn(t, rt, cookie, "https://gitlab.com", "cid-a")
	state, _ := startPKCEConnect(t, rt, cookie, c.ID)
	fake.wantChallenge = gitPKCEChallenge("some-other-verifier-than-the-stored-one")

	rec := gcRequest(t, rt, cookie, http.MethodGet, "/api/v1/gitlab-app/callback?code=code-1&state="+state, "")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status %d, want 502 when the provider rejects the verifier", rec.Code)
	}
	conn, err := rt.gitConns.GetGitConnection(t.Context(), c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if ok, _ := secrets.Exists(t.Context(), conn.SecretsKey, gitConnAccessTokenKey); ok {
		t.Error("an access token was stored after a rejected exchange")
	}
}

func TestGitConnectPKCE_MissingVerifierFailsClosed(t *testing.T) {
	fake := pkceFake()
	rt, _, cookie := newOAuthRouter(t, fake)
	c := createGitLabConn(t, rt, cookie, "https://gitlab.com", "cid-a")
	state, err := rt.gitConnState.begin(joinGitStateValue(c.ID, ""))
	if err != nil {
		t.Fatal(err)
	}
	rec := gcRequest(t, rt, cookie, http.MethodGet, "/api/v1/gitlab-app/callback?code=code-1&state="+state, "")
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "PKCE") {
		t.Fatalf("status %d body %s, want 400 about PKCE", rec.Code, rec.Body.String())
	}
	if fake.exchangeCalls != 0 {
		t.Error("the provider was called without a verifier")
	}
}

func TestGitConnectPKCE_GiteaAndForgejoSendChallenge(t *testing.T) {
	for _, provider := range []string{gitProviderGitea, gitProviderForgejo} {
		rt, _ := newGitConnTestRouter(t, newFakeGitConnSecrets())
		a, ok := rt.gitAdapter(provider)
		if !ok || !a.pkce {
			t.Fatalf("%s adapter missing or without pkce", provider)
		}
		got := a.authorizeURL(store.GitConnection{InstanceURL: "https://git.example.com", ClientID: "cid"}, "https://cp.example.com/cb", "st", "chal")
		if !strings.Contains(got, "code_challenge=chal") || !strings.Contains(got, "code_challenge_method=S256") {
			t.Errorf("%s authorize url %q lacks the S256 challenge", provider, got)
		}
	}
}

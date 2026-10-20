package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
	"golang.org/x/oauth2"
)

// fakeOAuthSecrets is a hand-written fake for OAuthSecrets: an in-memory
// map, no real envelope encryption or master key required.
type fakeOAuthSecrets struct {
	mu     sync.Mutex
	values map[string]string
}

func newFakeOAuthSecrets() *fakeOAuthSecrets {
	return &fakeOAuthSecrets{values: make(map[string]string)}
}

func (f *fakeOAuthSecrets) SetValue(_ context.Context, serviceName, envKey, plaintext string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.values[serviceName+"|"+envKey] = plaintext
	return nil
}

func (f *fakeOAuthSecrets) Resolve(_ context.Context, serviceName, envKey string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.values[serviceName+"|"+envKey]
	if !ok {
		return "", secrets.ErrValueNotFound
	}
	return v, nil
}

// fakeOAuthClient is the hand-written fake for oauthProviderClient: no
// network call, a test configures exactly what Exchange/FetchUserInfo
// return.
type fakeOAuthClient struct {
	exchangeErr  error
	userInfo     oauthUserInfo
	userInfoErr  error
	authOpts     int
	exchangeOpts int
}

func (f *fakeOAuthClient) AuthCodeURL(state string, opts ...oauth2.AuthCodeOption) string {
	f.authOpts = len(opts)
	return "https://provider.example.com/authorize?state=" + state
}

func (f *fakeOAuthClient) Exchange(_ context.Context, code string, opts ...oauth2.AuthCodeOption) (*oauth2.Token, error) {
	f.exchangeOpts = len(opts)
	if f.exchangeErr != nil {
		return nil, f.exchangeErr
	}
	return &oauth2.Token{AccessToken: "fake-token-for-" + code}, nil
}

func (f *fakeOAuthClient) FetchUserInfo(_ context.Context, _ *oauth2.Token) (oauthUserInfo, error) {
	if f.userInfoErr != nil {
		return oauthUserInfo{}, f.userInfoErr
	}
	return f.userInfo, nil
}

// enableOAuthProviderForTest turns a provider on with a fake client
// secret already stored, the minimum a real /start call needs to get
// past its own "is this configured and enabled" checks.
func enableOAuthProviderForTest(t *testing.T, rt *Router) {
	t.Helper()
	const provider = store.OAuthProviderGoogle
	if rt.oauthSecrets == nil {
		rt.oauthSecrets = newFakeOAuthSecrets()
	}
	if err := rt.oauthSecrets.SetValue(context.Background(), store.OAuthProviderSecretsKey(provider), store.OAuthProviderSecretEnvKey, "fake-client-secret"); err != nil {
		t.Fatalf("SetValue() error = %v", err)
	}
	if err := rt.oauthSettings.UpdateOAuthProviderSettings(context.Background(), store.OAuthProviderSettings{
		Provider: provider,
		Enabled:  true,
		ClientID: "fake-client-id",
	}); err != nil {
		t.Fatalf("UpdateOAuthProviderSettings() error = %v", err)
	}
}

// startOAuthFlow calls path (a /start or /link/start route) and extracts
// the state parameter from the resulting redirect, the value a real
// provider would echo back on its own callback.
func startOAuthFlow(t *testing.T, rt *Router, path string, cookie *http.Cookie) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("GET %s: status = %d, want %d, body = %s", path, rec.Code, http.StatusFound, rec.Body.String())
	}
	loc, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse Location header: %v", err)
	}
	state := loc.Query().Get("state")
	if state == "" {
		t.Fatalf("Location %q carries no state parameter", loc.String())
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == oauthBindingCookieName {
			oauthTestBindings.Store(state, &http.Cookie{Name: c.Name, Value: c.Value}) //nolint:gosec // test fixture
		}
	}
	return state
}

func TestHandleListPublicOAuthProviders_RevealsOnlyEnabledBooleans(t *testing.T) {
	rt, _ := newTestRouter(t)
	enableOAuthProviderForTest(t, rt)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/providers", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	if strings.Contains(body, "fake-client-id") || strings.Contains(body, "fake-client-secret") || strings.Contains(body, "client_id") {
		t.Errorf("public providers response leaked configuration: %s", body)
	}

	var got []struct {
		Provider string `json:"provider"`
		Enabled  bool   `json:"enabled"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	seen := map[string]bool{}
	for _, p := range got {
		seen[p.Provider] = p.Enabled
	}
	if !seen[store.OAuthProviderGoogle] {
		t.Error("google should report enabled = true")
	}
	if seen[store.OAuthProviderGitHub] {
		t.Error("github should report enabled = false")
	}
}

func TestHandleOAuthStart_NotConfigured(t *testing.T) {
	rt, _ := newTestRouter(t)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/google/start", nil))
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}
}

func TestHandleOAuthCallback_MissingStateOrCode(t *testing.T) {
	rt, _ := newTestRouter(t)
	enableOAuthProviderForTest(t, rt)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/google/callback", nil))
	assertOAuthErrorRedirect(t, rec, "invalid_request")
}

func TestHandleOAuthCallback_InvalidState(t *testing.T) {
	rt, _ := newTestRouter(t)
	enableOAuthProviderForTest(t, rt)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/google/callback?state=not-real&code=abc", nil))
	assertOAuthErrorRedirect(t, rec, "invalid_state")
}

func TestHandleOAuthLinkStart_RequiresSession(t *testing.T) {
	rt, _ := newTestRouter(t)
	enableOAuthProviderForTest(t, rt)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/google/link/start", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

// TestHandleOAuthCallback_LinkPurpose_AttachesIdentityToAuthenticatedUser
// is the safe path for connecting a new provider: requires a live
// session up front, unlike sign-in which never links by email alone.
func TestHandleOAuthCallback_LinkPurpose_AttachesIdentityToAuthenticatedUser(t *testing.T) {
	rt, db := newTestRouter(t)
	enableOAuthProviderForTest(t, rt)
	rt.oauthClientFactory = func(string, store.OAuthProviderSettings, string, string) (oauthProviderClient, error) {
		return &fakeOAuthClient{userInfo: oauthUserInfo{ProviderUserID: "link-me", Email: "whatever-the-provider-reports@example.com", DisplayName: "Whatever"}}, nil
	}

	user := storeUserForTest(t, db, "existing-user@example.com")
	cookie := sessionCookieForTest(t, rt, user.ID)

	state := startOAuthFlow(t, rt, "/api/v1/auth/oauth/google/link/start", cookie)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, oauthCallbackRequest(state))

	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/oauth/complete" {
		t.Fatalf("status = %d, location = %q, want a redirect to /oauth/complete", rec.Code, rec.Header().Get("Location"))
	}

	identity, err := db.GetOAuthIdentity(context.Background(), store.OAuthProviderGoogle, "link-me")
	if err != nil {
		t.Fatalf("GetOAuthIdentity() error = %v", err)
	}
	if identity.UserID != user.ID {
		t.Errorf("identity.UserID = %q, want the linking user %q, not a new account", identity.UserID, user.ID)
	}

	// Must not have created a second user under the provider's reported
	// email: linking attaches to the session's own account, full stop.
	if _, err := db.GetUserByEmail(context.Background(), "whatever-the-provider-reports@example.com"); !errors.Is(err, store.ErrUserNotFound) {
		t.Error("linking must not create a second user under the provider's reported email")
	}
}

func TestHandleOAuthCallback_LinkPurpose_AlreadyLinkedToAnotherUser_Rejected(t *testing.T) {
	rt, db := newTestRouter(t)
	enableOAuthProviderForTest(t, rt)
	rt.oauthClientFactory = func(string, store.OAuthProviderSettings, string, string) (oauthProviderClient, error) {
		return &fakeOAuthClient{userInfo: oauthUserInfo{ProviderUserID: "already-taken", Email: "x@example.com", DisplayName: "X"}}, nil
	}

	owner := storeUserForTest(t, db, "owner@example.com")
	if err := db.SaveOAuthIdentity(context.Background(), store.OAuthIdentity{
		ID: "oid_owner", UserID: owner.ID, Provider: store.OAuthProviderGoogle, ProviderUserID: "already-taken",
	}); err != nil {
		t.Fatalf("seed SaveOAuthIdentity() error = %v", err)
	}

	other := storeUserForTest(t, db, "other@example.com")
	cookie := sessionCookieForTest(t, rt, other.ID)

	state := startOAuthFlow(t, rt, "/api/v1/auth/oauth/google/link/start", cookie)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, oauthCallbackRequest(state))
	assertOAuthErrorRedirect(t, rec, "already_linked")

	identity, err := db.GetOAuthIdentity(context.Background(), store.OAuthProviderGoogle, "already-taken")
	if err != nil {
		t.Fatalf("GetOAuthIdentity() error = %v", err)
	}
	if identity.UserID != owner.ID {
		t.Errorf("identity.UserID = %q, want unchanged owner %q", identity.UserID, owner.ID)
	}
}

func assertOAuthErrorRedirect(t *testing.T, rec *httptest.ResponseRecorder, wantCode string) {
	t.Helper()
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d (a redirect back to /login)", rec.Code, http.StatusFound)
	}
	loc := rec.Header().Get("Location")
	if !strings.HasPrefix(loc, "/login?oauth_error=") {
		t.Fatalf("Location = %q, want a /login?oauth_error=... redirect", loc)
	}
	if !strings.Contains(loc, wantCode) {
		t.Errorf("Location = %q, want it to contain error code %q", loc, wantCode)
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookieName && c.Value != "" {
			t.Error("an error redirect must never set a session cookie")
		}
	}
}

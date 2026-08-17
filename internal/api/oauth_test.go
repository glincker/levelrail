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
	exchangeErr error
	userInfo    oauthUserInfo
	userInfoErr error
}

func (f *fakeOAuthClient) AuthCodeURL(state string) string {
	return "https://provider.example.com/authorize?state=" + state
}

func (f *fakeOAuthClient) Exchange(_ context.Context, code string) (*oauth2.Token, error) {
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
func enableOAuthProviderForTest(t *testing.T, rt *Router, allowedDomain string) {
	t.Helper()
	const provider = store.OAuthProviderGoogle
	if rt.oauthSecrets == nil {
		rt.oauthSecrets = newFakeOAuthSecrets()
	}
	if err := rt.oauthSecrets.SetValue(context.Background(), store.OAuthProviderSecretsKey(provider), store.OAuthProviderSecretEnvKey, "fake-client-secret"); err != nil {
		t.Fatalf("SetValue() error = %v", err)
	}
	if err := rt.oauthSettings.UpdateOAuthProviderSettings(context.Background(), store.OAuthProviderSettings{
		Provider:           provider,
		Enabled:            true,
		ClientID:           "fake-client-id",
		AllowedEmailDomain: allowedDomain,
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
	return state
}

func TestHandleListPublicOAuthProviders_RevealsOnlyEnabledBooleans(t *testing.T) {
	rt, _ := newTestRouter(t)
	enableOAuthProviderForTest(t, rt, "")

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

func TestHandleOAuthStart_UnknownProvider(t *testing.T) {
	rt, _ := newTestRouter(t)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/bogus/start", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
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

func TestHandleOAuthStart_DisabledProvider(t *testing.T) {
	rt, _ := newTestRouter(t)
	rt.oauthSecrets = newFakeOAuthSecrets() // configured, but never enabled

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/google/start", nil))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleOAuthStart_Success_RedirectsWithState(t *testing.T) {
	rt, _ := newTestRouter(t)
	enableOAuthProviderForTest(t, rt, "")

	state := startOAuthFlow(t, rt, "/api/v1/auth/oauth/google/start", nil)
	if state == "" {
		t.Error("expected a non-empty state token")
	}
}

func TestHandleOAuthCallback_MissingStateOrCode(t *testing.T) {
	rt, _ := newTestRouter(t)
	enableOAuthProviderForTest(t, rt, "")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/google/callback", nil))
	assertOAuthErrorRedirect(t, rec, "invalid_request")
}

func TestHandleOAuthCallback_InvalidState(t *testing.T) {
	rt, _ := newTestRouter(t)
	enableOAuthProviderForTest(t, rt, "")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/google/callback?state=not-real&code=abc", nil))
	assertOAuthErrorRedirect(t, rec, "invalid_state")
}

// TestHandleOAuthCallback_StateIsSingleUse proves a captured or replayed
// callback URL can't be reused: the state token consumed by the first
// callback must not authenticate a second request.
func TestHandleOAuthCallback_StateIsSingleUse(t *testing.T) {
	rt, _ := newTestRouter(t)
	enableOAuthProviderForTest(t, rt, "")
	rt.oauthClientFactory = func(string, store.OAuthProviderSettings, string, string) (oauthProviderClient, error) {
		return &fakeOAuthClient{userInfo: oauthUserInfo{ProviderUserID: "g1", Email: "new@example.com", DisplayName: "New"}}, nil
	}

	state := startOAuthFlow(t, rt, "/api/v1/auth/oauth/google/start", nil)

	first := httptest.NewRecorder()
	rt.Handler().ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/google/callback?state="+state+"&code=abc", nil))
	if first.Code != http.StatusFound || first.Header().Get("Location") != "/oauth/complete" {
		t.Fatalf("first callback: status = %d, location = %q, want a redirect to /oauth/complete", first.Code, first.Header().Get("Location"))
	}

	second := httptest.NewRecorder()
	rt.Handler().ServeHTTP(second, httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/google/callback?state="+state+"&code=abc", nil))
	assertOAuthErrorRedirect(t, second, "invalid_state")
}

func TestHandleOAuthCallback_NewIdentity_AutoProvisionsUser(t *testing.T) {
	rt, db := newTestRouter(t)
	enableOAuthProviderForTest(t, rt, "")
	rt.oauthClientFactory = func(string, store.OAuthProviderSettings, string, string) (oauthProviderClient, error) {
		return &fakeOAuthClient{userInfo: oauthUserInfo{ProviderUserID: "google-sub-1", Email: "brandnew@example.com", DisplayName: "Brand New"}}, nil
	}

	state := startOAuthFlow(t, rt, "/api/v1/auth/oauth/google/start", nil)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/google/callback?state="+state+"&code=abc", nil))

	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/oauth/complete" {
		t.Fatalf("status = %d, location = %q, want a redirect to /oauth/complete", rec.Code, rec.Header().Get("Location"))
	}
	var sessionCookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookieName {
			sessionCookie = c
		}
	}
	if sessionCookie == nil {
		t.Fatal("expected a session cookie on successful auto-provisioning")
	}

	user, err := db.GetUserByEmail(context.Background(), "brandnew@example.com")
	if err != nil {
		t.Fatalf("GetUserByEmail() error = %v", err)
	}
	if user.PasswordHash != nil {
		t.Error("auto-provisioned OAuth user must have no password")
	}
	if len(user.Abilities) != 1 || user.Abilities[0] != AbilityRead {
		t.Errorf("Abilities = %v, want [%s]: an empty value would fail every ability check including AbilityRead itself, locking this account out of the whole app", user.Abilities, AbilityRead)
	}
	identity, err := db.GetOAuthIdentity(context.Background(), store.OAuthProviderGoogle, "google-sub-1")
	if err != nil {
		t.Fatalf("GetOAuthIdentity() error = %v", err)
	}
	if identity.UserID != user.ID {
		t.Errorf("identity.UserID = %q, want %q", identity.UserID, user.ID)
	}
}

func TestHandleOAuthCallback_ExistingIdentity_SignsInSameUser(t *testing.T) {
	rt, db := newTestRouter(t)
	enableOAuthProviderForTest(t, rt, "")
	rt.oauthClientFactory = func(string, store.OAuthProviderSettings, string, string) (oauthProviderClient, error) {
		return &fakeOAuthClient{userInfo: oauthUserInfo{ProviderUserID: "google-sub-2", Email: "returning@example.com", DisplayName: "Returning"}}, nil
	}

	existing := storeUserForTest(t, db, "returning@example.com")
	if err := db.SaveOAuthIdentity(context.Background(), store.OAuthIdentity{
		ID: "oid_seed", UserID: existing.ID, Provider: store.OAuthProviderGoogle, ProviderUserID: "google-sub-2",
	}); err != nil {
		t.Fatalf("seed SaveOAuthIdentity() error = %v", err)
	}

	state := startOAuthFlow(t, rt, "/api/v1/auth/oauth/google/start", nil)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/google/callback?state="+state+"&code=abc", nil))

	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/oauth/complete" {
		t.Fatalf("status = %d, location = %q, want a redirect to /oauth/complete", rec.Code, rec.Header().Get("Location"))
	}

	// No second user must have been created for the same returning
	// identity.
	users, err := db.ListUsers(context.Background())
	if err != nil {
		t.Fatalf("ListUsers() error = %v", err)
	}
	if len(users) != 1 {
		t.Errorf("len(ListUsers()) = %d, want 1 (must reuse the existing user, not create a new one)", len(users))
	}
}

func TestHandleOAuthCallback_DomainNotAllowed_Rejected(t *testing.T) {
	rt, db := newTestRouter(t)
	enableOAuthProviderForTest(t, rt, "allowed.example.com")
	rt.oauthClientFactory = func(string, store.OAuthProviderSettings, string, string) (oauthProviderClient, error) {
		return &fakeOAuthClient{userInfo: oauthUserInfo{ProviderUserID: "g-outside", Email: "someone@notallowed.com", DisplayName: "Outsider"}}, nil
	}

	state := startOAuthFlow(t, rt, "/api/v1/auth/oauth/google/start", nil)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/google/callback?state="+state+"&code=abc", nil))
	assertOAuthErrorRedirect(t, rec, "domain_not_allowed")

	if _, err := db.GetUserByEmail(context.Background(), "someone@notallowed.com"); !errors.Is(err, store.ErrUserNotFound) {
		t.Error("a domain-rejected sign-in must not have created a user")
	}
}

// TestHandleOAuthCallback_EmailBelongsToExistingAccount_Rejected is the
// account-takeover guard: a new external identity reporting an email
// already owned by a different user must never get silently attached.
func TestHandleOAuthCallback_EmailBelongsToExistingAccount_Rejected(t *testing.T) {
	rt, db := newTestRouter(t)
	enableOAuthProviderForTest(t, rt, "")
	rt.oauthClientFactory = func(string, store.OAuthProviderSettings, string, string) (oauthProviderClient, error) {
		return &fakeOAuthClient{userInfo: oauthUserInfo{ProviderUserID: "attacker-google-id", Email: "victim@example.com", DisplayName: "Attacker Claiming To Be Victim"}}, nil
	}

	victim := storeUserForTest(t, db, "victim@example.com")

	state := startOAuthFlow(t, rt, "/api/v1/auth/oauth/google/start", nil)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/google/callback?state="+state+"&code=abc", nil))
	assertOAuthErrorRedirect(t, rec, "email_in_use")

	// No session must have been established for the victim's account.
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookieName && c.Value != "" {
			t.Error("a rejected email-collision callback must not set a session cookie")
		}
	}
	// The external identity must not have been linked to the victim.
	if _, err := db.GetOAuthIdentity(context.Background(), store.OAuthProviderGoogle, "attacker-google-id"); !errors.Is(err, store.ErrOAuthIdentityNotFound) {
		t.Error("a rejected email-collision callback must not have linked the identity to anyone")
	}
	identities, err := db.ListOAuthIdentitiesForUser(context.Background(), victim.ID)
	if err != nil {
		t.Fatalf("ListOAuthIdentitiesForUser() error = %v", err)
	}
	if len(identities) != 0 {
		t.Errorf("victim now has %d linked identities, want 0", len(identities))
	}
}

func TestHandleOAuthLinkStart_RequiresSession(t *testing.T) {
	rt, _ := newTestRouter(t)
	enableOAuthProviderForTest(t, rt, "")

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
	enableOAuthProviderForTest(t, rt, "")
	rt.oauthClientFactory = func(string, store.OAuthProviderSettings, string, string) (oauthProviderClient, error) {
		return &fakeOAuthClient{userInfo: oauthUserInfo{ProviderUserID: "link-me", Email: "whatever-the-provider-reports@example.com", DisplayName: "Whatever"}}, nil
	}

	user := storeUserForTest(t, db, "existing-user@example.com")
	cookie := sessionCookieForTest(t, rt, user.ID)

	state := startOAuthFlow(t, rt, "/api/v1/auth/oauth/google/link/start", cookie)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/google/callback?state="+state+"&code=abc", nil))

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
	enableOAuthProviderForTest(t, rt, "")
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
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/google/callback?state="+state+"&code=abc", nil))
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

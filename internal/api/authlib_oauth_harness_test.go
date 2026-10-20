package api

import (
	"context"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"

	"github.com/GLINCKER/levelrail/internal/authengine"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/kit/netguard"
)

const (
	modeLibrary = "library"
	testBaseURL = "http://app.test"
)

// oauthHarness runs sign-in requests against the library engine.
type oauthHarness struct {
	t       *testing.T
	mode    string
	rt      *Router
	db      *store.DB
	idp     *fakeIDP
	eng     *authengine.Engine
	secrets *fakeOAuthSecrets
	key     []byte
	jar     map[string]*http.Cookie
	// adminCookie is a root dashboard session, made on first use.
	adminCookie *http.Cookie
}

func newOAuthHarness(t *testing.T, mode string) *oauthHarness {
	t.Helper()
	if _, ok := os.LookupEnv(authengine.EnvOAuthProviderTTL); !ok {
		t.Setenv(authengine.EnvOAuthProviderTTL, "0")
	}
	t.Setenv(netguard.AllowPrivateEnv, "true") // the fake IdP listens on loopback
	h := &oauthHarness{t: t, mode: mode, db: openTestDB(t), idp: newFakeIDP(t), secrets: newFakeOAuthSecrets(), jar: map[string]*http.Cookie{}}
	h.key = make([]byte, 32)
	if _, err := rand.Read(h.key); err != nil {
		t.Fatal(err)
	}
	eng, err := authengine.New(h.db.DB, authengine.Config{
		BaseURL: testBaseURL, TokenPrefix: "tk", TOTPIssuer: "test", EncryptionKey: h.key, RateLimitPerIP: 1000,
		Directory: authengine.NewDirectory(h.db.DB),
		Sessions:  authengine.SessionsHooks{Mail: &authengine.MailRelay{}},
		OAuth: &authengine.OAuthWiring{
			Settings: h.db, Secrets: h.secrets, Users: h.db, Endpoints: h.idp.endpoints(),
		},
	})
	if err != nil {
		t.Fatalf("authengine.New: %v", err)
	}
	t.Cleanup(eng.Close)
	h.eng = eng
	h.rt = NewRouter(discardLogger(), testBrand(), h.db, WithOAuthSecrets(h.secrets), WithAuthEngine(eng))
	return h
}

// enable turns provider on with a stored secret, pointing the generic provider at the fake issuer.
func (h *oauthHarness) enable(provider, allowedDomain string) {
	h.t.Helper()
	ctx := context.Background()
	if err := h.secrets.SetValue(ctx, store.OAuthProviderSecretsKey(provider), store.OAuthProviderSecretEnvKey, "secret-"+provider); err != nil {
		h.t.Fatal(err)
	}
	s := store.OAuthProviderSettings{Provider: provider, Enabled: true, ClientID: "client-" + provider, AllowedEmailDomain: allowedDomain}
	if provider == store.OAuthProviderOIDC {
		s.IssuerURL = h.idp.Issuer()
	}
	if err := h.db.UpdateOAuthProviderSettings(ctx, s); err != nil {
		h.t.Fatal(err)
	}
	h.setPolicy(provider, store.OAuthSSOPolicy{GroupsClaim: "groups", DefaultRoleID: defaultOIDCRoleID, JITProvisioning: true})
}

func (h *oauthHarness) setPolicy(provider string, p store.OAuthSSOPolicy) {
	h.t.Helper()
	if err := h.db.SetOAuthSSOPolicy(context.Background(), provider, p); err != nil {
		h.t.Fatal(err)
	}
}

func (h *oauthHarness) startPath(provider string) string {
	return "/api/v1/auth/oauth/" + provider + "/start"
}

func (h *oauthHarness) callbackPath(provider string) string {
	return authengine.OAuthCallbackPath(provider)
}

func (h *oauthHarness) get(target string, withJar bool) *httptest.ResponseRecorder {
	h.t.Helper()
	req := httptest.NewRequest(http.MethodGet, testBaseURL+target, nil)
	req.RemoteAddr = "127.0.0.1:50000"
	if withJar {
		for _, c := range h.jar {
			req.AddCookie(&http.Cookie{Name: c.Name, Value: c.Value}) //nolint:gosec // test fixture
		}
	}
	rec := httptest.NewRecorder()
	h.rt.Handler().ServeHTTP(rec, req)
	for _, c := range rec.Result().Cookies() {
		if c.MaxAge < 0 || c.Value == "" {
			delete(h.jar, c.Name)
		} else {
			h.jar[c.Name] = c
		}
	}
	return rec
}

// start begins sign-in and returns the provider authorize URL and the state it carries.
func (h *oauthHarness) start(provider string) (authorizeURL *url.URL, state string) {
	h.t.Helper()
	rec := h.get(h.startPath(provider), false)
	if rec.Code != http.StatusFound {
		h.t.Fatalf("start %s: status = %d, body = %s", provider, rec.Code, rec.Body.String())
	}
	u, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		h.t.Fatal(err)
	}
	return u, u.Query().Get("state")
}

// signIn runs start, the provider consent and the callback for id, returning the callback response.
func (h *oauthHarness) signIn(provider string, id fakeIdentity) *httptest.ResponseRecorder {
	h.t.Helper()
	u, state := h.start(provider)
	code := h.idp.grant(h.t, u.String(), id)
	return h.get(h.callbackPath(provider)+"?state="+url.QueryEscape(state)+"&code="+code, true)
}

func (h *oauthHarness) sessionUser(rec *httptest.ResponseRecorder) (string, bool) {
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookieName {
			return h.rt.sessions.lookup(c.Value)
		}
	}
	return "", false
}

func TestOAuthStart_RefusesSpoofedHostFromUntrustedPeer(t *testing.T) {
	t.Setenv("APP_DASHBOARD_URL", "")
	h := newOAuthHarness(t, modeLibrary)
	h.enable("oidc", "")
	req := httptest.NewRequest(http.MethodGet, "http://attacker.example"+h.startPath("oidc"), nil)
	req.RemoteAddr = "203.0.113.50:4444"
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := httptest.NewRecorder()
	h.rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "" {
		t.Fatalf("redirected to %q", loc)
	}
}

func TestOAuthStart_ConfiguredDashboardURLWinsOverHost(t *testing.T) {
	t.Setenv("APP_DASHBOARD_URL", "https://dash.example.com")
	h := newOAuthHarness(t, modeLibrary)
	h.enable("oidc", "")
	req := httptest.NewRequest(http.MethodGet, "http://attacker.example"+h.startPath("oidc"), nil)
	req.RemoteAddr = "203.0.113.50:4444"
	rec := httptest.NewRecorder()
	h.rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	u, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	if got := u.Query().Get("redirect_uri"); got != "https://dash.example.com"+h.callbackPath("oidc") {
		t.Fatalf("redirect_uri = %q", got)
	}
}

package api

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

var oauthTestBindings sync.Map

func oauthCallbackRequest(state string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/google/callback?state="+state+"&code=abc", nil)
	if c, ok := oauthTestBindings.Load(state); ok {
		if cookie, isCookie := c.(*http.Cookie); isCookie {
			req.AddCookie(cookie)
		}
	}
	return req
}

func mismatchedBinding(c *http.Cookie) *http.Cookie {
	return &http.Cookie{Name: c.Name, Value: "attacker"} //nolint:gosec // test fixture
}

func TestHandleOAuthCallback_BrowserBinding(t *testing.T) {
	tests := []struct {
		name   string
		cookie func(valid *http.Cookie) *http.Cookie
		want   string
	}{
		{"missing cookie", func(*http.Cookie) *http.Cookie { return nil }, "invalid_state"},
		{"mismatched cookie", mismatchedBinding, "invalid_state"},
		{"matching cookie", func(c *http.Cookie) *http.Cookie { return c }, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rt, _ := newTestRouter(t)
			enableOAuthProviderForTest(t, rt, "")
			fake := &fakeOAuthClient{userInfo: oauthUserInfo{ProviderUserID: "g1", Email: "new@example.com", DisplayName: "New"}}
			rt.oauthClientFactory = func(string, store.OAuthProviderSettings, string, string) (oauthProviderClient, error) {
				return fake, nil
			}
			state := startOAuthFlow(t, rt, "/api/v1/auth/oauth/google/start", nil)
			v, _ := oauthTestBindings.Load(state)
			valid, ok := v.(*http.Cookie)
			if !ok {
				t.Fatal("start did not set the binding cookie")
			}
			req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/google/callback?state="+state+"&code=abc", nil)
			if c := tc.cookie(valid); c != nil {
				req.AddCookie(c)
			}
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, req)
			if tc.want != "" {
				assertOAuthErrorRedirect(t, rec, tc.want)
				return
			}
			if rec.Header().Get("Location") != "/oauth/complete" {
				t.Fatalf("location = %q, want /oauth/complete", rec.Header().Get("Location"))
			}
			if fake.authOpts != 1 || fake.exchangeOpts != 1 {
				t.Errorf("PKCE options: authorize=%d exchange=%d, want 1 and 1", fake.authOpts, fake.exchangeOpts)
			}
		})
	}
}

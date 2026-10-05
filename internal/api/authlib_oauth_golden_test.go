package api

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

var allOAuthProviders = []string{store.OAuthProviderGoogle, store.OAuthProviderGitHub, store.OAuthProviderMicrosoft, store.OAuthProviderOIDC}

func TestOAuthGoldenStartParameters(t *testing.T) {
	for _, provider := range allOAuthProviders {
		t.Run(provider, func(t *testing.T) {
			h := newOAuthHarness(t, modeLibrary)
			h.enable(provider, "")
			u, state := h.start(provider)
			if state == "" {
				t.Fatal("authorize URL carries no state")
			}
			q := u.Query()
			if q.Get("client_id") != "client-"+provider || q.Get("response_type") != "code" {
				t.Errorf("authorize parameters = %v", q)
			}
			if q.Get("code_challenge_method") != "S256" || q.Get("code_challenge") == "" {
				t.Errorf("PKCE S256 challenge missing: %v", q)
			}
			if want := testBaseURL + "/api/v1/auth-lib/providers/" + provider + "/callback"; q.Get("redirect_uri") != want {
				t.Errorf("redirect_uri = %q, want %q", q.Get("redirect_uri"), want)
			}
			for _, c := range h.jar {
				if !c.HttpOnly || c.SameSite != http.SameSiteLaxMode || (c.MaxAge <= 0 && c.Expires.IsZero()) || c.Value == "" {
					t.Errorf("binding cookie weakened: %+v", c)
				}
			}
		})
	}
}

func TestOAuthGoldenStartErrors(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		setup    func(h *oauthHarness)
		want     int
		wantBody string
	}{
		{"unknown provider", "bogus", func(*oauthHarness) {}, http.StatusNotFound, "unknown oauth provider"},
		{"disabled provider", "google", func(*oauthHarness) {}, http.StatusBadRequest, "this provider is not enabled"},
		{"secret missing fails closed", "google", func(h *oauthHarness) {
			h.enable("google", "")
			h.secrets.mu.Lock()
			delete(h.secrets.values, store.OAuthProviderSecretsKey("google")+"|"+store.OAuthProviderSecretEnvKey)
			h.secrets.mu.Unlock()
		}, http.StatusInternalServerError, "misconfigured"},
	}
	for _, tc := range tests {
		for _, mode := range []string{modeLibrary} {
			t.Run(tc.name+"/"+mode, func(t *testing.T) {
				h := newOAuthHarness(t, mode)
				tc.setup(h)
				rec := h.get(h.startPath(tc.provider), false)
				if rec.Code != tc.want || !strings.Contains(rec.Body.String(), tc.wantBody) {
					t.Errorf("status = %d body = %q, want %d containing %q", rec.Code, rec.Body.String(), tc.want, tc.wantBody)
				}
			})
		}
	}
}

func TestOAuthGoldenCallbackErrors(t *testing.T) {
	tests := []struct {
		name string
		run  func(h *oauthHarness) string
		want string
	}{
		{"bad state with valid binding cookie", func(h *oauthHarness) string {
			h.start("oidc")
			return h.callbackPath("oidc") + "?state=forged&code=abc"
		}, "invalid_state"},
		{"missing binding cookie", func(h *oauthHarness) string {
			_, state := h.start("oidc")
			h.jar = map[string]*http.Cookie{}
			return h.callbackPath("oidc") + "?state=" + state + "&code=abc"
		}, "invalid_state"},
		{"mismatched binding cookie", func(h *oauthHarness) string {
			_, state := h.start("oidc")
			for name := range h.jar {
				h.jar[name] = &http.Cookie{Name: name, Value: "attacker"} //nolint:gosec // test fixture
			}
			return h.callbackPath("oidc") + "?state=" + state + "&code=abc"
		}, "invalid_state"},
		{"missing code", func(h *oauthHarness) string {
			_, state := h.start("oidc")
			return h.callbackPath("oidc") + "?state=" + state
		}, "invalid_request"},
		{"provider denied", func(h *oauthHarness) string {
			return h.callbackPath("oidc") + "?error=access_denied"
		}, "provider_denied"},
		{"unknown provider", func(h *oauthHarness) string {
			return strings.Replace(h.callbackPath("oidc"), "oidc", "bogus", 1) + "?state=s&code=c"
		}, "invalid_provider"},
	}
	for _, tc := range tests {
		for _, mode := range []string{modeLibrary} {
			t.Run(tc.name+"/"+mode, func(t *testing.T) {
				h := newOAuthHarness(t, mode)
				h.enable("oidc", "")
				target := tc.run(h)
				if tc.want == "invalid_provider" {
					target = h.eng.Prefix() + "/providers/bogus/callback?state=s&code=c"
				}
				rec := h.get(target, true)
				wantLoc := "/login?oauth_error=" + tc.want
				if rec.Code != http.StatusFound || rec.Header().Get("Location") != wantLoc {
					t.Errorf("status = %d location = %q, want 302 %q", rec.Code, rec.Header().Get("Location"), wantLoc)
				}
			})
		}
	}
}

func TestOAuthGoldenCallbackSuccess(t *testing.T) {
	id := fakeIdentity{Sub: "sub-new", Email: "new.user@example.test", Name: "New User", Verified: true}
	for _, mode := range []string{modeLibrary} {
		t.Run(mode, func(t *testing.T) {
			h := newOAuthHarness(t, mode)
			h.enable("oidc", "")
			rec := h.signIn("oidc", id)
			if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/oauth/complete" {
				t.Fatalf("status = %d location = %q, want 302 /oauth/complete, body = %s", rec.Code, rec.Header().Get("Location"), rec.Body.String())
			}
			userID, ok := h.sessionUser(rec)
			if !ok {
				t.Fatal("no valid session issued")
			}
			ctx := context.Background()
			user, err := h.db.GetUserByEmail(ctx, id.Email)
			if err != nil || user.ID != userID {
				t.Fatalf("session user %q, store user %+v err %v", userID, user, err)
			}
			if len(user.Abilities) != 1 || user.Abilities[0] != AbilityRead {
				t.Errorf("abilities = %v, want [read]", user.Abilities)
			}
			ident, err := h.db.GetOAuthIdentity(ctx, "oidc", id.Sub)
			if err != nil || ident.UserID != userID {
				t.Errorf("legacy identity = %+v err %v, want linked to %s", ident, err, userID)
			}
			again := h.signIn("oidc", id)
			if again.Header().Get("Location") != "/oauth/complete" {
				t.Fatalf("second sign-in location = %q", again.Header().Get("Location"))
			}
			if second, _ := h.sessionUser(again); second != userID {
				t.Errorf("second sign-in user = %q, want %q", second, userID)
			}
		})
	}
}

func TestOAuthGoldenAllProvidersSignIn(t *testing.T) {
	ids := map[string]fakeIdentity{
		"google":    {Sub: "g-1", Email: "g@example.test", Name: "G", Verified: true},
		"github":    {Sub: "4242", Email: "gh@example.test", Name: "GH", Verified: true},
		"microsoft": {Sub: "ms-object-id", Email: "ms@example.test", Name: "MS", Verified: false},
		"oidc":      {Sub: "o-1", Email: "o@example.test", Name: "O", Verified: true},
	}
	for _, provider := range allOAuthProviders {
		t.Run(provider, func(t *testing.T) {
			h := newOAuthHarness(t, modeLibrary)
			h.enable(provider, "")
			rec := h.signIn(provider, ids[provider])
			if rec.Header().Get("Location") != "/oauth/complete" {
				t.Fatalf("location = %q, want /oauth/complete", rec.Header().Get("Location"))
			}
			if _, err := h.db.GetOAuthIdentity(context.Background(), provider, ids[provider].Sub); err != nil {
				t.Errorf("identity keyed by the in-house subject not stored: %v", err)
			}
		})
	}
}

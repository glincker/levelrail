package api

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

// fakeOIDCIssuer serves discovery, JWKS, and token-exchange endpoints
// backed by one RSA key, so newOIDCOAuthClient and FetchUserInfo can be
// tested against a real (if minimal) OIDC provider instead of mocks.
type fakeOIDCIssuer struct {
	srv         *httptest.Server
	key         *rsa.PrivateKey
	clientID    string
	nextIDToken string
}

func newFakeOIDCIssuer(t *testing.T) *fakeOIDCIssuer {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	f := &fakeOIDCIssuer{key: key, clientID: "test-client"}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", f.handleDiscovery)
	mux.HandleFunc("/keys", f.handleJWKS)
	mux.HandleFunc("/token", f.handleToken)
	mux.HandleFunc("/authorize", func(_ http.ResponseWriter, _ *http.Request) {})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeOIDCIssuer) handleDiscovery(w http.ResponseWriter, _ *http.Request) {
	_ = json.NewEncoder(w).Encode(map[string]string{
		"issuer":                 f.srv.URL,
		"authorization_endpoint": f.srv.URL + "/authorize",
		"token_endpoint":         f.srv.URL + "/token",
		"jwks_uri":               f.srv.URL + "/keys",
	})
}

func (f *fakeOIDCIssuer) handleJWKS(w http.ResponseWriter, _ *http.Request) {
	jwk := jose.JSONWebKey{Key: &f.key.PublicKey, Algorithm: "RS256", Use: "sig", KeyID: "test-key"}
	_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{jwk}})
}

func (f *fakeOIDCIssuer) handleToken(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"access_token": "test-access-token",
		"token_type":   "Bearer",
		"expires_in":   3600,
		"id_token":     f.nextIDToken,
	})
}

// signIDToken builds a signed ID token for this issuer's key, with sane
// defaults callers can override via claims before signing.
func (f *fakeOIDCIssuer) signIDToken(t *testing.T, claims map[string]any) string {
	t.Helper()
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: f.key}, &jose.SignerOptions{
		ExtraHeaders: map[jose.HeaderKey]any{"kid": "test-key"},
	})
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}
	base := map[string]any{
		"iss": f.srv.URL,
		"aud": f.clientID,
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Unix(),
	}
	for k, v := range claims {
		base[k] = v
	}
	raw, err := jwt.Signed(signer).Claims(base).Serialize()
	if err != nil {
		t.Fatalf("sign id token: %v", err)
	}
	return raw
}

func TestNewOIDCOAuthClient_DiscoveryAndVerify(t *testing.T) {
	issuer := newFakeOIDCIssuer(t)
	issuer.nextIDToken = issuer.signIDToken(t, map[string]any{
		"sub": "user-123", "email": "person@example.com", "email_verified": true, "name": "Person",
	})

	settings := store.OAuthProviderSettings{Provider: store.OAuthProviderOIDC, ClientID: issuer.clientID, IssuerURL: issuer.srv.URL}
	client, err := newOIDCOAuthClient(settings, "secret", "https://levelrail.example/callback")
	if err != nil {
		t.Fatalf("newOIDCOAuthClient: %v", err)
	}

	if url := client.AuthCodeURL("state123"); url == "" {
		t.Fatal("AuthCodeURL returned empty string")
	}

	token, err := client.Exchange(context.Background(), "unused-code")
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	info, err := client.FetchUserInfo(context.Background(), token)
	if err != nil {
		t.Fatalf("FetchUserInfo: %v", err)
	}
	if info.ProviderUserID != "user-123" || info.Email != "person@example.com" || info.DisplayName != "Person" {
		t.Errorf("FetchUserInfo = %+v, want sub=user-123 email=person@example.com name=Person", info)
	}
}

func TestOIDCOAuthClient_FetchUserInfo_RejectsInvalidClaims(t *testing.T) {
	tests := []struct {
		name   string
		claims map[string]any
	}{
		{"unverified email", map[string]any{"sub": "user-123", "email": "person@example.com", "email_verified": false}},
		{"wrong audience", map[string]any{"sub": "user-123", "email": "person@example.com", "email_verified": true, "aud": "someone-else"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			issuer := newFakeOIDCIssuer(t)
			issuer.nextIDToken = issuer.signIDToken(t, tc.claims)
			settings := store.OAuthProviderSettings{Provider: store.OAuthProviderOIDC, ClientID: issuer.clientID, IssuerURL: issuer.srv.URL}
			client, err := newOIDCOAuthClient(settings, "secret", "https://levelrail.example/callback")
			if err != nil {
				t.Fatalf("newOIDCOAuthClient: %v", err)
			}
			token, err := client.Exchange(context.Background(), "unused-code")
			if err != nil {
				t.Fatalf("Exchange: %v", err)
			}
			if _, err := client.FetchUserInfo(context.Background(), token); err == nil {
				t.Fatal("FetchUserInfo succeeded, want error")
			}
		})
	}
}

func TestNewOIDCOAuthClient_DiscoveryFailure(t *testing.T) {
	settings := store.OAuthProviderSettings{Provider: store.OAuthProviderOIDC, ClientID: "x", IssuerURL: "http://127.0.0.1:1/nonexistent"}
	if _, err := newOIDCOAuthClient(settings, "secret", "https://levelrail.example/callback"); err == nil {
		t.Fatal("newOIDCOAuthClient succeeded against an unreachable issuer, want error")
	}
}

func TestDefaultOAuthClientFactory_OIDC(t *testing.T) {
	issuer := newFakeOIDCIssuer(t)
	settings := store.OAuthProviderSettings{Provider: store.OAuthProviderOIDC, ClientID: issuer.clientID, IssuerURL: issuer.srv.URL}
	client, err := defaultOAuthClientFactory(store.OAuthProviderOIDC, settings, "secret", "https://levelrail.example/callback")
	if err != nil {
		t.Fatalf("defaultOAuthClientFactory: %v", err)
	}
	if _, ok := client.(*oidcOAuthClient); !ok {
		t.Errorf("defaultOAuthClientFactory returned %T, want *oidcOAuthClient", client)
	}
}

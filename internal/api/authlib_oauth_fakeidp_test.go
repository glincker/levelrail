package api

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"

	"github.com/GLINCKER/levelrail/internal/authengine"
)

// fakeIdentity is who the fake identity provider authenticates for one code.
type fakeIdentity struct {
	Sub      string
	Email    string
	Name     string
	Verified bool
}

type fakeGrant struct {
	id          fakeIdentity
	nonce       string
	clientID    string
	redirectURI string
}

// fakeIDP serves OIDC discovery, JWKS, token and userinfo, plus the Google,
// GitHub and Microsoft Graph shapes, so every provider can be exercised end to end.
type fakeIDP struct {
	srv    *httptest.Server
	key    *rsa.PrivateKey
	mu     sync.Mutex
	grants map[string]fakeGrant
	seq    int
}

func newFakeIDP(t *testing.T) *fakeIDP {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	f := &fakeIDP{key: key, grants: map[string]fakeGrant{}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/openid-configuration", f.discovery)
	mux.HandleFunc("GET /jwks", f.jwks)
	mux.HandleFunc("POST /token", f.token)
	mux.HandleFunc("GET /userinfo", f.userinfo)
	mux.HandleFunc("GET /github/user", f.githubUser)
	mux.HandleFunc("GET /github/emails", f.githubEmails)
	mux.HandleFunc("GET /graph/me", f.graphMe)
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeIDP) Issuer() string { return f.srv.URL }

func (f *fakeIDP) endpoints() authengine.OAuthEndpoints {
	u := f.srv.URL
	return authengine.OAuthEndpoints{
		GoogleAuthorize: u + "/authorize", GoogleToken: u + "/token", GoogleUserInfo: u + "/userinfo",
		GitHubAuthorize: u + "/authorize", GitHubToken: u + "/token", GitHubUser: u + "/github/user", GitHubEmails: u + "/github/emails",
		MicrosoftAuthorize: u + "/authorize", MicrosoftToken: u + "/token", MicrosoftGraphMe: u + "/graph/me",
		AllowInsecureOIDC: true,
	}
}

// grant registers a code for id from the authorize URL the application redirected to.
func (f *fakeIDP) grant(t *testing.T, authorizeURL string, id fakeIdentity) string {
	t.Helper()
	u, err := url.Parse(authorizeURL)
	if err != nil {
		t.Fatalf("parse authorize url: %v", err)
	}
	q := u.Query()
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seq++
	code := "code-" + strconv.Itoa(f.seq)
	f.grants[code] = fakeGrant{id: id, nonce: q.Get("nonce"), clientID: q.Get("client_id"), redirectURI: q.Get("redirect_uri")}
	return code
}

func (f *fakeIDP) byToken(r *http.Request) (fakeGrant, bool) {
	code := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer at-")
	f.mu.Lock()
	defer f.mu.Unlock()
	g, ok := f.grants[code]
	return g, ok
}

func writeFakeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func (f *fakeIDP) discovery(w http.ResponseWriter, _ *http.Request) {
	u := f.srv.URL
	writeFakeJSON(w, map[string]any{
		"issuer": u, "authorization_endpoint": u + "/authorize", "token_endpoint": u + "/token",
		"jwks_uri": u + "/jwks", "userinfo_endpoint": u + "/userinfo",
		"id_token_signing_alg_values_supported": []string{"RS256"},
		"code_challenge_methods_supported":      []string{"S256"},
	})
}

func (f *fakeIDP) jwks(w http.ResponseWriter, _ *http.Request) {
	writeFakeJSON(w, jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &f.key.PublicKey, KeyID: "k1", Algorithm: "RS256", Use: "sig"}}})
}

func (f *fakeIDP) token(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	g, ok := f.grants[r.PostForm.Get("code")]
	f.mu.Unlock()
	if !ok || r.PostForm.Get("redirect_uri") != g.redirectURI || r.PostForm.Get("code_verifier") == "" {
		w.WriteHeader(http.StatusBadRequest)
		writeFakeJSON(w, map[string]string{"error": "invalid_grant"})
		return
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: f.key}, (&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", "k1"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	claims := map[string]any{
		"iss": f.srv.URL, "aud": g.clientID, "sub": g.id.Sub, "email": g.id.Email, "email_verified": g.id.Verified,
		"name": g.id.Name, "nonce": g.nonce, "exp": time.Now().Add(time.Hour).Unix(), "iat": time.Now().Unix(),
	}
	raw, err := jwt.Signed(signer).Claims(claims).Serialize()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeFakeJSON(w, map[string]any{
		"access_token": "at-" + r.PostForm.Get("code"), "token_type": "Bearer", "expires_in": 3600, "id_token": raw,
	})
}

func (f *fakeIDP) userinfo(w http.ResponseWriter, r *http.Request) {
	g, ok := f.byToken(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	writeFakeJSON(w, map[string]any{"sub": g.id.Sub, "email": g.id.Email, "email_verified": g.id.Verified, "name": g.id.Name})
}

func (f *fakeIDP) githubUser(w http.ResponseWriter, r *http.Request) {
	g, ok := f.byToken(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	id, _ := strconv.ParseInt(g.id.Sub, 10, 64)
	writeFakeJSON(w, map[string]any{"id": id, "login": "login-" + g.id.Sub, "name": g.id.Name, "email": ""})
}

func (f *fakeIDP) githubEmails(w http.ResponseWriter, r *http.Request) {
	g, ok := f.byToken(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	writeFakeJSON(w, []map[string]any{{"email": g.id.Email, "primary": true, "verified": g.id.Verified}})
}

func (f *fakeIDP) graphMe(w http.ResponseWriter, r *http.Request) {
	g, ok := f.byToken(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	writeFakeJSON(w, map[string]any{"id": g.id.Sub, "mail": g.id.Email, "userPrincipalName": g.id.Email, "displayName": g.id.Name})
}

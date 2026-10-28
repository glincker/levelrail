package api

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

const (
	ciTestAudience = "https://cp.example.com"
	ciTestOrg      = "6f1d2c3b-0000-4000-8000-000000000001"
	ciTestProject  = "6f1d2c3b-0000-4000-8000-000000000002"
	ciTestContext  = "6f1d2c3b-0000-4000-8000-000000000003"
)

type ciTestIDP struct {
	srv     *httptest.Server
	rsa     *rsa.PrivateKey
	ec      *ecdsa.PrivateKey
	fetches atomic.Int32
	body    []byte
}

func newCITestIDP(t *testing.T) *ciTestIDP {
	t.Helper()
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	idp := &ciTestIDP{rsa: rsaKey, ec: ecKey}
	idp.body, err = json.Marshal(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{
		{Key: rsaKey.Public(), KeyID: "rsa1", Algorithm: string(jose.RS256), Use: "sig"},
		{Key: ecKey.Public(), KeyID: "ec1", Algorithm: string(jose.ES256), Use: "sig"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	idp.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		idp.fetches.Add(1)
		_, _ = w.Write(idp.body)
	}))
	t.Cleanup(idp.srv.Close)
	return idp
}

func allowAnyCIAddr(netip.Addr) bool { return true }

func testCIOIDCConfig() ciOIDCConfig {
	return ciOIDCConfig{ClockSkew: 30 * time.Second, FetchTimeout: 5 * time.Second, MaxJWKSBytes: 64 << 10, MinRefresh: time.Minute, KeysTTL: time.Hour}
}

func (idp *ciTestIDP) verifier(t *testing.T, cfg ciOIDCConfig, allow func(netip.Addr) bool) *ciJWTVerifier {
	t.Helper()
	v := newCIJWTVerifier(cfg, allow)
	pool := x509.NewCertPool()
	pool.AddCert(idp.srv.Certificate())
	v.client.Transport.(*http.Transport).TLSClientConfig = &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
	return v
}

func (idp *ciTestIDP) sign(t *testing.T, alg jose.SignatureAlgorithm, kid string, claims map[string]any) string {
	t.Helper()
	var key any
	switch alg {
	case jose.RS256:
		key = idp.rsa
	case jose.ES256:
		key = idp.ec
	default:
		key = []byte(strings.Repeat("k", 32))
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: alg, Key: jose.JSONWebKey{Key: key, KeyID: kid}}, (&jose.SignerOptions{}).WithType("JWT"))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := jwt.Signed(signer).Claims(claims).Serialize()
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func unsignedCIToken(t *testing.T, claims map[string]any) string {
	t.Helper()
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	enc := base64.RawURLEncoding.EncodeToString
	return enc([]byte(`{"alg":"none","kid":"rsa1"}`)) + "." + enc(payload) + "."
}

func gitLabTestClaims(issuer string, mut func(map[string]any)) map[string]any {
	now := time.Now()
	c := map[string]any{
		"iss": issuer, "aud": ciTestAudience, "sub": "project_path:acme/web:ref_type:branch:ref:main",
		"project_path": "acme/web", "namespace_path": "acme", "ref": "main", "ref_type": "branch",
		"ref_protected": "true", "environment": "production", "environment_protected": "true", "pipeline_source": "push", "job_id": "42",
		"exp": now.Add(5 * time.Minute).Unix(), "nbf": now.Add(-time.Minute).Unix(), "iat": now.Add(-time.Minute).Unix(),
	}
	if mut != nil {
		mut(c)
	}
	return c
}

func circleCITestClaims(issuer string, mut func(map[string]any)) map[string]any {
	now := time.Now()
	c := map[string]any{
		"iss": issuer, "aud": ciTestAudience, "sub": "org/" + ciTestOrg + "/project/" + ciTestProject + "/user/u1",
		circleCIClaimProjectID: ciTestProject, circleCIClaimContextIDs: []string{ciTestContext}, circleCIClaimVCSRef: "refs/heads/main",
		"exp": now.Add(5 * time.Minute).Unix(), "iat": now.Add(-time.Minute).Unix(),
	}
	if mut != nil {
		mut(c)
	}
	return c
}

func TestCIOIDCVerifierAcceptsEachProvider(t *testing.T) {
	idp := newCITestIDP(t)
	v := idp.verifier(t, testCIOIDCConfig(), allowAnyCIAddr)
	gl, _ := ciProviderSpecFor(ciProviderGitLab, idp.srv.URL)
	cc, _ := ciProviderSpecFor(ciProviderCircleCI, idp.srv.URL+"/org/"+ciTestOrg)
	cases := []struct {
		name string
		spec ciProviderSpec
		tok  string
		want CIClaims
	}{
		{"gitlab rs256", gl, idp.sign(t, jose.RS256, "rsa1", gitLabTestClaims(gl.Issuer, nil)),
			CIClaims{Provider: ciProviderGitLab, Repository: "acme/web", Ref: "refs/heads/main", Environment: "production", RefProtected: true}},
		{"gitlab es256 tag", gl, idp.sign(t, jose.ES256, "ec1", gitLabTestClaims(gl.Issuer, func(c map[string]any) {
			c["ref"], c["ref_type"], c["ref_protected"] = "v1.2.0", "tag", false
		})), CIClaims{Provider: ciProviderGitLab, Repository: "acme/web", Ref: "refs/tags/v1.2.0", Environment: "production"}},
		{"circleci", cc, idp.sign(t, jose.RS256, "rsa1", circleCITestClaims(cc.Issuer, nil)),
			CIClaims{Provider: ciProviderCircleCI, Repository: ciTestProject, Ref: "refs/heads/main"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := v.VerifyProvider(context.Background(), tc.tok, tc.spec, ciTestAudience)
			if err != nil {
				t.Fatalf("verify: %v", err)
			}
			if got.Provider != tc.want.Provider || got.Repository != tc.want.Repository || got.Ref != tc.want.Ref ||
				got.Environment != tc.want.Environment || got.RefProtected != tc.want.RefProtected || got.Issuer != tc.spec.Issuer {
				t.Fatalf("claims = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestCIOIDCVerifierRejects(t *testing.T) {
	idp := newCITestIDP(t)
	gl, _ := ciProviderSpecFor(ciProviderGitLab, idp.srv.URL)
	cc, _ := ciProviderSpecFor(ciProviderCircleCI, idp.srv.URL+"/org/"+ciTestOrg)
	now := time.Now()
	gitlab := func(mut func(map[string]any)) map[string]any { return gitLabTestClaims(gl.Issuer, mut) }
	cases := []struct {
		name string
		spec ciProviderSpec
		tok  string
	}{
		{"wrong issuer", gl, idp.sign(t, jose.RS256, "rsa1", gitlab(func(c map[string]any) { c["iss"] = "https://gitlab.com" }))},
		{"wrong audience", gl, idp.sign(t, jose.RS256, "rsa1", gitlab(func(c map[string]any) { c["aud"] = "https://other.example.com" }))},
		{"no audience", gl, idp.sign(t, jose.RS256, "rsa1", gitlab(func(c map[string]any) { delete(c, "aud") }))},
		{"expired", gl, idp.sign(t, jose.RS256, "rsa1", gitlab(func(c map[string]any) { c["exp"] = now.Add(-5 * time.Minute).Unix() }))},
		{"no exp", gl, idp.sign(t, jose.RS256, "rsa1", gitlab(func(c map[string]any) { delete(c, "exp") }))},
		{"future nbf", gl, idp.sign(t, jose.RS256, "rsa1", gitlab(func(c map[string]any) { c["nbf"] = now.Add(5 * time.Minute).Unix() }))},
		{"future iat", gl, idp.sign(t, jose.RS256, "rsa1", gitlab(func(c map[string]any) { c["iat"] = now.Add(5 * time.Minute).Unix() }))},
		{"alg none", gl, unsignedCIToken(t, gitlab(nil))},
		{"hs256", gl, idp.sign(t, jose.HS256, "rsa1", gitlab(nil))},
		{"unknown kid", gl, idp.sign(t, jose.RS256, "nope", gitlab(nil))},
		{"no kid", gl, idp.sign(t, jose.RS256, "", gitlab(nil))},
		{"rsa token on ec kid", gl, idp.sign(t, jose.RS256, "ec1", gitlab(nil))},
		{"gitlab merge ref type", gl, idp.sign(t, jose.RS256, "rsa1", gitlab(func(c map[string]any) { c["ref_type"] = "" }))},
		{"gitlab project outside namespace", gl, idp.sign(t, jose.RS256, "rsa1", gitlab(func(c map[string]any) { c["namespace_path"] = "evil" }))},
		{"circleci other org", cc, idp.sign(t, jose.RS256, "rsa1", circleCITestClaims(cc.Issuer, func(c map[string]any) {
			c["sub"] = "org/other/project/" + ciTestProject + "/user/u1"
		}))},
		{"circleci project mismatch", cc, idp.sign(t, jose.RS256, "rsa1", circleCITestClaims(cc.Issuer, func(c map[string]any) {
			c[circleCIClaimProjectID] = "someone-else"
		}))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := idp.verifier(t, testCIOIDCConfig(), allowAnyCIAddr)
			if _, err := v.VerifyProvider(context.Background(), tc.tok, tc.spec, ciTestAudience); err == nil {
				t.Fatal("token accepted")
			}
		})
	}

	cfg := testCIOIDCConfig()
	cfg.MaxTokenAge = time.Minute
	v := idp.verifier(t, cfg, allowAnyCIAddr)
	old := idp.sign(t, jose.RS256, "rsa1", gitlab(func(c map[string]any) { c["iat"] = now.Add(-10 * time.Minute).Unix() }))
	if _, err := v.VerifyProvider(context.Background(), old, gl, ciTestAudience); err == nil {
		t.Fatal("token older than APP_CI_OIDC_MAX_TOKEN_AGE accepted")
	}
}

func TestCIOIDCVerifierJWKSFetchGuards(t *testing.T) {
	idp := newCITestIDP(t)
	gl, _ := ciProviderSpecFor(ciProviderGitLab, idp.srv.URL)
	tok := idp.sign(t, jose.RS256, "rsa1", gitLabTestClaims(gl.Issuer, nil))
	ctx := context.Background()

	if _, err := idp.verifier(t, testCIOIDCConfig(), ciPublicAddr).VerifyProvider(ctx, tok, gl, ciTestAudience); !errors.Is(err, errCIJWKSBlockedAddr) {
		t.Fatalf("jwks on loopback: err = %v, want blocked", err)
	}
	plain := gl
	plain.JWKSURL = strings.Replace(gl.JWKSURL, "https://", "http://", 1)
	if _, err := idp.verifier(t, testCIOIDCConfig(), allowAnyCIAddr).VerifyProvider(ctx, tok, plain, ciTestAudience); err == nil {
		t.Fatal("http jwks url accepted")
	}

	small := testCIOIDCConfig()
	small.MaxJWKSBytes = 64
	if _, err := idp.verifier(t, small, allowAnyCIAddr).VerifyProvider(ctx, tok, gl, ciTestAudience); err == nil {
		t.Fatal("oversized jwks accepted")
	}

	redirect := httptest.NewTLSServer(http.RedirectHandler("https://example.com/keys", http.StatusFound))
	defer redirect.Close()
	rv := idp.verifier(t, testCIOIDCConfig(), allowAnyCIAddr)
	pool := x509.NewCertPool()
	pool.AddCert(redirect.Certificate())
	rv.client.Transport.(*http.Transport).TLSClientConfig = &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
	cross := gl
	cross.JWKSURL = redirect.URL + gitLabJWKSPath
	if _, err := rv.VerifyProvider(ctx, tok, cross, ciTestAudience); !errors.Is(err, errCIJWKSRedirect) {
		t.Fatalf("cross-host redirect: err = %v", err)
	}

	idp.fetches.Store(0)
	v := idp.verifier(t, testCIOIDCConfig(), allowAnyCIAddr)
	for _, kid := range []string{"forged-1", "forged-2", "forged-3"} {
		if _, err := v.VerifyProvider(ctx, idp.sign(t, jose.RS256, kid, gitLabTestClaims(gl.Issuer, nil)), gl, ciTestAudience); err == nil {
			t.Fatalf("kid %s accepted", kid)
		}
	}
	if _, err := v.VerifyProvider(ctx, tok, gl, ciTestAudience); err != nil {
		t.Fatalf("known kid after refresh: %v", err)
	}
	if n := idp.fetches.Load(); n != 1 {
		t.Fatalf("jwks fetched %d times, want 1 inside the refresh window", n)
	}
}

func TestCIPublicAddr(t *testing.T) {
	for addr, want := range map[string]bool{
		"127.0.0.1": false, "10.1.2.3": false, "172.16.0.1": false, "192.168.1.1": false, "169.254.169.254": false,
		"100.64.0.1": false, "0.0.0.0": false, "0.1.2.3": false, "::1": false, "fd00::1": false, "fe80::1": false,
		"::ffff:127.0.0.1": false, "::ffff:10.0.0.1": false, "8.8.8.8": true, "2606:4700::1111": true,
	} {
		if got := ciPublicAddr(netip.MustParseAddr(addr)); got != want {
			t.Errorf("ciPublicAddr(%s) = %v, want %v", addr, got, want)
		}
	}
}

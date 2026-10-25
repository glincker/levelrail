package api

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"

	"github.com/GLINCKER/levelrail/internal/store"
)

const stackOIDCAudience = "https://cp.example.com"

type fakeStackVerifier struct {
	claims   CIStackClaims
	err      error
	audience string
}

func (f *fakeStackVerifier) Verify(ctx context.Context, raw, aud string) (CIClaims, error) {
	c, err := f.VerifyStack(ctx, raw, aud)
	return c.CIClaims, err
}

func (f *fakeStackVerifier) VerifyStack(_ context.Context, _ string, aud string) (CIStackClaims, error) {
	f.audience = aud
	if f.err != nil {
		return CIStackClaims{}, f.err
	}
	if aud != stackOIDCAudience {
		return CIStackClaims{}, errors.New("audience mismatch")
	}
	return f.claims, nil
}

func ghClaims(repo, ref, env string) CIStackClaims {
	return CIStackClaims{
		CIClaims: CIClaims{Repository: repo, Ref: ref, Environment: env, Subject: "repo:" + repo + ":ref:" + ref},
		SHA:      "0123456789abcdef", RunID: "98765",
	}
}

type oidcStackEnv struct {
	rt     *Router
	db     *store.DB
	cookie *http.Cookie
	ver    *fakeStackVerifier
}

func newOIDCStackEnv(t *testing.T) *oidcStackEnv {
	t.Helper()
	rt, db, cookie, _, _ := stackTokenFixture(t, applyVerbs, []string{AbilityRead, AbilityWrite})
	if err := db.SetDashboardURL(t.Context(), stackOIDCAudience); err != nil {
		t.Fatal(err)
	}
	ver := &fakeStackVerifier{}
	rt.SetCIIDTokenVerifier(ver)
	return &oidcStackEnv{rt: rt, db: db, cookie: cookie, ver: ver}
}

func (e *oidcStackEnv) addTrust(t *testing.T, stack string, body map[string]any) (int, stackTrustResource) {
	t.Helper()
	var out stackTrustResource
	code := iacCall(t, e.rt, authedRequest(t, e.cookie, http.MethodPost, "/api/v1/stacks/"+stack+"/oidc-trusts", jsonBody(t, body)), &out)
	return code, out
}

func (e *oidcStackEnv) mustTrust(t *testing.T, body map[string]any) stackTrustResource {
	t.Helper()
	code, out := e.addTrust(t, "shop-prod", body)
	if code != http.StatusCreated {
		t.Fatalf("create trust %v = %d", body, code)
	}
	return out
}

func (e *oidcStackEnv) exchange(t *testing.T, stack string) (int, stackExchangeResponse) {
	t.Helper()
	var out stackExchangeResponse
	body := jsonBody(t, map[string]string{"stack": stack, "id_token": "jwt"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ci/stack-oidc-exchange", strings.NewReader(body))
	code := iacCall(t, e.rt, req, &out)
	return code, out
}

func (e *oidcStackEnv) auditActions(t *testing.T, action string) []store.AuditEntry {
	t.Helper()
	all, err := e.db.ListAuditEntries(t.Context(), 200, nil, store.AuditEntryFilter{})
	if err != nil {
		t.Fatal(err)
	}
	var out []store.AuditEntry
	for _, a := range all {
		if a.Action == action {
			out = append(out, a)
		}
	}
	return out
}

func TestStackOIDCExchangePlanTrustForPullRequests(t *testing.T) {
	e := newOIDCStackEnv(t)
	t.Setenv("APP_STACK_OIDC_TTL", "2m")
	e.mustTrust(t, map[string]any{"repository": "acme/shop", "ref_pattern": "refs/pull/*/merge", "verbs": []string{"plan"}})
	e.ver.claims = ghClaims("acme/shop", "refs/pull/42/merge", "")

	before := time.Now()
	code, out := e.exchange(t, "shop-prod")
	if code != http.StatusOK || out.Token == "" {
		t.Fatalf("exchange = %d %+v", code, out)
	}
	if got := out.ExpiresAt.Sub(before); got < time.Minute || got > 3*time.Minute {
		t.Fatalf("TTL = %s, want about 2m from APP_STACK_OIDC_TTL", got)
	}
	if len(out.Verbs) != 1 || out.Verbs[0] != "plan" {
		t.Fatalf("verbs = %v", out.Verbs)
	}
	rec := stackTokenRecord(t, e.db, out.Token)
	if rec.Name != "ci:stack:shop-prod" || rec.ExpiresAt == nil || rec.ExpiresAt.After(out.ExpiresAt.Add(time.Second)) {
		t.Fatalf("token record = %+v", rec)
	}

	plan := func(stack string) int {
		return callWithToken(t, e.rt, out.Token, http.MethodPost, "/api/v1/stacks/"+stack+"/plan", stackBody(t, iacTestDocs, nil))
	}
	if c := plan("shop-prod"); c != http.StatusOK {
		t.Fatalf("plan own stack = %d", c)
	}
	if c := callWithToken(t, e.rt, out.Token, http.MethodPost, "/api/v1/stacks/shop-prod/apply", stackBody(t, iacTestDocs, nil)); c != http.StatusForbidden {
		t.Fatalf("PR-ref token applied: %d", c)
	}
	if c := plan("blog-prod"); c != http.StatusForbidden {
		t.Fatalf("plan other stack = %d", c)
	}
}

func stackTokenRecord(t *testing.T, db *store.DB, plaintext string) store.APIToken {
	t.Helper()
	tokens, err := db.ListAPITokens(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	h := hashToken(plaintext)
	for _, tk := range tokens {
		if tk.TokenHash == h {
			return tk
		}
	}
	t.Fatal("minted token not stored")
	return store.APIToken{}
}

func TestStackOIDCApplyNeedsRefAndEnvironment(t *testing.T) {
	e := newOIDCStackEnv(t)
	e.mustTrust(t, map[string]any{"repository": "acme/shop", "ref_pattern": "refs/heads/main", "environment": "production", "verbs": []string{"plan", "apply"}})

	for name, c := range map[string]CIStackClaims{
		"main without environment":  ghClaims("acme/shop", "refs/heads/main", ""),
		"main in staging":           ghClaims("acme/shop", "refs/heads/main", "staging"),
		"production on other ref":   ghClaims("acme/shop", "refs/heads/dev", "production"),
		"production from a PR":      ghClaims("acme/shop", "refs/pull/1/merge", "production"),
		"wrong repo":                ghClaims("evil/shop", "refs/heads/main", "production"),
		"case-changed repo":         ghClaims("Acme/shop", "refs/heads/main", "production"),
		"prefix repo":               ghClaims("acme/shop-fork", "refs/heads/main", "production"),
		"missing ref claim":         ghClaims("acme/shop", "", "production"),
		"missing repository claim":  ghClaims("", "refs/heads/main", "production"),
		"ref with trailing segment": ghClaims("acme/shop", "refs/heads/main/x", "production"),
	} {
		e.ver.claims = c
		if code, out := e.exchange(t, "shop-prod"); code != http.StatusForbidden || out.Token != "" {
			t.Errorf("%s = %d, want 403", name, code)
		}
	}

	e.ver.claims = ghClaims("acme/shop", "refs/heads/main", "production")
	code, out := e.exchange(t, "shop-prod")
	if code != http.StatusOK {
		t.Fatalf("matching claims = %d", code)
	}
	var applied stackApplyResponse
	c := iacCall(t, e.rt, bearerRequest(http.MethodPost, "/api/v1/stacks/shop-prod/apply", stackBody(t, iacTestDocs, nil), out.Token), &applied)
	if c != http.StatusOK || applied.Revision == nil || applied.Revision.Status != store.StackRevApplied {
		t.Fatalf("apply = %d %+v", c, applied)
	}
	// The default lifetime is 15 minutes when the env var is unset.
	if d := time.Until(out.ExpiresAt); d < 14*time.Minute || d > 16*time.Minute {
		t.Fatalf("default TTL = %s", d)
	}
}

func TestStackOIDCTokenCannotLeaveItsStack(t *testing.T) {
	e := newOIDCStackEnv(t)
	e.mustTrust(t, map[string]any{"repository": "acme/shop", "ref_pattern": "refs/heads/main", "environment": "production", "verbs": []string{"plan", "apply"}})
	e.ver.claims = ghClaims("acme/shop", "refs/heads/main", "production")
	_, out := e.exchange(t, "shop-prod")

	for _, c := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/stacks"},
		{http.MethodPatch, "/api/v1/stacks/shop-prod"},
		{http.MethodDelete, "/api/v1/stacks/shop-prod"},
		{http.MethodPost, "/api/v1/stacks/shop-prod/unlock"},
		{http.MethodPut, "/api/v1/stacks/shop-prod/pin"},
		{http.MethodPost, "/api/v1/stacks/shop-prod/revisions/1/approve"},
		{http.MethodGet, "/api/v1/stacks/shop-prod/oidc-trusts"},
		{http.MethodPost, "/api/v1/stacks/shop-prod/oidc-trusts"},
		{http.MethodDelete, "/api/v1/stacks/shop-prod/oidc-trusts/cst_x"},
		{http.MethodGet, "/api/v1/stacks/blog-prod"},
		{http.MethodPost, "/api/v1/stacks/blog-prod/apply"},
		{http.MethodGet, "/api/v1/apps"},
		{http.MethodGet, "/api/v1/auth/tokens"},
		{http.MethodPost, "/api/v1/auth/tokens"},
	} {
		if code := callWithToken(t, e.rt, out.Token, c.method, c.path, `{}`); code < 400 {
			t.Errorf("%s %s = %d, a stack OIDC token reached it", c.method, c.path, code)
		}
	}
}

func TestStackOIDCTrustOfOneStackIsUselessOnAnother(t *testing.T) {
	e := newOIDCStackEnv(t)
	e.mustTrust(t, map[string]any{"repository": "acme/shop", "ref_pattern": "refs/heads/main", "environment": "production", "verbs": []string{"apply"}})
	e.ver.claims = ghClaims("acme/shop", "refs/heads/main", "production")
	for _, stack := range []string{"blog-prod", "nope", ""} {
		if code, _ := e.exchange(t, stack); code != http.StatusForbidden && code != http.StatusBadRequest {
			t.Errorf("exchange for %q = %d", stack, code)
		}
	}
	if code, _ := e.exchange(t, "shop-prod"); code != http.StatusOK {
		t.Fatalf("own stack = %d", code)
	}
}

func TestStackOIDCRejectsBadIdentityTokens(t *testing.T) {
	e := newOIDCStackEnv(t)
	e.mustTrust(t, map[string]any{"repository": "acme/shop", "ref_pattern": "refs/heads/main", "environment": "production", "verbs": []string{"apply"}})
	e.ver.claims = ghClaims("acme/shop", "refs/heads/main", "production")

	e.ver.err = errors.New("token is expired")
	if code, _ := e.exchange(t, "shop-prod"); code != http.StatusUnauthorized {
		t.Fatalf("expired = %d", code)
	}
	e.ver.err = nil
	if err := e.db.SetDashboardURL(t.Context(), "https://other.example.com"); err != nil {
		t.Fatal(err)
	}
	if code, _ := e.exchange(t, "shop-prod"); code != http.StatusUnauthorized {
		t.Fatalf("wrong audience = %d", code)
	}
	if err := e.db.SetDashboardURL(t.Context(), ""); err != nil {
		t.Fatal(err)
	}
	if code, _ := e.exchange(t, "shop-prod"); code != http.StatusConflict {
		t.Fatalf("no dashboard url = %d", code)
	}
}

func TestStackOIDCAmbiguousTrustsFailClosed(t *testing.T) {
	e := newOIDCStackEnv(t)
	e.mustTrust(t, map[string]any{"repository": "acme/shop", "ref_pattern": "refs/heads/release/*", "verbs": []string{"plan"}})
	e.mustTrust(t, map[string]any{"repository": "acme/shop", "ref_pattern": "refs/heads/rel*/1", "verbs": []string{"plan"}})
	e.ver.claims = ghClaims("acme/shop", "refs/heads/release/1", "")
	if code, _ := e.exchange(t, "shop-prod"); code != http.StatusForbidden {
		t.Fatalf("ambiguous = %d, want 403", code)
	}
	e.ver.claims = ghClaims("acme/shop", "refs/heads/release/2", "")
	if code, _ := e.exchange(t, "shop-prod"); code != http.StatusOK {
		t.Fatalf("single match = %d", code)
	}
}

func TestStackOIDCTrustValidation(t *testing.T) {
	e := newOIDCStackEnv(t)
	bad := map[string]map[string]any{
		"wildcard only ref":      {"repository": "acme/shop", "ref_pattern": "*"},
		"refs star":              {"repository": "acme/shop", "ref_pattern": "refs/*"},
		"refs star star":         {"repository": "acme/shop", "ref_pattern": "refs/*/*"},
		"double star":            {"repository": "acme/shop", "ref_pattern": "refs/heads/**"},
		"two stars in segment":   {"repository": "acme/shop", "ref_pattern": "refs/heads/a*b*"},
		"regex":                  {"repository": "acme/shop", "ref_pattern": "refs/heads/(main|dev)"},
		"char class":             {"repository": "acme/shop", "ref_pattern": "refs/heads/[a-z]"},
		"empty segment":          {"repository": "acme/shop", "ref_pattern": "refs/heads//x"},
		"no repository":          {"ref_pattern": "refs/heads/main"},
		"wildcard repository":    {"repository": "acme/*", "ref_pattern": "refs/heads/main"},
		"org wildcard":           {"repository": "*/*", "ref_pattern": "refs/heads/main"},
		"apply on PR":            {"repository": "acme/shop", "ref_pattern": "refs/pull/*/merge", "environment": "production", "verbs": []string{"apply"}},
		"apply on wildcard ref":  {"repository": "acme/shop", "ref_pattern": "refs/heads/*", "environment": "production", "verbs": []string{"apply"}},
		"apply without env":      {"repository": "acme/shop", "ref_pattern": "refs/heads/main", "verbs": []string{"apply"}},
		"wildcard environment":   {"repository": "acme/shop", "ref_pattern": "refs/heads/main", "environment": "prod*", "verbs": []string{"plan"}},
		"destroy verb":           {"repository": "acme/shop", "ref_pattern": "refs/heads/main", "verbs": []string{"destroy"}},
		"subject for other repo": {"repository": "acme/shop", "ref_pattern": "refs/heads/main", "subject_pattern": "repo:evil/x:*"},
	}
	for name, body := range bad {
		if code, _ := e.addTrust(t, "shop-prod", body); code != http.StatusBadRequest {
			t.Errorf("%s = %d, want 400", name, code)
		}
	}
	if code, _ := e.addTrust(t, "ghost", map[string]any{"repository": "acme/shop", "ref_pattern": "refs/heads/main"}); code != http.StatusNotFound {
		t.Errorf("unknown stack = %d", code)
	}
	good := map[string]any{"repository": "acme/shop", "ref_pattern": "refs/heads/release/*"}
	if code, tr := e.addTrust(t, "shop-prod", good); code != http.StatusCreated || len(tr.Verbs) != 1 || tr.Verbs[0] != "plan" {
		t.Fatalf("good trust = %d %+v (verbs default to plan)", code, tr)
	}
	if code, _ := e.addTrust(t, "shop-prod", good); code != http.StatusConflict {
		t.Errorf("duplicate = %d", code)
	}
}

func TestStackOIDCSubjectPatternNarrowsTrust(t *testing.T) {
	e := newOIDCStackEnv(t)
	e.mustTrust(t, map[string]any{
		"repository": "acme/shop", "ref_pattern": "refs/heads/main", "environment": "production",
		"subject_pattern": "repo:acme/shop:environment:production", "verbs": []string{"apply"},
	})
	c := ghClaims("acme/shop", "refs/heads/main", "production")
	e.ver.claims = c
	if code, _ := e.exchange(t, "shop-prod"); code != http.StatusForbidden {
		t.Fatalf("subject mismatch = %d", code)
	}
	c.Subject = "repo:acme/shop:environment:production"
	e.ver.claims = c
	if code, _ := e.exchange(t, "shop-prod"); code != http.StatusOK {
		t.Fatalf("subject match = %d", code)
	}
}

func TestStackOIDCTrustAdminOnly(t *testing.T) {
	e := newOIDCStackEnv(t)
	tr := e.mustTrust(t, map[string]any{"repository": "acme/shop", "ref_pattern": "refs/heads/main"})
	body := jsonBody(t, map[string]any{"repository": "acme/shop", "ref_pattern": "refs/heads/dev"})
	if code := iacCall(t, e.rt, httptest.NewRequest(http.MethodPost, "/api/v1/stacks/shop-prod/oidc-trusts", strings.NewReader(body)), nil); code != http.StatusUnauthorized {
		t.Fatalf("anonymous create = %d", code)
	}
	readTok, _ := mintStackToken(t, e.rt, e.cookie, "shop-prod", []string{"plan"}, []string{AbilityRead})
	if code := callWithToken(t, e.rt, readTok, http.MethodGet, "/api/v1/stacks/shop-prod/oidc-trusts", ""); code < 400 {
		t.Fatalf("stack token listed trusts: %d", code)
	}
	var list struct{ Trusts []stackTrustResource }
	if code := iacCall(t, e.rt, authedRequest(t, e.cookie, http.MethodGet, "/api/v1/stacks/shop-prod/oidc-trusts", ""), &list); code != http.StatusOK || len(list.Trusts) != 1 {
		t.Fatalf("list = %d %+v", code, list)
	}

	e.ver.claims = ghClaims("acme/shop", "refs/heads/main", "")
	if code, _ := e.exchange(t, "shop-prod"); code != http.StatusOK {
		t.Fatalf("exchange = %d", code)
	}
	// Deleting through the wrong stack must not remove the trust.
	if code := iacCall(t, e.rt, authedRequest(t, e.cookie, http.MethodDelete, "/api/v1/stacks/blog-prod/oidc-trusts/"+tr.ID, ""), nil); code != http.StatusNotFound {
		t.Fatalf("delete via other stack = %d", code)
	}
	if code := iacCall(t, e.rt, authedRequest(t, e.cookie, http.MethodDelete, "/api/v1/stacks/shop-prod/oidc-trusts/"+tr.ID, ""), nil); code != http.StatusNoContent {
		t.Fatalf("delete = %d", code)
	}
	if code, _ := e.exchange(t, "shop-prod"); code != http.StatusForbidden {
		t.Fatalf("exchange after delete = %d", code)
	}
}

func TestStackOIDCAuditNeverHoldsTheToken(t *testing.T) {
	e := newOIDCStackEnv(t)
	e.mustTrust(t, map[string]any{"repository": "acme/shop", "ref_pattern": "refs/heads/main"})
	e.ver.claims = ghClaims("acme/shop", "refs/heads/main", "")
	_, out := e.exchange(t, "shop-prod")
	e.ver.claims = ghClaims("evil/shop", "refs/heads/main", "")
	e.exchange(t, "shop-prod")

	ok := e.auditActions(t, auditStackOIDCExchange)
	if len(ok) != 1 {
		t.Fatalf("exchange audit entries = %d", len(ok))
	}
	for _, want := range []string{"acme/shop", "refs/heads/main", "0123456789abcdef", "98765", "shop-prod"} {
		if !strings.Contains(ok[0].Detail, want) {
			t.Errorf("audit detail %s lacks %q", ok[0].Detail, want)
		}
	}
	var all strings.Builder
	for _, a := range e.auditActions(t, auditStackOIDCExchange) {
		all.WriteString(a.Detail + a.Path)
	}
	if strings.Contains(all.String(), out.Token) {
		t.Fatal("token leaked into the audit trail")
	}
	if n := len(e.auditActions(t, auditStackOIDCDenied)); n != 1 {
		t.Fatalf("denied audit entries = %d", n)
	}
	if n := len(e.auditActions(t, auditStackTrustCreate)); n != 1 {
		t.Fatalf("trust create audit entries = %d", n)
	}
}

func TestGlobMatchIsSegmentWise(t *testing.T) {
	for _, c := range []struct {
		pattern, value string
		want           bool
	}{
		{"refs/heads/main", "refs/heads/main", true},
		{"refs/heads/main", "refs/heads/main2", false},
		{"refs/pull/*/merge", "refs/pull/42/merge", true},
		{"refs/pull/*/merge", "refs/pull/4/2/merge", false},
		{"refs/pull/*/merge", "refs/pull//merge", false},
		{"refs/heads/*", "refs/heads/feature/x", false},
		{"refs/tags/v*", "refs/tags/v1.2.3", true},
		{"refs/tags/v*", "refs/tags/v", false},
		{"refs/tags/v*", "refs/tags/x1", false},
		{"refs/heads/**", "refs/heads/a", false},
		{"refs/heads/[a]", "refs/heads/a", false},
		{"refs/heads/?", "refs/heads/a", false},
	} {
		if got := globMatch(c.pattern, c.value); got != c.want {
			t.Errorf("globMatch(%q, %q) = %v, want %v", c.pattern, c.value, got, c.want)
		}
	}
}

// The real verifier, against a local issuer: wrong audience, expiry and a foreign key all fail.
func TestGitHubStackVerifierChecksSignatureAudienceAndExpiry(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	other, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	var issuer string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(w).Encode(map[string]any{"issuer": issuer, "jwks_uri": issuer + "/jwks", "id_token_signing_alg_values_supported": []string{"RS256"}})
		case "/jwks":
			_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "k1", Algorithm: "RS256", Use: "sig"}}})
		}
	}))
	defer srv.Close()
	issuer = srv.URL
	provider, err := oidc.NewProvider(t.Context(), issuer)
	if err != nil {
		t.Fatal(err)
	}
	v := &githubOIDCVerifier{provider: provider}

	sign := func(k *rsa.PrivateKey, iss, aud string, exp time.Time) string {
		sig, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: jose.JSONWebKey{Key: k, KeyID: "k1"}}, (&jose.SignerOptions{}).WithType("JWT"))
		if err != nil {
			t.Fatal(err)
		}
		raw, err := jwt.Signed(sig).Claims(jwt.Claims{Issuer: iss, Subject: "repo:acme/shop:ref:refs/heads/main", Audience: jwt.Audience{aud}, Expiry: jwt.NewNumericDate(exp), IssuedAt: jwt.NewNumericDate(time.Now())}).
			Claims(map[string]any{"repository": "acme/shop", "ref": "refs/heads/main", "environment": "production", "sha": "abc", "run_id": "7"}).Serialize()
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	future := time.Now().Add(5 * time.Minute)
	got, err := v.VerifyStack(t.Context(), sign(key, issuer, stackOIDCAudience, future), stackOIDCAudience)
	if err != nil || got.Repository != "acme/shop" || got.Environment != "production" || got.SHA != "abc" || got.RunID != "7" {
		t.Fatalf("valid token: %+v, %v", got, err)
	}
	for name, raw := range map[string]string{
		"wrong audience": sign(key, issuer, "https://evil.example.com", future),
		"wrong issuer":   sign(key, "https://evil.example.com", stackOIDCAudience, future),
		"expired":        sign(key, issuer, stackOIDCAudience, time.Now().Add(-time.Hour)),
		"foreign key":    sign(other, issuer, stackOIDCAudience, future),
		"garbage":        "not.a.jwt",
	} {
		if _, err := v.VerifyStack(t.Context(), raw, stackOIDCAudience); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/experimental"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	oauthTestIssuer   = "https://lr.example.com"
	oauthTestVerifier = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOP0123456789-._~"
	oauthTestRedirect = "https://client.example.org/callback"
)

type mcpOAuthHarness struct {
	t      *testing.T
	rt     *Router
	db     *store.DB
	h      http.Handler
	cookie *http.Cookie
	client string
}

func newMCPOAuthHarness(t *testing.T) *mcpOAuthHarness {
	t.Helper()
	mcpEnabled(t)
	t.Setenv(envDashboardURL, oauthTestIssuer)
	rt, db := newTestRouter(t)
	o := &mcpOAuthHarness{t: t, rt: rt, db: db, h: rt.Handler(), cookie: loginTestSession(t, rt, db)}
	rec := o.json(o.cookie, http.MethodPost, "/api/v1/mcp/oauth/clients",
		`{"name":"Test Client","redirect_uris":["`+oauthTestRedirect+`","http://127.0.0.1/cb"],"scopes":["mcp:read","mcp:read:sensitive"]}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create client = %d %s", rec.Code, rec.Body.String())
	}
	var c mcpClientResource
	_ = json.Unmarshal(rec.Body.Bytes(), &c)
	o.client = c.ID
	return o
}

func (o *mcpOAuthHarness) json(c *http.Cookie, method, target, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, target, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if c != nil {
		r.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	o.h.ServeHTTP(rec, r)
	return rec
}

func (o *mcpOAuthHarness) authorizeQuery(extra map[string]string) url.Values {
	q := url.Values{
		"response_type": {"code"}, "client_id": {o.client}, "redirect_uri": {oauthTestRedirect}, "state": {"st-1"},
		"code_challenge": {sha256Hex(oauthTestVerifier)}, "code_challenge_method": {"S256"}, "scope": {"mcp:read mcp:read:sensitive"},
	}
	for k, v := range extra {
		if v == "" {
			q.Del(k)
		} else {
			q.Set(k, v)
		}
	}
	return q
}

func (o *mcpOAuthHarness) authorize(q url.Values) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	o.h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/oauth/authorize?"+q.Encode(), nil))
	return rec
}

// approve runs authorize then consent approval and returns the redirect the user is sent back to.
func (o *mcpOAuthHarness) approve(cookie *http.Cookie, q url.Values) *url.URL {
	o.t.Helper()
	rec := o.authorize(q)
	if rec.Code != http.StatusFound {
		o.t.Fatalf("authorize = %d %s", rec.Code, rec.Body.String())
	}
	loc, _ := url.Parse(rec.Header().Get("Location"))
	id := loc.Query().Get("request")
	if id == "" || loc.Path != mcpOAuthConsentPath {
		o.t.Fatalf("authorize redirected to %q, want the consent page with a request id", loc)
	}
	if get := o.json(cookie, http.MethodGet, "/api/v1/mcp/oauth/consent/"+id, ""); get.Code != http.StatusOK {
		o.t.Fatalf("consent get = %d %s", get.Code, get.Body.String())
	}
	ap := o.json(cookie, http.MethodPost, "/api/v1/mcp/oauth/consent/"+id+"/approve", `{}`)
	if ap.Code != http.StatusOK {
		o.t.Fatalf("approve = %d %s", ap.Code, ap.Body.String())
	}
	var res mcpConsentResult
	_ = json.Unmarshal(ap.Body.Bytes(), &res)
	u, _ := url.Parse(res.RedirectTo)
	return u
}

func (o *mcpOAuthHarness) token(form url.Values) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	o.h.ServeHTTP(rec, r)
	return rec
}

func (o *mcpOAuthHarness) exchange(code string) *httptest.ResponseRecorder {
	return o.token(url.Values{"grant_type": {"authorization_code"}, "client_id": {o.client}, "code": {code},
		"redirect_uri": {oauthTestRedirect}, "code_verifier": {oauthTestVerifier}})
}

func decodeTokens(t *testing.T, rec *httptest.ResponseRecorder) mcpTokenResponse {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("token = %d %s", rec.Code, rec.Body.String())
	}
	var tr mcpTokenResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &tr); err != nil {
		t.Fatal(err)
	}
	return tr
}

func (o *mcpOAuthHarness) issue() mcpTokenResponse {
	o.t.Helper()
	return decodeTokens(o.t, o.exchange(o.approve(o.cookie, o.authorizeQuery(nil)).Query().Get("code")))
}

func (o *mcpOAuthHarness) bearerGet(token, path string) int {
	r := httptest.NewRequest(http.MethodGet, path, nil)
	r.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	o.h.ServeHTTP(rec, r)
	return rec.Code
}

func TestMCPOAuth_FlagOffReturns404(t *testing.T) {
	rt, _ := newTestRouter(t)
	experimental.Set()
	t.Cleanup(func() { experimental.Set(experimental.All()...) })
	for _, p := range []string{"/.well-known/oauth-protected-resource", "/.well-known/oauth-authorization-server", "/oauth/authorize", "/api/v1/mcp/oauth/clients"} {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, p, nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s with the flag off = %d, want 404", p, rec.Code)
		}
	}
}

func TestMCPOAuth_Discovery(t *testing.T) {
	o := newMCPOAuthHarness(t)
	rec := httptest.NewRecorder()
	o.h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/.well-known/oauth-protected-resource", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), oauthTestIssuer+"/mcp") {
		t.Fatalf("protected resource = %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	o.h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/.well-known/oauth-authorization-server", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"code_challenge_methods_supported":["S256"]`) {
		t.Fatalf("authorization server = %d %s", rec.Code, rec.Body.String())
	}
	unauth := mcpPost(o.rt, o.h, "", mcpListBody, nil)
	if got := unauth.Header().Get("WWW-Authenticate"); !strings.Contains(got, `resource_metadata="`+oauthTestIssuer+`/.well-known/oauth-protected-resource"`) {
		t.Fatalf("401 challenge = %q, want resource_metadata hint", got)
	}
}

func TestMCPOAuth_PKCEMandatoryS256(t *testing.T) {
	o := newMCPOAuthHarness(t)
	for name, extra := range map[string]map[string]string{
		"plain":     {"code_challenge_method": "plain"},
		"missing":   {"code_challenge": "", "code_challenge_method": ""},
		"no-method": {"code_challenge_method": ""},
	} {
		rec := o.authorize(o.authorizeQuery(extra))
		loc, _ := url.Parse(rec.Header().Get("Location"))
		if rec.Code != http.StatusFound || loc.Query().Get("error") != "invalid_request" || loc.Query().Get("state") != "st-1" {
			t.Errorf("%s: authorize = %d %v, want redirect with invalid_request and state echoed", name, rec.Code, loc)
		}
	}
}

func TestMCPOAuth_RedirectMismatch(t *testing.T) {
	o := newMCPOAuthHarness(t)
	rec := o.authorize(o.authorizeQuery(map[string]string{"redirect_uri": oauthTestRedirect + "/other"}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("mismatched redirect = %d, want 400 and no redirect", rec.Code)
	}
	if rec := o.authorize(o.authorizeQuery(map[string]string{"redirect_uri": "http://127.0.0.1:51234/other"})); rec.Code != http.StatusBadRequest {
		t.Fatalf("loopback with other path = %d, want 400", rec.Code)
	}
}

func TestMCPOAuth_LoopbackPortIsFree(t *testing.T) {
	o := newMCPOAuthHarness(t)
	q := o.authorizeQuery(map[string]string{"redirect_uri": "http://127.0.0.1:51234/cb"})
	if rec := o.authorize(q); rec.Code != http.StatusFound {
		t.Fatalf("loopback port variation = %d %s, want accepted", rec.Code, rec.Body.String())
	}
}

func TestMCPOAuth_CodeSingleUseAndWrongVerifier(t *testing.T) {
	o := newMCPOAuthHarness(t)
	code := o.approve(o.cookie, o.authorizeQuery(nil)).Query().Get("code")
	bad := o.token(url.Values{"grant_type": {"authorization_code"}, "client_id": {o.client}, "code": {code},
		"redirect_uri": {oauthTestRedirect}, "code_verifier": {strings.Repeat("x", 50)}})
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("wrong verifier = %d, want 400", bad.Code)
	}
	// The failed attempt burned the code.
	if rec := o.exchange(code); rec.Code != http.StatusBadRequest {
		t.Fatalf("code after a failed attempt = %d, want 400", rec.Code)
	}
	code2 := o.approve(o.cookie, o.authorizeQuery(nil)).Query().Get("code")
	tr := decodeTokens(t, o.exchange(code2))
	if rec := o.exchange(code2); rec.Code != http.StatusBadRequest {
		t.Fatalf("second redemption = %d, want 400", rec.Code)
	}
	// Replaying a used code also ends the grant it created.
	if rec := mcpPost(o.rt, o.h, tr.AccessToken, mcpListBody, nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("token of a replayed code = %d, want 401", rec.Code)
	}
}

func TestMCPOAuth_AudienceBinding(t *testing.T) {
	o := newMCPOAuthHarness(t)
	tr := o.issue()
	if tr.TokenType != "Bearer" || tr.RefreshToken == "" || tr.ExpiresIn <= 0 {
		t.Fatalf("token response = %+v", tr)
	}
	rec := mcpPost(o.rt, o.h, tr.AccessToken, mcpListBody, nil)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"tools"`) {
		t.Fatalf("OAuth token on /mcp = %d %s", rec.Code, rec.Body.String())
	}
	for _, p := range []string{"/api/v1/apps", "/api/v1/auth/my-access", "/api/v1/auth/tokens"} {
		if code := o.bearerGet(tr.AccessToken, p); code != http.StatusUnauthorized {
			t.Errorf("OAuth token on %s = %d, want 401", p, code)
		}
	}
}

func TestMCPOAuth_TokenIsAgentNamedAndListed(t *testing.T) {
	o := newMCPOAuthHarness(t)
	tr := o.issue()
	rec := o.json(o.cookie, http.MethodGet, "/api/v1/auth/tokens", "")
	if !strings.Contains(rec.Body.String(), "mcp:Test Client") {
		t.Fatalf("token list lacks the OAuth token: %s", rec.Body.String())
	}
	_ = tr
}

func TestMCPOAuth_RefreshRotationAndReuseRevokesGrant(t *testing.T) {
	o := newMCPOAuthHarness(t)
	first := o.issue()
	refresh := func(tok string) *httptest.ResponseRecorder {
		return o.token(url.Values{"grant_type": {"refresh_token"}, "client_id": {o.client}, "refresh_token": {tok}})
	}
	second := decodeTokens(t, refresh(first.RefreshToken))
	if second.RefreshToken == first.RefreshToken || second.AccessToken == first.AccessToken {
		t.Fatal("refresh must rotate both tokens")
	}
	if rec := mcpPost(o.rt, o.h, second.AccessToken, mcpListBody, nil); rec.Code != http.StatusOK {
		t.Fatalf("rotated access token on /mcp = %d", rec.Code)
	}
	// Presenting the spent refresh token is theft: the whole grant dies.
	if rec := refresh(first.RefreshToken); rec.Code != http.StatusBadRequest {
		t.Fatalf("reused refresh token = %d, want 400", rec.Code)
	}
	if rec := refresh(second.RefreshToken); rec.Code != http.StatusBadRequest {
		t.Fatalf("newest refresh token after reuse = %d, want 400", rec.Code)
	}
	if rec := mcpPost(o.rt, o.h, second.AccessToken, mcpListBody, nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("access token after reuse = %d, want 401", rec.Code)
	}
}

func TestMCPOAuth_ScopeCappedAtApprover(t *testing.T) {
	o := newMCPOAuthHarness(t)
	u := storeUserWithAbilitiesForTest(t, o.db, "reader@example.com", []string{AbilityRead})
	cookie := sessionCookieForTest(t, o.rt, u.ID)
	tr := decodeTokens(t, o.exchange(o.approve(cookie, o.authorizeQuery(nil)).Query().Get("code")))
	if tr.Scope != mcpScopeRead {
		t.Fatalf("granted scope = %q, want only mcp:read for a user without read:sensitive", tr.Scope)
	}
	q := o.authorizeQuery(map[string]string{"scope": "mcp:read:sensitive"})
	rec := o.authorize(q)
	loc, _ := url.Parse(rec.Header().Get("Location"))
	ap := o.json(cookie, http.MethodPost, "/api/v1/mcp/oauth/consent/"+loc.Query().Get("request")+"/approve", `{}`)
	if ap.Code != http.StatusForbidden {
		t.Fatalf("approving a scope the user lacks = %d, want 403", ap.Code)
	}
	// Write tiers are never offered.
	if rec := o.authorize(o.authorizeQuery(map[string]string{"scope": "write"})); rec.Header().Get("Location") == "" || !strings.Contains(rec.Header().Get("Location"), "invalid_scope") {
		t.Fatalf("write scope = %d %v, want invalid_scope", rec.Code, rec.Header())
	}
}

func TestMCPOAuth_RevokedTokenAndGrant(t *testing.T) {
	o := newMCPOAuthHarness(t)
	tr := o.issue()
	list := o.json(o.cookie, http.MethodGet, "/api/v1/auth/tokens", "")
	var toks []struct {
		ID    string         `json:"id"`
		Agent *agentIdentity `json:"agent"`
	}
	_ = json.Unmarshal(list.Body.Bytes(), &toks)
	var id string
	for _, tk := range toks {
		if tk.Agent != nil && strings.HasPrefix(tk.Agent.Name, "mcp:") {
			id = tk.ID
		}
	}
	if id == "" {
		t.Fatalf("no mcp token in list: %s", list.Body.String())
	}
	if rec := o.json(o.cookie, http.MethodDelete, "/api/v1/auth/tokens/"+id, ""); rec.Code >= 300 {
		t.Fatalf("revoke = %d %s", rec.Code, rec.Body.String())
	}
	if rec := mcpPost(o.rt, o.h, tr.AccessToken, mcpListBody, nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("revoked token on /mcp = %d, want 401", rec.Code)
	}
	// Revoking the token also ends the refresh path.
	rec := o.token(url.Values{"grant_type": {"refresh_token"}, "client_id": {o.client}, "refresh_token": {tr.RefreshToken}})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("refresh after revoke = %d, want 400", rec.Code)
	}
}

func TestMCPOAuth_DenyAndDeleteClient(t *testing.T) {
	o := newMCPOAuthHarness(t)
	rec := o.authorize(o.authorizeQuery(nil))
	loc, _ := url.Parse(rec.Header().Get("Location"))
	deny := o.json(o.cookie, http.MethodPost, "/api/v1/mcp/oauth/consent/"+loc.Query().Get("request")+"/deny", `{}`)
	if deny.Code != http.StatusOK || !strings.Contains(deny.Body.String(), "access_denied") {
		t.Fatalf("deny = %d %s", deny.Code, deny.Body.String())
	}
	tr := o.issue()
	if rec := o.json(o.cookie, http.MethodDelete, "/api/v1/mcp/oauth/clients/"+o.client, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete client = %d %s", rec.Code, rec.Body.String())
	}
	if rec := mcpPost(o.rt, o.h, tr.AccessToken, mcpListBody, nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("token of a deleted client = %d, want 401", rec.Code)
	}
}

func TestMCPOAuth_ResourceIndicator(t *testing.T) {
	o := newMCPOAuthHarness(t)
	rec := o.authorize(o.authorizeQuery(map[string]string{"resource": "https://other.example.com/mcp"}))
	if !strings.Contains(rec.Header().Get("Location"), "invalid_target") {
		t.Fatalf("foreign resource = %d %v, want invalid_target", rec.Code, rec.Header())
	}
}

func TestMCPOAuth_TokenRevokeEndsGrantAndAuthorizeIsLimited(t *testing.T) {
	t.Setenv(envMCPOAuthAuthorizeRPM, "2")
	o := newMCPOAuthHarness(t)
	tr := o.issue()
	// Revoking the access token must end the grant even if the token row is later purged.
	rows, err := o.db.QueryContext(context.Background(), `SELECT token_id FROM mcp_oauth_tokens`)
	if err != nil {
		t.Fatal(err)
	}
	var tokenID string
	if rows.Next() {
		_ = rows.Scan(&tokenID)
	}
	_ = rows.Close()
	if err := o.rt.libraryRevokeToken(context.Background(), tokenID); err != nil {
		t.Fatalf("revoke = %v", err)
	}
	g, err := o.db.GetMCPOAuthGrantByToken(context.Background(), tokenID)
	if err != nil || !g.Revoked {
		t.Fatalf("grant after token revoke = %+v, %v; want revoked", g, err)
	}
	if rec := o.token(url.Values{"grant_type": {"refresh_token"}, "client_id": {o.client}, "refresh_token": {tr.RefreshToken}}); rec.Code != http.StatusBadRequest {
		t.Fatalf("refresh after revoke = %d", rec.Code)
	}
	var last int
	for range 4 {
		last = o.authorize(o.authorizeQuery(nil)).Code
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("authorize past the limit = %d, want 429", last)
	}
}

func TestMCPAbilitiesForSensitiveOnlyKeepsRead(t *testing.T) {
	got := mcpAbilitiesFor([]string{mcpScopeReadSensitive})
	if !hasAbility(got, AbilityRead) || !hasAbility(got, AbilityReadSensitive) {
		t.Fatalf("abilities = %v", got)
	}
}

func TestMCPOAuthAudienceFailsClosedWithoutStore(t *testing.T) {
	rt := &Router{}
	ctx := context.Background()
	if rt.mcpOAuthAudienceOK(ctx, &store.APIToken{ID: "t1", AgentName: mcpOAuthAgentPrefix + "client"}) {
		t.Fatal("an OAuth token must be refused when the grant store is missing")
	}
	if !rt.mcpOAuthAudienceOK(ctx, &store.APIToken{ID: "t2", AgentName: "ci-deploy"}) {
		t.Fatal("a non-OAuth token must not depend on the OAuth store")
	}
}

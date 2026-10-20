package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	locComplete       = "/oauth/complete"
	locNotProvisioned = "/login?oauth_error=sso_not_provisioned"
	locNoRole         = "/login?oauth_error=sso_no_role"
	locDomain         = "/login?oauth_error=domain_not_allowed"
)

func (h *oauthHarness) policy(p store.OAuthSSOPolicy) {
	h.t.Helper()
	if p.GroupsClaim == "" {
		p.GroupsClaim = "groups"
	}
	if p.DefaultRoleID == "" {
		p.DefaultRoleID = defaultOIDCRoleID
	}
	h.setPolicy("oidc", p)
}

func (h *oauthHarness) mapGroups(mappings ...store.SSOGroupMapping) {
	h.t.Helper()
	if err := h.db.ReplaceSSOGroupMappings(context.Background(), mappings, time.Now()); err != nil {
		h.t.Fatal(err)
	}
}

func (h *oauthHarness) roleOf(email string) string {
	h.t.Helper()
	u, err := h.db.GetUserByEmail(context.Background(), strings.ToLower(email))
	if err != nil {
		h.t.Fatalf("user %s: %v", email, err)
	}
	id, err := h.db.UserRoleID(context.Background(), u.ID)
	if err != nil {
		h.t.Fatal(err)
	}
	return id
}

func (h *oauthHarness) hasUser(email string) bool {
	_, err := h.db.GetUserByEmail(context.Background(), strings.ToLower(email))
	return err == nil
}

func (h *oauthHarness) auditPaths(action string) []string {
	h.t.Helper()
	rows, err := h.db.Query(`SELECT path FROM audit_log WHERE action = ? ORDER BY created_at`, action)
	if err != nil {
		h.t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			h.t.Fatal(err)
		}
		out = append(out, p)
	}
	return out
}

func groupsID(sub, email string, groups ...any) fakeIdentity {
	return fakeIdentity{Sub: sub, Email: email, Verified: true, Claims: map[string]any{"groups": groups}}
}

func TestOIDCSignInPolicy(t *testing.T) {
	tests := []struct {
		name      string
		policy    store.OAuthSSOPolicy
		mappings  []store.SSOGroupMapping
		id        fakeIdentity
		want      string
		wantRole  string
		wantAudit string
	}{
		{"jit off refuses an unknown user", store.OAuthSSOPolicy{}, nil, groupsID("s1", "new@corp.test"), locNotProvisioned, "", auditActionOIDCRejected},
		{"jit on provisions the default role", store.OAuthSSOPolicy{JITProvisioning: true}, nil, groupsID("s2", "new@corp.test", "eng"), locComplete, "role_viewer", auditActionOIDCProvisioned},
		{"mapped group wins over the default", store.OAuthSSOPolicy{JITProvisioning: true},
			[]store.SSOGroupMapping{{Group: "Eng", RoleID: "role_operator"}}, groupsID("s3", "new@corp.test", "eng"), locComplete, "role_operator", auditActionOIDCProvisioned},
		{"unmapped group falls back to viewer", store.OAuthSSOPolicy{JITProvisioning: true},
			[]store.SSOGroupMapping{{Group: "eng", RoleID: "role_operator"}}, groupsID("s4", "new@corp.test", "sales"), locComplete, "role_viewer", auditActionOIDCProvisioned},
		{"a bare string claim is one group, never split", store.OAuthSSOPolicy{JITProvisioning: true},
			[]store.SSOGroupMapping{{Group: "eng", RoleID: "role_operator"}},
			fakeIdentity{Sub: "s5", Email: "new@corp.test", Verified: true, Claims: map[string]any{"groups": "eng,ops"}}, locComplete, "role_viewer", auditActionOIDCProvisioned},
		{"claim name is configurable and may be nested", store.OAuthSSOPolicy{JITProvisioning: true, GroupsClaim: "realm_access.roles"},
			[]store.SSOGroupMapping{{Group: "deployer", RoleID: "role_operator"}},
			fakeIdentity{Sub: "s6", Email: "new@corp.test", Verified: true, Claims: map[string]any{"realm_access": map[string]any{"roles": []any{"deployer"}}}}, locComplete, "role_operator", auditActionOIDCProvisioned},
		{"unverified email is refused", store.OAuthSSOPolicy{JITProvisioning: true}, nil,
			fakeIdentity{Sub: "s7", Email: "new@corp.test", Verified: false}, "/login?oauth_error=userinfo_failed", "", auditActionOIDCRejected},
		{"email outside the allowed domains is refused", store.OAuthSSOPolicy{JITProvisioning: true, AllowedDomains: []string{"corp.test"}}, nil,
			groupsID("s8", "new@other.test"), locDomain, "", auditActionOIDCRejected},
		{"email inside the allowed domains is accepted", store.OAuthSSOPolicy{JITProvisioning: true, AllowedDomains: []string{"corp.test"}}, nil,
			groupsID("s9", "new@CORP.test"), locComplete, "role_viewer", auditActionOIDCProvisioned},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newOAuthHarness(t, modeLibrary)
			h.enable("oidc", "")
			h.policy(tc.policy)
			h.mapGroups(tc.mappings...)
			rec := h.signIn("oidc", tc.id)
			if got := oauthResult(rec); rec.Code != http.StatusFound || got != tc.want {
				t.Fatalf("status = %d location = %q, want 302 %q", rec.Code, got, tc.want)
			}
			if tc.wantRole == "" {
				if h.hasUser(tc.id.Email) {
					t.Errorf("a refused sign-in left an account for %s", tc.id.Email)
				}
			} else if got := h.roleOf(tc.id.Email); got != tc.wantRole {
				t.Errorf("role = %q, want %q", got, tc.wantRole)
			}
			if len(h.auditPaths(tc.wantAudit)) == 0 {
				t.Errorf("no %s audit row", tc.wantAudit)
			}
		})
	}
}

func TestOIDCRefusalAuditNamesTheCode(t *testing.T) {
	h := newOAuthHarness(t, modeLibrary)
	h.enable("oidc", "")
	h.policy(store.OAuthSSOPolicy{})
	h.signIn("oidc", groupsID("a1", "who@corp.test"))
	paths := h.auditPaths(auditActionOIDCRejected)
	if len(paths) != 1 || !strings.Contains(paths[0], "oidc") || !strings.Contains(paths[0], "code=sso_not_provisioned") {
		t.Fatalf("audit paths = %v", paths)
	}
}

func TestOIDCNeverGrantsRootFromClaims(t *testing.T) {
	h := newOAuthHarness(t, modeLibrary)
	h.enable("oidc", "")
	h.policy(store.OAuthSSOPolicy{JITProvisioning: true, SyncRoles: true})

	rec := h.admin(http.MethodPut, "/api/v1/sso/group-mappings", `{"mappings":[{"group":"admins","role_id":"role_admin"}]}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("mapping to a root role: %d %s, want 400", rec.Code, rec.Body)
	}
	if rec := h.admin(http.MethodPut, "/api/v1/sso/oidc/oidc", `{"default_role_id":"role_admin"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("root default role: %d %s, want 400", rec.Code, rec.Body)
	}

	// A mapping that reached the table by another way still cannot grant root.
	if _, err := h.db.Exec(`INSERT INTO sso_group_role_mappings (group_name, role_id, priority, created_at) VALUES ('admins', 'role_admin', 0, ?)`, time.Now().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if got := oauthResult(h.signIn("oidc", groupsID("r1", "root-try@corp.test", "admins"))); got != locNoRole {
		t.Fatalf("new user with a root mapping: %q, want %q", got, locNoRole)
	}
	if h.hasUser("root-try@corp.test") {
		t.Error("a refused root grant left an account behind")
	}

	existing := storeUserWithAbilitiesForTest(t, h.db, "viewer@corp.test", []string{AbilityRead})
	if _, err := h.rt.libSessions.SyncUser(context.Background(), toLibUser(existing)); err != nil {
		t.Fatal(err)
	}
	if got := oauthResult(h.signIn("oidc", groupsID("r2", "viewer@corp.test", "admins"))); got != locNoRole {
		t.Fatalf("existing user with a root mapping: %q, want %q", got, locNoRole)
	}
	u, err := h.db.GetUserByEmail(context.Background(), "viewer@corp.test")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(u.Abilities, []string{AbilityRead}) {
		t.Errorf("abilities = %v, want unchanged [read]", u.Abilities)
	}
}

func (h *oauthHarness) admin(method, path, body string) *httptest.ResponseRecorder {
	h.t.Helper()
	if h.adminCookie == nil {
		h.adminCookie = loginTestSession(h.t, h.rt, h.db)
	}
	rec := httptest.NewRecorder()
	h.rt.Handler().ServeHTTP(rec, authedRequest(h.t, h.adminCookie, method, path, body))
	return rec
}

func TestOIDCRoleSync(t *testing.T) {
	tests := []struct {
		name      string
		sync      bool
		abilities []string
		groups    []any
		wantRole  string
		wantAudit string
	}{
		{"sync off keeps an operator an operator", false, []string{AbilityRead, AbilityReadSensitive, AbilityWrite, AbilityDeploy}, nil, "", ""},
		{"sync on downgrades and audits it", true, []string{AbilityRead, AbilityReadSensitive, AbilityWrite, AbilityDeploy}, nil, "role_viewer", "direction=downgrade"},
		{"sync on upgrades through a mapping", true, []string{AbilityRead}, []any{"ops"}, "role_operator", "direction=upgrade"},
		{"sync on never touches a root user", true, []string{AbilityRoot}, nil, "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newOAuthHarness(t, modeLibrary)
			h.enable("oidc", "")
			h.policy(store.OAuthSSOPolicy{SyncRoles: tc.sync})
			h.mapGroups(store.SSOGroupMapping{Group: "ops", RoleID: "role_operator"})
			u := storeUserWithAbilitiesForTest(t, h.db, "person@corp.test", tc.abilities)
			if _, err := h.rt.libSessions.SyncUser(context.Background(), toLibUser(u)); err != nil {
				t.Fatal(err)
			}
			before, _ := h.db.UserRoleID(context.Background(), u.ID)
			if got := oauthResult(h.signIn("oidc", groupsID("p1", "person@corp.test", tc.groups...))); got != locComplete {
				t.Fatalf("sign-in: %q", got)
			}
			after := h.roleOf("person@corp.test")
			want := tc.wantRole
			if want == "" {
				want = before
			}
			if after != want {
				t.Errorf("role = %q, want %q", after, want)
			}
			paths := h.auditPaths(auditActionOIDCRoleChanged)
			if tc.wantAudit == "" && len(paths) != 0 {
				t.Errorf("unexpected role audit rows: %v", paths)
			}
			if tc.wantAudit != "" && (len(paths) != 1 || !strings.Contains(paths[0], tc.wantAudit)) {
				t.Errorf("role audit paths = %v, want one with %s", paths, tc.wantAudit)
			}
		})
	}
}

func TestOIDCOnlyVerifiedTokenClaimsAreTrusted(t *testing.T) {
	t.Run("a tampered ID token is refused", func(t *testing.T) {
		h := newOAuthHarness(t, modeLibrary)
		h.enable("oidc", "")
		h.policy(store.OAuthSSOPolicy{JITProvisioning: true})
		h.mapGroups(store.SSOGroupMapping{Group: "eng", RoleID: "role_operator"})
		h.idp.tamper = func(raw string) string {
			parts := strings.Split(raw, ".")
			payload, _ := base64.RawURLEncoding.DecodeString(parts[1])
			var claims map[string]any
			_ = json.Unmarshal(payload, &claims)
			claims["groups"] = []string{"eng"}
			forged, _ := json.Marshal(claims)
			parts[1] = base64.RawURLEncoding.EncodeToString(forged)
			return strings.Join(parts, ".")
		}
		if got := oauthResult(h.signIn("oidc", groupsID("t1", "forge@corp.test"))); got != "/login?oauth_error=exchange_failed" {
			t.Fatalf("location = %q", got)
		}
		if h.hasUser("forge@corp.test") {
			t.Error("a tampered token created an account")
		}
	})
	t.Run("groups from the userinfo endpoint are ignored", func(t *testing.T) {
		h := newOAuthHarness(t, modeLibrary)
		h.enable("oidc", "")
		h.policy(store.OAuthSSOPolicy{JITProvisioning: true})
		h.mapGroups(store.SSOGroupMapping{Group: "eng", RoleID: "role_operator"})
		id := fakeIdentity{Sub: "t2", Email: "inject@corp.test", Verified: true, UserinfoClaims: map[string]any{"groups": []string{"eng"}}}
		if got := oauthResult(h.signIn("oidc", id)); got != locComplete {
			t.Fatalf("location = %q", got)
		}
		if got := h.roleOf("inject@corp.test"); got != "role_viewer" {
			t.Errorf("role = %q, want viewer: only the verified ID token may carry groups", got)
		}
	})
}

func TestOIDCHalfSuccessIsRolledBack(t *testing.T) {
	h := newOAuthHarness(t, modeLibrary)
	h.enable("oidc", "")
	h.policy(store.OAuthSSOPolicy{JITProvisioning: true, DefaultRoleID: "role_gone"})
	id := groupsID("h1", "half@corp.test")
	if got := oauthResult(h.signIn("oidc", id)); got != locNoRole {
		t.Fatalf("location = %q, want %q", got, locNoRole)
	}
	if h.hasUser("half@corp.test") {
		t.Fatal("a failed role assignment left a platform account")
	}
	h.policy(store.OAuthSSOPolicy{JITProvisioning: true})
	if got := oauthResult(h.signIn("oidc", id)); got != locComplete {
		t.Fatalf("retry after the fix: %q", got)
	}
	if got := h.roleOf("half@corp.test"); got != "role_viewer" {
		t.Errorf("role = %q", got)
	}
}

func TestOIDCAllowedDomainsApplyToLinkedUsers(t *testing.T) {
	h := newOAuthHarness(t, modeLibrary)
	h.enable("oidc", "")
	h.policy(store.OAuthSSOPolicy{JITProvisioning: true})
	if got := oauthResult(h.signIn("oidc", groupsID("d1", "mover@corp.test"))); got != locComplete {
		t.Fatalf("first sign-in: %q", got)
	}
	h.policy(store.OAuthSSOPolicy{JITProvisioning: true, AllowedDomains: []string{"elsewhere.test"}})
	if got := oauthResult(h.signIn("oidc", groupsID("d1", "mover@corp.test"))); got != locDomain {
		t.Fatalf("linked user outside the domains: %q, want %q", got, locDomain)
	}
}

func TestOIDCTestLoginCreatesNothing(t *testing.T) {
	h := newOAuthHarness(t, modeLibrary)
	h.enable("oidc", "")
	h.policy(store.OAuthSSOPolicy{})
	h.mapGroups(store.SSOGroupMapping{Group: "eng", RoleID: "role_operator"})
	h.admin(http.MethodGet, "/api/v1/sso/oidc", "")

	start := h.adminNav("/api/v1/sso/oidc/oidc/test/start")
	if start.Code != http.StatusFound {
		t.Fatalf("test start: %d %s", start.Code, start.Body)
	}
	u, err := url.Parse(start.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	code := h.idp.grant(t, u.String(), groupsID("x1", "tester@corp.test", "eng"))
	cb := h.adminNav(h.callbackPath("oidc") + "?state=" + url.QueryEscape(u.Query().Get("state")) + "&code=" + code)
	if got := cb.Header().Get("Location"); got != "/settings/sso?sso_test=oidc" {
		t.Fatalf("callback location = %q", got)
	}
	if h.hasUser("tester@corp.test") {
		t.Fatal("a test login created an account")
	}
	if cookie := cb.Result().Cookies(); len(cookie) > 0 {
		for _, c := range cookie {
			if c.Name == sessionCookieName && c.Value != "" {
				t.Error("a test login issued a session")
			}
		}
	}
	res := h.admin(http.MethodGet, "/api/v1/sso/oidc/oidc/test/result", "")
	var got ssoTestResult
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Status != ssoTestStatusOK || got.Evaluation == nil {
		t.Fatalf("result = %s", res.Body)
	}
	ev := got.Evaluation
	if ev.RoleID != "role_operator" || ev.RoleSource != ssoRoleSourceMapping || !reflect.DeepEqual(ev.Groups, []string{"eng"}) {
		t.Errorf("evaluation = %+v", ev)
	}
	if ev.Allowed || ev.RefusalCode != "sso_not_provisioned" || ev.Issues[0].Fix.Action != "enable_jit" {
		t.Errorf("a jit-off policy must show the refusal and its typed fix: %+v", ev)
	}
	if again := h.admin(http.MethodGet, "/api/v1/sso/oidc/oidc/test/result", ""); !strings.Contains(again.Body.String(), ssoTestStatusNone) {
		t.Errorf("a result must be readable once: %s", again.Body)
	}
}

func (h *oauthHarness) adminNav(target string) *httptest.ResponseRecorder {
	h.t.Helper()
	if h.adminCookie == nil {
		h.adminCookie = loginTestSession(h.t, h.rt, h.db)
	}
	req := httptest.NewRequest(http.MethodGet, testBaseURL+target, nil)
	req.RemoteAddr = "127.0.0.1:50000"
	req.AddCookie(h.adminCookie)
	for _, c := range h.jar {
		req.AddCookie(&http.Cookie{Name: c.Name, Value: c.Value}) //nolint:gosec // test fixture
	}
	rec := httptest.NewRecorder()
	h.rt.Handler().ServeHTTP(rec, req)
	for _, c := range rec.Result().Cookies() {
		if c.MaxAge < 0 || c.Value == "" {
			delete(h.jar, c.Name)
		} else if c.Name != sessionCookieName {
			h.jar[c.Name] = c
		}
	}
	return rec
}

func TestOIDCEvaluateExplainsFixes(t *testing.T) {
	h := newOAuthHarness(t, modeLibrary)
	h.enable("oidc", "")
	h.policy(store.OAuthSSOPolicy{JITProvisioning: true, AllowedDomains: []string{"corp.test"}})
	h.mapGroups(store.SSOGroupMapping{Group: "eng", RoleID: "role_operator"})
	eval := func(body string) ssoEvaluation {
		rec := h.admin(http.MethodPost, "/api/v1/sso/oidc/oidc/evaluate", body)
		if rec.Code != http.StatusOK {
			t.Fatalf("evaluate: %d %s", rec.Code, rec.Body)
		}
		var ev ssoEvaluation
		if err := json.Unmarshal(rec.Body.Bytes(), &ev); err != nil {
			t.Fatal(err)
		}
		return ev
	}
	ok := eval(`{"email":"a@corp.test","email_verified":true,"groups":["ENG"]}`)
	if !ok.Allowed || ok.RoleID != "role_operator" || ok.AccountState != ssoAccountNew {
		t.Errorf("allowed case = %+v", ok)
	}
	bad := eval(`{"email":"a@evil.test","email_verified":false}`)
	codes := map[string]bool{}
	for _, i := range bad.Issues {
		codes[i.Code] = true
	}
	if bad.Allowed || !codes[ssoIssueEmailUnverified] || !codes[ssoIssueDomain] || !codes[ssoIssueNoGroups] {
		t.Errorf("refused case = %+v", bad)
	}
	if h.hasUser("a@corp.test") || h.hasUser("a@evil.test") {
		t.Error("evaluate created an account")
	}
}

func TestOIDCPolicyAdminValidationAndAuthz(t *testing.T) {
	h := newOAuthHarness(t, modeLibrary)
	h.enable("oidc", "")
	put := func(body string) *httptest.ResponseRecorder {
		return h.admin(http.MethodPut, "/api/v1/sso/oidc/oidc", body)
	}
	if rec := put(`{"jit_provisioning":true,"allowed_domains":["corp.test"],"default_role_id":"role_operator","sync_roles":true,"groups_claim":"roles"}`); rec.Code != http.StatusOK {
		t.Fatalf("valid put: %d %s", rec.Code, rec.Body)
	}
	if rec := put(`{"default_role_id":"role_nope"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("unknown role: %d", rec.Code)
	}
	if rec := put(`{"allowed_domains":["not a domain"]}`); rec.Code != http.StatusBadRequest {
		t.Errorf("bad domain: %d", rec.Code)
	}
	if rec := h.admin(http.MethodPut, "/api/v1/sso/oidc/nope", `{}`); rec.Code != http.StatusNotFound {
		t.Errorf("unknown provider: %d", rec.Code)
	}

	t.Run("a second connection cannot claim the same domain", func(t *testing.T) {
		if err := h.db.UpdateOAuthProviderSettings(context.Background(), store.OAuthProviderSettings{Provider: "google", Enabled: false, ClientID: "g"}); err != nil {
			t.Fatal(err)
		}
		rec := h.admin(http.MethodPut, "/api/v1/sso/oidc/google", `{"allowed_domains":["CORP.test"]}`)
		if rec.Code != http.StatusConflict {
			t.Fatalf("collision: %d %s, want 409", rec.Code, rec.Body)
		}
	})

	t.Run("secrets and tokens are never in a policy response", func(t *testing.T) {
		rec := h.admin(http.MethodGet, "/api/v1/sso/oidc/oidc", "")
		if strings.Contains(strings.ToLower(rec.Body.String()), "secret") {
			t.Errorf("policy body mentions a secret: %s", rec.Body)
		}
	})

	t.Run("only root may read or change a policy", func(t *testing.T) {
		const tok = "oidc-policy-low-token" //nolint:gosec // fake fixture, not a real credential
		if err := h.db.SaveAPIToken(context.Background(), store.APIToken{
			ID: "tok_oidc_low", Name: "low", TokenHash: hashToken(tok), Abilities: []string{AbilityRead, AbilityWrite}, CreatedAt: time.Now(),
		}); err != nil {
			t.Fatal(err)
		}
		for _, tc := range []struct{ method, path string }{
			{http.MethodGet, "/api/v1/sso/oidc"},
			{http.MethodPut, "/api/v1/sso/oidc/oidc"},
			{http.MethodPost, "/api/v1/sso/oidc/oidc/evaluate"},
			{http.MethodGet, "/api/v1/sso/oidc/oidc/test/start"},
		} {
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{}`))
			req.Header.Set("Authorization", "Bearer "+tok)
			rec := httptest.NewRecorder()
			h.rt.Handler().ServeHTTP(rec, req)
			if rec.Code != http.StatusForbidden {
				t.Errorf("%s %s as non-root: %d, want 403", tc.method, tc.path, rec.Code)
			}
		}
	})
}

func TestSSODomainRouting(t *testing.T) {
	h := newOAuthHarness(t, modeLibrary)
	h.enable("oidc", "")
	h.policy(store.OAuthSSOPolicy{AllowedDomains: []string{"corp.test"}})
	route := func(domain string) string {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/login-options?sso_domain="+url.QueryEscape(domain), nil)
		req.RemoteAddr = "127.0.0.1:50000"
		rec := httptest.NewRecorder()
		h.rt.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("login-options: %d", rec.Code)
		}
		var body struct {
			SSORoute *ssoRoute `json:"sso_route"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.SSORoute == nil {
			return "null"
		}
		return body.SSORoute.Kind + ":" + body.SSORoute.StartURL
	}
	if got := route("Corp.Test"); got != "oauth:/api/v1/auth/oauth/oidc/start" {
		t.Errorf("matching domain: %q", got)
	}
	for _, d := range []string{"unknown.test", "nodots", "a b.test", strings.Repeat("a", 300) + ".test", ""} {
		if got := route(d); got != "null" {
			t.Errorf("domain %q: %q, want null for every non-match, identically", d, got)
		}
	}
	h.setPolicy("oidc", store.OAuthSSOPolicy{GroupsClaim: "groups", DefaultRoleID: defaultOIDCRoleID})
	if got := route("corp.test"); got != "null" {
		t.Errorf("a domain no connection claims: %q", got)
	}

	t.Run("two connections claiming one domain route nowhere", func(t *testing.T) {
		for _, p := range []string{"oidc", "google"} {
			if _, err := h.db.Exec(`UPDATE oauth_provider_settings SET enabled = 1, allowed_domains = '["dup.test"]' WHERE provider = ?`, p); err != nil {
				t.Fatal(err)
			}
		}
		if got := route("dup.test"); got != "null" {
			t.Errorf("ambiguous domain: %q, want null", got)
		}
	})
}

func TestSSODomainRoutingIsRateLimited(t *testing.T) {
	t.Setenv(envSSORoutePerMinute, "3")
	h := newOAuthHarness(t, modeLibrary)
	h.enable("oidc", "")
	h.policy(store.OAuthSSOPolicy{AllowedDomains: []string{"corp.test"}})
	var routed int
	for range 6 {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/login-options?sso_domain=corp.test", nil)
		req.RemoteAddr = "127.0.0.1:50000"
		rec := httptest.NewRecorder()
		h.rt.Handler().ServeHTTP(rec, req)
		if strings.Contains(rec.Body.String(), `"kind":"oauth"`) {
			routed++
		}
	}
	if routed != 3 {
		t.Errorf("routed %d of 6 requests, want 3", routed)
	}
}

func TestSSODomainCollisionAcrossSAMLAndOIDC(t *testing.T) {
	h := newSAMLHarness(t, map[string]any{"allowed_domains": []string{"shared.test"}})
	rec := h.admin2(http.MethodPut, "/api/v1/sso/oidc/oidc", map[string]any{"allowed_domains": []string{"shared.test"}})
	if rec.Code != http.StatusConflict {
		t.Fatalf("oidc claiming a SAML domain: %d %s, want 409", rec.Code, rec.Body)
	}
	if rec := h.admin2(http.MethodPut, "/api/v1/sso/oidc/oidc", map[string]any{"allowed_domains": []string{"free.test"}}); rec.Code != http.StatusOK {
		t.Fatalf("free domain: %d %s", rec.Code, rec.Body)
	}
	rec = h.admin2(http.MethodPost, "/api/v1/sso/saml/connections", map[string]any{"name": "Second", "allowed_domains": []string{"free.test"}})
	if rec.Code != http.StatusConflict {
		t.Fatalf("saml claiming an OIDC domain: %d %s, want 409", rec.Code, rec.Body)
	}
}

func TestOIDCOrphanedIdentityCannotBypassJITOff(t *testing.T) {
	h := newOAuthHarness(t, modeLibrary)
	h.enable("oidc", "")
	h.policy(store.OAuthSSOPolicy{JITProvisioning: true, DefaultRoleID: "role_gone"})
	id := groupsID("o1", "orphan@corp.test")
	if got := oauthResult(h.signIn("oidc", id)); got != locNoRole {
		t.Fatalf("first sign-in: %q", got)
	}
	h.policy(store.OAuthSSOPolicy{})
	if got := oauthResult(h.signIn("oidc", id)); got != locNotProvisioned {
		t.Fatalf("sign-in with an orphaned identity and jit off: %q, want %q", got, locNotProvisioned)
	}
	if h.hasUser("orphan@corp.test") {
		t.Error("jit off still created a platform account")
	}
}

func TestOIDCSkipsAuditForAnonymousNoise(t *testing.T) {
	h := newOAuthHarness(t, modeLibrary)
	h.enable("oidc", "")
	rec := h.get(h.callbackPath("oidc")+"?state=nope&code=nope", false)
	if got := oauthResult(rec); got != "/login?oauth_error=invalid_state" {
		t.Fatalf("location = %q", got)
	}
	if n := len(h.auditPaths(auditActionOIDCRejected)); n != 0 {
		t.Errorf("an anonymous bad-state request wrote %d audit rows", n)
	}
}

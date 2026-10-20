package api

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	liveOIDCIssuer = "http://localhost:18080/realms/lab"
	liveOIDCClient = "lab-oidc"
	liveOIDCSecret = "lab-secret"
)

// liveOIDCBrowse signs in at the Keycloak login form and returns the redirect back to the app.
func liveOIDCBrowse(t *testing.T, authorizeURL, user, pass string) string {
	t.Helper()
	c := newLiveClient()
	c.CheckRedirect = func(req *http.Request, _ []*http.Request) error {
		if req.URL.Host == "app.test" {
			return http.ErrUseLastResponse
		}
		return nil
	}
	body, cur := liveGet(t, c, authorizeURL)
	f, ok := parseLiveForm(cur, body)
	if !ok {
		t.Fatalf("no login form on %s: %.300s", cur, body)
	}
	f.fields.Set("username", user)
	f.fields.Set("password", pass)
	resp, err := c.PostForm(f.action, f.fields)
	if err != nil {
		t.Fatalf("post login form: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	loc := resp.Header.Get("Location")
	if loc == "" {
		t.Fatalf("login did not redirect back (status %d): %.300s", resp.StatusCode, b)
	}
	return loc
}

func (h *oauthHarness) enableLiveOIDC(p store.OAuthSSOPolicy) {
	h.t.Helper()
	ctx := context.Background()
	if err := h.secrets.SetValue(ctx, store.OAuthProviderSecretsKey("oidc"), store.OAuthProviderSecretEnvKey, liveOIDCSecret); err != nil {
		h.t.Fatal(err)
	}
	s := store.OAuthProviderSettings{Provider: "oidc", Enabled: true, ClientID: liveOIDCClient, IssuerURL: liveOIDCIssuer}
	if err := h.db.UpdateOAuthProviderSettings(ctx, s); err != nil {
		h.t.Fatal(err)
	}
	h.policy(p)
}

// liveSignIn runs start, the real Keycloak login and the callback, returning the callback response.
func (h *oauthHarness) liveOIDCSignIn(user, pass string) (authorize *url.URL, loc string) {
	h.t.Helper()
	u, _ := h.start("oidc")
	back := liveOIDCBrowse(h.t, u.String(), user, pass)
	cb, err := url.Parse(back)
	if err != nil {
		h.t.Fatal(err)
	}
	rec := h.get(cb.RequestURI(), true)
	return u, oauthResult(rec)
}

func TestLiveOIDCAgainstKeycloak(t *testing.T) {
	skipUnlessLiveIDP(t)
	mapping := store.SSOGroupMapping{Group: "Engineering", RoleID: "role_operator", Priority: 5}

	t.Run("code flow with PKCE signs in, creates the account and maps the group", func(t *testing.T) {
		h := newOAuthHarness(t, modeLibrary)
		h.enableLiveOIDC(store.OAuthSSOPolicy{JITProvisioning: true, AllowedDomains: []string{"lab.example"}})
		h.mapGroups(mapping)
		u, loc := h.liveOIDCSignIn("ada", "ada-pass")
		if got := u.Query().Get("code_challenge_method"); got != "S256" || u.Query().Get("code_challenge") == "" {
			t.Fatalf("authorize URL has no S256 PKCE challenge: %s", u)
		}
		if loc != locComplete {
			t.Fatalf("callback location = %q, want %q", loc, locComplete)
		}
		if got := h.roleOf("ada@lab.example"); got != "role_operator" {
			t.Fatalf("role = %q, want role_operator", got)
		}
		first, _ := h.db.GetUserByEmail(context.Background(), "ada@lab.example")
		if _, loc := h.liveOIDCSignIn("ada", "ada-pass"); loc != locComplete {
			t.Fatalf("second sign-in location = %q", loc)
		}
		again, _ := h.db.GetUserByEmail(context.Background(), "ada@lab.example")
		if again.ID != first.ID {
			t.Fatalf("second sign-in made account %s, want %s", again.ID, first.ID)
		}
	})

	t.Run("a domain outside the allowed list is refused and leaves no account", func(t *testing.T) {
		h := newOAuthHarness(t, modeLibrary)
		h.enableLiveOIDC(store.OAuthSSOPolicy{JITProvisioning: true, AllowedDomains: []string{"lab.example"}})
		h.mapGroups(mapping)
		if _, loc := h.liveOIDCSignIn("mallory", "mallory-pass"); loc != locDomain {
			t.Fatalf("location = %q, want %q", loc, locDomain)
		}
		if h.hasUser("mallory@other.example") {
			t.Fatal("refused sign-in left an account behind")
		}
	})

	t.Run("jit off refuses a person with no account", func(t *testing.T) {
		h := newOAuthHarness(t, modeLibrary)
		h.enableLiveOIDC(store.OAuthSSOPolicy{AllowedDomains: []string{"lab.example"}})
		if _, loc := h.liveOIDCSignIn("ada", "ada-pass"); loc != locNotProvisioned {
			t.Fatalf("location = %q, want %q", loc, locNotProvisioned)
		}
	})

	t.Run("SSO never grants root", func(t *testing.T) {
		h := newOAuthHarness(t, modeLibrary)
		h.enableLiveOIDC(store.OAuthSSOPolicy{JITProvisioning: true, AllowedDomains: []string{"lab.example"}})
		body := `{"mappings":[{"group":"Admins","role_id":"role_admin"}]}`
		if rec := h.admin(http.MethodPut, "/api/v1/sso/group-mappings", body); rec.Code != http.StatusBadRequest {
			t.Fatalf("saving a root mapping: status %d, want 400", rec.Code)
		}
		if _, err := h.db.Exec(`INSERT INTO sso_group_role_mappings (group_name, role_id, priority, created_at) VALUES ('Admins', 'role_admin', 0, ?)`, time.Now().Format(time.RFC3339Nano)); err != nil {
			t.Fatal(err)
		}
		if _, loc := h.liveOIDCSignIn("grace", "grace-pass"); loc != locNoRole {
			t.Fatalf("location = %q, want %q", loc, locNoRole)
		}
		if h.hasUser("grace@lab.example") && strings.Contains(h.roleOf("grace@lab.example"), "admin") {
			t.Fatal("SSO granted a root role")
		}
	})
}

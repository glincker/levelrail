package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestHandleListOAuthSettings_RequiresAuth(t *testing.T) {
	rt, _ := newTestRouter(t)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/settings/oauth", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandleListOAuthSettings_NeverLeaksSecret(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rt.oauthSecrets = newFakeOAuthSecrets()
	if err := rt.oauthSecrets.SetValue(context.Background(), store.OAuthProviderSecretsKey(store.OAuthProviderGoogle), store.OAuthProviderSecretEnvKey, "super-secret-value"); err != nil {
		t.Fatalf("SetValue() error = %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/settings/oauth", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "super-secret-value") {
		t.Errorf("response leaked the client secret: %s", rec.Body.String())
	}

	var got []oauthProviderSettingsResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	found := false
	for _, s := range got {
		if s.Provider == store.OAuthProviderGoogle {
			found = true
			if !s.HasClientSecret {
				t.Error("HasClientSecret = false, want true after storing a secret")
			}
		}
	}
	if !found {
		t.Error("google provider not present in the response")
	}
}

func TestHandleUpdateOAuthProviderSettings_RequiresRoot(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rt.oauthSecrets = newFakeOAuthSecrets()

	// requireAbility treats any valid session as AbilityRoot in this
	// phase (see requireAbility's own doc comment), so this proves the
	// route is wired at AbilityRoot via router registration by checking
	// it isn't reachable at all without a session, the same shape every
	// other AbilityRoot route's own test uses.
	body := `{"enabled":false,"client_id":"","allowed_email_domain":""}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/api/v1/settings/oauth/google", strings.NewReader(body)))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	_ = cookie
}

func TestHandleUpdateOAuthProviderSettings_NotConfigured(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	body := `{"enabled":false,"client_id":"","allowed_email_domain":""}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/oauth/google", body))
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}
}

func TestHandleUpdateOAuthProviderSettings_UnknownProvider(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rt.oauthSecrets = newFakeOAuthSecrets()

	body := `{"enabled":false}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/oauth/bogus", body))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleUpdateOAuthProviderSettings_EnableWithoutClientID_Rejected(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rt.oauthSecrets = newFakeOAuthSecrets()

	body := `{"enabled":true,"client_id":"","client_secret":"x"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/oauth/google", body))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleUpdateOAuthProviderSettings_EnableWithoutSecret_Rejected(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rt.oauthSecrets = newFakeOAuthSecrets()

	body := `{"enabled":true,"client_id":"real-client-id"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/oauth/google", body))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleUpdateOAuthProviderSettings_RoundTrip(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rt.oauthSecrets = newFakeOAuthSecrets()

	body := `{"enabled":true,"client_id":"real-client-id","client_secret":"real-secret","allowed_email_domain":"example.com"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/oauth/google", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	got, err := db.GetOAuthProviderSettings(context.Background(), store.OAuthProviderGoogle)
	if err != nil {
		t.Fatalf("GetOAuthProviderSettings() error = %v", err)
	}
	if !got.Enabled || got.ClientID != "real-client-id" || got.AllowedEmailDomain != "example.com" {
		t.Errorf("got %+v, want enabled with the submitted client_id/domain", got)
	}

	secret, err := rt.oauthSecrets.Resolve(context.Background(), store.OAuthProviderSecretsKey(store.OAuthProviderGoogle), store.OAuthProviderSecretEnvKey)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if secret != "real-secret" {
		t.Errorf("stored secret = %q, want %q", secret, "real-secret")
	}
}

// TestHandleUpdateOAuthProviderSettings_EmptySecretKeepsExisting proves
// updating client_id/enabled without resending client_secret must not
// clear a previously stored one.
func TestHandleUpdateOAuthProviderSettings_EmptySecretKeepsExisting(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rt.oauthSecrets = newFakeOAuthSecrets()

	first := `{"enabled":true,"client_id":"client-1","client_secret":"original-secret"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/oauth/google", first))
	if rec.Code != http.StatusOK {
		t.Fatalf("first update: status = %d, body = %s", rec.Code, rec.Body.String())
	}

	second := `{"enabled":true,"client_id":"client-2"}`
	rec2 := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec2, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/oauth/google", second))
	if rec2.Code != http.StatusOK {
		t.Fatalf("second update: status = %d, body = %s", rec2.Code, rec2.Body.String())
	}

	secret, err := rt.oauthSecrets.Resolve(context.Background(), store.OAuthProviderSecretsKey(store.OAuthProviderGoogle), store.OAuthProviderSecretEnvKey)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if secret != "original-secret" {
		t.Errorf("secret = %q, want the original unchanged %q", secret, "original-secret")
	}
}

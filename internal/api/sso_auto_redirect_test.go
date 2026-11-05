package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestLoginOptions_SSOAutoRedirect(t *testing.T) {
	cases := []struct {
		name      string
		setting   string
		providers []string
		query     string
		want      bool
		wantProv  string
	}{
		{"setting off", "false", []string{store.OAuthProviderGoogle}, "", false, ""},
		{"on with one provider", "true", []string{store.OAuthProviderGoogle}, "", true, store.OAuthProviderGoogle},
		{"escape hatch", "true", []string{store.OAuthProviderGoogle}, "?password=1", false, ""},
		{"on with no provider", "true", nil, "", false, ""},
		{"on with two providers", "true", []string{store.OAuthProviderGoogle, store.OAuthProviderMicrosoft}, "", false, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt, db := newTestRouter(t)
			cookie := loginTestSession(t, rt, db)
			if rec := rsDo(t, rt, cookie, http.MethodPut, rsBase+"/"+runtimesettings.KeyAuthSSOAutoRedirect, `{"value":`+tc.setting+`}`); rec.Code != http.StatusOK {
				t.Fatalf("set = %d %s", rec.Code, rec.Body.String())
			}
			for _, p := range tc.providers {
				if err := rt.oauthSettings.UpdateOAuthProviderSettings(t.Context(), store.OAuthProviderSettings{Provider: p, Enabled: true, ClientID: "id-" + p}); err != nil {
					t.Fatal(err)
				}
			}
			req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/login-options"+tc.query, nil)
			req.RemoteAddr = "127.0.0.1:50000"
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("login-options = %d", rec.Code)
			}
			var body struct {
				Auto     bool   `json:"sso_auto_redirect"`
				Provider string `json:"sso_provider"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Auto != tc.want || body.Provider != tc.wantProv {
				t.Fatalf("got %+v, want auto=%v provider=%q", body, tc.want, tc.wantProv)
			}
		})
	}
}

package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func pkgRegistryRouter(t *testing.T, secrets *fakeRegistryCredentialSecretsSetter) (*Router, *http.Cookie) {
	t.Helper()
	db := openTestDB(t)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	rt := NewRouter(logger, testBrand(), db, WithRegistryCredentialSecrets(secrets))
	return rt, loginTestSession(t, rt, db)
}

func TestPackageRegistries_CreateNeverReturnsSecret(t *testing.T) {
	secrets := &fakeRegistryCredentialSecretsSetter{resolveValue: "tok"}
	rt, cookie := pkgRegistryRouter(t, secrets)
	do := func(method, path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, body))
		return rec
	}

	rec := do(http.MethodPost, "/api/v1/package-registries", `{"name":"corp-npm","ecosystem":"npm","url":"https://npm.example.com","secret":"super-secret-token"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "super-secret-token") {
		t.Error("create response echoed the secret")
	}
	var created packageRegistryResource
	_ = json.NewDecoder(rec.Body).Decode(&created)
	if created.SecretID != "npmrc" || created.ScopeKind != "instance" {
		t.Errorf("created = %+v", created)
	}
	if len(secrets.calls) != 1 || secrets.calls[0].plaintext != "super-secret-token" {
		t.Errorf("secret not stored: %+v", secrets.calls)
	}

	list := do(http.MethodGet, "/api/v1/package-registries", "")
	if list.Code != http.StatusOK || strings.Contains(list.Body.String(), "super-secret-token") || !strings.Contains(list.Body.String(), "corp-npm") {
		t.Errorf("list = %d %s", list.Code, list.Body.String())
	}
	if rec := do(http.MethodDelete, "/api/v1/package-registries/"+created.ID, ""); rec.Code != http.StatusNoContent {
		t.Errorf("delete = %d", rec.Code)
	}
}

func TestPackageRegistries_Validation(t *testing.T) {
	rt, cookie := pkgRegistryRouter(t, &fakeRegistryCredentialSecretsSetter{})
	cases := map[string]string{ //nolint:gosec // request bodies with fake values, not credentials
		"bad ecosystem":       `{"name":"x","ecosystem":"conan","url":"https://a.example.com","secret":"s"}`,
		"embedded creds":      `{"name":"x","ecosystem":"npm","url":"https://u:p@a.example.com","secret":"s"}`,
		"missing secret":      `{"name":"x","ecosystem":"npm","url":"https://a.example.com"}`,
		"app scope needs ref": `{"name":"x","ecosystem":"npm","url":"https://a.example.com","secret":"s","scope_kind":"app"}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/package-registries", body))
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, body = %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestProbeRegistryCredential_StoresNothingAndHidesPassword(t *testing.T) {
	tester := &fakeRegistryAuthTester{}
	secrets := &fakeRegistryCredentialSecretsSetter{}
	db := openTestDB(t)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	rt := NewRouter(logger, testBrand(), db, WithRegistryCredentialSecrets(secrets), WithRegistryAuthTester(tester))
	cookie := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/registry-credentials/test", `{"registry_host":"ghcr.io","username":"u","password":"pw-123"}`))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"ok":true`) {
		t.Fatalf("probe = %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "pw-123") || len(secrets.calls) != 0 {
		t.Error("probe leaked or stored the password")
	}
	if len(tester.calls) != 1 || tester.calls[0].host != "ghcr.io" {
		t.Errorf("tester calls = %+v", tester.calls)
	}
}

func TestRegistryVendorsAndUsage(t *testing.T) {
	rt, cookie := pkgRegistryRouter(t, &fakeRegistryCredentialSecretsSetter{})
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/registry-vendors", ""))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"ghcr.io"`) || !strings.Contains(rec.Body.String(), `"ecr"`) {
		t.Errorf("vendors = %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/registry-credentials/regcred_x/usage", ""))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"services":[]`) {
		t.Errorf("usage = %d %s", rec.Code, rec.Body.String())
	}
}

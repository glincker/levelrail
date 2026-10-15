package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/provision"
	"github.com/GLINCKER/levelrail/internal/store"
)

type refusingProvisioner struct{ fakeNodeProvisioner }

func (*refusingProvisioner) ListRegions(context.Context) ([]provision.Region, error) {
	return nil, &provision.ProviderError{Status: http.StatusForbidden, Body: "no"}
}

func connectTestRouter(t *testing.T, p NodeProvisioner) (*Router, *http.Cookie, *fakeNodeProviderSecrets) {
	t.Helper()
	secrets := newFakeNodeProviderSecrets()
	db := openTestDB(t)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	rt := NewRouter(logger, testBrand(), db, WithNodeProviderSecrets(secrets),
		WithNodeProvisionerFactory(func(string, string) (NodeProvisioner, error) { return p, nil }))
	return rt, loginTestSession(t, rt, db), secrets
}

func TestNodeProviderCredentialValue_Methods(t *testing.T) {
	cases := []struct {
		name    string
		req     setNodeProviderCredentialRequest
		wantMsg string
	}{
		{"token ok", setNodeProviderCredentialRequest{Provider: "hetzner", Method: methodAPIToken, Token: "t"}, ""},
		{"token missing", setNodeProviderCredentialRequest{Provider: "hetzner", Method: methodAPIToken}, "token is required"},
		{"access key needs secret", setNodeProviderCredentialRequest{Provider: "aws", Method: methodAccessKey, Token: "AKIA"}, "secret_access_key"},
		{"assume role needs arn", setNodeProviderCredentialRequest{Provider: "aws", Method: methodAssumeRole, UseAmbientCredentials: true}, "role_arn"},
		{"assume role ambient ok", setNodeProviderCredentialRequest{Provider: "aws", Method: methodAssumeRole, RoleARN: "arn:aws:iam::1:role/x", UseAmbientCredentials: true}, ""},
		{"ambient ok", setNodeProviderCredentialRequest{Provider: "aws", Method: methodAmbient}, ""},
		{"azure secret ok", setNodeProviderCredentialRequest{Provider: "azure", Method: methodServiceAccountJSON, Token: `{"client_secret":"s"}`}, ""},
		{"azure federation rejects secret", setNodeProviderCredentialRequest{Provider: "azure", Method: methodWorkloadIdentityFederation, Token: `{"client_secret":"s","federated_token_file":"/f"}`}, "federated"},
		{"gcp not json", setNodeProviderCredentialRequest{Provider: "gcp", Method: methodServiceAccountJSON, Token: "nope"}, "JSON object"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, msg := nodeProviderCredentialValue(tc.req)
			if tc.wantMsg == "" && msg != "" || tc.wantMsg != "" && !strings.Contains(msg, tc.wantMsg) {
				t.Fatalf("msg = %q, want contains %q", msg, tc.wantMsg)
			}
		})
	}
}

func TestConnectNodeProvider_VerifyFailureStoresNothing(t *testing.T) {
	rt, cookie, secrets := connectTestRouter(t, &refusingProvisioner{})
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/node-providers", `{"provider":"hetzner","method":"api_token","token":"sekret","verify":true}`))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "sekret") {
		t.Error("response echoed the credential")
	}
	if exists, _ := secrets.Exists(context.Background(), store.NodeProviderSecretsKey("hetzner"), store.NodeProviderTokenEnvKey); exists {
		t.Error("credential stored despite failed verification")
	}
}

func TestConnectTestDisconnectNodeProvider(t *testing.T) {
	rt, cookie, secrets := connectTestRouter(t, &fakeNodeProvisioner{})
	do := func(method, path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, body))
		return rec
	}
	if rec := do(http.MethodPost, "/api/v1/node-providers", `{"provider":"hetzner","method":"api_token","token":"sekret","verify":true}`); rec.Code != http.StatusOK {
		t.Fatalf("connect status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if rec := do(http.MethodPost, "/api/v1/node-providers/hetzner/test", ""); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"ok":true`) {
		t.Fatalf("test = %d %s", rec.Code, rec.Body.String())
	}
	rec := do(http.MethodGet, "/api/v1/node-providers", "")
	var list []nodeProviderResource
	if err := json.NewDecoder(rec.Body).Decode(&list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, p := range list {
		if p.Provider == "hetzner" && (p.Method != methodAPIToken || p.LastTestOK == nil || !*p.LastTestOK || p.Status != nodeProviderStatusConnected) {
			t.Errorf("hetzner = %+v", p)
		}
	}
	if rec := do(http.MethodDelete, "/api/v1/node-providers/hetzner", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("disconnect status = %d", rec.Code)
	}
	if exists, _ := secrets.Exists(context.Background(), store.NodeProviderSecretsKey("hetzner"), store.NodeProviderTokenEnvKey); exists {
		t.Error("credential still stored after disconnect")
	}
}

func TestConnectCustomProvider(t *testing.T) {
	rt, cookie, _ := connectTestRouter(t, &fakeNodeProvisioner{})
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/node-providers", `{"provider":"custom","label":"Rack 4","location":"Frankfurt"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var res nodeProviderResource
	_ = json.NewDecoder(rec.Body).Decode(&res)
	if res.Status != nodeProviderStatusConnected || res.Label != "Rack 4" || res.Method != methodManual {
		t.Errorf("res = %+v", res)
	}
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/node-providers", `{"provider":"custom"}`))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("missing label status = %d", rec.Code)
	}
}

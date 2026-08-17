package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeRegistryCredentialSecretsSetter is a hand-written fake for
// RegistryCredentialSecretsSetter, the same pattern
// fakeBackupSecretsSetter already establishes for the backup-target
// equivalent.
type fakeRegistryCredentialSecretsSetter struct {
	err   error
	calls []struct{ serviceName, envKey, plaintext string }
}

func (f *fakeRegistryCredentialSecretsSetter) SetValue(_ context.Context, serviceName, envKey, plaintext string) error {
	f.calls = append(f.calls, struct{ serviceName, envKey, plaintext string }{serviceName, envKey, plaintext})
	return f.err
}

func newTestRouterWithRegistryCredentialSecrets(t *testing.T, secrets RegistryCredentialSecretsSetter) (*Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	return NewRouter(logger, testBrand(), db, WithRegistryCredentialSecrets(secrets)), db
}

func TestValidateCreateRegistryCredentialRequest(t *testing.T) {
	valid := createRegistryCredentialRequest{Name: "ghcr-bot", RegistryHost: "ghcr.io", Username: "bot", Password: "tok"}
	tests := []struct {
		name    string
		req     createRegistryCredentialRequest
		wantErr bool
	}{
		{name: "valid", req: valid, wantErr: false},
		{name: "missing name", req: withRegistryCredentialField(valid, func(r *createRegistryCredentialRequest) { r.Name = "" }), wantErr: true},
		{name: "missing registry_host", req: withRegistryCredentialField(valid, func(r *createRegistryCredentialRequest) { r.RegistryHost = "" }), wantErr: true},
		{name: "missing username", req: withRegistryCredentialField(valid, func(r *createRegistryCredentialRequest) { r.Username = "" }), wantErr: true},
		{name: "missing password", req: withRegistryCredentialField(valid, func(r *createRegistryCredentialRequest) { r.Password = "" }), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCreateRegistryCredentialRequest(tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateCreateRegistryCredentialRequest(%+v) error = %v, wantErr %v", tt.req, err, tt.wantErr)
			}
		})
	}
}

func withRegistryCredentialField(base createRegistryCredentialRequest, mutate func(*createRegistryCredentialRequest)) createRegistryCredentialRequest {
	mutate(&base)
	return base
}

func TestRegistryCredentialRoutes_RequireAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	routes := []struct {
		method string
		target string
	}{
		{http.MethodGet, "/api/v1/registry-credentials"},
		{http.MethodPost, "/api/v1/registry-credentials"},
		{http.MethodDelete, "/api/v1/registry-credentials/regcred_x"},
	}
	for _, r := range routes {
		t.Run(r.method+" "+r.target, func(t *testing.T) {
			req := httptest.NewRequest(r.method, r.target, nil)
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
			}
		})
	}
}

func TestHandleCreateRegistryCredential_NoSetterConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithRegistryCredentialSecrets
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	body := `{"name":"ghcr-bot","registry_host":"ghcr.io","username":"bot","password":"tok"}`
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/registry-credentials", body))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}
}

func TestHandleCreateRegistryCredential_Success(t *testing.T) {
	setter := &fakeRegistryCredentialSecretsSetter{}
	rt, db := newTestRouterWithRegistryCredentialSecrets(t, setter)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	body := `{"name":"ghcr-bot","registry_host":"ghcr.io","username":"bot","password":"gh-token-real"}`
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/registry-credentials", body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var got registryCredentialResource
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.ID == "" || !strings.HasPrefix(got.ID, "regcred_") {
		t.Errorf("ID = %q, want a regcred_-prefixed id", got.ID)
	}
	if got.Name != "ghcr-bot" || got.RegistryHost != "ghcr.io" || got.Username != "bot" {
		t.Errorf("response = %+v, want name/registry_host/username to match the request", got)
	}
	if strings.Contains(rec.Body.String(), "gh-token-real") {
		t.Errorf("response body = %s, password must never be echoed back", rec.Body.String())
	}

	if len(setter.calls) != 1 {
		t.Fatalf("SetValue calls = %d, want 1", len(setter.calls))
	}
	wantKey := store.RegistryCredentialSecretsKey(got.ID)
	if setter.calls[0].serviceName != wantKey || setter.calls[0].envKey != "password" || setter.calls[0].plaintext != "gh-token-real" {
		t.Errorf("SetValue call = %+v, want serviceName=%q envKey=password plaintext=gh-token-real", setter.calls[0], wantKey)
	}

	stored, err := db.GetRegistryCredential(context.Background(), got.ID)
	if err != nil {
		t.Fatalf("GetRegistryCredential() error = %v", err)
	}
	if stored.Name != "ghcr-bot" || stored.Username != "bot" {
		t.Errorf("stored credential = %+v, want name/username to match the request", stored)
	}
}

func TestHandleCreateRegistryCredential_InvalidRequest(t *testing.T) {
	rt, db := newTestRouterWithRegistryCredentialSecrets(t, &fakeRegistryCredentialSecretsSetter{})
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/registry-credentials", `{"name":""}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleCreateRegistryCredential_SecretSetFails(t *testing.T) {
	setter := &fakeRegistryCredentialSecretsSetter{err: errors.New("master key not configured")}
	rt, db := newTestRouterWithRegistryCredentialSecrets(t, setter)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	body := `{"name":"ghcr-bot","registry_host":"ghcr.io","username":"bot","password":"tok"}`
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/registry-credentials", body))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}

	got, err := db.ListRegistryCredentials(context.Background())
	if err != nil {
		t.Fatalf("ListRegistryCredentials() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("ListRegistryCredentials() = %+v, want no row saved when the secret write fails first", got)
	}
}

func TestHandleCreateRegistryCredential_DuplicateName_Conflict(t *testing.T) {
	setter := &fakeRegistryCredentialSecretsSetter{}
	rt, db := newTestRouterWithRegistryCredentialSecrets(t, setter)
	cookie := loginTestSession(t, rt, db)

	body := `{"name":"ghcr-bot","registry_host":"ghcr.io","username":"bot","password":"tok"}`
	firstRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(firstRec, authedRequest(t, cookie, http.MethodPost, "/api/v1/registry-credentials", body))
	if firstRec.Code != http.StatusCreated {
		t.Fatalf("first create status = %d, want %d", firstRec.Code, http.StatusCreated)
	}

	secondRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(secondRec, authedRequest(t, cookie, http.MethodPost, "/api/v1/registry-credentials", body))
	if secondRec.Code != http.StatusConflict {
		t.Fatalf("second create status = %d, want %d", secondRec.Code, http.StatusConflict)
	}
}

func TestHandleListAndDeleteRegistryCredential(t *testing.T) {
	setter := &fakeRegistryCredentialSecretsSetter{}
	rt, db := newTestRouterWithRegistryCredentialSecrets(t, setter)
	cookie := loginTestSession(t, rt, db)

	createRec := httptest.NewRecorder()
	body := `{"name":"ghcr-bot","registry_host":"ghcr.io","username":"bot","password":"tok"}`
	rt.Handler().ServeHTTP(createRec, authedRequest(t, cookie, http.MethodPost, "/api/v1/registry-credentials", body))
	var created registryCredentialResource
	if err := json.NewDecoder(createRec.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	listRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(listRec, authedRequest(t, cookie, http.MethodGet, "/api/v1/registry-credentials", ""))
	var list []registryCredentialResource
	if err := json.NewDecoder(listRec.Body).Decode(&list); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(list) != 1 || list[0].ID != created.ID {
		t.Fatalf("list = %+v, want exactly the created credential", list)
	}

	deleteRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(deleteRec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/registry-credentials/"+created.ID, ""))
	if deleteRec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want %d", deleteRec.Code, http.StatusNoContent)
	}

	missingDeleteRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(missingDeleteRec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/registry-credentials/regcred_missing", ""))
	if missingDeleteRec.Code != http.StatusNotFound {
		t.Fatalf("delete missing status = %d, want %d", missingDeleteRec.Code, http.StatusNotFound)
	}
}

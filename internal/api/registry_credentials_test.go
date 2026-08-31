package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	cerrdefs "github.com/containerd/errdefs"

	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeRegistryCredentialSecretsSetter is a hand-written fake for
// RegistryCredentialSecretsSetter, the same pattern
// fakeBackupSecretsSetter already establishes for the backup-target
// equivalent.
type fakeRegistryCredentialSecretsSetter struct {
	err          error
	calls        []struct{ serviceName, envKey, plaintext string }
	resolveValue string
	resolveErr   error
}

func (f *fakeRegistryCredentialSecretsSetter) SetValue(_ context.Context, serviceName, envKey, plaintext string) error {
	f.calls = append(f.calls, struct{ serviceName, envKey, plaintext string }{serviceName, envKey, plaintext})
	return f.err
}

func (f *fakeRegistryCredentialSecretsSetter) Resolve(_ context.Context, _, _ string) (string, error) {
	return f.resolveValue, f.resolveErr
}

// fakeRegistryAuthTester is a hand-written fake for RegistryAuthTester.
type fakeRegistryAuthTester struct {
	err   error
	calls []struct{ host, username, password string }
}

func (f *fakeRegistryAuthTester) TestRegistryAuth(_ context.Context, host, username, password string) error {
	f.calls = append(f.calls, struct{ host, username, password string }{host, username, password})
	return f.err
}

func newTestRouterWithRegistryCredentialSecrets(t *testing.T, secrets RegistryCredentialSecretsSetter) (*Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	return NewRouter(logger, testBrand(), db, WithRegistryCredentialSecrets(secrets)), db
}

func newTestRouterWithRegistryCredentialTest(t *testing.T, secrets RegistryCredentialSecretsSetter, tester RegistryAuthTester) (*Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	return NewRouter(logger, testBrand(), db, WithRegistryCredentialSecrets(secrets), WithRegistryAuthTester(tester)), db
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

func TestValidateUpdateRegistryCredentialRequest(t *testing.T) {
	valid := updateRegistryCredentialRequest{Name: "ghcr-bot", RegistryHost: "ghcr.io", Username: "bot"}
	tests := []struct {
		name    string
		req     updateRegistryCredentialRequest
		wantErr bool
	}{
		{name: "valid, no password rotation", req: valid, wantErr: false},
		{name: "valid, password rotated", req: withUpdateRegistryCredentialField(valid, func(r *updateRegistryCredentialRequest) { r.Password = "new-tok" }), wantErr: false},
		{name: "missing name", req: withUpdateRegistryCredentialField(valid, func(r *updateRegistryCredentialRequest) { r.Name = "" }), wantErr: true},
		{name: "missing registry_host", req: withUpdateRegistryCredentialField(valid, func(r *updateRegistryCredentialRequest) { r.RegistryHost = "" }), wantErr: true},
		{name: "missing username", req: withUpdateRegistryCredentialField(valid, func(r *updateRegistryCredentialRequest) { r.Username = "" }), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateUpdateRegistryCredentialRequest(tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateUpdateRegistryCredentialRequest(%+v) error = %v, wantErr %v", tt.req, err, tt.wantErr)
			}
		})
	}
}

func withUpdateRegistryCredentialField(base updateRegistryCredentialRequest, mutate func(*updateRegistryCredentialRequest)) updateRegistryCredentialRequest {
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
		{http.MethodGet, "/api/v1/registry-credentials/regcred_x"},
		{http.MethodPut, "/api/v1/registry-credentials/regcred_x"},
		{http.MethodDelete, "/api/v1/registry-credentials/regcred_x"},
		{http.MethodPost, "/api/v1/registry-credentials/regcred_x/test"},
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

func TestHandleGetRegistryCredential_NotFound(t *testing.T) {
	rt, db := newTestRouterWithRegistryCredentialSecrets(t, &fakeRegistryCredentialSecretsSetter{})
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/registry-credentials/regcred_missing", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleUpdateRegistryCredential_Success(t *testing.T) {
	setter := &fakeRegistryCredentialSecretsSetter{}
	rt, db := newTestRouterWithRegistryCredentialSecrets(t, setter)
	cookie := loginTestSession(t, rt, db)

	createRec := httptest.NewRecorder()
	createBody := `{"name":"ghcr-bot","registry_host":"ghcr.io","username":"bot","password":"tok"}`
	rt.Handler().ServeHTTP(createRec, authedRequest(t, cookie, http.MethodPost, "/api/v1/registry-credentials", createBody))
	var created registryCredentialResource
	if err := json.NewDecoder(createRec.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	setter.calls = nil

	updateRec := httptest.NewRecorder()
	updateBody := `{"name":"ghcr-bot-renamed","registry_host":"ghcr.io","username":"new-user"}`
	rt.Handler().ServeHTTP(updateRec, authedRequest(t, cookie, http.MethodPut, "/api/v1/registry-credentials/"+created.ID, updateBody))
	if updateRec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", updateRec.Code, http.StatusOK, updateRec.Body.String())
	}

	var got registryCredentialResource
	if err := json.NewDecoder(updateRec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Name != "ghcr-bot-renamed" || got.Username != "new-user" {
		t.Errorf("response = %+v, want the updated fields", got)
	}
	if len(setter.calls) != 0 {
		t.Errorf("SetValue calls = %d, want 0 when the request carries no password", len(setter.calls))
	}
}

func TestHandleUpdateRegistryCredential_RotatesPassword(t *testing.T) {
	setter := &fakeRegistryCredentialSecretsSetter{}
	rt, db := newTestRouterWithRegistryCredentialSecrets(t, setter)
	cookie := loginTestSession(t, rt, db)

	createRec := httptest.NewRecorder()
	createBody := `{"name":"ghcr-bot","registry_host":"ghcr.io","username":"bot","password":"tok"}`
	rt.Handler().ServeHTTP(createRec, authedRequest(t, cookie, http.MethodPost, "/api/v1/registry-credentials", createBody))
	var created registryCredentialResource
	if err := json.NewDecoder(createRec.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	setter.calls = nil

	updateRec := httptest.NewRecorder()
	updateBody := `{"name":"ghcr-bot","registry_host":"ghcr.io","username":"bot","password":"rotated-tok"}`
	rt.Handler().ServeHTTP(updateRec, authedRequest(t, cookie, http.MethodPut, "/api/v1/registry-credentials/"+created.ID, updateBody))
	if updateRec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", updateRec.Code, http.StatusOK, updateRec.Body.String())
	}
	if strings.Contains(updateRec.Body.String(), "rotated-tok") {
		t.Errorf("response body = %s, password must never be echoed back", updateRec.Body.String())
	}

	if len(setter.calls) != 1 {
		t.Fatalf("SetValue calls = %d, want 1", len(setter.calls))
	}
	wantKey := store.RegistryCredentialSecretsKey(created.ID)
	if setter.calls[0].serviceName != wantKey || setter.calls[0].envKey != "password" || setter.calls[0].plaintext != "rotated-tok" {
		t.Errorf("SetValue call = %+v, want serviceName=%q envKey=password plaintext=rotated-tok", setter.calls[0], wantKey)
	}
}

func TestHandleUpdateRegistryCredential_NotFound(t *testing.T) {
	rt, db := newTestRouterWithRegistryCredentialSecrets(t, &fakeRegistryCredentialSecretsSetter{})
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	body := `{"name":"x","registry_host":"ghcr.io","username":"bot"}`
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/registry-credentials/regcred_missing", body))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestHandleUpdateRegistryCredential_DuplicateName_Conflict(t *testing.T) {
	setter := &fakeRegistryCredentialSecretsSetter{}
	rt, db := newTestRouterWithRegistryCredentialSecrets(t, setter)
	cookie := loginTestSession(t, rt, db)

	firstRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(firstRec, authedRequest(t, cookie, http.MethodPost, "/api/v1/registry-credentials", `{"name":"ghcr-bot","registry_host":"ghcr.io","username":"bot","password":"tok"}`))
	var first registryCredentialResource
	if err := json.NewDecoder(firstRec.Body).Decode(&first); err != nil {
		t.Fatalf("decode first create response: %v", err)
	}

	secondRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(secondRec, authedRequest(t, cookie, http.MethodPost, "/api/v1/registry-credentials", `{"name":"other-bot","registry_host":"ghcr.io","username":"bot","password":"tok"}`))
	var second registryCredentialResource
	if err := json.NewDecoder(secondRec.Body).Decode(&second); err != nil {
		t.Fatalf("decode second create response: %v", err)
	}

	updateRec := httptest.NewRecorder()
	updateBody := `{"name":"ghcr-bot","registry_host":"ghcr.io","username":"bot"}`
	rt.Handler().ServeHTTP(updateRec, authedRequest(t, cookie, http.MethodPut, "/api/v1/registry-credentials/"+second.ID, updateBody))
	if updateRec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d, body = %s", updateRec.Code, http.StatusConflict, updateRec.Body.String())
	}
}

func TestHandleUpdateRegistryCredential_SameNameNoConflict(t *testing.T) {
	setter := &fakeRegistryCredentialSecretsSetter{}
	rt, db := newTestRouterWithRegistryCredentialSecrets(t, setter)
	cookie := loginTestSession(t, rt, db)

	createRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(createRec, authedRequest(t, cookie, http.MethodPost, "/api/v1/registry-credentials", `{"name":"ghcr-bot","registry_host":"ghcr.io","username":"bot","password":"tok"}`))
	var created registryCredentialResource
	if err := json.NewDecoder(createRec.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	updateRec := httptest.NewRecorder()
	updateBody := `{"name":"ghcr-bot","registry_host":"ghcr.io","username":"renamed-user"}`
	rt.Handler().ServeHTTP(updateRec, authedRequest(t, cookie, http.MethodPut, "/api/v1/registry-credentials/"+created.ID, updateBody))
	if updateRec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s (unchanged own name must not conflict with itself)", updateRec.Code, http.StatusOK, updateRec.Body.String())
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

	getRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(getRec, authedRequest(t, cookie, http.MethodGet, "/api/v1/registry-credentials/"+created.ID, ""))
	if getRec.Code != http.StatusOK {
		t.Fatalf("get status = %d, want %d, body = %s", getRec.Code, http.StatusOK, getRec.Body.String())
	}
	var got registryCredentialResource
	if err := json.NewDecoder(getRec.Body).Decode(&got); err != nil {
		t.Fatalf("decode get response: %v", err)
	}
	if got.ID != created.ID || strings.Contains(getRec.Body.String(), "tok") {
		t.Errorf("get response = %s, want the created credential with no password field", getRec.Body.String())
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

func createTestRegistryCredential(t *testing.T, rt *Router, cookie *http.Cookie, body string) registryCredentialResource {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/registry-credentials", body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d, body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var created registryCredentialResource
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	return created
}

func TestHandleTestRegistryCredential_NotConfigured(t *testing.T) {
	setter := &fakeRegistryCredentialSecretsSetter{}
	rt, db := newTestRouterWithRegistryCredentialSecrets(t, setter) // no WithRegistryAuthTester
	cookie := loginTestSession(t, rt, db)

	created := createTestRegistryCredential(t, rt, cookie, `{"name":"ghcr-bot","registry_host":"ghcr.io","username":"bot","password":"tok"}`)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/registry-credentials/"+created.ID+"/test", ""))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}
}

func TestHandleTestRegistryCredential_NotFound(t *testing.T) {
	setter := &fakeRegistryCredentialSecretsSetter{}
	tester := &fakeRegistryAuthTester{}
	rt, db := newTestRouterWithRegistryCredentialTest(t, setter, tester)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/registry-credentials/regcred_missing/test", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleTestRegistryCredential_Success(t *testing.T) {
	setter := &fakeRegistryCredentialSecretsSetter{resolveValue: "gh-token-real"}
	tester := &fakeRegistryAuthTester{}
	rt, db := newTestRouterWithRegistryCredentialTest(t, setter, tester)
	cookie := loginTestSession(t, rt, db)

	created := createTestRegistryCredential(t, rt, cookie, `{"name":"ghcr-bot","registry_host":"ghcr.io","username":"bot","password":"tok"}`)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/registry-credentials/"+created.ID+"/test", ""))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
	if len(tester.calls) != 1 {
		t.Fatalf("TestRegistryAuth calls = %d, want 1", len(tester.calls))
	}
	call := tester.calls[0]
	if call.host != "ghcr.io" || call.username != "bot" || call.password != "gh-token-real" {
		t.Errorf("TestRegistryAuth call = %+v, want host=ghcr.io username=bot password=gh-token-real", call)
	}
}

func TestHandleTestRegistryCredential_AuthRejected(t *testing.T) {
	setter := &fakeRegistryCredentialSecretsSetter{resolveValue: "bad-tok"}
	tester := &fakeRegistryAuthTester{err: fmt.Errorf("login failed: %w", cerrdefs.ErrUnauthenticated)}
	rt, db := newTestRouterWithRegistryCredentialTest(t, setter, tester)
	cookie := loginTestSession(t, rt, db)

	created := createTestRegistryCredential(t, rt, cookie, `{"name":"ghcr-bot","registry_host":"ghcr.io","username":"bot","password":"tok"}`)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/registry-credentials/"+created.ID+"/test", ""))
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadGateway, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "authentication rejected") {
		t.Errorf("body = %s, want an authentication-rejected message", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "bad-tok") {
		t.Errorf("body = %s, password must never be echoed back", rec.Body.String())
	}
}

func TestHandleTestRegistryCredential_Unreachable(t *testing.T) {
	setter := &fakeRegistryCredentialSecretsSetter{resolveValue: "tok"}
	tester := &fakeRegistryAuthTester{err: errors.New("dial tcp: connection refused")}
	rt, db := newTestRouterWithRegistryCredentialTest(t, setter, tester)
	cookie := loginTestSession(t, rt, db)

	created := createTestRegistryCredential(t, rt, cookie, `{"name":"ghcr-bot","registry_host":"ghcr.io","username":"bot","password":"tok"}`)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/registry-credentials/"+created.ID+"/test", ""))
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadGateway, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "could not reach registry") {
		t.Errorf("body = %s, want a could-not-reach message", rec.Body.String())
	}
}

func TestHandleCreateRegistryCredential_ExpiresAt(t *testing.T) {
	setter := &fakeRegistryCredentialSecretsSetter{}
	rt, db := newTestRouterWithRegistryCredentialSecrets(t, setter)
	cookie := loginTestSession(t, rt, db)

	expires := time.Now().Add(2 * time.Hour).UTC().Truncate(time.Second)
	body := fmt.Sprintf(`{"name":"ghcr-bot","registry_host":"ghcr.io","username":"bot","password":"tok","expires_at":%q}`, expires.Format(time.RFC3339))
	created := createTestRegistryCredential(t, rt, cookie, body)

	if created.ExpiresAt == nil || !created.ExpiresAt.Equal(expires) {
		t.Fatalf("ExpiresAt = %v, want %v", created.ExpiresAt, expires)
	}
	if created.ExpiryStatus != "expiring_soon" {
		t.Errorf("ExpiryStatus = %q, want expiring_soon for an expiry 2 hours out", created.ExpiryStatus)
	}
}

func TestHandleCreateRegistryCredential_NoExpiresAt(t *testing.T) {
	setter := &fakeRegistryCredentialSecretsSetter{}
	rt, db := newTestRouterWithRegistryCredentialSecrets(t, setter)
	cookie := loginTestSession(t, rt, db)

	created := createTestRegistryCredential(t, rt, cookie, `{"name":"ghcr-bot","registry_host":"ghcr.io","username":"bot","password":"tok"}`)

	if created.ExpiresAt != nil {
		t.Errorf("ExpiresAt = %v, want nil when the request carries none", created.ExpiresAt)
	}
	if created.ExpiryStatus != "" {
		t.Errorf("ExpiryStatus = %q, want empty when no expiry was set", created.ExpiryStatus)
	}
}

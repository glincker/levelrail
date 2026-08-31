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

	smithyhttp "github.com/aws/smithy-go/transport/http"

	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeBackupSecretsSetter is a hand-written fake for BackupSecretsSetter,
// the same pattern fakeSecretSetter (secrets_test.go) already
// establishes for the app-secrets equivalent.
type fakeBackupSecretsSetter struct {
	err   error
	calls []struct{ serviceName, envKey, plaintext string }
}

func (f *fakeBackupSecretsSetter) SetValue(_ context.Context, serviceName, envKey, plaintext string) error {
	f.calls = append(f.calls, struct{ serviceName, envKey, plaintext string }{serviceName, envKey, plaintext})
	return f.err
}

func newTestRouterWithBackupSecrets(t *testing.T, secrets BackupSecretsSetter) (*Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	return NewRouter(logger, testBrand(), db, WithBackupSecrets(secrets)), db
}

// fakeBackupTargetTester is a hand-written fake for BackupTargetTester.
type fakeBackupTargetTester struct {
	err   error
	calls []string
}

func (f *fakeBackupTargetTester) TestTarget(_ context.Context, targetID string) error {
	f.calls = append(f.calls, targetID)
	return f.err
}

func newTestRouterWithBackupTargetTest(t *testing.T, secrets BackupSecretsSetter, tester BackupTargetTester) (*Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	return NewRouter(logger, testBrand(), db, WithBackupSecrets(secrets), WithBackupTargetTester(tester)), db
}

// createTestBackupTarget mirrors createTestRegistryCredential's shape
// (registry_credentials_test.go), narrowed to a fixed body: every
// handleTestBackupTarget test below needs is one connected target to
// test against, never a specific name/provider/bucket combination.
func createTestBackupTarget(t *testing.T, rt *Router, cookie *http.Cookie) backupTargetResource {
	t.Helper()
	rec := httptest.NewRecorder()
	body := `{"name":"primary","provider":"aws","bucket":"backups","access_key_id":"AKID","secret_access_key":"topsecret"}`
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/backup-targets", body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d, body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var created backupTargetResource
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	return created
}

func TestValidateCreateBackupTargetRequest(t *testing.T) {
	valid := createBackupTargetRequest{
		Name: "primary", Provider: store.BackupProviderR2, Endpoint: "https://x.r2.cloudflarestorage.com",
		Bucket: "backups", AccessKeyID: "AKID", SecretAccessKey: "shh",
	}
	tests := []struct {
		name    string
		req     createBackupTargetRequest
		wantErr bool
	}{
		{name: "valid r2", req: valid, wantErr: false},
		{name: "valid aws no endpoint", req: createBackupTargetRequest{Name: "p", Provider: store.BackupProviderAWS, Bucket: "b", AccessKeyID: "a", SecretAccessKey: "s"}, wantErr: false},
		{name: "missing name", req: withField(valid, func(r *createBackupTargetRequest) { r.Name = "" }), wantErr: true},
		{name: "unknown provider", req: withField(valid, func(r *createBackupTargetRequest) { r.Provider = "backblaze" }), wantErr: true},
		{name: "missing bucket", req: withField(valid, func(r *createBackupTargetRequest) { r.Bucket = "" }), wantErr: true},
		{name: "r2 missing endpoint", req: withField(valid, func(r *createBackupTargetRequest) { r.Endpoint = "" }), wantErr: true},
		{name: "missing access key id", req: withField(valid, func(r *createBackupTargetRequest) { r.AccessKeyID = "" }), wantErr: true},
		{name: "missing secret access key", req: withField(valid, func(r *createBackupTargetRequest) { r.SecretAccessKey = "" }), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCreateBackupTargetRequest(tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateCreateBackupTargetRequest(%+v) error = %v, wantErr %v", tt.req, err, tt.wantErr)
			}
		})
	}
}

func withField(base createBackupTargetRequest, mutate func(*createBackupTargetRequest)) createBackupTargetRequest {
	mutate(&base)
	return base
}

func TestValidateUpdateBackupTargetRequest(t *testing.T) {
	valid := updateBackupTargetRequest{Name: "primary", Provider: store.BackupProviderR2, Endpoint: "https://x.r2.cloudflarestorage.com", Bucket: "backups"}
	tests := []struct {
		name    string
		req     updateBackupTargetRequest
		wantErr bool
	}{
		{name: "valid, no credential rotation", req: valid, wantErr: false},
		{name: "valid, credentials rotated together", req: withUpdateField(valid, func(r *updateBackupTargetRequest) { r.AccessKeyID, r.SecretAccessKey = "AKID", "shh" }), wantErr: false},
		{name: "missing name", req: withUpdateField(valid, func(r *updateBackupTargetRequest) { r.Name = "" }), wantErr: true},
		{name: "unknown provider", req: withUpdateField(valid, func(r *updateBackupTargetRequest) { r.Provider = "backblaze" }), wantErr: true},
		{name: "missing bucket", req: withUpdateField(valid, func(r *updateBackupTargetRequest) { r.Bucket = "" }), wantErr: true},
		{name: "only access_key_id set", req: withUpdateField(valid, func(r *updateBackupTargetRequest) { r.AccessKeyID = "AKID" }), wantErr: true},
		{name: "only secret_access_key set", req: withUpdateField(valid, func(r *updateBackupTargetRequest) { r.SecretAccessKey = "shh" }), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateUpdateBackupTargetRequest(tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateUpdateBackupTargetRequest(%+v) error = %v, wantErr %v", tt.req, err, tt.wantErr)
			}
		})
	}
}

func withUpdateField(base updateBackupTargetRequest, mutate func(*updateBackupTargetRequest)) updateBackupTargetRequest {
	mutate(&base)
	return base
}

func TestBackupTargetRoutes_RequireAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	routes := []struct {
		method string
		target string
	}{
		{http.MethodGet, "/api/v1/backup-targets"},
		{http.MethodPost, "/api/v1/backup-targets"},
		{http.MethodGet, "/api/v1/backup-targets/bkt_x"},
		{http.MethodPut, "/api/v1/backup-targets/bkt_x"},
		{http.MethodDelete, "/api/v1/backup-targets/bkt_x"},
		{http.MethodPost, "/api/v1/backup-targets/bkt_x/test"},
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

func TestHandleCreateBackupTarget_NoSetterConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithBackupSecrets
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	body := `{"name":"primary","provider":"r2","endpoint":"https://x.r2.cloudflarestorage.com","bucket":"b","access_key_id":"a","secret_access_key":"s"}`
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/backup-targets", body))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}
}

func TestHandleCreateBackupTarget_Success(t *testing.T) {
	setter := &fakeBackupSecretsSetter{}
	rt, db := newTestRouterWithBackupSecrets(t, setter)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	body := `{"name":"primary","provider":"r2","endpoint":"https://x.r2.cloudflarestorage.com","region":"auto","bucket":"backups","access_key_id":"AKID","secret_access_key":"topsecret"}`
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/backup-targets", body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var got backupTargetResource
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.ID == "" || !strings.HasPrefix(got.ID, "bkt_") {
		t.Errorf("ID = %q, want a bkt_-prefixed id", got.ID)
	}
	if got.Name != "primary" || got.Provider != "r2" || got.Bucket != "backups" {
		t.Errorf("response = %+v, want name/provider/bucket to match the request", got)
	}
	if strings.Contains(rec.Body.String(), "AKID") || strings.Contains(rec.Body.String(), "topsecret") {
		t.Errorf("response body = %s, credentials must never be echoed back", rec.Body.String())
	}

	if len(setter.calls) != 2 {
		t.Fatalf("SetValue calls = %d, want 2 (access_key_id, secret_access_key)", len(setter.calls))
	}
	wantKey := store.BackupTargetSecretsKey(got.ID)
	for _, c := range setter.calls {
		if c.serviceName != wantKey {
			t.Errorf("SetValue serviceName = %q, want %q", c.serviceName, wantKey)
		}
	}
	if setter.calls[0].envKey != "access_key_id" || setter.calls[0].plaintext != "AKID" {
		t.Errorf("first SetValue call = %+v, want access_key_id=AKID", setter.calls[0])
	}
	if setter.calls[1].envKey != "secret_access_key" || setter.calls[1].plaintext != "topsecret" {
		t.Errorf("second SetValue call = %+v, want secret_access_key=topsecret", setter.calls[1])
	}

	stored, err := db.GetBackupTarget(context.Background(), got.ID)
	if err != nil {
		t.Fatalf("GetBackupTarget() error = %v", err)
	}
	if stored.Name != "primary" || stored.Bucket != "backups" {
		t.Errorf("stored target = %+v, want name/bucket to match the request", stored)
	}
}

func TestHandleCreateBackupTarget_InvalidRequest(t *testing.T) {
	rt, db := newTestRouterWithBackupSecrets(t, &fakeBackupSecretsSetter{})
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/backup-targets", `{"name":""}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleCreateBackupTarget_SecretSetFails(t *testing.T) {
	setter := &fakeBackupSecretsSetter{err: errors.New("master key not configured")}
	rt, db := newTestRouterWithBackupSecrets(t, setter)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	body := `{"name":"primary","provider":"aws","bucket":"backups","access_key_id":"AKID","secret_access_key":"topsecret"}`
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/backup-targets", body))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}

	got, err := db.ListBackupTargets(context.Background())
	if err != nil {
		t.Fatalf("ListBackupTargets() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("ListBackupTargets() = %+v, want no row saved when the secret write fails first", got)
	}
}

func TestHandleListAndGetBackupTarget(t *testing.T) {
	setter := &fakeBackupSecretsSetter{}
	rt, db := newTestRouterWithBackupSecrets(t, setter)
	cookie := loginTestSession(t, rt, db)

	createRec := httptest.NewRecorder()
	body := `{"name":"primary","provider":"aws","bucket":"backups","access_key_id":"AKID","secret_access_key":"topsecret"}`
	rt.Handler().ServeHTTP(createRec, authedRequest(t, cookie, http.MethodPost, "/api/v1/backup-targets", body))
	var created backupTargetResource
	if err := json.NewDecoder(createRec.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	listRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(listRec, authedRequest(t, cookie, http.MethodGet, "/api/v1/backup-targets", ""))
	var list []backupTargetResource
	if err := json.NewDecoder(listRec.Body).Decode(&list); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(list) != 1 || list[0].ID != created.ID {
		t.Fatalf("list = %+v, want exactly the created target", list)
	}

	getRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(getRec, authedRequest(t, cookie, http.MethodGet, "/api/v1/backup-targets/"+created.ID, ""))
	if getRec.Code != http.StatusOK {
		t.Fatalf("get status = %d, want %d", getRec.Code, http.StatusOK)
	}

	missingRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(missingRec, authedRequest(t, cookie, http.MethodGet, "/api/v1/backup-targets/bkt_missing", ""))
	if missingRec.Code != http.StatusNotFound {
		t.Fatalf("get missing status = %d, want %d", missingRec.Code, http.StatusNotFound)
	}
}

func TestHandleUpdateBackupTarget_Success(t *testing.T) {
	setter := &fakeBackupSecretsSetter{}
	rt, db := newTestRouterWithBackupSecrets(t, setter)
	cookie := loginTestSession(t, rt, db)

	createRec := httptest.NewRecorder()
	createBody := `{"name":"primary","provider":"aws","bucket":"backups","access_key_id":"AKID","secret_access_key":"topsecret"}`
	rt.Handler().ServeHTTP(createRec, authedRequest(t, cookie, http.MethodPost, "/api/v1/backup-targets", createBody))
	var created backupTargetResource
	if err := json.NewDecoder(createRec.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	setter.calls = nil

	updateRec := httptest.NewRecorder()
	updateBody := `{"name":"renamed","provider":"r2","endpoint":"https://x.r2.cloudflarestorage.com","region":"auto","bucket":"new-bucket"}`
	rt.Handler().ServeHTTP(updateRec, authedRequest(t, cookie, http.MethodPut, "/api/v1/backup-targets/"+created.ID, updateBody))
	if updateRec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", updateRec.Code, http.StatusOK, updateRec.Body.String())
	}

	var got backupTargetResource
	if err := json.NewDecoder(updateRec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Name != "renamed" || got.Provider != "r2" || got.Bucket != "new-bucket" {
		t.Errorf("response = %+v, want the updated fields", got)
	}
	if len(setter.calls) != 0 {
		t.Errorf("SetValue calls = %d, want 0 when the request carries no credentials", len(setter.calls))
	}

	stored, err := db.GetBackupTarget(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetBackupTarget() error = %v", err)
	}
	if stored.Name != "renamed" || stored.Bucket != "new-bucket" {
		t.Errorf("stored target = %+v, want the updated fields", stored)
	}
}

func TestHandleUpdateBackupTarget_RotatesCredentials(t *testing.T) {
	setter := &fakeBackupSecretsSetter{}
	rt, dbForLogin := newTestRouterWithBackupSecrets(t, setter)
	cookie := loginTestSession(t, rt, dbForLogin)

	createRec := httptest.NewRecorder()
	createBody := `{"name":"primary","provider":"aws","bucket":"backups","access_key_id":"AKID","secret_access_key":"topsecret"}`
	rt.Handler().ServeHTTP(createRec, authedRequest(t, cookie, http.MethodPost, "/api/v1/backup-targets", createBody))
	var created backupTargetResource
	if err := json.NewDecoder(createRec.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	setter.calls = nil

	updateRec := httptest.NewRecorder()
	updateBody := `{"name":"primary","provider":"aws","bucket":"backups","access_key_id":"AKID2","secret_access_key":"rotated"}`
	rt.Handler().ServeHTTP(updateRec, authedRequest(t, cookie, http.MethodPut, "/api/v1/backup-targets/"+created.ID, updateBody))
	if updateRec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", updateRec.Code, http.StatusOK, updateRec.Body.String())
	}
	if strings.Contains(updateRec.Body.String(), "AKID2") || strings.Contains(updateRec.Body.String(), "rotated") {
		t.Errorf("response body = %s, credentials must never be echoed back", updateRec.Body.String())
	}

	if len(setter.calls) != 2 {
		t.Fatalf("SetValue calls = %d, want 2 (access_key_id, secret_access_key)", len(setter.calls))
	}
	wantKey := store.BackupTargetSecretsKey(created.ID)
	if setter.calls[0].serviceName != wantKey || setter.calls[0].envKey != "access_key_id" || setter.calls[0].plaintext != "AKID2" {
		t.Errorf("first SetValue call = %+v, want access_key_id=AKID2", setter.calls[0])
	}
	if setter.calls[1].serviceName != wantKey || setter.calls[1].envKey != "secret_access_key" || setter.calls[1].plaintext != "rotated" {
		t.Errorf("second SetValue call = %+v, want secret_access_key=rotated", setter.calls[1])
	}
}

func TestHandleUpdateBackupTarget_NotFound(t *testing.T) {
	setter := &fakeBackupSecretsSetter{}
	rt, db := newTestRouterWithBackupSecrets(t, setter)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	body := `{"name":"x","provider":"aws","bucket":"b"}`
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/backup-targets/bkt_missing", body))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestHandleUpdateBackupTarget_InvalidRequest(t *testing.T) {
	setter := &fakeBackupSecretsSetter{}
	rt, db := newTestRouterWithBackupSecrets(t, setter)
	cookie := loginTestSession(t, rt, db)

	createRec := httptest.NewRecorder()
	createBody := `{"name":"primary","provider":"aws","bucket":"backups","access_key_id":"AKID","secret_access_key":"topsecret"}`
	rt.Handler().ServeHTTP(createRec, authedRequest(t, cookie, http.MethodPost, "/api/v1/backup-targets", createBody))
	var created backupTargetResource
	if err := json.NewDecoder(createRec.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/backup-targets/"+created.ID, `{"name":""}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleDeleteBackupTarget(t *testing.T) {
	setter := &fakeBackupSecretsSetter{}
	rt, db := newTestRouterWithBackupSecrets(t, setter)
	cookie := loginTestSession(t, rt, db)

	createRec := httptest.NewRecorder()
	body := `{"name":"primary","provider":"aws","bucket":"backups","access_key_id":"AKID","secret_access_key":"topsecret"}`
	rt.Handler().ServeHTTP(createRec, authedRequest(t, cookie, http.MethodPost, "/api/v1/backup-targets", body))
	var created backupTargetResource
	if err := json.NewDecoder(createRec.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	delRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(delRec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/backup-targets/"+created.ID, ""))
	if delRec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want %d, body = %s", delRec.Code, http.StatusNoContent, delRec.Body.String())
	}

	getRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(getRec, authedRequest(t, cookie, http.MethodGet, "/api/v1/backup-targets/"+created.ID, ""))
	if getRec.Code != http.StatusNotFound {
		t.Fatalf("get after delete status = %d, want %d", getRec.Code, http.StatusNotFound)
	}

	missingRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(missingRec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/backup-targets/bkt_missing", ""))
	if missingRec.Code != http.StatusNotFound {
		t.Fatalf("delete missing status = %d, want %d", missingRec.Code, http.StatusNotFound)
	}
}

func TestHandleDeleteBackupTarget_BlockedByHistory(t *testing.T) {
	setter := &fakeBackupSecretsSetter{}
	rt, db := newTestRouterWithBackupSecrets(t, setter)
	cookie := loginTestSession(t, rt, db)

	createRec := httptest.NewRecorder()
	body := `{"name":"primary","provider":"aws","bucket":"backups","access_key_id":"AKID","secret_access_key":"topsecret"}`
	rt.Handler().ServeHTTP(createRec, authedRequest(t, cookie, http.MethodPost, "/api/v1/backup-targets", body))
	var created backupTargetResource
	if err := json.NewDecoder(createRec.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	if err := db.StartBackupHistory(context.Background(), store.BackupHistory{
		ID: "bkh_1", DatabaseName: "mydb", TargetID: created.ID,
		ObjectKey: "mydb/mydb-1.dump", StartedAt: "2026-08-14T00:00:00Z",
	}); err != nil {
		t.Fatalf("seed backup history: %v", err)
	}

	delRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(delRec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/backup-targets/"+created.ID, ""))
	if delRec.Code != http.StatusConflict {
		t.Fatalf("delete status = %d, want %d, body = %s", delRec.Code, http.StatusConflict, delRec.Body.String())
	}
}

func TestHandleTestBackupTarget_NotConfigured(t *testing.T) {
	setter := &fakeBackupSecretsSetter{}
	rt, db := newTestRouterWithBackupSecrets(t, setter) // no WithBackupTargetTester
	cookie := loginTestSession(t, rt, db)

	created := createTestBackupTarget(t, rt, cookie)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/backup-targets/"+created.ID+"/test", ""))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}
}

func TestHandleTestBackupTarget_NotFound(t *testing.T) {
	setter := &fakeBackupSecretsSetter{}
	tester := &fakeBackupTargetTester{}
	rt, db := newTestRouterWithBackupTargetTest(t, setter, tester)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/backup-targets/bkt_missing/test", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleTestBackupTarget_Success(t *testing.T) {
	setter := &fakeBackupSecretsSetter{}
	tester := &fakeBackupTargetTester{}
	rt, db := newTestRouterWithBackupTargetTest(t, setter, tester)
	cookie := loginTestSession(t, rt, db)

	created := createTestBackupTarget(t, rt, cookie)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/backup-targets/"+created.ID+"/test", ""))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
	if len(tester.calls) != 1 || tester.calls[0] != created.ID {
		t.Fatalf("TestTarget calls = %v, want [%s]", tester.calls, created.ID)
	}
}

func TestHandleTestBackupTarget_AuthRejected(t *testing.T) {
	setter := &fakeBackupSecretsSetter{}
	tester := &fakeBackupTargetTester{err: &smithyhttp.ResponseError{
		Response: &smithyhttp.Response{Response: &http.Response{StatusCode: http.StatusForbidden}},
		Err:      errors.New("403 Forbidden"),
	}}
	rt, db := newTestRouterWithBackupTargetTest(t, setter, tester)
	cookie := loginTestSession(t, rt, db)

	created := createTestBackupTarget(t, rt, cookie)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/backup-targets/"+created.ID+"/test", ""))
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadGateway, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "authentication rejected") {
		t.Errorf("body = %s, want an authentication-rejected message", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "topsecret") || strings.Contains(rec.Body.String(), "AKID") {
		t.Errorf("body = %s, credentials must never be echoed back", rec.Body.String())
	}
}

func TestHandleTestBackupTarget_BucketNotFound(t *testing.T) {
	setter := &fakeBackupSecretsSetter{}
	tester := &fakeBackupTargetTester{err: &smithyhttp.ResponseError{
		Response: &smithyhttp.Response{Response: &http.Response{StatusCode: http.StatusNotFound}},
		Err:      errors.New("404 Not Found"),
	}}
	rt, db := newTestRouterWithBackupTargetTest(t, setter, tester)
	cookie := loginTestSession(t, rt, db)

	created := createTestBackupTarget(t, rt, cookie)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/backup-targets/"+created.ID+"/test", ""))
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadGateway, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "bucket not found") {
		t.Errorf("body = %s, want a bucket-not-found message", rec.Body.String())
	}
}

func TestHandleTestBackupTarget_Unreachable(t *testing.T) {
	setter := &fakeBackupSecretsSetter{}
	tester := &fakeBackupTargetTester{err: errors.New("dial tcp: connection refused")}
	rt, db := newTestRouterWithBackupTargetTest(t, setter, tester)
	cookie := loginTestSession(t, rt, db)

	created := createTestBackupTarget(t, rt, cookie)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/backup-targets/"+created.ID+"/test", ""))
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadGateway, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "could not reach backup target") {
		t.Errorf("body = %s, want a could-not-reach message", rec.Body.String())
	}
}

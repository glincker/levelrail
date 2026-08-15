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

func TestBackupTargetRoutes_RequireAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	routes := []struct {
		method string
		target string
	}{
		{http.MethodGet, "/api/v1/backup-targets"},
		{http.MethodPost, "/api/v1/backup-targets"},
		{http.MethodGet, "/api/v1/backup-targets/bkt_x"},
		{http.MethodDelete, "/api/v1/backup-targets/bkt_x"},
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

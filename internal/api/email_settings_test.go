package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeEmailSecretsStore is a hand-written fake for EmailSecretsStore, the
// same pattern fakeBackupSecretsSetter (backup_targets_test.go) already
// establishes for the structurally identical backup-target interface.
type fakeEmailSecretsStore struct {
	err    error
	values map[string]string // envKey -> plaintext, keyed loosely (serviceName is always the same sentinel in practice)
}

func newFakeEmailSecretsStore() *fakeEmailSecretsStore {
	return &fakeEmailSecretsStore{values: map[string]string{}}
}

func (f *fakeEmailSecretsStore) SetValue(_ context.Context, _, envKey, plaintext string) error {
	if f.err != nil {
		return f.err
	}
	f.values[envKey] = plaintext
	return nil
}

func (f *fakeEmailSecretsStore) Exists(_ context.Context, _, envKey string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	_, ok := f.values[envKey]
	return ok, nil
}

func newTestRouterWithEmailSecrets(t *testing.T, secrets EmailSecretsStore) (*Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	return NewRouter(logger, testBrand(), db, WithEmailSecrets(secrets)), db
}

func TestHandleGetEmailSettings_SeededDefault(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/settings/email", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got emailSettingsResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Backend != "" {
		t.Errorf("Backend = %q, want empty", got.Backend)
	}
	if got.SMTPPasswordSet || got.SESSecretAccessKeySet {
		t.Errorf("got %+v, want both secret-set flags false with no secrets configured", got)
	}
}

func TestHandleGetEmailSettings_NeverReturnsCredentialValues(t *testing.T) {
	secrets := newFakeEmailSecretsStore()
	rt, db := newTestRouterWithEmailSecrets(t, secrets)
	cookie := loginTestSession(t, rt, db)

	body := `{"backend":"smtp","smtp_host":"smtp.example.com","smtp_port":587,"smtp_from":"a@example.com","smtp_password":"super-secret"}`
	rt.Handler().ServeHTTP(httptest.NewRecorder(), authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/email", body))

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/settings/email", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	bodyStr := rec.Body.String()
	if strings.Contains(bodyStr, "super-secret") {
		t.Errorf("GET response contains the raw SMTP password: %s", bodyStr)
	}

	var got emailSettingsResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.SMTPPasswordSet {
		t.Error("SMTPPasswordSet = false, want true after a save with a password")
	}
}

func TestHandleUpdateEmailSettings_NoSecretsConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithEmailSecrets
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	body := `{"backend":"smtp","smtp_host":"smtp.example.com","smtp_port":587,"smtp_from":"a@example.com"}`
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/email", body))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}
}

func TestHandleUpdateEmailSettings_SMTP_Success(t *testing.T) {
	secrets := newFakeEmailSecretsStore()
	rt, db := newTestRouterWithEmailSecrets(t, secrets)
	cookie := loginTestSession(t, rt, db)

	body := `{"backend":"smtp","smtp_host":"smtp.example.com","smtp_port":587,"smtp_username":"apikey","smtp_from":"a@example.com","smtp_password":"shh"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/email", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	settings, err := db.GetEmailSettings(context.Background())
	if err != nil {
		t.Fatalf("GetEmailSettings() error = %v", err)
	}
	if settings.Backend != store.EmailBackendSMTP || settings.SMTPHost != "smtp.example.com" || settings.SMTPPort != 587 {
		t.Errorf("got %+v, want smtp backend with the submitted fields", settings)
	}
	if secrets.values[emailSecretsSMTPPasswordKey] != "shh" {
		t.Errorf("stored smtp password = %q, want shh", secrets.values[emailSecretsSMTPPasswordKey])
	}
}

func TestHandleUpdateEmailSettings_OmittedPasswordLeavesExistingIntact(t *testing.T) {
	secrets := newFakeEmailSecretsStore()
	rt, db := newTestRouterWithEmailSecrets(t, secrets)
	cookie := loginTestSession(t, rt, db)

	first := `{"backend":"smtp","smtp_host":"smtp.example.com","smtp_port":587,"smtp_from":"a@example.com","smtp_password":"original"}`
	rt.Handler().ServeHTTP(httptest.NewRecorder(), authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/email", first))

	// Second save changes only the host, omitting smtp_password entirely.
	second := `{"backend":"smtp","smtp_host":"smtp2.example.com","smtp_port":587,"smtp_from":"a@example.com"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/email", second))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	if secrets.values[emailSecretsSMTPPasswordKey] != "original" {
		t.Errorf("stored smtp password = %q, want it left untouched at 'original'", secrets.values[emailSecretsSMTPPasswordKey])
	}
	settings, err := db.GetEmailSettings(context.Background())
	if err != nil {
		t.Fatalf("GetEmailSettings() error = %v", err)
	}
	if settings.SMTPHost != "smtp2.example.com" {
		t.Errorf("SMTPHost = %q, want smtp2.example.com", settings.SMTPHost)
	}
}

func TestHandleUpdateEmailSettings_SES_Success(t *testing.T) {
	secrets := newFakeEmailSecretsStore()
	rt, db := newTestRouterWithEmailSecrets(t, secrets)
	cookie := loginTestSession(t, rt, db)

	body := `{"backend":"ses","ses_region":"us-east-1","ses_access_key_id":"AKID","ses_from":"a@example.com","ses_secret_access_key":"shh"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/email", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if secrets.values[emailSecretsSESSecretAccessKey] != "shh" {
		t.Errorf("stored ses secret = %q, want shh", secrets.values[emailSecretsSESSecretAccessKey])
	}
}

func TestValidateEmailSettingsRequest(t *testing.T) {
	tests := []struct {
		name    string
		req     emailSettingsResource
		wantErr bool
	}{
		{name: "empty backend valid", req: emailSettingsResource{Backend: ""}, wantErr: false},
		{name: "unknown backend", req: emailSettingsResource{Backend: "mailgun"}, wantErr: true},
		{name: "smtp missing host", req: emailSettingsResource{Backend: store.EmailBackendSMTP, SMTPFrom: "a@example.com", SMTPPort: 587}, wantErr: true},
		{name: "smtp missing from", req: emailSettingsResource{Backend: store.EmailBackendSMTP, SMTPHost: "h", SMTPPort: 587}, wantErr: true},
		{name: "smtp missing port", req: emailSettingsResource{Backend: store.EmailBackendSMTP, SMTPHost: "h", SMTPFrom: "a@example.com"}, wantErr: true},
		{name: "smtp valid", req: emailSettingsResource{Backend: store.EmailBackendSMTP, SMTPHost: "h", SMTPFrom: "a@example.com", SMTPPort: 587}, wantErr: false},
		{name: "ses missing region", req: emailSettingsResource{Backend: store.EmailBackendSES, SESFrom: "a@example.com", SESAccessKeyID: "id"}, wantErr: true},
		{name: "ses missing from", req: emailSettingsResource{Backend: store.EmailBackendSES, SESRegion: "us-east-1", SESAccessKeyID: "id"}, wantErr: true},
		{name: "ses missing access key id", req: emailSettingsResource{Backend: store.EmailBackendSES, SESRegion: "us-east-1", SESFrom: "a@example.com"}, wantErr: true},
		{name: "ses valid", req: emailSettingsResource{Backend: store.EmailBackendSES, SESRegion: "us-east-1", SESFrom: "a@example.com", SESAccessKeyID: "id"}, wantErr: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateEmailSettingsRequest(tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateEmailSettingsRequest(%+v) error = %v, wantErr %v", tt.req, err, tt.wantErr)
			}
		})
	}
}

// TestHandleUpdateEmailSettings_PlainWriteToken_Forbidden proves PUT
// /settings/email really sits behind AbilityRoot, not just AbilityWrite:
// the same "real infrastructure, not an ordinary per-app write" blast
// radius TestHandleUpdateIngressSettings_PlainWriteToken_Forbidden
// already draws this exact line for.
func TestHandleUpdateEmailSettings_PlainWriteToken_Forbidden(t *testing.T) {
	secrets := newFakeEmailSecretsStore()
	rt, db := newTestRouterWithEmailSecrets(t, secrets)
	ctx := context.Background()

	const plaintext = "write-scoped-token" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(ctx, store.APIToken{
		ID: "tok_write", Name: "writer", TokenHash: hashToken(plaintext), Abilities: []string{AbilityWrite}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}

	body := `{"backend":"smtp","smtp_host":"smtp.example.com","smtp_port":587,"smtp_from":"a@example.com"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings/email", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d: a plain write token must not reach the email settings write", rec.Code, http.StatusForbidden)
	}

	settings, err := db.GetEmailSettings(ctx)
	if err != nil {
		t.Fatalf("GetEmailSettings() error = %v", err)
	}
	if settings.Backend != "" {
		t.Errorf("Backend = %q, want unchanged (empty) after a rejected request", settings.Backend)
	}
}

// TestHandleUpdateEmailSettings_RootToken_Succeeds is the positive half
// of the ability gate above: a token actually holding AbilityRoot must
// be able to write, proving the gate checks for root specifically, not
// merely rejecting everything.
func TestHandleUpdateEmailSettings_RootToken_Succeeds(t *testing.T) {
	secrets := newFakeEmailSecretsStore()
	rt, db := newTestRouterWithEmailSecrets(t, secrets)
	ctx := context.Background()

	const plaintext = "root-token" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(ctx, store.APIToken{
		ID: "tok_root", Name: "root", TokenHash: hashToken(plaintext), Abilities: []string{AbilityRoot}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}

	body := `{"backend":"smtp","smtp_host":"smtp.example.com","smtp_port":587,"smtp_from":"a@example.com"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings/email", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

// TestHandleUpdateEmailSettings_RealSecretsManager_RoundTripsThroughEncryption
// is the whole-chain proof (not a fake) that the SMTP password and SES
// secret access key this handler writes actually go through
// internal/secrets' envelope encryption: a real master key, a real
// store, a real secrets.Manager. It verifies both directions: the value
// PUT accepts comes back out exactly via secretsManager.Resolve, and the
// bytes actually persisted in service_secret_values are real ciphertext
// (never equal to, and never containing, the plaintext), the same "only
// ciphertext ever reaches disk" property
// TestController_Reconcile_Live_SecretEnv verifies for app secrets.
func TestHandleUpdateEmailSettings_RealSecretsManager_RoundTripsThroughEncryption(t *testing.T) {
	db := openTestDB(t)
	mk, err := secrets.GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() error = %v", err)
	}
	secretsManager := secrets.NewManager(db, mk)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	rt := NewRouter(logger, testBrand(), db, WithEmailSecrets(secretsManager))
	cookie := loginTestSession(t, rt, db)

	const smtpPassword = "correct-horse-battery-staple-smtp" //nolint:gosec // fake fixture, not a real credential
	const sesSecret = "correct-horse-battery-staple-ses"     //nolint:gosec // fake fixture, not a real credential
	body := `{"backend":"smtp","smtp_host":"smtp.example.com","smtp_port":587,"smtp_from":"a@example.com","smtp_password":"` + smtpPassword + `"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/email", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	// Switch to ses on a second save, so both credential fields have been
	// exercised through the real manager by the end of this test.
	sesBody := `{"backend":"ses","ses_region":"us-east-1","ses_access_key_id":"AKID","ses_from":"a@example.com","ses_secret_access_key":"` + sesSecret + `"}`
	rec2 := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec2, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/email", sesBody))
	if rec2.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec2.Code, http.StatusOK, rec2.Body.String())
	}

	ctx := context.Background()
	key := store.EmailSettingsSecretsKey()

	gotSMTPPassword, err := secretsManager.Resolve(ctx, key, emailSecretsSMTPPasswordKey)
	if err != nil {
		t.Fatalf("Resolve(smtp_password) error = %v", err)
	}
	if gotSMTPPassword != smtpPassword {
		t.Errorf("Resolve(smtp_password) = %q, want %q", gotSMTPPassword, smtpPassword)
	}
	gotSESSecret, err := secretsManager.Resolve(ctx, key, emailSecretsSESSecretAccessKey)
	if err != nil {
		t.Fatalf("Resolve(ses_secret_access_key) error = %v", err)
	}
	if gotSESSecret != sesSecret {
		t.Errorf("Resolve(ses_secret_access_key) = %q, want %q", gotSESSecret, sesSecret)
	}

	// Only ciphertext ever reaches disk: the raw ciphertext column must
	// never equal, and must never even contain, either plaintext value.
	smtpCiphertext, err := db.GetSecretValue(ctx, key, emailSecretsSMTPPasswordKey)
	if err != nil {
		t.Fatalf("GetSecretValue(smtp_password) error = %v", err)
	}
	if strings.Contains(string(smtpCiphertext), smtpPassword) {
		t.Error("smtp_password ciphertext contains the raw plaintext: encryption is not actually happening")
	}
	sesCiphertext, err := db.GetSecretValue(ctx, key, emailSecretsSESSecretAccessKey)
	if err != nil {
		t.Fatalf("GetSecretValue(ses_secret_access_key) error = %v", err)
	}
	if strings.Contains(string(sesCiphertext), sesSecret) {
		t.Error("ses_secret_access_key ciphertext contains the raw plaintext: encryption is not actually happening")
	}
}

func TestEmailSettingsRoutes_RequireAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	routes := []struct {
		method string
		target string
	}{
		{http.MethodGet, "/api/v1/settings/email"},
		{http.MethodPut, "/api/v1/settings/email"},
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

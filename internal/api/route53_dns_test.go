package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeRoute53DNSSecrets mirrors fakeCloudflareDNSSecrets exactly, for
// the structurally identical Route53DNSSecrets interface.
type fakeRoute53DNSSecrets struct {
	err    error
	values map[string]string
}

func newFakeRoute53DNSSecrets() *fakeRoute53DNSSecrets {
	return &fakeRoute53DNSSecrets{values: map[string]string{}}
}

func (f *fakeRoute53DNSSecrets) SetValue(_ context.Context, _, envKey, plaintext string) error {
	if f.err != nil {
		return f.err
	}
	f.values[envKey] = plaintext
	return nil
}

func (f *fakeRoute53DNSSecrets) Exists(_ context.Context, _, envKey string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	_, ok := f.values[envKey]
	return ok, nil
}

func (f *fakeRoute53DNSSecrets) DeleteAll(_ context.Context, _ string) error {
	if f.err != nil {
		return f.err
	}
	f.values = map[string]string{}
	return nil
}

func newTestRouterWithRoute53DNSSecrets(t *testing.T, s Route53DNSSecrets) (*Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	return NewRouter(logger, testBrand(), db, WithRoute53DNSSecrets(s)), db
}

func TestHandleGetRoute53DNSSettings_SeededDefault(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/settings/route53-dns", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got route53DNSResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Enabled {
		t.Errorf("Enabled = true, want false")
	}
	if got.HasAccessKeyID || got.HasSecretAccessKey {
		t.Errorf("HasAccessKeyID/HasSecretAccessKey = %v/%v, want both false with no secrets configured", got.HasAccessKeyID, got.HasSecretAccessKey)
	}
}

func TestHandleGetRoute53DNSSettings_NeverReturnsCredentialValues(t *testing.T) {
	s := newFakeRoute53DNSSecrets()
	rt, db := newTestRouterWithRoute53DNSSecrets(t, s)
	cookie := loginTestSession(t, rt, db)

	body := `{"enabled":true,"access_key_id":"AKIASECRETID","secret_access_key":"super-secret-key"}`
	rt.Handler().ServeHTTP(httptest.NewRecorder(), authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/route53-dns", body))

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/settings/route53-dns", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	bodyStr := rec.Body.String()
	if strings.Contains(bodyStr, "AKIASECRETID") || strings.Contains(bodyStr, "super-secret-key") {
		t.Errorf("GET response contains a raw credential: %s", bodyStr)
	}

	var got route53DNSResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.HasAccessKeyID || !got.HasSecretAccessKey {
		t.Errorf("HasAccessKeyID/HasSecretAccessKey = %v/%v, want both true after a save", got.HasAccessKeyID, got.HasSecretAccessKey)
	}
	if !got.Enabled {
		t.Error("Enabled = false, want true")
	}
}

func TestHandleUpdateRoute53DNSSettings_NoSecretsConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithRoute53DNSSecrets
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	body := `{"enabled":true,"access_key_id":"a","secret_access_key":"b"}`
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/route53-dns", body))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}
}

func TestHandleUpdateRoute53DNSSettings_EnableWithoutCredentialsRejected(t *testing.T) {
	s := newFakeRoute53DNSSecrets()
	rt, db := newTestRouterWithRoute53DNSSecrets(t, s)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	body := `{"enabled":true}`
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/route53-dns", body))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleUpdateRoute53DNSSettings_PartialCredentialRejected(t *testing.T) {
	s := newFakeRoute53DNSSecrets()
	rt, db := newTestRouterWithRoute53DNSSecrets(t, s)
	cookie := loginTestSession(t, rt, db)

	for _, body := range []string{
		`{"enabled":true,"access_key_id":"only-one-half"}`,
		`{"enabled":true,"secret_access_key":"only-the-other-half"}`,
	} {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/route53-dns", body))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("body %s: status = %d, want %d, body = %s", body, rec.Code, http.StatusBadRequest, rec.Body.String())
		}
	}
}

func TestHandleUpdateRoute53DNSSettings_Success(t *testing.T) {
	s := newFakeRoute53DNSSecrets()
	rt, db := newTestRouterWithRoute53DNSSecrets(t, s)
	cookie := loginTestSession(t, rt, db)

	body := `{"enabled":true,"access_key_id":"AKIA123","secret_access_key":"shh","region":"us-east-1","hosted_zone_id":"Z123"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/route53-dns", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	settings, err := db.GetRoute53DNSSettings(context.Background())
	if err != nil {
		t.Fatalf("GetRoute53DNSSettings() error = %v", err)
	}
	if !settings.Enabled || settings.Region != "us-east-1" || settings.HostedZoneID != "Z123" {
		t.Errorf("settings = %+v, want Enabled=true Region=us-east-1 HostedZoneID=Z123", settings)
	}
	if s.values[store.Route53DNSAccessKeyIDEnvKey] != "AKIA123" {
		t.Errorf("stored access key id = %q, want AKIA123", s.values[store.Route53DNSAccessKeyIDEnvKey])
	}
	if s.values[store.Route53DNSSecretAccessKeyEnvKey] != "shh" {
		t.Errorf("stored secret access key = %q, want shh", s.values[store.Route53DNSSecretAccessKeyEnvKey])
	}
}

func TestHandleUpdateRoute53DNSSettings_OmittedCredentialsLeaveExistingIntact(t *testing.T) {
	s := newFakeRoute53DNSSecrets()
	rt, db := newTestRouterWithRoute53DNSSecrets(t, s)
	cookie := loginTestSession(t, rt, db)

	first := `{"enabled":true,"access_key_id":"original-id","secret_access_key":"original-secret"}`
	rt.Handler().ServeHTTP(httptest.NewRecorder(), authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/route53-dns", first))

	second := `{"enabled":true,"region":"eu-west-1"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/route53-dns", second))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if s.values[store.Route53DNSAccessKeyIDEnvKey] != "original-id" {
		t.Errorf("stored access key id = %q, want it left untouched at 'original-id'", s.values[store.Route53DNSAccessKeyIDEnvKey])
	}
	if s.values[store.Route53DNSSecretAccessKeyEnvKey] != "original-secret" {
		t.Errorf("stored secret access key = %q, want it left untouched at 'original-secret'", s.values[store.Route53DNSSecretAccessKeyEnvKey])
	}

	settings, err := db.GetRoute53DNSSettings(context.Background())
	if err != nil {
		t.Fatalf("GetRoute53DNSSettings() error = %v", err)
	}
	if settings.Region != "eu-west-1" {
		t.Errorf("Region = %q, want eu-west-1", settings.Region)
	}
}

func TestHandleDisconnectRoute53DNS_ClearsCredentialsAndDisables(t *testing.T) {
	s := newFakeRoute53DNSSecrets()
	rt, db := newTestRouterWithRoute53DNSSecrets(t, s)
	cookie := loginTestSession(t, rt, db)

	rt.Handler().ServeHTTP(httptest.NewRecorder(), authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/route53-dns", `{"enabled":true,"access_key_id":"a","secret_access_key":"b"}`))

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/settings/route53-dns", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	settings, err := db.GetRoute53DNSSettings(context.Background())
	if err != nil {
		t.Fatalf("GetRoute53DNSSettings() error = %v", err)
	}
	if settings.Enabled {
		t.Errorf("Enabled = true, want false after disconnect")
	}
	if _, ok := s.values[store.Route53DNSAccessKeyIDEnvKey]; ok {
		t.Errorf("access key id still present after disconnect")
	}
}

func TestHandleDisconnectRoute53DNS_Idempotent(t *testing.T) {
	s := newFakeRoute53DNSSecrets()
	rt, db := newTestRouterWithRoute53DNSSecrets(t, s)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/settings/route53-dns", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d on an already-disconnected setting, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

// TestHandleUpdateRoute53DNSSettings_RealSecretsManager_RoundTripsThroughEncryption
// mirrors the same whole-chain proof cloudflare_dns_test.go's own
// TestHandleUpdateCloudflareDNSSettings_RealSecretsManager... test
// gives, against the distinct Route53DNSSecretsKey() namespace.
func TestHandleUpdateRoute53DNSSettings_RealSecretsManager_RoundTripsThroughEncryption(t *testing.T) {
	db := openTestDB(t)
	mk, err := secrets.GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() error = %v", err)
	}
	secretsManager := secrets.NewManager(db, mk)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	rt := NewRouter(logger, testBrand(), db, WithRoute53DNSSecrets(secretsManager))
	cookie := loginTestSession(t, rt, db)

	const accessKeyID = "AKIAROUNDTRIP"      //nolint:gosec // fake fixture, not a real credential
	const secretAccessKey = "shh-round-trip" //nolint:gosec // fake fixture, not a real credential
	body := `{"enabled":true,"access_key_id":"` + accessKeyID + `","secret_access_key":"` + secretAccessKey + `"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/route53-dns", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	ctx := context.Background()
	key := store.Route53DNSSecretsKey()

	gotAccessKeyID, err := secretsManager.Resolve(ctx, key, store.Route53DNSAccessKeyIDEnvKey)
	if err != nil {
		t.Fatalf("Resolve(access key id) error = %v", err)
	}
	if gotAccessKeyID != accessKeyID {
		t.Errorf("Resolve(access key id) = %q, want %q", gotAccessKeyID, accessKeyID)
	}

	ciphertext, err := db.GetSecretValue(ctx, key, store.Route53DNSSecretAccessKeyEnvKey)
	if err != nil {
		t.Fatalf("GetSecretValue(secret access key) error = %v", err)
	}
	if strings.Contains(string(ciphertext), secretAccessKey) {
		t.Error("secret access key ciphertext contains the raw plaintext: encryption is not actually happening")
	}
}

func TestRoute53DNSRoutes_RequireAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	assertRoutesRequireAuth(t, rt, []routeCase{
		{http.MethodGet, "/api/v1/settings/route53-dns"},
		{http.MethodPut, "/api/v1/settings/route53-dns"},
		{http.MethodDelete, "/api/v1/settings/route53-dns"},
	})
}

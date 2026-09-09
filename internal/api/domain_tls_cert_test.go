package api

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

// genCertKeyPEM generates a real, self-signed leaf certificate/key pair
// for app.example.com (the fixed domain seedAppWithDomain sets up) with
// the given notBefore/notAfter, PEM-encoding both, the same shape
// TestHandleSetDomainTLSCert_Success needs to exercise real
// crypto/tls.X509KeyPair validation rather than a hand-typed fixture.
func genCertKeyPEM(t *testing.T, notBefore, notAfter time.Time) (certPEM, keyPEM string) {
	t.Helper()

	const domain = "app.example.com"
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: domain},
		DNSNames:     []string{domain},
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		Issuer:       pkix.Name{CommonName: "Test CA"},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	certBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})

	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		t.Fatalf("marshal private key: %v", err)
	}
	keyBytes := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	return string(certBytes), string(keyBytes)
}

// fakeDomainTLSCertSecrets is a hand-written fake for
// DomainTLSCertSecrets, mirroring fakeDomainBasicAuthSecrets' shape,
// minus Exists (this feature has no "leave it unchanged" partial-update
// case to check for).
type fakeDomainTLSCertSecrets struct {
	err    error
	values map[string]string // keyed by serviceName+"/"+envKey
}

func newFakeDomainTLSCertSecrets() *fakeDomainTLSCertSecrets {
	return &fakeDomainTLSCertSecrets{values: map[string]string{}}
}

func (f *fakeDomainTLSCertSecrets) SetValue(_ context.Context, serviceName, envKey, plaintext string) error {
	if f.err != nil {
		return f.err
	}
	f.values[serviceName+"/"+envKey] = plaintext
	return nil
}

func (f *fakeDomainTLSCertSecrets) DeleteAll(_ context.Context, serviceName string) error {
	if f.err != nil {
		return f.err
	}
	delete(f.values, serviceName+"/"+store.DomainTLSCertCertificateEnvKey)
	delete(f.values, serviceName+"/"+store.DomainTLSCertPrivateKeyEnvKey)
	return nil
}

func newTestRouterWithDomainTLSCertSecrets(t *testing.T, s DomainTLSCertSecrets) (*Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	return NewRouter(logger, testBrand(), db, WithDomainTLSCertSecrets(s)), db
}

func TestHandleGetDomainTLSCert_NoneConfigured(t *testing.T) {
	rt, db := newTestRouter(t)
	seedAppWithDomain(t, db)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/domains/app.example.com/tls-cert", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got domainTLSCertResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Enabled || got.ExpiresAt != "" || got.UploadedAt != "" {
		t.Errorf("got = %+v, want all zero values before any cert is uploaded", got)
	}
}

func TestHandleGetDomainTLSCert_DomainNotOwnedByApp(t *testing.T) {
	rt, db := newTestRouter(t)
	seedAppWithDomain(t, db)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/domains/other.example.com/tls-cert", ""))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d for a domain the app does not own", rec.Code, http.StatusNotFound)
	}
}

func TestHandleSetDomainTLSCert_NoSecretsConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithDomainTLSCertSecrets
	seedAppWithDomain(t, db)
	cookie := loginTestSession(t, rt, db)

	certPEM, keyPEM := genCertKeyPEM(t, time.Now().Add(-time.Hour), time.Now().AddDate(1, 0, 0))
	body, err := json.Marshal(map[string]string{"cert": certPEM, "key": keyPEM})
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/domains/app.example.com/tls-cert", string(body)))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}
}

func TestHandleSetDomainTLSCert_MissingFieldsRejected(t *testing.T) {
	s := newFakeDomainTLSCertSecrets()
	rt, db := newTestRouterWithDomainTLSCertSecrets(t, s)
	seedAppWithDomain(t, db)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/domains/app.example.com/tls-cert", `{"cert":"only-cert"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleSetDomainTLSCert_GarbageInputRejected(t *testing.T) {
	s := newFakeDomainTLSCertSecrets()
	rt, db := newTestRouterWithDomainTLSCertSecrets(t, s)
	seedAppWithDomain(t, db)
	cookie := loginTestSession(t, rt, db)

	body := `{"cert":"not a pem certificate","key":"not a pem key"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/domains/app.example.com/tls-cert", body))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleSetDomainTLSCert_MismatchedKeyRejected(t *testing.T) {
	s := newFakeDomainTLSCertSecrets()
	rt, db := newTestRouterWithDomainTLSCertSecrets(t, s)
	seedAppWithDomain(t, db)
	cookie := loginTestSession(t, rt, db)

	certPEM, _ := genCertKeyPEM(t, time.Now().Add(-time.Hour), time.Now().AddDate(1, 0, 0))
	_, otherKeyPEM := genCertKeyPEM(t, time.Now().Add(-time.Hour), time.Now().AddDate(1, 0, 0))

	body, err := json.Marshal(map[string]string{"cert": certPEM, "key": otherKeyPEM})
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/domains/app.example.com/tls-cert", string(body)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d for a certificate/key that don't match, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleSetDomainTLSCert_ExpiredCertificateRejected(t *testing.T) {
	s := newFakeDomainTLSCertSecrets()
	rt, db := newTestRouterWithDomainTLSCertSecrets(t, s)
	seedAppWithDomain(t, db)
	cookie := loginTestSession(t, rt, db)

	certPEM, keyPEM := genCertKeyPEM(t, time.Now().AddDate(-1, 0, 0), time.Now().Add(-time.Hour))
	body, err := json.Marshal(map[string]string{"cert": certPEM, "key": keyPEM})
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/domains/app.example.com/tls-cert", string(body)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d for an already-expired certificate, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleSetDomainTLSCert_Success(t *testing.T) {
	s := newFakeDomainTLSCertSecrets()
	rt, db := newTestRouterWithDomainTLSCertSecrets(t, s)
	seedAppWithDomain(t, db)
	cookie := loginTestSession(t, rt, db)

	notAfter := time.Now().AddDate(1, 0, 0)
	certPEM, keyPEM := genCertKeyPEM(t, time.Now().Add(-time.Hour), notAfter)
	body, err := json.Marshal(map[string]string{"cert": certPEM, "key": keyPEM})
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/domains/app.example.com/tls-cert", string(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got domainTLSCertResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.Enabled || got.UploadedAt == "" || got.ExpiresAt == "" {
		t.Errorf("got = %+v, want Enabled=true with non-empty timestamps", got)
	}
	gotExpires, err := time.Parse(time.RFC3339, got.ExpiresAt)
	if err != nil {
		t.Fatalf("parse expires_at: %v", err)
	}
	if gotExpires.Unix() != notAfter.Unix() {
		t.Errorf("expires_at = %v, want %v", gotExpires, notAfter)
	}

	cert, found, err := db.GetDomainTLSCert(context.Background(), "app.example.com")
	if err != nil {
		t.Fatalf("GetDomainTLSCert() error = %v", err)
	}
	if !found {
		t.Fatalf("GetDomainTLSCert() found = false, want true")
	}
	if cert.ExpiresAt.Unix() != notAfter.Unix() {
		t.Errorf("stored ExpiresAt = %v, want %v", cert.ExpiresAt, notAfter)
	}

	key := store.DomainTLSCertSecretsKey("app.example.com")
	if s.values[key+"/"+store.DomainTLSCertCertificateEnvKey] != strings.TrimSpace(certPEM) {
		t.Errorf("stored certificate does not match the uploaded PEM")
	}
	if s.values[key+"/"+store.DomainTLSCertPrivateKeyEnvKey] != strings.TrimSpace(keyPEM) {
		t.Errorf("stored private key does not match the uploaded PEM")
	}
}

func TestHandleSetDomainTLSCert_NeverReturnsKeyMaterial(t *testing.T) {
	s := newFakeDomainTLSCertSecrets()
	rt, db := newTestRouterWithDomainTLSCertSecrets(t, s)
	seedAppWithDomain(t, db)
	cookie := loginTestSession(t, rt, db)

	certPEM, keyPEM := genCertKeyPEM(t, time.Now().Add(-time.Hour), time.Now().AddDate(1, 0, 0))
	body, err := json.Marshal(map[string]string{"cert": certPEM, "key": keyPEM})
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/domains/app.example.com/tls-cert", string(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), keyPEM) || strings.Contains(rec.Body.String(), certPEM) {
		t.Errorf("PUT response contains the raw certificate or key material: %s", rec.Body.String())
	}

	getRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(getRec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/domains/app.example.com/tls-cert", ""))
	if strings.Contains(getRec.Body.String(), keyPEM) || strings.Contains(getRec.Body.String(), certPEM) {
		t.Errorf("GET response contains the raw certificate or key material: %s", getRec.Body.String())
	}
}

func TestHandleClearDomainTLSCert_RemovesCertAndKey(t *testing.T) {
	s := newFakeDomainTLSCertSecrets()
	rt, db := newTestRouterWithDomainTLSCertSecrets(t, s)
	seedAppWithDomain(t, db)
	cookie := loginTestSession(t, rt, db)

	certPEM, keyPEM := genCertKeyPEM(t, time.Now().Add(-time.Hour), time.Now().AddDate(1, 0, 0))
	body, err := json.Marshal(map[string]string{"cert": certPEM, "key": keyPEM})
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	rt.Handler().ServeHTTP(httptest.NewRecorder(), authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/domains/app.example.com/tls-cert", string(body)))

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/web/domains/app.example.com/tls-cert", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	if _, found, err := db.GetDomainTLSCert(context.Background(), "app.example.com"); err != nil {
		t.Fatalf("GetDomainTLSCert() error = %v", err)
	} else if found {
		t.Errorf("found = true, want false after clearing")
	}
	key := store.DomainTLSCertSecretsKey("app.example.com")
	if _, ok := s.values[key+"/"+store.DomainTLSCertCertificateEnvKey]; ok {
		t.Errorf("certificate still present after clearing")
	}
	if _, ok := s.values[key+"/"+store.DomainTLSCertPrivateKeyEnvKey]; ok {
		t.Errorf("private key still present after clearing")
	}
}

func TestHandleClearDomainTLSCert_Idempotent(t *testing.T) {
	s := newFakeDomainTLSCertSecrets()
	rt, db := newTestRouterWithDomainTLSCertSecrets(t, s)
	seedAppWithDomain(t, db)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/web/domains/app.example.com/tls-cert", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d on an already-cleared domain, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

// TestDomainTLSCertWriteRoutes_PlainWriteToken_Forbidden proves PUT/
// DELETE .../domains/{domain}/tls-cert sit behind AbilityRoot, not just
// AbilityWrite, mirroring TestDomainBasicAuthWriteRoutes_PlainWriteToken_Forbidden.
func TestDomainTLSCertWriteRoutes_PlainWriteToken_Forbidden(t *testing.T) {
	s := newFakeDomainTLSCertSecrets()
	rt, db := newTestRouterWithDomainTLSCertSecrets(t, s)
	seedAppWithDomain(t, db)
	ctx := context.Background()

	const plaintext = "write-scoped-token" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(ctx, store.APIToken{
		ID: "tok_write", Name: "writer", TokenHash: hashToken(plaintext), Abilities: []string{AbilityWrite}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}

	certPEM, keyPEM := genCertKeyPEM(t, time.Now().Add(-time.Hour), time.Now().AddDate(1, 0, 0))
	body, err := json.Marshal(map[string]string{"cert": certPEM, "key": keyPEM})
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}

	req := httptest.NewRequest(http.MethodPut, "/api/v1/apps/web/domains/app.example.com/tls-cert", strings.NewReader(string(body)))
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d: a plain write token must not reach the domain tls cert write", rec.Code, http.StatusForbidden)
	}
}

func TestHandleSetDomainTLSCert_RealSecretsManager_RoundTripsThroughEncryption(t *testing.T) {
	db := openTestDB(t)
	seedAppWithDomain(t, db)
	mk, err := secrets.GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() error = %v", err)
	}
	secretsManager := secrets.NewManager(db, mk)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	rt := NewRouter(logger, testBrand(), db, WithDomainTLSCertSecrets(secretsManager))
	cookie := loginTestSession(t, rt, db)

	certPEM, keyPEM := genCertKeyPEM(t, time.Now().Add(-time.Hour), time.Now().AddDate(1, 0, 0))
	body, err := json.Marshal(map[string]string{"cert": certPEM, "key": keyPEM})
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/domains/app.example.com/tls-cert", string(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	ctx := context.Background()
	key := store.DomainTLSCertSecretsKey("app.example.com")

	gotKeyPEM, err := secretsManager.Resolve(ctx, key, store.DomainTLSCertPrivateKeyEnvKey)
	if err != nil {
		t.Fatalf("Resolve(private_key) error = %v", err)
	}
	if gotKeyPEM != strings.TrimSpace(keyPEM) {
		t.Errorf("Resolve(private_key) = %q, want %q", gotKeyPEM, strings.TrimSpace(keyPEM))
	}

	ciphertext, err := db.GetSecretValue(ctx, key, store.DomainTLSCertPrivateKeyEnvKey)
	if err != nil {
		t.Fatalf("GetSecretValue(private_key) error = %v", err)
	}
	if strings.Contains(string(ciphertext), keyPEM) {
		t.Error("private key ciphertext contains the raw PEM: encryption is not actually happening")
	}
}

func TestDomainTLSCertRoutes_RequireAuth(t *testing.T) {
	rt, db := newTestRouter(t)
	seedAppWithDomain(t, db)

	assertRoutesRequireAuth(t, rt, []routeCase{
		{http.MethodGet, "/api/v1/apps/web/domains/app.example.com/tls-cert"},
		{http.MethodPut, "/api/v1/apps/web/domains/app.example.com/tls-cert"},
		{http.MethodDelete, "/api/v1/apps/web/domains/app.example.com/tls-cert"},
	})
}

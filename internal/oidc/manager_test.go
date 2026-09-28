package oidc

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeKeyStore is an in-memory KeyStore, mirroring what
// internal/secrets.Manager does for a (serviceName, envKey) pair
// without any encryption, since Manager's own KeyStore boundary is
// exactly the persistence contract, not the encryption itself (that's
// internal/secrets' job, already tested there).
type fakeKeyStore struct {
	mu     sync.Mutex
	values map[string]string
	// failResolve/failSet/failExists, when set, make the matching call
	// return this error instead, for the half-succeeded-operation cases.
	failResolve, failSet, failExists error
}

func newFakeKeyStore() *fakeKeyStore {
	return &fakeKeyStore{values: map[string]string{}}
}

func (f *fakeKeyStore) key(serviceName, envKey string) string { return serviceName + "\x00" + envKey }

func (f *fakeKeyStore) Exists(_ context.Context, serviceName, envKey string) (bool, error) {
	if f.failExists != nil {
		return false, f.failExists
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.values[f.key(serviceName, envKey)]
	return ok, nil
}

func (f *fakeKeyStore) SetValue(_ context.Context, serviceName, envKey, plaintext string) error {
	if f.failSet != nil {
		return f.failSet
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.values[f.key(serviceName, envKey)] = plaintext
	return nil
}

func (f *fakeKeyStore) Resolve(_ context.Context, serviceName, envKey string) (string, error) {
	if f.failResolve != nil {
		return "", f.failResolve
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.values[f.key(serviceName, envKey)]
	if !ok {
		return "", errors.New("not found")
	}
	return v, nil
}

func fixedNow() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }

func TestNewManager_RequiresKeyStoreAndIssuerURL(t *testing.T) {
	if _, err := NewManager(Config{IssuerURL: "https://cp.example.com"}); err == nil {
		t.Fatal("expected error with no KeyStore")
	}
	if _, err := NewManager(Config{KeyStore: newFakeKeyStore()}); err == nil {
		t.Fatal("expected error with no IssuerURL")
	}
}

func TestManager_IssueToken_RequiresAudience(t *testing.T) {
	m, err := NewManager(Config{KeyStore: newFakeKeyStore(), IssuerURL: "https://cp.example.com", Now: fixedNow})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.IssueToken(context.Background(), TokenRequest{}); err == nil {
		t.Fatal("expected error with no audience")
	}
}

func decodeJWTClaims(t *testing.T, token string) claims {
	t.Helper()
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("token has %d parts, want 3", len(parts))
	}
	body, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode claims: %v", err)
	}
	var c claims
	if err := json.Unmarshal(body, &c); err != nil {
		t.Fatalf("unmarshal claims: %v", err)
	}
	return c
}

func TestManager_IssueToken_GeneratesAndPersistsKeyOnFirstUse(t *testing.T) {
	ks := newFakeKeyStore()
	m, err := NewManager(Config{KeyStore: ks, IssuerURL: "https://cp.example.com", Now: fixedNow})
	if err != nil {
		t.Fatal(err)
	}
	token, err := m.IssueToken(context.Background(), TokenRequest{
		Audience: "sts.amazonaws.com", Subject: "repo:demo:ref:main:job:build", Repo: "demo", Ref: "main", PipelineID: "pl1",
	})
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}
	if strings.Count(token, ".") != 2 {
		t.Fatalf("token = %q, want 3 dot-separated parts", token)
	}
	c := decodeJWTClaims(t, token)
	if c.Issuer != "https://cp.example.com" || c.Audience != "sts.amazonaws.com" || c.Subject != "repo:demo:ref:main:job:build" {
		t.Errorf("claims = %+v", c)
	}
	if want := fixedNow().Add(DefaultTTL).Unix(); c.ExpiresAt != want {
		t.Errorf("exp = %d, want %d", c.ExpiresAt, want)
	}
	if exists, _ := ks.Exists(context.Background(), oidcSigningKeyServiceName, oidcSigningKeyEnvKey); !exists {
		t.Error("expected signing key to be persisted after first IssueToken")
	}
}

func TestManager_IssueToken_ReusesPersistedKeyAcrossManagers(t *testing.T) {
	ks := newFakeKeyStore()
	m1, err := NewManager(Config{KeyStore: ks, IssuerURL: "https://cp.example.com", Now: fixedNow})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m1.IssueToken(context.Background(), TokenRequest{Audience: "aud"}); err != nil {
		t.Fatal(err)
	}
	jwks1, err := m1.JWKS(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	m2, err := NewManager(Config{KeyStore: ks, IssuerURL: "https://cp.example.com", Now: fixedNow})
	if err != nil {
		t.Fatal(err)
	}
	jwks2, err := m2.JWKS(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if jwks1.Keys[0].Kid != jwks2.Keys[0].Kid || jwks1.Keys[0].X != jwks2.Keys[0].X {
		t.Errorf("expected the same key reloaded across managers, got %+v vs %+v", jwks1, jwks2)
	}
}

func TestManager_JWKS_Shape(t *testing.T) {
	m, err := NewManager(Config{KeyStore: newFakeKeyStore(), IssuerURL: "https://cp.example.com", Now: fixedNow})
	if err != nil {
		t.Fatal(err)
	}
	jwks, err := m.JWKS(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(jwks.Keys) != 1 {
		t.Fatalf("len(Keys) = %d, want 1", len(jwks.Keys))
	}
	k := jwks.Keys[0]
	if k.Kty != "EC" || k.Crv != "P-256" || k.Alg != "ES256" || k.Use != "sig" || k.Kid == "" || k.X == "" || k.Y == "" {
		t.Errorf("jwk = %+v", k)
	}
}

func TestManager_EnsureKey_HalfSucceededPersistFails(t *testing.T) {
	ks := newFakeKeyStore()
	ks.failSet = errors.New("store unavailable")
	m, err := NewManager(Config{KeyStore: ks, IssuerURL: "https://cp.example.com", Now: fixedNow})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.IssueToken(context.Background(), TokenRequest{Audience: "aud"}); err == nil {
		t.Fatal("expected error when persisting the generated key fails")
	}
	// A later call with the store healthy again must not be stuck on a
	// half-generated, never-cached key from the failed attempt.
	ks.failSet = nil
	if _, err := m.IssueToken(context.Background(), TokenRequest{Audience: "aud"}); err != nil {
		t.Fatalf("IssueToken after recovery: %v", err)
	}
}

func TestManager_IssueToken_SignatureVerifiesAgainstJWKS(t *testing.T) {
	m, err := NewManager(Config{KeyStore: newFakeKeyStore(), IssuerURL: "https://cp.example.com", Now: fixedNow})
	if err != nil {
		t.Fatal(err)
	}
	token, err := m.IssueToken(context.Background(), TokenRequest{Audience: "aud"})
	if err != nil {
		t.Fatal(err)
	}
	jwks, err := m.JWKS(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	jwk := jwks.Keys[0]

	parts := strings.Split(token, ".")
	signingInput := parts[0] + "." + parts[1]
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatalf("decode signature: %v", err)
	}
	x, _ := base64.RawURLEncoding.DecodeString(jwk.X)
	y, _ := base64.RawURLEncoding.DecodeString(jwk.Y)
	pub := &ecdsa.PublicKey{Curve: elliptic.P256(), X: new(big.Int).SetBytes(x), Y: new(big.Int).SetBytes(y)}

	digest := sha256.Sum256([]byte(signingInput))
	r := new(big.Int).SetBytes(sig[:32])
	s := new(big.Int).SetBytes(sig[32:])
	if !ecdsa.Verify(pub, digest[:], r, s) {
		t.Fatal("signature did not verify against the published JWKS public key")
	}
}

func TestManager_EnsureKey_ResolveFails(t *testing.T) {
	ks := newFakeKeyStore()
	ks.values[ks.key(oidcSigningKeyServiceName, oidcSigningKeyEnvKey)] = "irrelevant"
	ks.failResolve = errors.New("store unavailable")
	m, err := NewManager(Config{KeyStore: ks, IssuerURL: "https://cp.example.com", Now: fixedNow})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.JWKS(context.Background()); err == nil {
		t.Fatal("expected error when resolving an existing key fails")
	}
}

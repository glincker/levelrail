package githubapp

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"strings"
	"testing"
	"time"
)

// testRSAKeyPEM generates a fresh 2048-bit RSA key and returns it
// PKCS#1-PEM-encoded, matching the format GitHub's own app-manifest
// conversion API documents its "pem" response field as. Generated once
// per test via t.Helper, not a fixture on disk: cheap enough (a few ms)
// and avoids ever having even a throwaway test private key checked into
// the repo.
func testRSAKeyPEM(t *testing.T) (*rsa.PrivateKey, []byte) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey() error = %v", err)
	}
	der := x509.MarshalPKCS1PrivateKey(key)
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: der})
	return key, pemBytes
}

func TestSignAppJWT_WellFormedAndVerifiable(t *testing.T) {
	key, pemBytes := testRSAKeyPEM(t)
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)

	token, err := SignAppJWT(424242, pemBytes, now)
	if err != nil {
		t.Fatalf("SignAppJWT() error = %v", err)
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("SignAppJWT() token has %d segments, want 3 (header.claims.signature)", len(parts))
	}
	headerB64, claimsB64, sigB64 := parts[0], parts[1], parts[2]

	// Header: exactly alg=RS256, typ=JWT, no other keys smuggled in.
	headerJSON, err := base64.RawURLEncoding.DecodeString(headerB64)
	if err != nil {
		t.Fatalf("decode header segment: %v", err)
	}
	var header map[string]string
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		t.Fatalf("unmarshal header: %v", err)
	}
	if header["alg"] != "RS256" {
		t.Errorf("header[alg] = %q, want %q", header["alg"], "RS256")
	}
	if header["typ"] != "JWT" {
		t.Errorf("header[typ] = %q, want %q", header["typ"], "JWT")
	}

	// Claims: iss is the numeric App ID as a string, iat is backdated by
	// the documented 60s clock-skew buffer, exp-iat is within GitHub's
	// documented 10-minute (600s) maximum.
	claimsJSON, err := base64.RawURLEncoding.DecodeString(claimsB64)
	if err != nil {
		t.Fatalf("decode claims segment: %v", err)
	}
	var claims struct {
		IAT int64  `json:"iat"`
		EXP int64  `json:"exp"`
		ISS string `json:"iss"`
	}
	if err := json.Unmarshal(claimsJSON, &claims); err != nil {
		t.Fatalf("unmarshal claims: %v", err)
	}
	if claims.ISS != "424242" {
		t.Errorf("claims.iss = %q, want %q", claims.ISS, "424242")
	}
	wantIAT := now.Add(-60 * time.Second).Unix()
	if claims.IAT != wantIAT {
		t.Errorf("claims.iat = %d, want %d (now - 60s clock-skew buffer)", claims.IAT, wantIAT)
	}
	if span := claims.EXP - claims.IAT; span <= 0 || span > 600 {
		t.Errorf("claims.exp - claims.iat = %ds, want (0, 600] per GitHub's documented max", span)
	}
	if claims.EXP <= now.Unix() {
		t.Errorf("claims.exp = %d is not after now (%d): token would already be expired", claims.EXP, now.Unix())
	}

	// Signature: verifies against the same key's public half, over
	// exactly the header.claims signing input, using RS256
	// (RSASSA-PKCS1-v1_5 with SHA-256) per the alg header above.
	sig, err := base64.RawURLEncoding.DecodeString(sigB64)
	if err != nil {
		t.Fatalf("decode signature segment: %v", err)
	}
	digest := sha256.Sum256([]byte(headerB64 + "." + claimsB64))
	if err := rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA256, digest[:], sig); err != nil {
		t.Errorf("signature does not verify against the signing key's public half: %v", err)
	}
}

func TestSignAppJWT_DifferentKeyFailsVerification(t *testing.T) {
	_, pemBytes := testRSAKeyPEM(t)
	otherKey, _ := testRSAKeyPEM(t)
	now := time.Now()

	token, err := SignAppJWT(1, pemBytes, now)
	if err != nil {
		t.Fatalf("SignAppJWT() error = %v", err)
	}
	parts := strings.Split(token, ".")
	sig, _ := base64.RawURLEncoding.DecodeString(parts[2])
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))

	if err := rsa.VerifyPKCS1v15(&otherKey.PublicKey, crypto.SHA256, digest[:], sig); err == nil {
		t.Error("signature verified against an unrelated key's public half, want a verification failure")
	}
}

func TestSignAppJWT_InvalidPEM(t *testing.T) {
	_, err := SignAppJWT(1, []byte("not a pem block"), time.Now())
	if err == nil {
		t.Fatal("SignAppJWT() with invalid PEM: want error, got nil")
	}
}

func TestSignAppJWT_PKCS8Key(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey() error = %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("MarshalPKCS8PrivateKey() error = %v", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})

	token, err := SignAppJWT(1, pemBytes, time.Now())
	if err != nil {
		t.Fatalf("SignAppJWT() with a PKCS#8-encoded key: error = %v, want success (fallback parse path)", err)
	}
	if strings.Count(token, ".") != 2 {
		t.Errorf("SignAppJWT() token = %q, want 3 dot-separated segments", token)
	}
}

func TestValidatePrivateKeyPEM_ValidKey(t *testing.T) {
	_, pemBytes := testRSAKeyPEM(t)
	if err := ValidatePrivateKeyPEM(pemBytes); err != nil {
		t.Errorf("ValidatePrivateKeyPEM() error = %v, want nil for a real key", err)
	}
}

func TestValidatePrivateKeyPEM_NotPEM(t *testing.T) {
	if err := ValidatePrivateKeyPEM([]byte("not a pem block")); err == nil {
		t.Error("ValidatePrivateKeyPEM() error = nil, want an error for non-PEM input")
	}
}

func TestValidatePrivateKeyPEM_NonRSAKey(t *testing.T) {
	block := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("not a key at all")})
	if err := ValidatePrivateKeyPEM(block); err == nil {
		t.Error("ValidatePrivateKeyPEM() error = nil, want an error for a non-key PEM block")
	}
}

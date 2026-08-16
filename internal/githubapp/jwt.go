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
	"fmt"
	"strconv"
	"time"
)

// jwtClockSkewBuffer backdates iat by this much, per GitHub's own
// documented recommendation for generating a JWT for a GitHub App: "to
// protect against clock drift between your machine and GitHub's, set
// this 60 seconds in the past". Not configurable: this is a spec
// requirement, not a tuning knob.
const jwtClockSkewBuffer = 60 * time.Second

// jwtValidityWindow is exp minus the *unadjusted* now, chosen well
// under GitHub's documented hard maximum of 10 minutes between iat and
// exp. With iat backdated by jwtClockSkewBuffer, the actual iat-to-exp
// span this produces is jwtClockSkewBuffer+jwtValidityWindow = 60s+8m =
// 540s (9 minutes), comfortably under the 600s ceiling with a minute of
// margin, rather than cutting it to the exact limit.
const jwtValidityWindow = 8 * time.Minute

// jwtHeader is the fixed RS256 JWT header GitHub's App auth spec
// requires.
type jwtHeader struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
}

// jwtClaims is the three claims a GitHub App JWT carries. No other
// claims: GitHub's spec defines exactly these three.
type jwtClaims struct {
	IAT int64  `json:"iat"`
	EXP int64  `json:"exp"`
	ISS string `json:"iss"`
}

// SignAppJWT builds and RS256-signs a GitHub App JWT for appID, using
// privateKeyPEM (the App's own PEM-encoded RSA private key, as returned
// by the manifest code-exchange and decrypted from internal/secrets
// immediately before this call). now is the caller's current time,
// passed in rather than read via time.Now() here so tests can assert on
// exact claim values without a clock race.
//
// iss is the numeric App ID formatted as a decimal string: GitHub's
// docs accept either the App ID or the client ID here, this always uses
// the App ID specifically, matching what internal/api's callers already
// have on hand (store.GitHubAppConnection.AppID) without an extra
// lookup.
//
// The returned string is the compact JWS serialization
// (base64url(header).base64url(claims).base64url(signature), no
// padding), exactly what every "Authorization: Bearer <jwt>" call this
// package makes expects.
func SignAppJWT(appID int64, privateKeyPEM []byte, now time.Time) (string, error) {
	key, err := parseRSAPrivateKeyPEM(privateKeyPEM)
	if err != nil {
		return "", fmt.Errorf("githubapp: parse app private key: %w", err)
	}

	header, err := json.Marshal(jwtHeader{Alg: "RS256", Typ: "JWT"})
	if err != nil {
		return "", fmt.Errorf("githubapp: marshal jwt header: %w", err)
	}
	claims, err := json.Marshal(jwtClaims{
		IAT: now.Add(-jwtClockSkewBuffer).Unix(),
		EXP: now.Add(jwtValidityWindow).Unix(),
		ISS: strconv.FormatInt(appID, 10),
	})
	if err != nil {
		return "", fmt.Errorf("githubapp: marshal jwt claims: %w", err)
	}

	signingInput := base64URLEncode(header) + "." + base64URLEncode(claims)

	digest := sha256.Sum256([]byte(signingInput))
	// SignPKCS1v15 (a signature scheme) is RS256 itself, GitHub's own
	// required algorithm, not the encryption padding scheme of the
	// same family with known padding-oracle weaknesses
	// (EncryptPKCS1v15/DecryptPKCS1v15, which this never calls).
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		return "", fmt.Errorf("githubapp: sign jwt: %w", err)
	}

	return signingInput + "." + base64URLEncode(signature), nil
}

// parseRSAPrivateKeyPEM decodes a PEM block and parses it as an RSA
// private key, trying PKCS#1 first (the format GitHub's own
// app-manifest conversion API documents its "pem" response field as:
// "-----BEGIN RSA PRIVATE KEY-----") and falling back to PKCS#8 for
// robustness against a key that was re-encoded or re-issued in the
// other common format. Never logs any part of der or the parsed key.
func parseRSAPrivateKeyPEM(pemBytes []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, fmt.Errorf("githubapp: no PEM block found in private key")
	}

	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}

	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("githubapp: parse private key (tried PKCS#1 and PKCS#8): %w", err)
	}
	rsaKey, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("githubapp: private key is not RSA")
	}
	return rsaKey, nil
}

// base64URLEncode is the JWS-compact-serialization encoding every
// segment of a JWT uses: base64url, no padding.
func base64URLEncode(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

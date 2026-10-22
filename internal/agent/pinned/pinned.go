// Package pinned holds the small trust primitives shared by every reverse-dialed
// client: CA fingerprint pinning and key plus CSR generation. It imports no
// Docker, exec or build code so a minimal client can link it alone.
package pinned

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
)

// ErrMismatch means the control plane's CA does not match the pinned fingerprint.
var ErrMismatch = errors.New("agent: control plane CA does not match the pinned fingerprint")

// Fingerprint returns the hex SHA-256 of a DER-encoded certificate.
func Fingerprint(der []byte) string {
	sum := sha256.Sum256(der)
	return hex.EncodeToString(sum[:])
}

// Normalize lowercases a fingerprint and drops colons and spaces.
func Normalize(fp string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(fp), ":", ""))
}

// FingerprintPEM returns the fingerprint of the first certificate in certPEM.
func FingerprintPEM(certPEM []byte) (string, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil || block.Type != "CERTIFICATE" {
		return "", errors.New("agent: no certificate in PEM")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return "", fmt.Errorf("agent: parse certificate: %w", err)
	}
	return Fingerprint(cert.Raw), nil
}

// CheckCA compares a CA PEM against a pinned fingerprint in constant time.
func CheckCA(caPEM []byte, fingerprint string) error {
	if fingerprint == "" {
		return nil
	}
	got, err := FingerprintPEM(caPEM)
	if err != nil {
		return err
	}
	if subtle.ConstantTimeCompare([]byte(got), []byte(fingerprint)) != 1 {
		return fmt.Errorf("%w: response carries CA %s", ErrMismatch, got)
	}
	return nil
}

// EnrollTLSConfig builds the enrollment TLS config. Go's own verification is
// skipped because no root exists yet; with a pin, VerifyChain replaces it.
func EnrollTLSConfig(caFingerprint string) *tls.Config {
	cfg := &tls.Config{InsecureSkipVerify: true, NextProtos: []string{"h2"}} //nolint:gosec // replaced by VerifyChain when a CA fingerprint is pinned
	if caFingerprint != "" {
		cfg.VerifyConnection = func(cs tls.ConnectionState) error {
			return VerifyChain(cs.PeerCertificates, caFingerprint)
		}
	}
	return cfg
}

// SessionTLSConfig builds the long-lived session TLS config from the CA the
// agent received at enrollment and its own client certificate.
func SessionTLSConfig(caPEM, certPEM, keyPEM []byte) (*tls.Config, error) {
	pair, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("agent: load client key pair: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, errors.New("agent: load CA certificate")
	}
	return &tls.Config{RootCAs: pool, Certificates: []tls.Certificate{pair}, NextProtos: []string{"h2"}, MinVersion: tls.VersionTLS13}, nil
}

// VerifyChain accepts the connection only if the chain includes the pinned CA
// and the leaf is a valid server certificate signed by it.
func VerifyChain(chain []*x509.Certificate, caFingerprint string) error {
	if len(chain) == 0 {
		return fmt.Errorf("%w: server presented no certificate", ErrMismatch)
	}
	roots := x509.NewCertPool()
	found := false
	for _, c := range chain[1:] {
		if subtle.ConstantTimeCompare([]byte(Fingerprint(c.Raw)), []byte(caFingerprint)) == 1 {
			roots.AddCert(c)
			found = true
		}
	}
	if !found {
		return fmt.Errorf("%w: server did not present the pinned CA", ErrMismatch)
	}
	if _, err := chain[0].Verify(x509.VerifyOptions{Roots: roots, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}); err != nil {
		return fmt.Errorf("%w: %w", ErrMismatch, err)
	}
	return nil
}

// NewKeyAndCSR generates an ed25519 key and a CSR for it. The key stays with the caller.
func NewKeyAndCSR(commonName string) (keyPEM, csrDER []byte, err error) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("agent: generate key: %w", err)
	}
	csrDER, err = x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		Subject: pkix.Name{CommonName: commonName},
	}, priv)
	if err != nil {
		return nil, nil, fmt.Errorf("agent: create CSR: %w", err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return nil, nil, fmt.Errorf("agent: marshal key: %w", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), csrDER, nil
}

package database

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"time"
)

// TLSMaterial is a self-signed certificate and private key pair for one
// managed database's server-side TLS, generated once at database
// creation time (cmd/levelrail's tlsMaterialFor) and persisted through
// internal/secrets the same way PostgresCredentials' password already is
// (TLSCertEnvKey/TLSKeyEnvKey, address.go). Self-signed, not issued by a
// shared platform CA: the only clients that ever use it
// (internal/reconcile/application's resolveDatabaseURL, via
// sslmode=require for Postgres or rediss:// for Redis) encrypt without
// verifying the certificate's issuer, so there is no second party that
// ever needs to trust it.
type TLSMaterial struct {
	CertPEM []byte
	KeyPEM  []byte
}

// tlsCertValidity is deliberately long (10 years), the same reasoning
// agent/pki.go's caValidity gives for its own CA: this certificate is
// never distributed to a party that verifies its issuer, so there is no
// external trust lifecycle bounding it. Rotation is an explicit operator
// action, not something a short expiry should silently force.
const tlsCertValidity = 10 * 365 * 24 * time.Hour

// GenerateSelfSignedCert creates a new self-signed ed25519 certificate
// for commonName (a database's own container name, ContainerName), for
// cmd/levelrail's tlsMaterialFor to generate once and persist. ed25519
// for the same reasons agent/pki.go's GenerateCA already gives: fast key
// generation, small keys, no RSA parameter-size decision to make.
func GenerateSelfSignedCert(commonName string) (certPEM, keyPEM []byte, err error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("database: generate tls key: %w", err)
	}

	serial, err := randTLSSerial()
	if err != nil {
		return nil, nil, err
	}

	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: commonName},
		DNSNames:     []string{commonName},
		NotBefore:    time.Now().Add(-time.Hour), // clock-skew tolerance
		NotAfter:     time.Now().Add(tlsCertValidity),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, pub, priv)
	if err != nil {
		return nil, nil, fmt.Errorf("database: create tls certificate: %w", err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return nil, nil, fmt.Errorf("database: marshal tls key: %w", err)
	}

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM, nil
}

func randTLSSerial() (*big.Int, error) {
	limit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, limit)
	if err != nil {
		return nil, fmt.Errorf("database: generate tls certificate serial: %w", err)
	}
	return serial, nil
}

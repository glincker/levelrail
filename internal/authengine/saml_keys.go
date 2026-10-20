package authengine

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/GLINCKER/levelrail/internal/secrets"
)

const (
	samlKeyService  = "sso/saml-sp"
	samlKeyName     = "private-key"
	samlCertName    = "certificate"
	samlKeyBits     = 2048
	samlCertTTL     = 10 * 365 * 24 * time.Hour
	samlCertSubject = "SAML service provider"
)

// SAMLKeyPair is the service provider's signing keypair in PEM form.
type SAMLKeyPair struct {
	CertPEM []byte
	KeyPEM  []byte
}

// LoadOrCreateSAMLKeyPair returns the SP keypair kept in the secrets store,
// generating it on first use. Both halves are stored encrypted.
func LoadOrCreateSAMLKeyPair(ctx context.Context, ks KeyStore) (SAMLKeyPair, error) {
	key, kerr := ks.Resolve(ctx, samlKeyService, samlKeyName)
	cert, cerr := ks.Resolve(ctx, samlKeyService, samlCertName)
	if kerr == nil && cerr == nil {
		kp := SAMLKeyPair{CertPEM: []byte(cert), KeyPEM: []byte(key)}
		if err := checkSAMLKeyPair(kp); err != nil {
			return SAMLKeyPair{}, err
		}
		return kp, nil
	}
	for _, err := range []error{kerr, cerr} {
		if err != nil && !errors.Is(err, secrets.ErrValueNotFound) {
			return SAMLKeyPair{}, fmt.Errorf("authengine: load saml keypair: %w", err)
		}
	}
	kp, err := GenerateSAMLKeyPair(time.Now())
	if err != nil {
		return SAMLKeyPair{}, err
	}
	// The key goes first: a cert without its key is unusable, a key alone is regenerated over.
	if err := ks.SetValue(ctx, samlKeyService, samlKeyName, string(kp.KeyPEM)); err != nil {
		return SAMLKeyPair{}, fmt.Errorf("authengine: store saml key: %w", err)
	}
	if err := ks.SetValue(ctx, samlKeyService, samlCertName, string(kp.CertPEM)); err != nil {
		return SAMLKeyPair{}, fmt.Errorf("authengine: store saml certificate: %w", err)
	}
	return kp, nil
}

// GenerateSAMLKeyPair creates an RSA key and a self-signed certificate for it.
func GenerateSAMLKeyPair(now time.Time) (SAMLKeyPair, error) {
	key, err := rsa.GenerateKey(rand.Reader, samlKeyBits)
	if err != nil {
		return SAMLKeyPair{}, fmt.Errorf("authengine: generate saml key: %w", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
	if err != nil {
		return SAMLKeyPair{}, fmt.Errorf("authengine: generate serial: %w", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: samlCertSubject},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(samlCertTTL),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return SAMLKeyPair{}, fmt.Errorf("authengine: self-sign saml certificate: %w", err)
	}
	return SAMLKeyPair{
		CertPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		KeyPEM:  pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}),
	}, nil
}

func checkSAMLKeyPair(kp SAMLKeyPair) error {
	cb, _ := pem.Decode(kp.CertPEM)
	kb, _ := pem.Decode(kp.KeyPEM)
	if cb == nil || kb == nil {
		return errors.New("authengine: stored saml keypair is not PEM")
	}
	cert, err := x509.ParseCertificate(cb.Bytes)
	if err != nil {
		return fmt.Errorf("authengine: parse saml certificate: %w", err)
	}
	key, err := x509.ParsePKCS1PrivateKey(kb.Bytes)
	if err != nil {
		return fmt.Errorf("authengine: parse saml key: %w", err)
	}
	pub, ok := cert.PublicKey.(*rsa.PublicKey)
	if !ok || pub.N.Cmp(key.N) != 0 || pub.E != key.E {
		return errors.New("authengine: saml certificate does not match its key")
	}
	return nil
}

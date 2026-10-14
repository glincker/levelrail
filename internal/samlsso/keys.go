// Package samlsso is the SAML 2.0 service provider: SP keys and metadata,
// IDP metadata import, AuthnRequest creation, and strict response validation.
// It holds no state; storage, replay tracking and sessions belong to callers.
package samlsso

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strings"
	"time"
)

const (
	spKeyBits      = 2048
	spCertValidity = 10 * 365 * 24 * time.Hour
)

// GenerateSPKeyPair creates the RSA key and self-signed certificate the SP
// signs AuthnRequests with and IDPs may encrypt assertions to.
func GenerateSPKeyPair(commonName string, now time.Time) (certPEM, keyPEM string, err error) {
	key, err := rsa.GenerateKey(rand.Reader, spKeyBits)
	if err != nil {
		return "", "", fmt.Errorf("samlsso: generate sp key: %w", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
	if err != nil {
		return "", "", fmt.Errorf("samlsso: generate serial: %w", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: commonName},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(spCertValidity),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return "", "", fmt.Errorf("samlsso: self-sign sp certificate: %w", err)
	}
	certPEM = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	keyPEM = string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
	return certPEM, keyPEM, nil
}

// ParseSPKeyPair decodes a stored SP keypair and checks the halves match.
func ParseSPKeyPair(certPEM, keyPEM string) (*x509.Certificate, *rsa.PrivateKey, error) {
	certs, err := ParseCertificates(certPEM)
	if err != nil || len(certs) != 1 {
		return nil, nil, errors.New("samlsso: sp certificate is not a single PEM certificate")
	}
	block, _ := pem.Decode([]byte(keyPEM))
	if block == nil {
		return nil, nil, errors.New("samlsso: sp private key is not PEM")
	}
	key, err := parseRSAKey(block)
	if err != nil {
		return nil, nil, err
	}
	pub, ok := certs[0].PublicKey.(*rsa.PublicKey)
	if !ok || pub.N.Cmp(key.N) != 0 || pub.E != key.E {
		return nil, nil, errors.New("samlsso: sp certificate does not match the private key")
	}
	return certs[0], key, nil
}

func parseRSAKey(block *pem.Block) (*rsa.PrivateKey, error) {
	if block.Type == "RSA PRIVATE KEY" {
		key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("samlsso: parse sp key: %w", err)
		}
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("samlsso: parse sp key: %w", err)
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("samlsso: sp key must be RSA")
	}
	return key, nil
}

var whitespace = regexp.MustCompile(`\s+`)

// ParseCertificates accepts PEM blocks or bare base64 DER, the two shapes IDP
// consoles hand out, and returns every certificate found. Empty input is an error.
func ParseCertificates(raw string) ([]*x509.Certificate, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.New("samlsso: no certificate given")
	}
	if !strings.Contains(raw, "-----BEGIN") {
		der, err := base64.StdEncoding.DecodeString(whitespace.ReplaceAllString(raw, ""))
		if err != nil {
			return nil, errors.New("samlsso: certificate is neither PEM nor base64")
		}
		cert, err := x509.ParseCertificate(der)
		if err != nil {
			return nil, fmt.Errorf("samlsso: parse certificate: %w", err)
		}
		return []*x509.Certificate{cert}, nil
	}
	var out []*x509.Certificate
	rest := []byte(raw)
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			return nil, fmt.Errorf("samlsso: unexpected PEM block %q", block.Type)
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("samlsso: parse certificate: %w", err)
		}
		out = append(out, cert)
	}
	if len(out) == 0 {
		return nil, errors.New("samlsso: no PEM certificate found")
	}
	return out, nil
}

// EncodeCertificates renders certificates as concatenated PEM.
func EncodeCertificates(certs []*x509.Certificate) string {
	var b strings.Builder
	for _, c := range certs {
		b.Write(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: c.Raw}))
	}
	return b.String()
}

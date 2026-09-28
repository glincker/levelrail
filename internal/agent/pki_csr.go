package agent

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"time"
)

// ErrInvalidCSR is returned by SignClientCSR for a malformed request, a bad
// signature, or a key type this CA does not issue for.
var ErrInvalidCSR = errors.New("agent: invalid certificate signing request")

// NewKeyAndCSR generates a fresh ed25519 private key on this machine and a
// CSR for it. The key never leaves the caller; only the CSR is sent.
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

// IssuedCert is a signed client certificate plus the fields the control
// plane records about it.
type IssuedCert struct {
	PEM         []byte
	Fingerprint string
	Serial      string
	NotAfter    time.Time
}

// SignClientCSR issues a client certificate for commonName over the CSR's
// public key. The CSR's own subject, SANs and extensions are ignored: the
// control plane alone decides what the certificate asserts.
func (ca *CA) SignClientCSR(commonName string, csrDER []byte, validFor time.Duration, now time.Time) (IssuedCert, error) {
	csr, err := x509.ParseCertificateRequest(csrDER)
	if err != nil {
		return IssuedCert{}, fmt.Errorf("%w: %w", ErrInvalidCSR, err)
	}
	if err := csr.CheckSignature(); err != nil {
		return IssuedCert{}, fmt.Errorf("%w: signature: %w", ErrInvalidCSR, err)
	}
	if err := checkCSRKey(csr.PublicKey); err != nil {
		return IssuedCert{}, err
	}
	serial, err := randomSerial()
	if err != nil {
		return IssuedCert{}, err
	}
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: commonName},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(validFor),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca.cert, csr.PublicKey, ca.key)
	if err != nil {
		return IssuedCert{}, fmt.Errorf("agent: sign client certificate for %q: %w", commonName, err)
	}
	return issuedFromDER(der)
}

func checkCSRKey(pub crypto.PublicKey) error {
	switch k := pub.(type) {
	case ed25519.PublicKey:
		return nil
	case *ecdsa.PublicKey:
		if k.Curve == elliptic.P256() || k.Curve == elliptic.P384() {
			return nil
		}
		return fmt.Errorf("%w: unsupported ECDSA curve %s", ErrInvalidCSR, k.Curve.Params().Name)
	default:
		return fmt.Errorf("%w: unsupported key type %T", ErrInvalidCSR, pub)
	}
}

func issuedFromDER(der []byte) (IssuedCert, error) {
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return IssuedCert{}, fmt.Errorf("agent: parse issued certificate: %w", err)
	}
	return IssuedCert{
		PEM:         pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		Fingerprint: CertFingerprint(der),
		Serial:      certSerial(cert),
		NotAfter:    cert.NotAfter,
	}, nil
}

func certSerial(cert *x509.Certificate) string {
	return hex.EncodeToString(cert.SerialNumber.Bytes())
}

func parseCertPEM(certPEM []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return nil, fmt.Errorf("agent: no PEM block found in certificate data")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("agent: parse certificate: %w", err)
	}
	return cert, nil
}

package agent

// PKI for ADR 003's mTLS: a private trust root for one relationship, control plane and agents.
// Stdlib crypto/x509 only, no external dependencies.

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"time"

	"github.com/GLINCKER/levelrail/internal/agent/pinned"
)

// CA is the control plane's self-signed root, signing its server cert and every agent's client cert.
type CA struct {
	cert    *x509.Certificate
	certDER []byte
	key     ed25519.PrivateKey
}

// caValidity is 10 years: the CA key never leaves this data directory, so no third party needs a short expiry.
// Rotation is a deliberate operator action, not a schedule.
const caValidity = 10 * 365 * 24 * time.Hour

// GenerateCA creates a new self-signed CA using ed25519 keys.
func GenerateCA() (*CA, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("agent: generate CA key: %w", err)
	}

	serial, err := randomSerial()
	if err != nil {
		return nil, err
	}

	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "levelrail-agent-ca"},
		NotBefore:             time.Now().Add(-time.Hour), // clock-skew tolerance
		NotAfter:              time.Now().Add(caValidity),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, pub, priv)
	if err != nil {
		return nil, fmt.Errorf("agent: create CA certificate: %w", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, fmt.Errorf("agent: parse newly created CA certificate: %w", err)
	}

	return &CA{cert: cert, certDER: der, key: priv}, nil
}

// LoadCA restores a CA persisted via CertPEM/KeyPEM; regenerating it would invalidate every enrolled agent.
func LoadCA(certPEM, keyPEM []byte) (*CA, error) {
	certBlock, _ := pem.Decode(certPEM)
	if certBlock == nil {
		return nil, fmt.Errorf("agent: load CA: no PEM block found in certificate data")
	}
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("agent: load CA: parse certificate: %w", err)
	}

	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		return nil, fmt.Errorf("agent: load CA: no PEM block found in key data")
	}
	parsedKey, err := x509.ParsePKCS8PrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("agent: load CA: parse private key: %w", err)
	}
	key, ok := parsedKey.(ed25519.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("agent: load CA: private key is %T, want ed25519.PrivateKey", parsedKey)
	}

	return &CA{cert: cert, certDER: certBlock.Bytes, key: key}, nil
}

// CertPEM returns the CA certificate, PEM-encoded, for agents to verify the control plane and for persistence.
func (ca *CA) CertPEM() []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.certDER})
}

// Fingerprint returns the hex SHA-256 of the CA certificate's DER, the
// value an agent pins at enrollment (APP_CA_FINGERPRINT).
func (ca *CA) Fingerprint() string {
	return CertFingerprint(ca.certDER)
}

// KeyPEM returns the CA's own private key, PEM-encoded, PKCS#8: the
// other half LoadCA needs. Never sent over the wire to an agent, only
// ever persisted locally by the control plane's own startup code.
func (ca *CA) KeyPEM() []byte {
	der, err := x509.MarshalPKCS8PrivateKey(ca.key)
	if err != nil {
		panic(fmt.Sprintf("agent: marshal CA private key: %v", err))
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
}

// IssueServerCert issues the control plane's gRPC listener certificate, signed by ca.
func (ca *CA) IssueServerCert(hosts []string, validFor time.Duration) (certPEM, keyPEM []byte, err error) {
	if len(hosts) == 0 {
		return nil, nil, fmt.Errorf("agent: issue server cert: at least one host is required")
	}
	return ca.issueServerLeaf(hosts, validFor)
}

// issueServerLeaf is only for the control plane's own listener key; agent
// keys are never generated here.
func (ca *CA) issueServerLeaf(hosts []string, validFor time.Duration) (certPEM, keyPEM []byte, err error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("agent: generate leaf key: %w", err)
	}

	serial, err := randomSerial()
	if err != nil {
		return nil, nil, err
	}

	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: hosts[0]},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(validFor),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			tmpl.IPAddresses = append(tmpl.IPAddresses, ip)
		} else {
			tmpl.DNSNames = append(tmpl.DNSNames, h)
		}
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca.cert, pub, ca.key)
	if err != nil {
		return nil, nil, fmt.Errorf("agent: create leaf certificate for %q: %w", hosts[0], err)
	}

	keyDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return nil, nil, fmt.Errorf("agent: marshal leaf private key for %q: %w", hosts[0], err)
	}

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM, nil
}

// CertFingerprint returns the hex SHA-256 of a DER-encoded certificate.
// Sessions compare against this rather than trusting CommonName, which another certificate could also claim.
func CertFingerprint(der []byte) string {
	return pinned.Fingerprint(der)
}

func randomSerial() (*big.Int, error) {
	limit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, limit)
	if err != nil {
		return nil, fmt.Errorf("agent: generate certificate serial: %w", err)
	}
	return serial, nil
}

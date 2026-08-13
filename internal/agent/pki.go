package agent

// This file: PKI for ADR 003's mTLS relationship between the control
// plane and its agents. Deliberately not a general-purpose CA library:
// this is a private trust root for exactly one relationship (the
// control plane issues every certificate that will ever be trusted
// here, both its own server certificate and every agent's client
// certificate), the same "minimal crypto primitives over a full
// framework" precedent internal/secrets already set (age instead of a
// general PKI toolchain there; stdlib crypto/x509 here, still zero
// external dependencies, since standing up a small CA is squarely what
// net/x509 already documents supporting directly).

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"time"
)

// CA is the control plane's own certificate authority: one self-signed
// root, generated once (or loaded from disk across restarts) and used
// to sign both the control plane's own gRPC server certificate and
// every enrolling agent's client certificate.
type CA struct {
	cert    *x509.Certificate
	certDER []byte
	key     ed25519.PrivateKey
}

// caValidity is deliberately long (10 years): this CA's private key is
// held only in this process's own data directory, never distributed,
// so there is no third-party trust to time-bound the way a publicly
// trusted CA would be. Rotating it is an explicit operational action
// (regenerate, re-enroll every node), not something a short expiry
// should force on a schedule nobody asked for.
const caValidity = 10 * 365 * 24 * time.Hour

// GenerateCA creates a new self-signed CA. ed25519 throughout this
// package (CA and every leaf certificate): fast key generation, small
// keys, and full support in Go's crypto/tls for both signing and TLS
// handshakes, with none of RSA's parameter-size decisions to make.
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

// LoadCA parses a CA previously persisted via CertPEM/KeyPEM, the
// counterpart GenerateCA's caller needs across a control plane restart:
// generating a fresh CA on every startup would invalidate every
// already-enrolled agent's certificate for no reason.
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

// CertPEM returns the CA's own certificate, PEM-encoded: what every
// agent needs to verify the control plane's server certificate
// (returned to an enrolling agent as EnrollResponse.CaCertPem), and what
// this process needs to persist to disk to survive a restart via
// LoadCA.
func (ca *CA) CertPEM() []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.certDER})
}

// KeyPEM returns the CA's own private key, PEM-encoded, PKCS#8: the
// other half LoadCA needs. Never sent over the wire to an agent, only
// ever persisted locally by the control plane's own startup code.
func (ca *CA) KeyPEM() []byte {
	der, err := x509.MarshalPKCS8PrivateKey(ca.key)
	if err != nil {
		// ed25519.PrivateKey always marshals successfully; this branch
		// exists only so KeyPEM's signature doesn't need an error return
		// for a case that cannot actually occur, matching how this
		// codebase avoids threading an error through calls that can't
		// fail rather than inventing one that could.
		panic(fmt.Sprintf("agent: marshal CA private key: %v", err))
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
}

// IssueClientCert issues a client certificate identifying commonName
// (a node's store.Node.ID), signed by ca, for TASKS.md 3.2's enrollment
// flow. validFor has no auto-renewal built in here: certificate
// rotation on a schedule is real, named Phase 3 scope (ADR 003's
// Consequences section), a caller-level concern layered on top of this
// primitive, not solved by this one function alone.
func (ca *CA) IssueClientCert(commonName string, validFor time.Duration) (certPEM, keyPEM []byte, err error) {
	return ca.issueLeaf(commonName, nil, x509.ExtKeyUsageClientAuth, validFor)
}

// IssueServerCert issues the control plane's own gRPC listener TLS
// certificate, signed by ca, valid for the given hostnames/IPs agents
// will dial. Not agent-facing in the enrollment response the way
// IssueClientCert's output is: the control plane loads this cert
// locally for its own gRPC server, per cmd/levelrail's own startup
// wiring, not built here.
func (ca *CA) IssueServerCert(hosts []string, validFor time.Duration) (certPEM, keyPEM []byte, err error) {
	if len(hosts) == 0 {
		return nil, nil, fmt.Errorf("agent: issue server cert: at least one host is required")
	}
	return ca.issueLeaf(hosts[0], hosts, x509.ExtKeyUsageServerAuth, validFor)
}

func (ca *CA) issueLeaf(commonName string, hosts []string, usage x509.ExtKeyUsage, validFor time.Duration) (certPEM, keyPEM []byte, err error) {
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
		Subject:      pkix.Name{CommonName: commonName},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(validFor),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{usage},
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
		return nil, nil, fmt.Errorf("agent: create leaf certificate for %q: %w", commonName, err)
	}

	keyDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return nil, nil, fmt.Errorf("agent: marshal leaf private key for %q: %w", commonName, err)
	}

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM, nil
}

// CertFingerprint returns a stable, hex-encoded SHA-256 digest of a
// DER-encoded certificate: store.Node.CertFingerprint's own value, and
// what the control plane's Session handler (TASKS.md 3.2) recomputes
// from an incoming mTLS connection's actual peer certificate to confirm
// it matches the fingerprint recorded at enrollment, rather than
// trusting the certificate's CommonName field alone (which a
// differently-issued certificate could also claim).
func CertFingerprint(der []byte) string {
	sum := sha256.Sum256(der)
	return hex.EncodeToString(sum[:])
}

func randomSerial() (*big.Int, error) {
	limit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, limit)
	if err != nil {
		return nil, fmt.Errorf("agent: generate certificate serial: %w", err)
	}
	return serial, nil
}

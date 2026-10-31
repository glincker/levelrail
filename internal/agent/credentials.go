package agent

// Builds gRPC TransportCredentials for the control plane's listener, so a second listener can reuse them.

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"time"

	"google.golang.org/grpc/credentials"

	"github.com/GLINCKER/levelrail/internal/store"
)

// NewServerCredentials returns mTLS credentials that verify a client certificate when presented but do not require one.
// Must go to grpc.Creds, not a raw tls.Listener: grpc only sees the peer TLS state when it performs the handshake.
func NewServerCredentials(ca *CA, hosts []string, validFor time.Duration, opts ...CredentialOption) (credentials.TransportCredentials, error) {
	var co credentialConfig
	for _, opt := range opts {
		opt(&co)
	}
	certPEM, keyPEM, err := ca.IssueServerCert(hosts, validFor)
	if err != nil {
		return nil, fmt.Errorf("agent: issue server certificate: %w", err)
	}
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("agent: parse server certificate: %w", err)
	}
	// Sending the CA too lets an enrolling agent check it against a
	// pinned fingerprint before it has the CA on disk.
	cert.Certificate = append(cert.Certificate, ca.certDER)
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(ca.CertPEM()) {
		return nil, fmt.Errorf("agent: load CA certificate into client cert pool")
	}
	base := &tls.Config{
		Certificates: []tls.Certificate{cert},
		// Every agent dial site is Go and negotiates 1.3 already; this stops a downgrade.
		MinVersion: tls.VersionTLS13,
		// Client certs are requested, not required, and verified in verifyClientChain so a rejection can name the node.
		ClientAuth: tls.RequestClientCert,
		// grpc-go >= 1.67 enforces ALPN negotiation; without this, every
		// real connection fails with an opaque "missing selected ALPN
		// property" error rather than a TLS-config-shaped one.
		NextProtos: []string{"h2"},
		Time:       co.now,
	}
	base.GetConfigForClient = func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
		cfg := base.Clone()
		cfg.GetConfigForClient = nil
		ip := remoteHost(hello.Conn)
		cfg.VerifyConnection = func(cs tls.ConnectionState) error {
			if len(cs.PeerCertificates) == 0 {
				return nil
			}
			now := time.Now()
			if co.now != nil {
				now = co.now()
			}
			err := verifyClientChain(cs.PeerCertificates, pool, now)
			if err != nil && co.onReject != nil {
				leaf := cs.PeerCertificates[0]
				co.onReject(RejectedHandshake{Subject: leaf.Subject.CommonName, SourceIP: ip, Reason: rejectReason(err, now), Fingerprint: CertFingerprint(leaf.Raw)})
			}
			return err
		}
		return cfg, nil
	}
	return credentials.NewTLS(base), nil
}

// CredentialOption configures NewServerCredentials.
type CredentialOption func(*credentialConfig)

type credentialConfig struct {
	onReject func(RejectedHandshake)
	now      func() time.Time
}

// RejectedHandshake describes a client certificate refused during the TLS
// handshake. Subject is what the certificate claims, not a verified identity.
type RejectedHandshake struct {
	Subject, SourceIP, Reason, Fingerprint string
}

// WithRejectObserver calls f for every client certificate the handshake
// refuses. f runs on the handshake path and must not block.
func WithRejectObserver(f func(RejectedHandshake)) CredentialOption {
	return func(c *credentialConfig) { c.onReject = f }
}

// WithCredentialClock overrides the time source for certificate validity.
func WithCredentialClock(now func() time.Time) CredentialOption {
	return func(c *credentialConfig) { c.now = now }
}

// verifyClientChain accepts chain only if its leaf is a client
// certificate that chains to pool and is valid at now.
func verifyClientChain(chain []*x509.Certificate, pool *x509.CertPool, now time.Time) error {
	inter := x509.NewCertPool()
	for _, c := range chain[1:] {
		inter.AddCert(c)
	}
	_, err := chain[0].Verify(x509.VerifyOptions{Roots: pool, Intermediates: inter, CurrentTime: now, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}})
	if err != nil {
		return fmt.Errorf("agent: client certificate refused: %w", err)
	}
	return nil
}

func rejectReason(err error, now time.Time) string {
	var invalid x509.CertificateInvalidError
	var unknown x509.UnknownAuthorityError
	switch {
	case errors.As(err, &unknown):
		return store.SecurityReasonUnknownCA
	case errors.As(err, &invalid) && invalid.Reason == x509.Expired:
		// x509 uses one reason for both ends of the validity window.
		if invalid.Cert != nil && invalid.Cert.NotBefore.After(now) {
			return store.SecurityReasonNotYetValid
		}
		return store.SecurityReasonExpired
	default:
		return store.SecurityReasonInvalidCert
	}
}

func remoteHost(c net.Conn) string {
	if c == nil {
		return ""
	}
	host, _, err := net.SplitHostPort(c.RemoteAddr().String())
	if err != nil {
		return ""
	}
	return host
}

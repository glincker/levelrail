package agent

// This file: building grpc TransportCredentials for the control plane's
// own gRPC listener (TASKS.md 3.2). Factored out of cmd/levelrail/
// main.go so it's directly testable and reusable wherever a second
// listener needs the identical shape (this package's own live test, in
// particular).

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"time"

	"google.golang.org/grpc/credentials"
)

// NewServerCredentials issues a fresh server certificate from ca
// (covering hosts) and returns grpc TransportCredentials configured for
// TASKS.md 3.2's mTLS model: a client certificate is verified when
// presented but not required (VerifyClientCertIfGiven), because Enroll
// (ADR 003's join-token exchange) is called with no client certificate
// at all, by design (DialEnroll's own doc comment explains why), while
// Session always presents one, verified against ca when it does.
//
// Must be passed to grpc.NewServer via grpc.Creds, not used to build a
// raw tls.Listener handed to Serve directly: grpc-go's own peer-info
// extraction (what Server.Session's peerIdentity relies on to read the
// client certificate back out of a request's context) only populates
// correctly when grpc's credentials layer performs the handshake
// itself. A raw tls.Listener does the TLS handshake transparently at
// the net.Conn level, which works for plain data transfer but leaves
// grpc with no TLS state to attach to the connection's context,
// confirmed the hard way: an earlier version of this wiring, tested
// against a real listener, failed with "no peer TLS info on this
// connection" on every real mTLS connection until this file switched
// to grpc.Creds.
func NewServerCredentials(ca *CA, hosts []string, validFor time.Duration) (credentials.TransportCredentials, error) {
	certPEM, keyPEM, err := ca.IssueServerCert(hosts, validFor)
	if err != nil {
		return nil, fmt.Errorf("agent: issue server certificate: %w", err)
	}
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("agent: parse server certificate: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(ca.CertPEM()) {
		return nil, fmt.Errorf("agent: load CA certificate into client cert pool")
	}
	return credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientCAs:    pool,
		ClientAuth:   tls.VerifyClientCertIfGiven,
		// grpc-go >= 1.67 enforces ALPN negotiation; without this, every
		// real connection fails with an opaque "missing selected ALPN
		// property" error rather than a TLS-config-shaped one.
		NextProtos: []string{"h2"},
	}), nil
}

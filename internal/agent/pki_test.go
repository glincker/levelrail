package agent

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"io"
	"testing"
	"time"
)

func TestGenerateCA_ProducesUsableCertAndKey(t *testing.T) {
	ca, err := GenerateCA()
	if err != nil {
		t.Fatalf("GenerateCA() error = %v", err)
	}
	if !ca.cert.IsCA {
		t.Error("generated certificate is not marked IsCA")
	}
	block, _ := pem.Decode(ca.CertPEM())
	if block == nil || block.Type != "CERTIFICATE" {
		t.Fatal("CertPEM() did not produce a valid CERTIFICATE PEM block")
	}
}

func TestLoadCA_RoundTrips(t *testing.T) {
	original, err := GenerateCA()
	if err != nil {
		t.Fatalf("GenerateCA() error = %v", err)
	}

	loaded, err := LoadCA(original.CertPEM(), original.KeyPEM())
	if err != nil {
		t.Fatalf("LoadCA() error = %v", err)
	}
	if loaded.cert.SerialNumber.Cmp(original.cert.SerialNumber) != 0 {
		t.Error("loaded CA has a different serial number than the original")
	}

	// The loaded CA's key must actually work for signing, not just parse:
	// issue a leaf cert with it and verify against the original's own
	// certificate.
	certPEM, _, err := loaded.IssueClientCert("node-1", time.Hour)
	if err != nil {
		t.Fatalf("IssueClientCert() with loaded CA error = %v", err)
	}
	verifyChain(t, original.CertPEM(), certPEM)
}

func TestLoadCA_InvalidPEM(t *testing.T) {
	ca, err := GenerateCA()
	if err != nil {
		t.Fatalf("GenerateCA() error = %v", err)
	}
	if _, err := LoadCA([]byte("not pem"), ca.KeyPEM()); err == nil {
		t.Error("LoadCA() with invalid cert PEM error = nil, want an error")
	}
	if _, err := LoadCA(ca.CertPEM(), []byte("not pem")); err == nil {
		t.Error("LoadCA() with invalid key PEM error = nil, want an error")
	}
}

func TestIssueClientCert_SignedByCA(t *testing.T) {
	ca, err := GenerateCA()
	if err != nil {
		t.Fatalf("GenerateCA() error = %v", err)
	}

	certPEM, keyPEM, err := ca.IssueClientCert("node-abc123", time.Hour)
	if err != nil {
		t.Fatalf("IssueClientCert() error = %v", err)
	}

	verifyChain(t, ca.CertPEM(), certPEM)

	// The returned cert and key must actually pair up into a usable TLS
	// certificate, the same check crypto/tls itself performs.
	if _, err := tls.X509KeyPair(certPEM, keyPEM); err != nil {
		t.Errorf("tls.X509KeyPair() error = %v, want a valid cert/key pair", err)
	}

	block, _ := pem.Decode(certPEM)
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse issued cert: %v", err)
	}
	if cert.Subject.CommonName != "node-abc123" {
		t.Errorf("cert CommonName = %q, want node-abc123", cert.Subject.CommonName)
	}
	if cert.IsCA {
		t.Error("issued client cert is itself marked IsCA, want false")
	}
	found := false
	for _, eku := range cert.ExtKeyUsage {
		if eku == x509.ExtKeyUsageClientAuth {
			found = true
		}
	}
	if !found {
		t.Error("issued client cert missing ExtKeyUsageClientAuth")
	}
}

func TestIssueServerCert_IncludesHosts(t *testing.T) {
	ca, err := GenerateCA()
	if err != nil {
		t.Fatalf("GenerateCA() error = %v", err)
	}

	certPEM, _, err := ca.IssueServerCert([]string{"levelrail.internal", "127.0.0.1"}, time.Hour)
	if err != nil {
		t.Fatalf("IssueServerCert() error = %v", err)
	}
	verifyChain(t, ca.CertPEM(), certPEM)

	block, _ := pem.Decode(certPEM)
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse issued cert: %v", err)
	}
	if len(cert.DNSNames) != 1 || cert.DNSNames[0] != "levelrail.internal" {
		t.Errorf("cert.DNSNames = %v, want [levelrail.internal]", cert.DNSNames)
	}
	if len(cert.IPAddresses) != 1 || cert.IPAddresses[0].String() != "127.0.0.1" {
		t.Errorf("cert.IPAddresses = %v, want [127.0.0.1]", cert.IPAddresses)
	}
}

func TestIssueServerCert_NoHosts_Errors(t *testing.T) {
	ca, err := GenerateCA()
	if err != nil {
		t.Fatalf("GenerateCA() error = %v", err)
	}
	if _, _, err := ca.IssueServerCert(nil, time.Hour); err == nil {
		t.Error("IssueServerCert(nil) error = nil, want an error")
	}
}

func TestCertFingerprint_DeterministicAndDistinct(t *testing.T) {
	ca, err := GenerateCA()
	if err != nil {
		t.Fatalf("GenerateCA() error = %v", err)
	}
	certA, _, err := ca.IssueClientCert("node-a", time.Hour)
	if err != nil {
		t.Fatalf("IssueClientCert() error = %v", err)
	}
	certB, _, err := ca.IssueClientCert("node-b", time.Hour)
	if err != nil {
		t.Fatalf("IssueClientCert() error = %v", err)
	}

	blockA, _ := pem.Decode(certA)
	blockB, _ := pem.Decode(certB)

	fpA1 := CertFingerprint(blockA.Bytes)
	fpA2 := CertFingerprint(blockA.Bytes)
	if fpA1 != fpA2 {
		t.Error("CertFingerprint() is not deterministic for the same input")
	}
	fpB := CertFingerprint(blockB.Bytes)
	if fpA1 == fpB {
		t.Error("CertFingerprint() produced the same digest for two different certificates")
	}
}

// TestPKI_Live_MutualTLSHandshake is this file's real end-to-end proof:
// not just that x509.Verify accepts the certificate chains in isolation
// (every test above), but that a real net.Listener performing a real
// TLS handshake, both directions authenticated, actually succeeds with
// exactly the certificate shapes this package issues, and actually
// fails when a client presents a certificate from a different,
// unrelated CA. This is the literal mechanism ADR 003's mTLS enrollment
// depends on: if this passes, Enroll's issued client certificate is
// provably usable to open a real mTLS connection to a real server using
// this CA's own issued server certificate, not just "signature
// verifies as a value."
func TestPKI_Live_MutualTLSHandshake(t *testing.T) {
	ca, err := GenerateCA()
	if err != nil {
		t.Fatalf("GenerateCA() error = %v", err)
	}

	serverCertPEM, serverKeyPEM, err := ca.IssueServerCert([]string{"127.0.0.1"}, time.Hour)
	if err != nil {
		t.Fatalf("IssueServerCert() error = %v", err)
	}
	serverCert, err := tls.X509KeyPair(serverCertPEM, serverKeyPEM)
	if err != nil {
		t.Fatalf("tls.X509KeyPair(server) error = %v", err)
	}

	clientCertPEM, clientKeyPEM, err := ca.IssueClientCert("node-live-test", time.Hour)
	if err != nil {
		t.Fatalf("IssueClientCert() error = %v", err)
	}
	clientCert, err := tls.X509KeyPair(clientCertPEM, clientKeyPEM)
	if err != nil {
		t.Fatalf("tls.X509KeyPair(client) error = %v", err)
	}

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(ca.CertPEM()) {
		t.Fatal("failed to load CA into pool")
	}

	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientCAs:    caPool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
	})
	if err != nil {
		t.Fatalf("tls.Listen() error = %v", err)
	}
	defer func() { _ = ln.Close() }()

	serverDone := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			serverDone <- err
			return
		}
		defer func() { _ = conn.Close() }()
		buf := make([]byte, 5)
		if _, err := io.ReadFull(conn, buf); err != nil {
			serverDone <- err
			return
		}
		if string(buf) != "hello" {
			serverDone <- errUnexpectedPayload
			return
		}
		serverDone <- nil
	}()

	clientConn, err := tls.Dial("tcp", ln.Addr().String(), &tls.Config{
		Certificates: []tls.Certificate{clientCert},
		RootCAs:      caPool,
		ServerName:   "127.0.0.1",
	})
	if err != nil {
		t.Fatalf("tls.Dial() error = %v, want a successful mTLS handshake", err)
	}
	defer func() { _ = clientConn.Close() }()

	if _, err := clientConn.Write([]byte("hello")); err != nil {
		t.Fatalf("write over mTLS connection: %v", err)
	}
	if err := <-serverDone; err != nil {
		t.Fatalf("server side of the handshake failed: %v", err)
	}
}

func TestPKI_Live_MutualTLSHandshake_RejectsUnrelatedCA(t *testing.T) {
	ca, err := GenerateCA()
	if err != nil {
		t.Fatalf("GenerateCA() error = %v", err)
	}
	otherCA, err := GenerateCA()
	if err != nil {
		t.Fatalf("GenerateCA() (other) error = %v", err)
	}

	serverCertPEM, serverKeyPEM, err := ca.IssueServerCert([]string{"127.0.0.1"}, time.Hour)
	if err != nil {
		t.Fatalf("IssueServerCert() error = %v", err)
	}
	serverCert, err := tls.X509KeyPair(serverCertPEM, serverKeyPEM)
	if err != nil {
		t.Fatalf("tls.X509KeyPair(server) error = %v", err)
	}

	// A client certificate signed by a completely different CA: must be
	// rejected by a server that only trusts ca's own pool.
	rogueCertPEM, rogueKeyPEM, err := otherCA.IssueClientCert("node-rogue", time.Hour)
	if err != nil {
		t.Fatalf("IssueClientCert() (rogue) error = %v", err)
	}
	rogueCert, err := tls.X509KeyPair(rogueCertPEM, rogueKeyPEM)
	if err != nil {
		t.Fatalf("tls.X509KeyPair(rogue) error = %v", err)
	}

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(ca.CertPEM()) {
		t.Fatal("failed to load CA into pool")
	}

	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientCAs:    caPool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
	})
	if err != nil {
		t.Fatalf("tls.Listen() error = %v", err)
	}
	defer func() { _ = ln.Close() }()

	go func() {
		conn, err := ln.Accept()
		if err == nil {
			_ = conn.Close()
		}
	}()

	rogueCAPool := x509.NewCertPool()
	if !rogueCAPool.AppendCertsFromPEM(ca.CertPEM()) {
		t.Fatal("failed to load CA into rogue client's pool")
	}
	_, err = tls.Dial("tcp", ln.Addr().String(), &tls.Config{
		Certificates: []tls.Certificate{rogueCert},
		RootCAs:      rogueCAPool,
		ServerName:   "127.0.0.1",
	})
	if err == nil {
		t.Error("tls.Dial() with a certificate from an unrelated CA succeeded, want the handshake to be rejected")
	}
}

var errUnexpectedPayload = errors.New("unexpected payload")

// verifyChain confirms leafPEM was actually signed by caPEM, using
// crypto/x509's own verification, not just checking that CreateCertificate
// didn't error.
func verifyChain(t *testing.T, caPEM, leafPEM []byte) {
	t.Helper()
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		t.Fatal("failed to load CA cert into pool")
	}
	block, _ := pem.Decode(leafPEM)
	if block == nil {
		t.Fatal("failed to decode leaf cert PEM")
	}
	leaf, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse leaf cert: %v", err)
	}
	if _, err := leaf.Verify(x509.VerifyOptions{
		Roots:     pool,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
	}); err != nil {
		t.Errorf("leaf.Verify() against CA pool error = %v, want the leaf to chain to the CA", err)
	}
}

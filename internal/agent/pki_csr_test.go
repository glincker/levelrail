package agent

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"testing"
	"time"
)

func csrFor(t *testing.T, key any, cn string, dns []string) []byte {
	t.Helper()
	der, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: cn}, DNSNames: dns}, key)
	if err != nil {
		t.Fatalf("CreateCertificateRequest() error = %v", err)
	}
	return der
}

func TestSignClientCSR(t *testing.T) {
	ca, err := GenerateCA()
	if err != nil {
		t.Fatalf("GenerateCA() error = %v", err)
	}
	now := time.Now()
	_, edCSR, err := NewKeyAndCSR("ignored-name")
	if err != nil {
		t.Fatalf("NewKeyAndCSR() error = %v", err)
	}
	p256, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	p224, _ := ecdsa.GenerateKey(elliptic.P224(), rand.Reader)
	rsaKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	tampered := append([]byte(nil), edCSR...)
	tampered[len(tampered)-1] ^= 0xff

	tests := []struct {
		name    string
		csr     []byte
		wantErr bool
	}{
		{"ed25519", edCSR, false},
		{"ecdsa p256 with requested SANs", csrFor(t, p256, "evil", []string{"control-plane.example"}), false},
		{"ecdsa p224 refused", csrFor(t, p224, "x", nil), true},
		{"rsa refused", csrFor(t, rsaKey, "x", nil), true},
		{"bad signature", tampered, true},
		{"garbage", []byte("not a csr"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issued, err := ca.SignClientCSR("node-1", tt.csr, time.Hour, now)
			if tt.wantErr {
				if !errors.Is(err, ErrInvalidCSR) {
					t.Fatalf("err = %v, want ErrInvalidCSR", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("SignClientCSR() error = %v", err)
			}
			cert, err := parseCertPEM(issued.PEM)
			if err != nil {
				t.Fatalf("parse issued: %v", err)
			}
			if cert.Subject.CommonName != "node-1" || len(cert.DNSNames) != 0 {
				t.Errorf("issued cert asserts CN %q DNS %v, want only CN node-1", cert.Subject.CommonName, cert.DNSNames)
			}
			if len(cert.ExtKeyUsage) != 1 || cert.ExtKeyUsage[0] != x509.ExtKeyUsageClientAuth {
				t.Errorf("ExtKeyUsage = %v, want client auth only", cert.ExtKeyUsage)
			}
			if !issued.NotAfter.Equal(cert.NotAfter) || issued.Fingerprint != CertFingerprint(cert.Raw) || issued.Serial == "" {
				t.Errorf("IssuedCert fields do not describe the certificate: %+v", issued)
			}
			if _, err := cert.Verify(x509.VerifyOptions{Roots: poolOf(ca), KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}); err != nil {
				t.Errorf("issued cert does not chain to the CA: %v", err)
			}
		})
	}
}

func poolOf(ca *CA) *x509.CertPool {
	p := x509.NewCertPool()
	p.AddCert(ca.cert)
	return p
}

func TestValidateIssued(t *testing.T) {
	ca, _ := GenerateCA()
	other, _ := GenerateCA()
	now := time.Now()
	keyPEM, csr, _ := NewKeyAndCSR("n")
	otherKey, _, _ := NewKeyAndCSR("n")
	issued, err := ca.SignClientCSR("node-1", csr, time.Hour, now)
	if err != nil {
		t.Fatalf("SignClientCSR() error = %v", err)
	}
	tests := []struct {
		name    string
		nodeID  string
		key     []byte
		caPEM   []byte
		wantErr bool
	}{
		{"matches", "node-1", keyPEM, ca.CertPEM(), false},
		{"wrong key", "node-1", otherKey, ca.CertPEM(), true},
		{"wrong node", "node-2", keyPEM, ca.CertPEM(), true},
		{"wrong CA", "node-1", keyPEM, other.CertPEM(), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateIssued(tt.nodeID, issued.PEM, tt.key, tt.caPEM, now)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateIssued() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

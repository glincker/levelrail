package pinned

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"testing"
	"time"
)

func selfSigned(t *testing.T) ([]byte, *x509.Certificate) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "ca"}, NotBefore: time.Now().Add(-time.Hour),
		NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, pub, priv)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), cert
}

func TestNormalize(t *testing.T) {
	if got := Normalize(" AB:cd:EF "); got != "abcdef" {
		t.Fatalf("got %q", got)
	}
}

func TestCheckCA(t *testing.T) {
	caPEM, cert := selfSigned(t)
	fp := Fingerprint(cert.Raw)
	cases := []struct {
		name    string
		pin     string
		pem     []byte
		wantErr bool
	}{
		{"match", fp, caPEM, false},
		{"no pin is allowed here, callers enforce it", "", caPEM, false},
		{"wrong pin", "00" + fp[2:], caPEM, true},
		{"not a certificate", fp, []byte("junk"), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := CheckCA(tc.pem, tc.pin)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, tc.wantErr)
			}
		})
	}
	if err := CheckCA(caPEM, "ff"+fp[2:]); !errors.Is(err, ErrMismatch) && fp[:2] != "ff" {
		t.Fatalf("mismatch must wrap ErrMismatch, got %v", err)
	}
}

func TestVerifyChainRequiresThePinnedCA(t *testing.T) {
	_, ca := selfSigned(t)
	if err := VerifyChain(nil, Fingerprint(ca.Raw)); !errors.Is(err, ErrMismatch) {
		t.Fatalf("empty chain got %v", err)
	}
	if err := VerifyChain([]*x509.Certificate{ca}, Fingerprint(ca.Raw)); !errors.Is(err, ErrMismatch) {
		t.Fatalf("a chain without the CA after the leaf got %v", err)
	}
	if err := VerifyChain([]*x509.Certificate{ca, ca}, "deadbeef"); !errors.Is(err, ErrMismatch) {
		t.Fatalf("wrong pin got %v", err)
	}
}

func TestNewKeyAndCSR(t *testing.T) {
	keyPEM, csr, err := NewKeyAndCSR("migration-source")
	if err != nil || len(keyPEM) == 0 {
		t.Fatalf("key: %v", err)
	}
	parsed, err := x509.ParseCertificateRequest(csr)
	if err != nil || parsed.CheckSignature() != nil || parsed.Subject.CommonName != "migration-source" {
		t.Fatalf("bad CSR: %v", err)
	}
}

package database

import (
	"crypto/x509"
	"encoding/pem"
	"testing"
	"time"
)

func TestGenerateSelfSignedCert(t *testing.T) {
	tests := []struct {
		name       string
		commonName string
	}{
		{name: "postgres container name", commonName: "db-main"},
		{name: "redis container name", commonName: "db-cache"},
		{name: "empty common name still produces a parseable cert", commonName: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			certPEM, keyPEM, err := GenerateSelfSignedCert(tt.commonName)
			if err != nil {
				t.Fatalf("GenerateSelfSignedCert(%q) error = %v", tt.commonName, err)
			}

			certBlock, _ := pem.Decode(certPEM)
			if certBlock == nil || certBlock.Type != "CERTIFICATE" {
				t.Fatalf("cert PEM did not decode to a CERTIFICATE block: %+v", certBlock)
			}
			cert, err := x509.ParseCertificate(certBlock.Bytes)
			if err != nil {
				t.Fatalf("parse certificate: %v", err)
			}
			if cert.Subject.CommonName != tt.commonName {
				t.Errorf("Subject.CommonName = %q, want %q", cert.Subject.CommonName, tt.commonName)
			}
			if len(cert.DNSNames) != 1 || cert.DNSNames[0] != tt.commonName {
				t.Errorf("DNSNames = %v, want [%q]", cert.DNSNames, tt.commonName)
			}
			// Self-signed: the certificate is its own issuer, verified by
			// checking it against itself as a trust root, not by string
			// comparison of Subject/Issuer (which self-signed always
			// satisfies trivially but proves less).
			pool := x509.NewCertPool()
			pool.AddCert(cert)
			if _, err := cert.Verify(x509.VerifyOptions{Roots: pool, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}); err != nil {
				t.Errorf("certificate does not verify against itself as a self-signed root: %v", err)
			}
			if time.Until(cert.NotAfter) < 5*365*24*time.Hour {
				t.Errorf("NotAfter = %v, want at least 5 years out (long-lived, no external trust lifecycle)", cert.NotAfter)
			}

			keyBlock, _ := pem.Decode(keyPEM)
			if keyBlock == nil || keyBlock.Type != "PRIVATE KEY" {
				t.Fatalf("key PEM did not decode to a PRIVATE KEY block: %+v", keyBlock)
			}
			if _, err := x509.ParsePKCS8PrivateKey(keyBlock.Bytes); err != nil {
				t.Errorf("parse private key: %v", err)
			}
		})
	}
}

// TestGenerateSelfSignedCert_DistinctKeysPerCall guards against a
// hypothetical shared/reused key source: two calls, even for the same
// commonName, must never produce the same key material, since two
// databases' certificates must never be swappable.
func TestGenerateSelfSignedCert_DistinctKeysPerCall(t *testing.T) {
	cert1, key1, err := GenerateSelfSignedCert("db-main")
	if err != nil {
		t.Fatalf("first GenerateSelfSignedCert() error = %v", err)
	}
	cert2, key2, err := GenerateSelfSignedCert("db-main")
	if err != nil {
		t.Fatalf("second GenerateSelfSignedCert() error = %v", err)
	}
	if string(cert1) == string(cert2) {
		t.Error("two calls with the same commonName produced identical certificates")
	}
	if string(key1) == string(key2) {
		t.Error("two calls with the same commonName produced identical keys")
	}
}

func TestSupportsTLS(t *testing.T) {
	tests := []struct {
		engine string
		want   bool
	}{
		{engine: "postgres", want: true},
		{engine: "redis", want: true},
		{engine: "mysql", want: false},
		{engine: "mariadb", want: false},
		{engine: "mongodb", want: false},
		{engine: "clickhouse", want: false},
		{engine: "keydb", want: false},
		{engine: "dragonfly", want: false},
		{engine: "unknown-engine", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.engine, func(t *testing.T) {
			if got := SupportsTLS(tt.engine); got != tt.want {
				t.Errorf("SupportsTLS(%q) = %v, want %v", tt.engine, got, tt.want)
			}
		})
	}
}

func TestTLSContainerPort(t *testing.T) {
	tests := []struct {
		engine   string
		wantPort int
		wantOK   bool
	}{
		{engine: "postgres", wantPort: 5432, wantOK: true},
		{engine: "redis", wantPort: 6380, wantOK: true},
		{engine: "mysql", wantPort: 0, wantOK: false},
		{engine: "unknown-engine", wantPort: 0, wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.engine, func(t *testing.T) {
			port, ok := TLSContainerPort(tt.engine)
			if port != tt.wantPort || ok != tt.wantOK {
				t.Errorf("TLSContainerPort(%q) = (%d, %v), want (%d, %v)", tt.engine, port, ok, tt.wantPort, tt.wantOK)
			}
		})
	}
}

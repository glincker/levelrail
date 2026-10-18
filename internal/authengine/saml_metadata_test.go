package authengine_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/authengine"
)

func testCert(t *testing.T, cn string, notBefore, notAfter time.Time) *x509.Certificate {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: cn}, NotBefore: notBefore, NotAfter: notAfter}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	c, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestParseSAMLIdPMetadataKeepsNewestValidCertificate(t *testing.T) {
	now := time.Now()
	current := testCert(t, "current", now.Add(-48*time.Hour), now.Add(time.Hour))
	next := testCert(t, "next", now.Add(-time.Hour), now.Add(48*time.Hour))
	future := testCert(t, "future", now.Add(time.Hour), now.Add(72*time.Hour))
	var kds strings.Builder
	for _, c := range []*x509.Certificate{current, next, future} {
		fmt.Fprintf(&kds, `<md:KeyDescriptor use="signing"><ds:KeyInfo xmlns:ds="http://www.w3.org/2000/09/xmldsig#"><ds:X509Data><ds:X509Certificate>%s</ds:X509Certificate></ds:X509Data></ds:KeyInfo></md:KeyDescriptor>`,
			base64.StdEncoding.EncodeToString(c.Raw))
	}
	md := `<md:EntityDescriptor xmlns:md="urn:oasis:names:tc:SAML:2.0:metadata" entityID="https://idp.example.test">
<md:IDPSSODescriptor protocolSupportEnumeration="urn:oasis:names:tc:SAML:2.0:protocol">` + kds.String() + `
<md:SingleSignOnService Binding="urn:oasis:names:tc:SAML:2.0:bindings:HTTP-Redirect" Location="https://idp.example.test/sso"/>
</md:IDPSSODescriptor></md:EntityDescriptor>`
	idp, err := authengine.ParseSAMLIdPMetadata([]byte(md), now)
	if err != nil {
		t.Fatal(err)
	}
	if subject, _, _ := authengine.IDPCertificateInfo(idp.CertPEM); subject != "CN=next" {
		t.Fatalf("kept %q, want the newest certificate valid now", subject)
	}
	if idp.SSOURL != "https://idp.example.test/sso" || idp.EntityID != "https://idp.example.test" {
		t.Fatalf("idp = %+v", idp)
	}
}

func TestNormalizeIDPCertificate(t *testing.T) {
	c := testCert(t, "one", time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	pemOne := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: c.Raw}))
	got, err := authengine.NormalizeIDPCertificate(base64.StdEncoding.EncodeToString(c.Raw))
	if err != nil || got != pemOne {
		t.Fatalf("base64 DER: %q %v", got, err)
	}
	if _, err := authengine.NormalizeIDPCertificate(pemOne + pemOne); err == nil {
		t.Fatal("two certificates were accepted")
	}
	if _, err := authengine.NormalizeIDPCertificate("nope"); err == nil {
		t.Fatal("garbage was accepted")
	}
}

func TestFetchSAMLIdPMetadata_ClientRequired(t *testing.T) {
	_, err := authengine.FetchSAMLIdPMetadata(context.Background(), nil, "https://example.com/metadata")
	if err == nil || err.Error() != "http client is required" {
		t.Fatalf("expected error 'http client is required', got %v", err)
	}
}

func TestFetchSAMLIdPMetadata_ValidClient(t *testing.T) {
	// Simple server to return minimal SAML metadata
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<md:EntityDescriptor xmlns:md="urn:oasis:names:tc:SAML:2.0:metadata" entityID="test-entity" />`))
	}))
	defer srv.Close()

	// Need a client that trusts the test server's cert
	client := srv.Client()

	body, err := authengine.FetchSAMLIdPMetadata(context.Background(), client, srv.URL)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if string(body) != `<md:EntityDescriptor xmlns:md="urn:oasis:names:tc:SAML:2.0:metadata" entityID="test-entity" />` {
		t.Errorf("unexpected body: %s", string(body))
	}
}

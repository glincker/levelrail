package samlsso

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func idpMetadataXML(cert *x509.Certificate, sso string) string {
	return fmt.Sprintf(`<?xml version="1.0"?>
<md:EntityDescriptor xmlns:md="urn:oasis:names:tc:SAML:2.0:metadata" entityID="%s">
  <md:IDPSSODescriptor protocolSupportEnumeration="urn:oasis:names:tc:SAML:2.0:protocol">
    <md:KeyDescriptor use="signing"><ds:KeyInfo xmlns:ds="http://www.w3.org/2000/09/xmldsig#"><ds:X509Data><ds:X509Certificate>%s</ds:X509Certificate></ds:X509Data></ds:KeyInfo></md:KeyDescriptor>
    <md:SingleSignOnService Binding="urn:oasis:names:tc:SAML:2.0:bindings:HTTP-POST" Location="%s/post"/>
    <md:SingleSignOnService Binding="urn:oasis:names:tc:SAML:2.0:bindings:HTTP-Redirect" Location="%s"/>
  </md:IDPSSODescriptor>
</md:EntityDescriptor>`, testIDPEntity, base64.StdEncoding.EncodeToString(cert.Raw), sso, sso)
}

func TestParseIDPMetadata(t *testing.T) {
	idp := newTestIDP(t)
	got, err := ParseIDPMetadata([]byte(idpMetadataXML(idp.Cert, "https://idp.example.test/sso")))
	if err != nil {
		t.Fatal(err)
	}
	if got.EntityID != testIDPEntity || got.SSOURL != "https://idp.example.test/sso" {
		t.Fatalf("idp = %+v", got)
	}
	certs, err := ParseCertificates(got.CertificatesPEM)
	if err != nil || len(certs) != 1 || !certs[0].Equal(idp.Cert) {
		t.Fatalf("certificates = %v, %v", certs, err)
	}
	wrapped := `<md:EntitiesDescriptor xmlns:md="urn:oasis:names:tc:SAML:2.0:metadata">` +
		strings.TrimPrefix(idpMetadataXML(idp.Cert, "https://idp.example.test/sso"), `<?xml version="1.0"?>`) + `</md:EntitiesDescriptor>`
	if _, err := ParseIDPMetadata([]byte(wrapped)); err != nil {
		t.Fatalf("EntitiesDescriptor: %v", err)
	}
}

func TestParseIDPMetadataRejects(t *testing.T) {
	idp := newTestIDP(t)
	good := idpMetadataXML(idp.Cert, "https://idp.example.test/sso")
	for name, doc := range map[string]string{
		"empty":        "",
		"not xml":      "<<<",
		"not metadata": `<foo/>`,
		"http sso":     idpMetadataXML(idp.Cert, "http://idp.example.test/sso"),
		"no cert":      strings.Replace(good, base64.StdEncoding.EncodeToString(idp.Cert.Raw), "", 1),
		"two idps": `<md:EntitiesDescriptor xmlns:md="urn:oasis:names:tc:SAML:2.0:metadata">` +
			strings.TrimPrefix(good, `<?xml version="1.0"?>`) + strings.TrimPrefix(good, `<?xml version="1.0"?>`) + `</md:EntitiesDescriptor>`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseIDPMetadata([]byte(doc)); err == nil {
				t.Fatal("accepted bad metadata")
			}
		})
	}
}

func TestFetchIDPMetadata(t *testing.T) {
	idp := newTestIDP(t)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(idpMetadataXML(idp.Cert, "https://idp.example.test/sso")))
	}))
	defer srv.Close()
	body, err := FetchIDPMetadata(context.Background(), srv.Client(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseIDPMetadata(body); err != nil {
		t.Fatal(err)
	}
	if _, err := FetchIDPMetadata(context.Background(), srv.Client(), strings.Replace(srv.URL, "https", "http", 1)); err == nil {
		t.Fatal("fetched over http")
	}
}

func TestParseCertificatesBareBase64(t *testing.T) {
	idp := newTestIDP(t)
	certs, err := ParseCertificates(base64.StdEncoding.EncodeToString(idp.Cert.Raw))
	if err != nil || len(certs) != 1 {
		t.Fatalf("certs %v err %v", certs, err)
	}
	if _, err := ParseCertificates("-----BEGIN PRIVATE KEY-----\nAAAA\n-----END PRIVATE KEY-----"); err == nil {
		t.Fatal("accepted a key block as a certificate")
	}
}

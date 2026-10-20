package api

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/beevik/etree"
	"github.com/crewjam/saml"
)

const testBearerMethod = "urn:oasis:names:tc:SAML:2.0:cm:bearer" //nolint:gosec // SAML confirmation method URI, not a credential

// testIDP is a minimal SAML identity provider that signs with its own key.
type testIDP struct {
	EntityID string
	Key      *rsa.PrivateKey
	Cert     *x509.Certificate
}

func newTestIDP(t testing.TB, entityID string) *testIDP {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test idp"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return &testIDP{EntityID: entityID, Key: key, Cert: cert}
}

func (i *testIDP) CertPEM() string {
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: i.Cert.Raw}))
}

func (i *testIDP) MetadataXML(ssoURL string) string {
	return fmt.Sprintf(`<?xml version="1.0"?>
<md:EntityDescriptor xmlns:md="urn:oasis:names:tc:SAML:2.0:metadata" entityID="%s">
  <md:IDPSSODescriptor protocolSupportEnumeration="urn:oasis:names:tc:SAML:2.0:protocol">
    <md:KeyDescriptor use="signing"><ds:KeyInfo xmlns:ds="http://www.w3.org/2000/09/xmldsig#"><ds:X509Data><ds:X509Certificate>%s</ds:X509Certificate></ds:X509Data></ds:KeyInfo></md:KeyDescriptor>
    <md:SingleSignOnService Binding="urn:oasis:names:tc:SAML:2.0:bindings:HTTP-Redirect" Location="%s"/>
  </md:IDPSSODescriptor>
</md:EntityDescriptor>`, i.EntityID, base64.StdEncoding.EncodeToString(i.Cert.Raw), ssoURL)
}

type idpTarget struct {
	SPEntityID string
	ACSURL     string
	RequestID  string
}

func (i *testIDP) Assertion(target idpTarget, nameID string, attrs map[string][]string, now time.Time) *saml.Assertion {
	var list []saml.Attribute
	for name, vals := range attrs {
		a := saml.Attribute{Name: name}
		for _, v := range vals {
			a.Values = append(a.Values, saml.AttributeValue{Type: "xs:string", Value: v})
		}
		list = append(list, a)
	}
	return &saml.Assertion{
		ID: "id-" + idpRandHex(), IssueInstant: now, Version: "2.0",
		Issuer: saml.Issuer{Format: "urn:oasis:names:tc:SAML:2.0:nameid-format:entity", Value: i.EntityID},
		Subject: &saml.Subject{
			NameID: &saml.NameID{Format: string(saml.UnspecifiedNameIDFormat), Value: nameID},
			SubjectConfirmations: []saml.SubjectConfirmation{{
				Method: testBearerMethod,
				SubjectConfirmationData: &saml.SubjectConfirmationData{
					InResponseTo: target.RequestID, NotOnOrAfter: now.Add(5 * time.Minute), Recipient: target.ACSURL,
				},
			}},
		},
		Conditions: &saml.Conditions{
			NotBefore: now.Add(-time.Minute), NotOnOrAfter: now.Add(5 * time.Minute),
			AudienceRestrictions: []saml.AudienceRestriction{{Audience: saml.Audience{Value: target.SPEntityID}}},
		},
		AuthnStatements:     []saml.AuthnStatement{{AuthnInstant: now, SessionIndex: "s1"}},
		AttributeStatements: []saml.AttributeStatement{{Attributes: list}},
	}
}

type idpOptions struct {
	Destination  string
	InResponseTo string
	Issuer       string
	SignResponse bool
	SignAssert   bool
}

func (i *testIDP) DefaultOptions(target idpTarget) idpOptions {
	return idpOptions{Destination: target.ACSURL, InResponseTo: target.RequestID, Issuer: i.EntityID, SignResponse: true, SignAssert: true}
}

// Respond renders a base64 SAMLResponse carrying a, signed as o asks.
func (i *testIDP) Respond(t testing.TB, a *saml.Assertion, o idpOptions) string {
	t.Helper()
	md, err := url.Parse(o.Issuer)
	if err != nil {
		t.Fatal(err)
	}
	req := &saml.IdpAuthnRequest{
		IDP:                     &saml.IdentityProvider{Key: i.Key, Certificate: i.Cert, MetadataURL: *md},
		HTTPRequest:             httptest.NewRequest("POST", "https://idp.invalid/sso", nil),
		Request:                 saml.AuthnRequest{ID: o.InResponseTo},
		ServiceProviderMetadata: &saml.EntityDescriptor{},
		SPSSODescriptor:         &saml.SPSSODescriptor{},
		ACSEndpoint:             &saml.IndexedEndpoint{Binding: saml.HTTPPostBinding, Location: o.Destination},
		Assertion:               a,
		Now:                     a.IssueInstant,
	}
	assertionEl := a.Element()
	if o.SignAssert {
		if err := req.MakeAssertionEl(); err != nil {
			t.Fatal(err)
		}
		assertionEl = req.AssertionEl
	}
	var root *etree.Element
	if o.SignResponse {
		req.AssertionEl = assertionEl
		if err := req.MakeResponse(); err != nil {
			t.Fatal(err)
		}
		root = req.ResponseEl
	} else {
		resp := &saml.Response{
			Destination: o.Destination, ID: "id-" + idpRandHex(), InResponseTo: o.InResponseTo,
			IssueInstant: a.IssueInstant, Version: "2.0", Issuer: &saml.Issuer{Value: o.Issuer},
			Status: saml.Status{StatusCode: saml.StatusCode{Value: saml.StatusSuccess}},
		}
		root = resp.Element()
		root.AddChild(assertionEl)
	}
	doc := etree.NewDocument()
	doc.SetRoot(root)
	buf, err := doc.WriteToBytes()
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(buf)
}

func idpRandHex() string {
	b := make([]byte, 10)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}

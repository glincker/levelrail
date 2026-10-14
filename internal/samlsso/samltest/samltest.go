// Package samltest is a minimal SAML identity provider for tests: it signs
// responses with its own key so validation can be exercised end to end.
package samltest

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

const bearerMethod = "urn:oasis:names:tc:SAML:2.0:cm:bearer" //nolint:gosec // SAML confirmation method URI, not a credential

// IDP signs assertions as EntityID.
type IDP struct {
	EntityID string
	Key      *rsa.PrivateKey
	Cert     *x509.Certificate
}

// NewIDP makes an IDP with a fresh self-signed RSA key.
func NewIDP(t testing.TB, entityID string) *IDP {
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
	return &IDP{EntityID: entityID, Key: key, Cert: cert}
}

// CertPEM is the signing certificate in PEM form.
func (i *IDP) CertPEM() string {
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: i.Cert.Raw}))
}

// MetadataXML is an IDP EntityDescriptor pointing at ssoURL.
func (i *IDP) MetadataXML(ssoURL string) string {
	return fmt.Sprintf(`<?xml version="1.0"?>
<md:EntityDescriptor xmlns:md="urn:oasis:names:tc:SAML:2.0:metadata" entityID="%s">
  <md:IDPSSODescriptor protocolSupportEnumeration="urn:oasis:names:tc:SAML:2.0:protocol">
    <md:KeyDescriptor use="signing"><ds:KeyInfo xmlns:ds="http://www.w3.org/2000/09/xmldsig#"><ds:X509Data><ds:X509Certificate>%s</ds:X509Certificate></ds:X509Data></ds:KeyInfo></md:KeyDescriptor>
    <md:SingleSignOnService Binding="urn:oasis:names:tc:SAML:2.0:bindings:HTTP-POST" Location="%s/post"/>
    <md:SingleSignOnService Binding="urn:oasis:names:tc:SAML:2.0:bindings:HTTP-Redirect" Location="%s"/>
  </md:IDPSSODescriptor>
</md:EntityDescriptor>`, i.EntityID, base64.StdEncoding.EncodeToString(i.Cert.Raw), ssoURL, ssoURL)
}

// Target names the SP and request an assertion answers.
type Target struct {
	SPEntityID string
	ACSURL     string
	RequestID  string
}

// Assertion is a valid assertion for target about nameID with the given attributes.
func (i *IDP) Assertion(target Target, nameID string, attrs map[string][]string, now time.Time) *saml.Assertion {
	var list []saml.Attribute
	for name, vals := range attrs {
		a := saml.Attribute{Name: name}
		for _, v := range vals {
			a.Values = append(a.Values, saml.AttributeValue{Type: "xs:string", Value: v})
		}
		list = append(list, a)
	}
	return &saml.Assertion{
		ID: "id-" + randHex(), IssueInstant: now, Version: "2.0",
		Issuer: saml.Issuer{Format: "urn:oasis:names:tc:SAML:2.0:nameid-format:entity", Value: i.EntityID},
		Subject: &saml.Subject{
			NameID: &saml.NameID{Format: string(saml.UnspecifiedNameIDFormat), Value: nameID},
			SubjectConfirmations: []saml.SubjectConfirmation{{
				Method: bearerMethod,
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

// Options shape the Response wrapper around an assertion.
type Options struct {
	Destination  string
	InResponseTo string
	Issuer       string
	Status       string
	SignResponse bool
	SignAssert   bool
}

// DefaultOptions signs both levels and answers target.
func (i *IDP) DefaultOptions(target Target) Options {
	return Options{Destination: target.ACSURL, InResponseTo: target.RequestID, Issuer: i.EntityID,
		Status: saml.StatusSuccess, SignResponse: true, SignAssert: true}
}

// Respond renders a base64 SAMLResponse carrying a.
func (i *IDP) Respond(t testing.TB, a *saml.Assertion, o Options) string {
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
	if o.SignResponse && o.Status == saml.StatusSuccess {
		req.AssertionEl = assertionEl
		if err := req.MakeResponse(); err != nil {
			t.Fatal(err)
		}
		root = req.ResponseEl
	} else {
		resp := &saml.Response{
			Destination: o.Destination, ID: "id-" + randHex(), InResponseTo: o.InResponseTo,
			IssueInstant: a.IssueInstant, Version: "2.0", Issuer: &saml.Issuer{Value: o.Issuer},
			Status: saml.Status{StatusCode: saml.StatusCode{Value: o.Status}},
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

// ElementString serializes an unsigned assertion, for injection tests.
func ElementString(t testing.TB, a *saml.Assertion) string {
	t.Helper()
	doc := etree.NewDocument()
	doc.SetRoot(a.Element())
	s, err := doc.WriteToString()
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func randHex() string {
	b := make([]byte, 10)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}

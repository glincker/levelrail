package samlsso

import (
	"crypto/rsa"
	"crypto/x509"
	"testing"
	"time"

	"github.com/crewjam/saml"

	"github.com/GLINCKER/levelrail/internal/samlsso/samltest"
)

const (
	testIDPEntity = "https://idp.example.test/metadata"
	testSPEntity  = "https://cp.example.test/api/v1/auth/saml/conn_1/metadata"
	testACS       = "https://cp.example.test/api/v1/auth/saml/conn_1/acs"
	testRequestID = "id-request-1"
)

var testTarget = samltest.Target{SPEntityID: testSPEntity, ACSURL: testACS, RequestID: testRequestID}

func newKeyPair(t *testing.T, cn string) (*x509.Certificate, *rsa.PrivateKey) {
	t.Helper()
	certPEM, keyPEM, err := GenerateSPKeyPair(cn, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	cert, key, err := ParseSPKeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	return cert, key
}

func newTestIDP(t *testing.T) *samltest.IDP { return samltest.NewIDP(t, testIDPEntity) }

func newTestSP(t *testing.T, idp *samltest.IDP) SP {
	cert, key := newKeyPair(t, "sp")
	return SP{
		EntityID: testSPEntity, ACSURL: testACS, Cert: cert, Key: key,
		IDP: IDP{EntityID: testIDPEntity, SSOURL: "https://idp.example.test/sso", CertificatesPEM: idp.CertPEM()},
	}
}

func goodAssertion(idp *samltest.IDP, now time.Time) *saml.Assertion {
	a := idp.Assertion(testTarget, "user-123", map[string][]string{
		"email":       {"Ada@Example.com"},
		"displayName": {"Ada"},
		"groups":      {"eng", "ops"},
	}, now)
	a.ID = "id-assertion-1"
	return a
}

func defaultOpts() samltest.Options {
	return samltest.Options{Destination: testACS, InResponseTo: testRequestID, Issuer: testIDPEntity,
		Status: saml.StatusSuccess, SignResponse: true, SignAssert: true}
}

func setAttr(a *saml.Assertion, name, value string) {
	for i, attr := range a.AttributeStatements[0].Attributes {
		if attr.Name == name {
			a.AttributeStatements[0].Attributes[i].Values = []saml.AttributeValue{{Type: "xs:string", Value: value}}
		}
	}
}

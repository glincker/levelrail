package authengine_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"math/big"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	"github.com/beevik/etree"
	"github.com/crewjam/saml"

	"github.com/GLINCKER/levelrail/internal/authengine"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	vfIDPEntity = "https://idp.example.test/metadata"
	vfSPEntity  = "https://cp.example.test/sp"
	vfACS       = "https://cp.example.test/acs"
	vfRequestID = "req-1"
	vfBearer    = "urn:oasis:names:tc:SAML:2.0:cm:bearer" //nolint:gosec // SAML confirmation method URI, not a credential
)

type vfIDP struct {
	key  *rsa.PrivateKey
	cert *x509.Certificate
}

func newVFIDP(t *testing.T) *vfIDP {
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
	return &vfIDP{key: key, cert: cert}
}

func (i *vfIDP) certPEM() string {
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: i.cert.Raw}))
}

func vfAssertion(now time.Time) *saml.Assertion {
	return &saml.Assertion{
		ID: "a-1", IssueInstant: now, Version: "2.0",
		Issuer: saml.Issuer{Format: "urn:oasis:names:tc:SAML:2.0:nameid-format:entity", Value: vfIDPEntity},
		Subject: &saml.Subject{
			NameID: &saml.NameID{Format: string(saml.UnspecifiedNameIDFormat), Value: "nid-1"},
			SubjectConfirmations: []saml.SubjectConfirmation{{
				Method: vfBearer,
				SubjectConfirmationData: &saml.SubjectConfirmationData{
					InResponseTo: vfRequestID, NotOnOrAfter: now.Add(5 * time.Minute), Recipient: vfACS,
				},
			}},
		},
		Conditions: &saml.Conditions{
			NotBefore: now.Add(-time.Minute), NotOnOrAfter: now.Add(5 * time.Minute),
			AudienceRestrictions: []saml.AudienceRestriction{{Audience: saml.Audience{Value: vfSPEntity}}},
		},
		AuthnStatements: []saml.AuthnStatement{{AuthnInstant: now, SessionIndex: "s1"}},
	}
}

// respond signs the assertion and the response with the test IdP key.
func (i *vfIDP) respond(t *testing.T, a *saml.Assertion) string {
	t.Helper()
	md, err := url.Parse(vfIDPEntity)
	if err != nil {
		t.Fatal(err)
	}
	req := &saml.IdpAuthnRequest{
		IDP:                     &saml.IdentityProvider{Key: i.key, Certificate: i.cert, MetadataURL: *md},
		HTTPRequest:             httptest.NewRequest("POST", "https://idp.invalid/sso", nil),
		Request:                 saml.AuthnRequest{ID: vfRequestID},
		ServiceProviderMetadata: &saml.EntityDescriptor{},
		SPSSODescriptor:         &saml.SPSSODescriptor{},
		ACSEndpoint:             &saml.IndexedEndpoint{Binding: saml.HTTPPostBinding, Location: vfACS},
		Assertion:               a,
		Now:                     a.IssueInstant,
	}
	if err := req.MakeAssertionEl(); err != nil {
		t.Fatal(err)
	}
	if err := req.MakeResponse(); err != nil {
		t.Fatal(err)
	}
	doc := etree.NewDocument()
	doc.SetRoot(req.ResponseEl)
	buf, err := doc.WriteToBytes()
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(buf)
}

func newVFEngine(t *testing.T, idp *vfIDP) (*authengine.Engine, string) {
	t.Helper()
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "levelrail.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	kp, err := authengine.GenerateSAMLKeyPair(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	eng, err := authengine.New(db.DB, authengine.Config{
		BaseURL: "https://cp.example.test", TokenPrefix: "tk", Directory: authengine.NewDirectory(db.DB), SAML: kp,
	})
	if err != nil {
		t.Fatalf("authengine.New: %v", err)
	}
	t.Cleanup(eng.Close)
	id, err := eng.CreateSAMLConnection(ctx, authengine.SAMLConnection{
		IDPEntityID: vfIDPEntity, IDPSSOURL: "https://idp.example.test/sso", IDPCertPEM: idp.certPEM(),
		SPEntityID: vfSPEntity, SPACSURL: vfACS,
	})
	if err != nil {
		t.Fatalf("CreateSAMLConnection: %v", err)
	}
	return eng, id
}

func TestVerifySAMLResponseSignedAssertions(t *testing.T) {
	idp := newVFIDP(t)
	eng, connID := newVFEngine(t, idp)
	now := time.Now()

	noNameID := vfAssertion(now)
	noNameID.Subject.NameID = nil
	noConditions := vfAssertion(now)
	noConditions.Conditions = nil
	noConfData := vfAssertion(now)
	noConfData.Subject.SubjectConfirmations[0].SubjectConfirmationData = nil

	cases := []struct {
		name    string
		a       *saml.Assertion
		wantErr bool
	}{
		{"valid with NameID", vfAssertion(now), false},
		{"missing NameID", noNameID, true},
		{"missing Conditions", noConditions, true},
		{"missing SubjectConfirmationData", noConfData, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := eng.VerifySAMLResponse(context.Background(), connID, idp.respond(t, c.a), vfRequestID)
			if c.wantErr {
				if !errors.Is(err, authengine.ErrSAMLInvalid) {
					t.Fatalf("err = %v, want ErrSAMLInvalid", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("valid response refused: %v", err)
			}
			if got.NameID != "nid-1" {
				t.Errorf("NameID = %q", got.NameID)
			}
		})
	}
}

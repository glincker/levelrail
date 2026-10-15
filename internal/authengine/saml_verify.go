package authengine

import (
	"bytes"
	"compress/flate"
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/crewjam/saml"
	theauth "github.com/glincker/theauth-go/v2"
)

const (
	maxSAMLResponseBytes = 256 << 10
	bearerMethod         = "urn:oasis:names:tc:SAML:2.0:cm:bearer" //nolint:gosec // SAML confirmation method URI, not a credential
)

// ErrSAMLInvalid wraps every reason a response is refused before the library sees it.
var ErrSAMLInvalid = errors.New("authengine: saml response invalid")

// SAMLAssertion is what a verified response says about the user.
type SAMLAssertion struct {
	NameID      string
	Email       string
	DisplayName string
	Groups      []string
}

// VerifySAMLResponse is the platform's strict gate in front of the library,
// whose SP accepts IdP-initiated responses and so never checks InResponseTo.
// The response must answer requestID, name this SP as its audience and carry
// a bearer confirmation for that request, on top of the signature and time checks.
func (e *Engine) VerifySAMLResponse(ctx context.Context, connID, samlResponse, requestID string) (a SAMLAssertion, err error) {
	conn, err := e.SAMLConnection(ctx, connID)
	if err != nil {
		return SAMLAssertion{}, err
	}
	if requestID == "" {
		return SAMLAssertion{}, fmt.Errorf("%w: no request to answer", ErrSAMLInvalid)
	}
	raw, err := base64.StdEncoding.DecodeString(samlResponse)
	if err != nil || len(raw) == 0 || len(raw) > maxSAMLResponseBytes {
		return SAMLAssertion{}, fmt.Errorf("%w: not a base64 response of acceptable size", ErrSAMLInvalid)
	}
	sp, acs, err := e.strictSP(conn)
	if err != nil {
		return SAMLAssertion{}, err
	}
	// crewjam dereferences optional elements without nil checks; a malformed response must fail, not crash.
	defer func() {
		if r := recover(); r != nil {
			a, err = SAMLAssertion{}, fmt.Errorf("%w: malformed response", ErrSAMLInvalid)
		}
	}()
	assertion, perr := sp.ParseXMLResponse(raw, []string{requestID}, acs)
	if perr != nil {
		return SAMLAssertion{}, fmt.Errorf("%w: %s", ErrSAMLInvalid, safeReason(perr))
	}
	if !answersRequest(assertion, requestID) {
		return SAMLAssertion{}, fmt.Errorf("%w: no bearer confirmation for this request", ErrSAMLInvalid)
	}
	return mapSAMLAssertion(conn, assertion), nil
}

func (e *Engine) strictSP(conn SAMLConnection) (*saml.ServiceProvider, url.URL, error) {
	if e.samlCert == nil || e.samlKey == nil {
		return nil, url.URL{}, ErrSAMLDisabled
	}
	block, _ := pem.Decode([]byte(conn.IDPCertPEM))
	if block == nil {
		return nil, url.URL{}, fmt.Errorf("%w: idp certificate is not PEM", ErrSAMLInvalid)
	}
	idpCert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, url.URL{}, fmt.Errorf("%w: idp certificate: %s", ErrSAMLInvalid, err.Error())
	}
	acs, err := url.Parse(conn.SPACSURL)
	if err != nil {
		return nil, url.URL{}, fmt.Errorf("%w: acs url", ErrSAMLInvalid)
	}
	md := &saml.EntityDescriptor{
		EntityID: conn.IDPEntityID,
		IDPSSODescriptors: []saml.IDPSSODescriptor{{
			SSODescriptor: saml.SSODescriptor{RoleDescriptor: saml.RoleDescriptor{KeyDescriptors: []saml.KeyDescriptor{{
				Use:     "signing",
				KeyInfo: saml.KeyInfo{X509Data: saml.X509Data{X509Certificates: []saml.X509Certificate{{Data: base64.StdEncoding.EncodeToString(idpCert.Raw)}}}},
			}}}},
			SingleSignOnServices: []saml.Endpoint{{Binding: saml.HTTPPostBinding, Location: conn.IDPSSOURL}},
		}},
	}
	entityID := conn.SPEntityID
	sp := &saml.ServiceProvider{
		EntityID: entityID, Key: e.samlKey, Certificate: e.samlCert, AcsURL: *acs, IDPMetadata: md,
		AllowIDPInitiated: false,
		ValidateAudienceRestriction: func(a *saml.Assertion) error {
			if a.Conditions == nil {
				return errors.New("no conditions")
			}
			for _, r := range a.Conditions.AudienceRestrictions {
				if r.Audience.Value == entityID {
					return nil
				}
			}
			return errors.New("audience is not this service provider")
		},
	}
	return sp, *acs, nil
}

// safeReason keeps crewjam's validation reason for the log but drops the ones
// that quote parts of the document, so no assertion content is ever logged.
func safeReason(err error) string {
	var ire *saml.InvalidResponseError
	if !errors.As(err, &ire) || ire.PrivateErr == nil {
		return "rejected"
	}
	msg := ire.PrivateErr.Error()
	for _, quoting := range []string{"invalid xml", "cannot unmarshal", "xml:", "XML syntax"} {
		if strings.Contains(msg, quoting) {
			return "malformed xml"
		}
	}
	return msg
}

func answersRequest(a *saml.Assertion, requestID string) bool {
	if a.Subject == nil {
		return false
	}
	for _, sc := range a.Subject.SubjectConfirmations {
		if sc.Method == bearerMethod && sc.SubjectConfirmationData != nil && sc.SubjectConfirmationData.InResponseTo == requestID {
			return true
		}
	}
	return false
}

func mapSAMLAssertion(conn SAMLConnection, a *saml.Assertion) SAMLAssertion {
	def := theauth.DefaultSAMLAttributeMap()
	pick := func(set, fallback string) string {
		if set != "" {
			return set
		}
		return fallback
	}
	attrs := map[string][]string{}
	for _, st := range a.AttributeStatements {
		for _, at := range st.Attributes {
			for _, v := range at.Values {
				attrs[at.Name] = append(attrs[at.Name], v.Value)
			}
		}
	}
	first := func(name string) string {
		if v := attrs[name]; len(v) > 0 {
			return strings.TrimSpace(v[0])
		}
		return ""
	}
	out := SAMLAssertion{
		Email:       normEmail(first(pick(conn.Attributes.Email, def.Email))),
		DisplayName: first(pick(conn.Attributes.Name, def.Name)),
		Groups:      attrs[pick(conn.Attributes.Groups, def.Groups)],
	}
	if a.Subject != nil && a.Subject.NameID != nil {
		out.NameID = a.Subject.NameID.Value
	}
	// The library keys the identity on the email when the subject has no NameID; mirror it.
	if out.NameID == "" {
		out.NameID = out.Email
	}
	return out
}

// authnRequestID pulls the ID out of the HTTP-Redirect SAMLRequest parameter.
func authnRequestID(redirect string) (string, error) {
	u, err := url.Parse(redirect)
	if err != nil {
		return "", fmt.Errorf("authengine: parse saml redirect: %w", err)
	}
	enc := u.Query().Get("SAMLRequest")
	deflated, err := base64.StdEncoding.DecodeString(enc)
	if err != nil || enc == "" {
		return "", errors.New("authengine: saml redirect has no request")
	}
	body, err := io.ReadAll(io.LimitReader(flate.NewReader(bytes.NewReader(deflated)), maxSAMLResponseBytes))
	if err != nil {
		return "", fmt.Errorf("authengine: inflate saml request: %w", err)
	}
	var req struct {
		ID string `xml:"ID,attr"`
	}
	if err := xml.Unmarshal(body, &req); err != nil || req.ID == "" {
		return "", errors.New("authengine: saml request has no ID")
	}
	return req.ID, nil
}

func parseSAMLKeyPair(kp SAMLKeyPair) (*x509.Certificate, *rsa.PrivateKey, error) {
	if err := checkSAMLKeyPair(kp); err != nil {
		return nil, nil, err
	}
	cb, _ := pem.Decode(kp.CertPEM)
	kb, _ := pem.Decode(kp.KeyPEM)
	cert, err := x509.ParseCertificate(cb.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("authengine: parse saml certificate: %w", err)
	}
	key, err := x509.ParsePKCS1PrivateKey(kb.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("authengine: parse saml key: %w", err)
	}
	return cert, key, nil
}

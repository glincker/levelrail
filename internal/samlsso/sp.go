package samlsso

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"net/url"

	"github.com/crewjam/saml"
)

const signatureRSASHA256 = "http://www.w3.org/2001/04/xmldsig-more#rsa-sha256"

// SP is one service provider bound to one IDP connection.
type SP struct {
	// EntityID doubles as the metadata URL, so it is also the only accepted audience.
	EntityID string
	ACSURL   string
	Cert     *x509.Certificate
	Key      *rsa.PrivateKey
	IDP      IDP
}

func (s SP) serviceProvider(withIDP bool) (*saml.ServiceProvider, error) {
	if s.Cert == nil || s.Key == nil {
		return nil, errors.New("samlsso: sp keypair is missing")
	}
	if err := requireHTTPS(s.EntityID); err != nil {
		return nil, fmt.Errorf("samlsso: sp entity id %w", err)
	}
	if err := requireHTTPS(s.ACSURL); err != nil {
		return nil, fmt.Errorf("samlsso: sp acs url %w", err)
	}
	acs, _ := url.Parse(s.ACSURL)
	md, _ := url.Parse(s.EntityID)
	sp := &saml.ServiceProvider{
		EntityID:          s.EntityID,
		Key:               s.Key,
		Certificate:       s.Cert,
		MetadataURL:       *md,
		AcsURL:            *acs,
		AuthnNameIDFormat: saml.UnspecifiedNameIDFormat,
		SignatureMethod:   signatureRSASHA256,
		AllowIDPInitiated: false,
	}
	if !withIDP {
		return sp, nil
	}
	if err := s.IDP.Check(); err != nil {
		return nil, err
	}
	certs, err := ParseCertificates(s.IDP.CertificatesPEM)
	if err != nil {
		return nil, err
	}
	sp.IDPMetadata = idpDescriptor(s.IDP, certs)
	sp.ValidateAudienceRestriction = strictAudience(s.EntityID)
	return sp, nil
}

func idpDescriptor(idp IDP, certs []*x509.Certificate) *saml.EntityDescriptor {
	keys := make([]saml.KeyDescriptor, 0, len(certs))
	for _, c := range certs {
		keys = append(keys, saml.KeyDescriptor{
			Use:     "signing",
			KeyInfo: saml.KeyInfo{X509Data: saml.X509Data{X509Certificates: []saml.X509Certificate{{Data: base64.StdEncoding.EncodeToString(c.Raw)}}}},
		})
	}
	return &saml.EntityDescriptor{
		EntityID: idp.EntityID,
		IDPSSODescriptors: []saml.IDPSSODescriptor{{
			SSODescriptor: saml.SSODescriptor{RoleDescriptor: saml.RoleDescriptor{KeyDescriptors: keys}},
			SingleSignOnServices: []saml.Endpoint{
				{Binding: saml.HTTPRedirectBinding, Location: idp.SSOURL},
				{Binding: saml.HTTPPostBinding, Location: idp.SSOURL},
			},
		}},
	}
}

// crewjam accepts an assertion with no AudienceRestriction at all; we do not.
func strictAudience(entityID string) func(*saml.Assertion) error {
	return func(a *saml.Assertion) error {
		if a.Conditions == nil {
			return errors.New("assertion has no conditions")
		}
		for _, ar := range a.Conditions.AudienceRestrictions {
			if ar.Audience.Value == entityID {
				return nil
			}
		}
		return fmt.Errorf("assertion audience is not %q", entityID)
	}
}

// MetadataXML renders the SP metadata an IDP administrator imports.
func (s SP) MetadataXML() ([]byte, error) {
	sp, err := s.serviceProvider(false)
	if err != nil {
		return nil, err
	}
	out, err := xml.MarshalIndent(sp.Metadata(), "", "  ")
	if err != nil {
		return nil, fmt.Errorf("samlsso: marshal sp metadata: %w", err)
	}
	return append([]byte(xml.Header), out...), nil
}

// BeginLogin builds a signed HTTP-Redirect AuthnRequest. The caller must
// persist requestID and bind it to the browser: Validate requires it.
func (s SP) BeginLogin() (redirectURL, requestID string, err error) {
	sp, err := s.serviceProvider(true)
	if err != nil {
		return "", "", err
	}
	req, err := sp.MakeAuthenticationRequest(sp.GetSSOBindingLocation(saml.HTTPRedirectBinding), saml.HTTPRedirectBinding, saml.HTTPPostBinding)
	if err != nil {
		return "", "", fmt.Errorf("samlsso: build authn request: %w", err)
	}
	u, err := req.Redirect("", sp)
	if err != nil {
		return "", "", fmt.Errorf("samlsso: sign authn request: %w", err)
	}
	return u.String(), req.ID, nil
}

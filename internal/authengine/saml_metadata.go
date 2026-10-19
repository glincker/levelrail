package authengine

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/crewjam/saml"
	xrv "github.com/mattermost/xml-roundtrip-validator"

	"github.com/GLINCKER/levelrail/kit/netguard"
)

// MaxSAMLMetadataBytes caps an imported IdP metadata document.
const MaxSAMLMetadataBytes = 1 << 20

const metadataFetchTimeout = 15 * time.Second

var certWhitespace = regexp.MustCompile(`\s+`)

// SAMLIdP is what the library needs to trust one identity provider.
type SAMLIdP struct {
	EntityID string
	SSOURL   string
	CertPEM  string
}

// Check validates a hand-entered or imported IdP: an entity ID, an https SSO
// URL and exactly one parseable signing certificate.
func (i SAMLIdP) Check() error {
	if strings.TrimSpace(i.EntityID) == "" {
		return errors.New("idp entity id is required")
	}
	if err := requireHTTPS(i.SSOURL); err != nil {
		return fmt.Errorf("idp sso url: %w", err)
	}
	if _, err := NormalizeIDPCertificate(i.CertPEM); err != nil {
		return err
	}
	return nil
}

// NormalizeIDPCertificate accepts one certificate as PEM or bare base64 DER and
// returns it as PEM. The library verifies against a single certificate.
func NormalizeIDPCertificate(raw string) (string, error) {
	certs, err := parseCertificates(raw)
	if err != nil {
		return "", err
	}
	if len(certs) != 1 {
		return "", errors.New("give exactly one idp signing certificate")
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certs[0].Raw})), nil
}

// IDPCertificateInfo describes the stored certificate for display.
func IDPCertificateInfo(certPEM string) (subject string, notAfter time.Time, ok bool) {
	certs, err := parseCertificates(certPEM)
	if err != nil || len(certs) == 0 {
		return "", time.Time{}, false
	}
	return certs[0].Subject.String(), certs[0].NotAfter, true
}

func parseCertificates(raw string) ([]*x509.Certificate, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.New("no idp certificate given")
	}
	if !strings.Contains(raw, "-----BEGIN") {
		der, err := base64.StdEncoding.DecodeString(certWhitespace.ReplaceAllString(raw, ""))
		if err != nil {
			return nil, errors.New("idp certificate is neither PEM nor base64")
		}
		cert, err := x509.ParseCertificate(der)
		if err != nil {
			return nil, fmt.Errorf("parse idp certificate: %w", err)
		}
		return []*x509.Certificate{cert}, nil
	}
	var out []*x509.Certificate
	rest := []byte(raw)
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			return nil, fmt.Errorf("unexpected PEM block %q", block.Type)
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parse idp certificate: %w", err)
		}
		out = append(out, cert)
	}
	if len(out) == 0 {
		return nil, errors.New("no PEM certificate found")
	}
	return out, nil
}

// ParseSAMLIdPMetadata reads an EntityDescriptor (or an EntitiesDescriptor
// holding exactly one IdP). With several signing certificates it keeps the
// newest one valid at now, since the library verifies against one.
func ParseSAMLIdPMetadata(raw []byte, now time.Time) (SAMLIdP, error) {
	if len(raw) == 0 || len(raw) > MaxSAMLMetadataBytes {
		return SAMLIdP{}, errors.New("metadata is empty or larger than 1 MiB")
	}
	if err := xrv.Validate(bytes.NewReader(raw)); err != nil {
		return SAMLIdP{}, fmt.Errorf("metadata is not well-formed XML: %w", err)
	}
	ed, err := pickIDPDescriptor(raw)
	if err != nil {
		return SAMLIdP{}, err
	}
	desc := ed.IDPSSODescriptors[0]
	sso := ssoLocation(desc.SingleSignOnServices)
	if sso == "" {
		return SAMLIdP{}, errors.New("metadata has no HTTP-Redirect or HTTP-POST SingleSignOnService")
	}
	var best *x509.Certificate
	for _, kd := range desc.KeyDescriptors {
		if kd.Use != "" && kd.Use != "signing" {
			continue
		}
		for _, c := range kd.KeyInfo.X509Data.X509Certificates {
			certs, err := parseCertificates(c.Data)
			if err != nil {
				return SAMLIdP{}, fmt.Errorf("metadata signing certificate: %w", err)
			}
			for _, cert := range certs {
				if now.Before(cert.NotBefore) || now.After(cert.NotAfter) {
					continue
				}
				if best == nil || cert.NotBefore.After(best.NotBefore) {
					best = cert
				}
			}
		}
	}
	if best == nil {
		return SAMLIdP{}, errors.New("metadata has no signing certificate valid now")
	}
	idp := SAMLIdP{EntityID: strings.TrimSpace(ed.EntityID), SSOURL: sso,
		CertPEM: string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: best.Raw}))}
	if err := idp.Check(); err != nil {
		return SAMLIdP{}, err
	}
	return idp, nil
}

func pickIDPDescriptor(raw []byte) (*saml.EntityDescriptor, error) {
	var single saml.EntityDescriptor
	if err := xml.Unmarshal(raw, &single); err == nil && len(single.IDPSSODescriptors) > 0 {
		return &single, nil
	}
	var many saml.EntitiesDescriptor
	if err := xml.Unmarshal(raw, &many); err != nil {
		return nil, errors.New("metadata is not a SAML EntityDescriptor")
	}
	var found []*saml.EntityDescriptor
	for i := range many.EntityDescriptors {
		if len(many.EntityDescriptors[i].IDPSSODescriptors) > 0 {
			found = append(found, &many.EntityDescriptors[i])
		}
	}
	if len(found) != 1 {
		return nil, fmt.Errorf("metadata must describe exactly one identity provider, found %d", len(found))
	}
	return found[0], nil
}

func ssoLocation(endpoints []saml.Endpoint) string {
	for _, binding := range []string{saml.HTTPRedirectBinding, saml.HTTPPostBinding} {
		for _, e := range endpoints {
			if e.Binding == binding && e.Location != "" {
				return strings.TrimSpace(e.Location)
			}
		}
	}
	return ""
}

func requireHTTPS(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return errors.New("must be an absolute URL")
	}
	if u.Scheme != "https" {
		return errors.New("must use https")
	}
	return nil
}

// FetchSAMLIdPMetadata downloads metadata from an https URL, following only
// https redirects and reading at most MaxSAMLMetadataBytes.
func FetchSAMLIdPMetadata(ctx context.Context, client *http.Client, rawURL string) ([]byte, error) {
	if err := requireHTTPS(rawURL); err != nil {
		return nil, fmt.Errorf("metadata url %w", err)
	}
	if client == nil {
		client = netguard.NewClient()
	}
	c := *client
	c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 || req.URL.Scheme != "https" {
			return errors.New("metadata redirect refused")
		}
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, metadataFetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("build metadata request: %w", err)
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch metadata: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch metadata: status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxSAMLMetadataBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read metadata: %w", err)
	}
	if len(body) > MaxSAMLMetadataBytes {
		return nil, errors.New("metadata is larger than 1 MiB")
	}
	return body, nil
}

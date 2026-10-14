package samlsso

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/crewjam/saml"
	xrv "github.com/mattermost/xml-roundtrip-validator"
)

// MaxMetadataBytes caps an imported IDP metadata document.
const MaxMetadataBytes = 1 << 20

const fetchTimeout = 15 * time.Second

// IDP is what the SP needs to trust one identity provider.
type IDP struct {
	EntityID string
	SSOURL   string
	// CertificatesPEM holds every signing certificate, so a rotation that
	// publishes the next certificate early keeps sign-in working.
	CertificatesPEM string
}

// ParseIDPMetadata reads an IDP EntityDescriptor (or an EntitiesDescriptor
// holding exactly one IDP) and extracts the entity ID, SSO URL and signing certificates.
func ParseIDPMetadata(raw []byte) (IDP, error) {
	if len(raw) == 0 || len(raw) > MaxMetadataBytes {
		return IDP{}, errors.New("samlsso: metadata is empty or larger than 1 MiB")
	}
	if err := xrv.Validate(bytes.NewReader(raw)); err != nil {
		return IDP{}, fmt.Errorf("samlsso: metadata is not well-formed XML: %w", err)
	}
	ed, err := pickIDPDescriptor(raw)
	if err != nil {
		return IDP{}, err
	}
	desc := ed.IDPSSODescriptors[0]
	sso := ssoLocation(desc.SingleSignOnServices)
	if sso == "" {
		return IDP{}, errors.New("samlsso: metadata has no HTTP-Redirect or HTTP-POST SingleSignOnService")
	}
	var pems strings.Builder
	for _, kd := range desc.KeyDescriptors {
		if kd.Use != "" && kd.Use != "signing" {
			continue
		}
		for _, c := range kd.KeyInfo.X509Data.X509Certificates {
			certs, err := ParseCertificates(c.Data)
			if err != nil {
				return IDP{}, fmt.Errorf("samlsso: metadata signing certificate: %w", err)
			}
			pems.WriteString(EncodeCertificates(certs))
		}
	}
	idp := IDP{EntityID: strings.TrimSpace(ed.EntityID), SSOURL: sso, CertificatesPEM: pems.String()}
	if err := idp.Check(); err != nil {
		return IDP{}, err
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
		return nil, errors.New("samlsso: metadata is not a SAML EntityDescriptor")
	}
	var found []*saml.EntityDescriptor
	for i := range many.EntityDescriptors {
		if len(many.EntityDescriptors[i].IDPSSODescriptors) > 0 {
			found = append(found, &many.EntityDescriptors[i])
		}
	}
	if len(found) != 1 {
		return nil, fmt.Errorf("samlsso: metadata must describe exactly one identity provider, found %d", len(found))
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

// Check validates a hand-entered or imported IDP: an entity ID, an https SSO
// URL and at least one parseable certificate.
func (i IDP) Check() error {
	if strings.TrimSpace(i.EntityID) == "" {
		return errors.New("samlsso: idp entity id is required")
	}
	if err := requireHTTPS(i.SSOURL); err != nil {
		return fmt.Errorf("samlsso: idp sso url: %w", err)
	}
	if _, err := ParseCertificates(i.CertificatesPEM); err != nil {
		return err
	}
	return nil
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

// FetchIDPMetadata downloads metadata from an https URL, following only https
// redirects and reading at most MaxMetadataBytes.
func FetchIDPMetadata(ctx context.Context, client *http.Client, rawURL string) ([]byte, error) {
	if err := requireHTTPS(rawURL); err != nil {
		return nil, fmt.Errorf("samlsso: metadata url %w", err)
	}
	if client == nil {
		client = &http.Client{}
	}
	c := *client
	c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 || req.URL.Scheme != "https" {
			return errors.New("samlsso: metadata redirect refused")
		}
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("samlsso: build metadata request: %w", err)
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("samlsso: fetch metadata: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("samlsso: fetch metadata: status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxMetadataBytes+1))
	if err != nil {
		return nil, fmt.Errorf("samlsso: read metadata: %w", err)
	}
	if len(body) > MaxMetadataBytes {
		return nil, errors.New("samlsso: metadata is larger than 1 MiB")
	}
	return body, nil
}

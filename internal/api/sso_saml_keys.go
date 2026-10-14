package api

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/samlsso"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	samlSPKeyService = "sso/saml-sp"
	samlSPKeyEnv     = "private-key"
	samlPathPrefix   = "/api/v1/auth/saml/"
)

var (
	errSAMLNoMasterKey = errors.New("SAML sign-in needs the control plane master key")
	errSAMLNeedsHTTPS  = errors.New("SAML sign-in needs an https dashboard URL: set it under Settings, General")
	errSSOUnavailable  = errors.New("single sign-on is not available on this control plane")
)

// SSOStore is the persistence the SAML and group mapping routes need. *store.DB satisfies it.
type SSOStore interface {
	ListSAMLConnections(ctx context.Context) ([]store.SAMLConnection, error)
	GetSAMLConnection(ctx context.Context, id string) (store.SAMLConnection, error)
	CreateSAMLConnection(ctx context.Context, c store.SAMLConnection, now time.Time) error
	UpdateSAMLConnection(ctx context.Context, c store.SAMLConnection, now time.Time) error
	DeleteSAMLConnection(ctx context.Context, id string) error
	SaveSAMLAuthnRequest(ctx context.Context, id, connectionID, browserHash string, expires, now time.Time) error
	ConsumeSAMLAuthnRequest(ctx context.Context, connectionID, browserHash string, now time.Time) (string, error)
	RecordSAMLAssertion(ctx context.Context, connectionID, assertionID string, until, now time.Time) error
	SAMLIdentityUser(ctx context.Context, connectionID, nameID string) (string, error)
	LinkSAMLIdentity(ctx context.Context, connectionID, nameID, userID string, now time.Time) error
	TouchSAMLIdentity(ctx context.Context, connectionID, nameID string, now time.Time) error
	CreateProvisionedUser(ctx context.Context, p store.ProvisionedUser) error
	ListSSOGroupMappings(ctx context.Context) ([]store.SSOGroupMapping, error)
	ReplaceSSOGroupMappings(ctx context.Context, mappings []store.SSOGroupMapping, now time.Time) error
	GetSSOSettings(ctx context.Context) (store.SSOSettings, error)
	SetSSOSPCertificate(ctx context.Context, certPEM string, now time.Time) (bool, error)
	SetSSOEnforce(ctx context.Context, on bool, now time.Time) error
}

// samlKeyCache holds the SP keypair once loaded; the mutex also serializes
// first-use generation so one process never writes two keypairs.
type samlKeyCache struct {
	mu   sync.Mutex
	cert string
	key  string
}

func (rt *Router) attachSSO(s Store) {
	if db, ok := s.(SSOStore); ok {
		rt.sso = db
	}
	if db, ok := s.(SCIMStore); ok {
		rt.scim = db
	}
	rt.scimLimiter = newWindowRateLimiter(envInt(envSCIMFailuresPerIP, defaultSCIMFailuresPerIP), time.Minute)
	rt.samlKeys = &samlKeyCache{}
	rt.samlLimiter = newWindowRateLimiter(envInt(envSAMLRatePerMinute, defaultSAMLRatePerMinute), time.Minute)
}

// samlBaseURL is the https origin the SP entity ID and ACS URL hang off. It
// never comes from the request Host, so a forged Host cannot move the audience.
func (rt *Router) samlBaseURL(ctx context.Context) (string, error) {
	base := ""
	if rt.ingressSettings != nil {
		u, err := rt.ingressSettings.GetDashboardURL(ctx)
		if err != nil {
			return "", fmt.Errorf("read dashboard url: %w", err)
		}
		base = u
	}
	if base == "" {
		base = configuredDashboardURL()
	}
	u, err := url.Parse(strings.TrimRight(base, "/"))
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return "", errSAMLNeedsHTTPS
	}
	return u.Scheme + "://" + u.Host, nil
}

func samlEndpoints(base, connectionID string) (entityID, acs string) {
	prefix := base + samlPathPrefix + url.PathEscape(connectionID)
	return prefix + "/metadata", prefix + "/acs"
}

// samlKeyPair returns the SP keypair, generating and persisting it on first use.
func (rt *Router) samlKeyPair(ctx context.Context) (certPEM, keyPEM string, err error) {
	if rt.oauthSecrets == nil {
		return "", "", errSAMLNoMasterKey
	}
	c := rt.samlKeys
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cert != "" && c.key != "" {
		return c.cert, c.key, nil
	}
	settings, err := rt.sso.GetSSOSettings(ctx)
	if err != nil {
		return "", "", err
	}
	if settings.SPCertificatePEM == "" {
		if err := rt.generateSAMLKeyPair(ctx); err != nil {
			return "", "", err
		}
		if settings, err = rt.sso.GetSSOSettings(ctx); err != nil {
			return "", "", err
		}
	}
	key, err := rt.oauthSecrets.Resolve(ctx, samlSPKeyService, samlSPKeyEnv)
	if err != nil {
		return "", "", fmt.Errorf("load saml sp key: %w", err)
	}
	if _, _, err := samlsso.ParseSPKeyPair(settings.SPCertificatePEM, key); err != nil {
		return "", "", err
	}
	c.cert, c.key = settings.SPCertificatePEM, key
	return c.cert, c.key, nil
}

func (rt *Router) generateSAMLKeyPair(ctx context.Context) error {
	certPEM, keyPEM, err := samlsso.GenerateSPKeyPair(rt.brand.ShortName+" SAML SP", time.Now())
	if err != nil {
		return err
	}
	if err := rt.oauthSecrets.SetValue(ctx, samlSPKeyService, samlSPKeyEnv, keyPEM); err != nil {
		return fmt.Errorf("store saml sp key: %w", err)
	}
	if _, err := rt.sso.SetSSOSPCertificate(ctx, certPEM, time.Now()); err != nil {
		return err
	}
	return nil
}

// samlSPFor assembles the service provider for one connection.
func (rt *Router) samlSPFor(ctx context.Context, c store.SAMLConnection) (samlsso.SP, error) {
	base, err := rt.samlBaseURL(ctx)
	if err != nil {
		return samlsso.SP{}, err
	}
	certPEM, keyPEM, err := rt.samlKeyPair(ctx)
	if err != nil {
		return samlsso.SP{}, err
	}
	cert, key, err := samlsso.ParseSPKeyPair(certPEM, keyPEM)
	if err != nil {
		return samlsso.SP{}, err
	}
	entityID, acs := samlEndpoints(base, c.ID)
	return samlsso.SP{
		EntityID: entityID, ACSURL: acs, Cert: cert, Key: key,
		IDP: samlsso.IDP{EntityID: c.IDPEntityID, SSOURL: c.IDPSSOURL, CertificatesPEM: c.IDPCertificates},
	}, nil
}

func samlConfigured(c store.SAMLConnection) bool {
	return samlsso.IDP{EntityID: c.IDPEntityID, SSOURL: c.IDPSSOURL, CertificatesPEM: c.IDPCertificates}.Check() == nil
}

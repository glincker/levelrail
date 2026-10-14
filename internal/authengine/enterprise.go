package authengine

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	sqlitestore "github.com/glincker/theauth-go/storage/sqlite"
	theauth "github.com/glincker/theauth-go/v2"
	"github.com/oklog/ulid/v2"
)

// The instance is one library organization: it is the SAML and SCIM tenant.
// Teams stay on the platform side and never map onto library organizations.
const (
	defaultOrgSlug = "instance"
	defaultOrgName = "Instance"
	scimMaxPage    = 200
)

// applyEnterprise turns on the organization, SAML, SCIM, RBAC and account
// lifecycle capabilities of the library.
func applyEnterprise(tcfg *theauth.Config, cfg Config) {
	tcfg.Organizations = &theauth.OrganizationsConfig{}
	tcfg.RBAC = &theauth.RBACConfig{}
	// The platform enforces https for SCIM itself, with its trusted-proxy rules.
	tcfg.SCIM = &theauth.SCIMConfig{RequireHTTPS: false, MaxPageSize: scimMaxPage}
	if len(cfg.SAML.CertPEM) > 0 && len(cfg.SAML.KeyPEM) > 0 {
		tcfg.SAML = &theauth.SAMLConfig{SPCertificatePEM: cfg.SAML.CertPEM, SPPrivateKeyPEM: cfg.SAML.KeyPEM}
	}
}

// ensureDefaultOrg returns the instance organization, creating it on first
// start. It is created without an owner: nobody manages it through the library.
func ensureDefaultOrg(ctx context.Context, store *sqlitestore.Store) (theauth.ULID, error) {
	org, err := store.OrganizationBySlug(ctx, defaultOrgSlug)
	if err == nil {
		return org.ID, nil
	}
	if !errors.Is(err, theauth.ErrStorageNotFound) {
		return theauth.ULID{}, fmt.Errorf("authengine: look up instance organization: %w", err)
	}
	now := time.Now()
	created, err := store.InsertOrganization(ctx, theauth.Organization{
		ID: ulid.Make(), Name: defaultOrgName, Slug: defaultOrgSlug, CreatedAt: now, UpdatedAt: now,
	})
	if err == nil {
		return created.ID, nil
	}
	// A concurrent start may have inserted it first.
	if org, lerr := store.OrganizationBySlug(ctx, defaultOrgSlug); lerr == nil {
		return org.ID, nil
	}
	return theauth.ULID{}, fmt.Errorf("authengine: create instance organization: %w", err)
}

// OrganizationID is the library organization that holds SAML connections,
// SCIM tokens and directory users.
func (e *Engine) OrganizationID() string { return e.orgID.String() }

// SAMLEnabled reports whether the engine runs with an SP keypair.
func (e *Engine) SAMLEnabled() bool { return e.samlOn }

// publicHandler hides the library's own organization and SAML routes: the
// platform serves SAML through its own guarded routes and has no
// library-managed organizations to expose.
func publicHandler(prefix string, next http.Handler) http.Handler {
	blocked := []string{prefix + "/orgs", prefix + "/saml/"}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		for _, b := range blocked {
			if p == b || strings.HasPrefix(p, b+"/") || (strings.HasSuffix(b, "/") && strings.HasPrefix(p, b)) {
				http.NotFound(w, r)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

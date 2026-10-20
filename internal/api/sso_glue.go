package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/authengine"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	envSAMLRatePerMinute        = "APP_SAML_RATE_PER_MINUTE"
	defaultSAMLRatePerMinute    = 30
	envSCIMFailuresPerIP        = "APP_SCIM_AUTH_FAILURES_PER_MINUTE"
	defaultSCIMFailuresPerIP    = 30
	envSCIMRequestsPerToken     = "APP_SCIM_REQUESTS_PER_TOKEN_PER_MINUTE" //nolint:gosec // env var name
	defaultSCIMRequestsPerToken = 600
	samlPathPrefix              = "/api/v1/auth/saml/"
	scimBasePath                = "/api/v1/scim/v2"
	signInMethodSAML            = "saml"
)

var (
	errSAMLNoKeypair  = errors.New("SAML sign-in needs the control plane master key")
	errSAMLNeedsHTTPS = errors.New("SAML sign-in needs an https dashboard URL: set it under Settings, General")
	errSSOUnavailable = errors.New("single sign-on is not available on this control plane")
	errLastRoot       = errors.New("this is the last root user: grant root to someone else before deprovisioning it")
)

// SSOStore is the platform-side SSO state: connection policy, group to role
// mappings, enforcement and SCIM token labels. The IdP bindings, identities,
// directory and token hashes live in the auth library. *store.DB satisfies it.
type SSOStore interface {
	ListSSOConnections(ctx context.Context) ([]store.SSOConnection, error)
	GetSSOConnection(ctx context.Context, id string) (store.SSOConnection, error)
	SaveSSOConnection(ctx context.Context, c store.SSOConnection, create bool, now time.Time) error
	DeleteSSOConnection(ctx context.Context, id string) error
	ListSSOGroupMappings(ctx context.Context) ([]store.SSOGroupMapping, error)
	ReplaceSSOGroupMappings(ctx context.Context, mappings []store.SSOGroupMapping, now time.Time) error
	GetSSOSettings(ctx context.Context) (store.SSOSettings, error)
	SetSSOEnforce(ctx context.Context, on bool, now time.Time) error
	SetSCIMDefaultRole(ctx context.Context, roleID string, now time.Time) error
	SaveSCIMTokenMeta(ctx context.Context, tokenID string, m store.SCIMTokenMeta) error
	SCIMTokenMetas(ctx context.Context) (map[string]store.SCIMTokenMeta, error)
	CreateProvisionedUser(ctx context.Context, u store.User, roleID string) error
}

func (rt *Router) attachSSO(s Store) {
	if db, ok := s.(SSOStore); ok {
		rt.sso = db
	}
	if db, ok := s.(AccessPolicyStore); ok {
		rt.accessPolicy = db
	}
	rt.policyTouches = &touchCache{}
	rt.policyCapChecks = &touchCache{}
	rt.scimLimiter = newWindowRateLimiter(envInt(envSCIMFailuresPerIP, defaultSCIMFailuresPerIP), time.Minute)
	rt.scimTokenLimiter = newWindowRateLimiter(envInt(envSCIMRequestsPerToken, defaultSCIMRequestsPerToken), time.Minute)
	rt.samlLimiter = newWindowRateLimiter(envInt(envSAMLRatePerMinute, defaultSAMLRatePerMinute), time.Minute)
	rt.samlFlows = newSAMLFlowStore()
	rt.ssoTests = newSSOTestStore()
	rt.ssoRouteLimiter = newSSORouteLimiter()
	if rt.authEngine != nil {
		rt.scimLib = rt.authEngine.SCIMHandler(scimBasePath)
	}
}

func (rt *Router) ssoReady(w http.ResponseWriter) bool {
	if rt.sso == nil || rt.authEngine == nil {
		writeError(w, http.StatusNotImplemented, errSSOUnavailable.Error())
		return false
	}
	return true
}

// samlReady also needs the SP keypair, which comes from the master key.
func (rt *Router) samlReady(w http.ResponseWriter) bool {
	if !rt.ssoReady(w) {
		return false
	}
	if !rt.authEngine.SAMLEnabled() {
		writeError(w, http.StatusNotImplemented, errSAMLNoKeypair.Error())
		return false
	}
	return true
}

// accountDisabled reports whether a user is disabled; a lookup failure counts as disabled.
func (rt *Router) accountDisabled(ctx context.Context, userID string) bool {
	if rt.authEngine == nil {
		return false
	}
	disabled, err := rt.authEngine.UserDisabled(ctx, userID)
	if err != nil {
		rt.logger.Error("api: read disabled flag failed, refusing", slog.String("user_id", userID), slog.String("error", err.Error()))
		return true
	}
	return disabled
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

// scimBase is the absolute SCIM base URL, or the path alone without a dashboard URL.
func (rt *Router) scimBase(ctx context.Context) string {
	if base, err := rt.samlBaseURL(ctx); err == nil {
		return base + scimBasePath
	}
	return scimBasePath
}

// resolveSSORole picks the role of the highest priority mapping whose group
// the user is in, case-insensitively; ties go to the alphabetically first group.
func resolveSSORole(groups []string, mappings []store.SSOGroupMapping) (string, bool) {
	member := make(map[string]bool, len(groups))
	for _, g := range groups {
		member[strings.ToLower(strings.TrimSpace(g))] = true
	}
	var best *store.SSOGroupMapping
	for i := range mappings {
		m := &mappings[i]
		if m.RoleID == "" || !member[strings.ToLower(strings.TrimSpace(m.Group))] {
			continue
		}
		if best == nil || m.Priority > best.Priority ||
			(m.Priority == best.Priority && strings.ToLower(m.Group) < strings.ToLower(best.Group)) {
			best = m
		}
	}
	if best == nil {
		return "", false
	}
	return best.RoleID, true
}

// emailDomainAllowed reports whether email is in one of domains; an empty list allows all.
func emailDomainAllowed(email string, domains []string) bool {
	return authengine.EmailDomainAllowed(email, domains)
}

// errSSORoleGrantsRoot is returned when a claim or mapping would hand out root.
var errSSORoleGrantsRoot = errors.New("a sign-in claim cannot grant a role that holds root")

// ssoGrantableRole loads roleID and refuses one that holds root: only a
// person with root can grant root, never an assertion or a claim.
func (rt *Router) ssoGrantableRole(ctx context.Context, roleID string) error {
	role, err := rt.roles.GetRole(ctx, roleID)
	if err != nil {
		return fmt.Errorf("sso role %q: %w", roleID, err)
	}
	if slices.Contains(role.Abilities, AbilityRoot) {
		return errSSORoleGrantsRoot
	}
	return nil
}

// applySSORole moves a user to roleID. A user who holds root is never changed
// by SSO, and a role that holds root is never granted.
func (rt *Router) applySSORole(ctx context.Context, user store.User, roleID string) (store.User, error) {
	if slices.Contains(user.Abilities, AbilityRoot) {
		rt.logger.Warn("api: sso: left a root user's role alone", slog.String("user_id", user.ID))
		return user, nil
	}
	current, err := rt.roles.UserRoleID(ctx, user.ID)
	if err != nil {
		return store.User{}, err
	}
	if current == roleID {
		return user, nil
	}
	if err := rt.ssoGrantableRole(ctx, roleID); err != nil {
		return store.User{}, err
	}
	if err := rt.roles.SetUserRole(ctx, user.ID, roleID); err != nil {
		return store.User{}, err
	}
	updated, err := rt.auth.GetUserByID(ctx, user.ID)
	if err != nil {
		return store.User{}, err
	}
	return *updated, nil
}

// guardLastRoot refuses to take the last root user out of service.
func (rt *Router) guardLastRoot(ctx context.Context, userID string) error {
	user, err := rt.auth.GetUserByID(ctx, userID)
	if err != nil {
		return err
	}
	if !slices.Contains(user.Abilities, AbilityRoot) {
		return nil
	}
	others, err := rt.roles.CountRootUsersExcept(ctx, userID)
	if err != nil {
		return err
	}
	if others == 0 {
		return errLastRoot
	}
	return nil
}

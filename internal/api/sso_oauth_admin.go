package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	ssoKindSAML  = "saml"
	ssoKindOAuth = "oauth"

	defaultGroupsClaim = "groups"
	maxGroupsClaimLen  = 256
)

// oauthSSOPolicyResource is the SAML-style sign-in policy of one OAuth or
// OIDC provider. It carries no credential.
type oauthSSOPolicyResource struct {
	Provider        string   `json:"provider"`
	Name            string   `json:"name"`
	Enabled         bool     `json:"enabled"`
	GroupsSupported bool     `json:"groups_supported"`
	GroupsClaim     string   `json:"groups_claim"`
	SyncRoles       bool     `json:"sync_roles"`
	DefaultRoleID   string   `json:"default_role_id"`
	JITProvisioning bool     `json:"jit_provisioning"`
	AllowedDomains  []string `json:"allowed_domains"`
}

type oauthSSOPolicyRequest struct {
	GroupsClaim     string   `json:"groups_claim"`
	SyncRoles       bool     `json:"sync_roles"`
	DefaultRoleID   string   `json:"default_role_id"`
	JITProvisioning bool     `json:"jit_provisioning"`
	AllowedDomains  []string `json:"allowed_domains"`
}

type oauthSSOEvaluateRequest struct {
	Email         string   `json:"email"`
	EmailVerified bool     `json:"email_verified"`
	Groups        []string `json:"groups"`
}

// groupsClaimSupported is true where the sign-in reads an ID token the library
// verifies: the other providers return no token the groups could come from.
func groupsClaimSupported(provider string) bool {
	return provider == store.OAuthProviderOIDC || provider == store.OAuthProviderCognito
}

func toOAuthSSOPolicyResource(s store.OAuthProviderSettings) oauthSSOPolicyResource {
	p := s.SSOPolicy
	role := p.DefaultRoleID
	if role == "" {
		role = defaultOIDCRoleID
	}
	claim := p.GroupsClaim
	if claim == "" {
		claim = defaultGroupsClaim
	}
	return oauthSSOPolicyResource{Provider: s.Provider, Name: oauthDisplayName(s), Enabled: s.Enabled,
		GroupsSupported: groupsClaimSupported(s.Provider), GroupsClaim: claim, SyncRoles: p.SyncRoles,
		DefaultRoleID: role, JITProvisioning: p.JITProvisioning, AllowedDomains: nonNilStrings(p.AllowedDomains)}
}

func (rt *Router) oauthPolicyReady(w http.ResponseWriter) bool {
	if rt.oauthSettings == nil {
		writeError(w, http.StatusNotImplemented, errSSOUnavailable.Error())
		return false
	}
	return true
}

func (rt *Router) loadOAuthSettings(w http.ResponseWriter, r *http.Request) (store.OAuthProviderSettings, bool) {
	provider := r.PathValue("provider")
	if !isValidOAuthProvider(provider) {
		writeError(w, http.StatusNotFound, "unknown oauth provider")
		return store.OAuthProviderSettings{}, false
	}
	if !rt.oauthPolicyReady(w) {
		return store.OAuthProviderSettings{}, false
	}
	s, err := rt.oauthSettings.GetOAuthProviderSettings(r.Context(), provider)
	if err != nil {
		rt.internalError(w, "api: load oauth sso policy failed", err)
		return store.OAuthProviderSettings{}, false
	}
	return s, true
}

// handleListOAuthSSOPolicies lists the providers that are enabled or have a client id.
func (rt *Router) handleListOAuthSSOPolicies(w http.ResponseWriter, r *http.Request) {
	if !rt.oauthPolicyReady(w) {
		return
	}
	all, err := rt.oauthSettings.ListOAuthProviderSettings(r.Context())
	if err != nil {
		rt.internalError(w, "api: list oauth sso policies failed", err)
		return
	}
	out := []oauthSSOPolicyResource{}
	for _, s := range all {
		if s.Enabled || s.ClientID != "" {
			out = append(out, toOAuthSSOPolicyResource(s))
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (rt *Router) handleGetOAuthSSOPolicy(w http.ResponseWriter, r *http.Request) {
	if s, ok := rt.loadOAuthSettings(w, r); ok {
		writeJSON(w, http.StatusOK, toOAuthSSOPolicyResource(s))
	}
}

// handlePutOAuthSSOPolicy replaces one provider's policy. A role that holds
// root and a domain another connection already claims are both refused.
func (rt *Router) handlePutOAuthSSOPolicy(w http.ResponseWriter, r *http.Request) {
	s, ok := rt.loadOAuthSettings(w, r)
	if !ok {
		return
	}
	var req oauthSSOPolicyRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	p, status, err := rt.validateOAuthSSOPolicy(r, s.Provider, req)
	if err != nil {
		writeError(w, status, err.Error())
		return
	}
	if err := rt.oauthSettings.SetOAuthSSOPolicy(r.Context(), s.Provider, p); err != nil {
		rt.internalError(w, "api: save oauth sso policy failed", err)
		return
	}
	rt.authLibOAuthInvalidate(s.Provider)
	s.SSOPolicy = p
	writeJSON(w, http.StatusOK, toOAuthSSOPolicyResource(s))
}

func (rt *Router) validateOAuthSSOPolicy(r *http.Request, provider string, req oauthSSOPolicyRequest) (store.OAuthSSOPolicy, int, error) {
	claim := strings.TrimSpace(req.GroupsClaim)
	if claim == "" {
		claim = defaultGroupsClaim
	}
	if len(claim) > maxGroupsClaimLen {
		return store.OAuthSSOPolicy{}, http.StatusBadRequest, errors.New("groups_claim is at most 256 characters")
	}
	role := req.DefaultRoleID
	if role == "" {
		role = defaultOIDCRoleID
	}
	if err := rt.ssoGrantableRole(r.Context(), role); err != nil {
		if errors.Is(err, errSSORoleGrantsRoot) {
			return store.OAuthSSOPolicy{}, http.StatusBadRequest, errors.New("default_role_id cannot be a role that holds root")
		}
		if errors.Is(err, store.ErrRoleNotFound) {
			return store.OAuthSSOPolicy{}, http.StatusBadRequest, errors.New("default_role_id does not name a stored role")
		}
		return store.OAuthSSOPolicy{}, http.StatusInternalServerError, errors.New("internal error")
	}
	domains, err := normalizeSAMLDomains(req.AllowedDomains)
	if err != nil {
		return store.OAuthSSOPolicy{}, http.StatusBadRequest, err
	}
	for _, d := range domains {
		owner, err := rt.ssoDomainOwner(r.Context(), d, ssoKindOAuth, provider)
		if err != nil {
			return store.OAuthSSOPolicy{}, http.StatusInternalServerError, errors.New("internal error")
		}
		if owner != "" {
			return store.OAuthSSOPolicy{}, http.StatusConflict, errors.New("allowed_domains: " + d + " is already claimed by " + owner)
		}
	}
	return store.OAuthSSOPolicy{GroupsClaim: claim, SyncRoles: req.SyncRoles, DefaultRoleID: role,
		JITProvisioning: req.JITProvisioning, AllowedDomains: domains}, 0, nil
}

// handleEvaluateOAuthSSO answers what a sign-in with the posted facts would do,
// without creating or changing anything.
func (rt *Router) handleEvaluateOAuthSSO(w http.ResponseWriter, r *http.Request) {
	s, ok := rt.loadOAuthSettings(w, r)
	if !ok {
		return
	}
	var req oauthSSOEvaluateRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	facts := evaluationFacts(s.Provider, req)
	ev, err := rt.evaluateOAuthSignIn(r.Context(), s.Provider, withDefaultClaim(s.SSOPolicy), facts)
	if err != nil {
		rt.internalError(w, "api: evaluate oauth sso failed", err)
		return
	}
	writeJSON(w, http.StatusOK, ev)
}

func withDefaultClaim(p store.OAuthSSOPolicy) store.OAuthSSOPolicy {
	if p.GroupsClaim == "" {
		p.GroupsClaim = defaultGroupsClaim
	}
	return p
}

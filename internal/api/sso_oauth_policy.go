package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"slices"

	"github.com/GLINCKER/levelrail/internal/authengine"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	defaultOIDCRoleID = "role_viewer"

	ssoRoleSourceMapping = "mapping"
	ssoRoleSourceDefault = "default"

	auditActionOIDCSignedIn    = "sso.oidc.signed_in"
	auditActionOIDCRejected    = "sso.oidc.rejected"
	auditActionOIDCProvisioned = "sso.oidc.user_provisioned"
	auditActionOIDCRoleChanged = "sso.oidc.role_changed"

	oauthCallbackAuditPath = "/api/v1/auth/oauth/"
)

// oauthSettled is a signed-in OAuth user after the SSO policy ran.
type oauthSettled struct {
	user    store.User
	session string
	created bool
}

// oauthDesiredRole picks the role a sign-in earns: the best group mapping, else
// the provider's default role (viewer unless set). It never invents a role.
func (rt *Router) oauthDesiredRole(ctx context.Context, p store.OAuthSSOPolicy, groups []string) (roleID, source string, err error) {
	var mappings []store.SSOGroupMapping
	if rt.sso != nil {
		if mappings, err = rt.sso.ListSSOGroupMappings(ctx); err != nil {
			return "", "", err
		}
	}
	if id, ok := resolveSSORole(groups, mappings); ok {
		return id, ssoRoleSourceMapping, nil
	}
	if p.DefaultRoleID != "" {
		return p.DefaultRoleID, ssoRoleSourceDefault, nil
	}
	return defaultOIDCRoleID, ssoRoleSourceDefault, nil
}

func (rt *Router) oauthPolicy(ctx context.Context, provider string) (store.OAuthSSOPolicy, error) {
	if rt.oauthSettings == nil {
		return store.OAuthSSOPolicy{}, nil
	}
	s, err := rt.oauthSettings.GetOAuthProviderSettings(ctx, provider)
	if err != nil {
		return store.OAuthSSOPolicy{}, err
	}
	return s.SSOPolicy, nil
}

// unauditedOAuthCodes are refusals an anonymous request can cause without the
// identity provider vouching for anyone, so they are logged but not audited.
var unauditedOAuthCodes = []string{authengine.OAuthErrInvalidProvider, authengine.OAuthErrInvalidRequest,
	authengine.OAuthErrInvalidState, authengine.OAuthErrProviderDenied}

// auditOAuthReject records a refused OAuth sign-in with its short reason code.
func (rt *Router) auditOAuthReject(r *http.Request, provider, code string) {
	if slices.Contains(unauditedOAuthCodes, code) {
		return
	}
	rt.auditSignIn(r.Context(), r, anonymousSignIn(""), auditActionOIDCRejected,
		oauthCallbackAuditPath+provider+"/callback?code="+url.QueryEscape(code), http.StatusForbidden)
}

func (rt *Router) auditOAuth(r *http.Request, user store.User, action, provider string, extra url.Values) {
	path := oauthCallbackAuditPath + provider + "/callback"
	if len(extra) > 0 {
		path += "?" + extra.Encode()
	}
	rt.auditSignIn(r.Context(), r, signInActor{kind: auditActorTypeSession, id: user.ID, name: user.Email}, action, path, http.StatusFound)
}

// oauthRollback removes a platform user this sign-in created.
func (rt *Router) oauthRollback(ctx context.Context, userID string) {
	if err := rt.auth.DeleteUser(context.WithoutCancel(ctx), userID); err != nil {
		rt.logger.Error("api: oauth: roll back provisioned user failed", slog.String("user_id", userID), slog.String("error", err.Error()))
	}
}

// settleOAuthUser applies the provider's SSO policy to a verified sign-in: the
// platform user is resolved (created only when JIT allowed it, with the role
// the claims earn), and role sync runs for an existing user. Any failure
// revokes the session and undoes what this call created.
func (rt *Router) settleOAuthUser(w http.ResponseWriter, r *http.Request, provider string, out authengine.OAuthOutcome) (oauthSettled, bool) {
	ctx := r.Context()
	session := out.SessionCookie.Value
	var created bool
	var userID string
	refuse := func(code string, err error) {
		rt.libSessions.Revoke(ctx, session)
		if created {
			rt.oauthRollback(ctx, userID)
		}
		attrs := []any{slog.String("provider", provider), slog.String("code", code)}
		if err != nil {
			attrs = append(attrs, slog.String("error", err.Error()))
		}
		rt.logger.Warn("api: oauth sign-in refused", attrs...)
		rt.auditOAuthReject(r, provider, code)
		redirectOAuthError(w, r, code)
	}
	policy, err := rt.oauthPolicy(ctx, provider)
	if err != nil {
		refuse(samlErrInternal, err)
		return oauthSettled{}, false
	}
	desired, _, err := rt.oauthDesiredRole(ctx, policy, out.Facts.Claims.Groups)
	if err != nil {
		refuse(samlErrInternal, err)
		return oauthSettled{}, false
	}
	grantErr := rt.ssoGrantableRole(ctx, desired)
	newUserRole := desired
	if grantErr != nil {
		newUserRole = ""
	}
	userID, created, err = rt.authLibOAuth.ResolveOAuthUser(ctx, out.Provider, out.ProviderUserID, newUserRole)
	if err != nil {
		if grantErr != nil {
			refuse(samlErrNoRole, grantErr)
		} else {
			refuse(samlErrInternal, err)
		}
		return oauthSettled{}, false
	}
	if created && !policy.JITProvisioning {
		refuse(samlErrNotProvisioned, nil)
		return oauthSettled{}, false
	}
	user, err := rt.auth.GetUserByID(ctx, userID)
	if err != nil {
		refuse(samlErrInternal, err)
		return oauthSettled{}, false
	}
	if rt.accountDisabled(ctx, user.ID) {
		refuse(samlErrDisabled, nil)
		return oauthSettled{}, false
	}
	switch {
	case created:
		rt.auditOAuth(r, *user, auditActionOIDCProvisioned, provider, url.Values{"role": {desired}})
	case policy.SyncRoles:
		synced, err := rt.syncOAuthRole(r, provider, *user, desired)
		if err != nil {
			code := samlErrInternal
			if errors.Is(err, errSSORoleGrantsRoot) {
				code = samlErrNoRole
			}
			refuse(code, err)
			return oauthSettled{}, false
		}
		user = &synced
	}
	return oauthSettled{user: *user, session: session, created: created}, true
}

// syncOAuthRole moves an existing user to desired and audits a change, marking
// a loss of abilities as a downgrade.
func (rt *Router) syncOAuthRole(r *http.Request, provider string, user store.User, desired string) (store.User, error) {
	ctx := r.Context()
	before, err := rt.roles.UserRoleID(ctx, user.ID)
	if err != nil {
		return store.User{}, err
	}
	synced, err := rt.applySSORole(ctx, user, desired)
	if err != nil {
		return store.User{}, err
	}
	after, err := rt.roles.UserRoleID(ctx, user.ID)
	if err != nil {
		return store.User{}, err
	}
	if before != after {
		direction := "change"
		switch {
		case isStrictSubset(synced.Abilities, user.Abilities):
			direction = "downgrade"
		case isStrictSubset(user.Abilities, synced.Abilities):
			direction = "upgrade"
		}
		rt.auditOAuth(r, synced, auditActionOIDCRoleChanged, provider, url.Values{"from": {before}, "to": {after}, "direction": {direction}})
	}
	return synced, nil
}

func isStrictSubset(small, big []string) bool {
	if len(small) >= len(big) {
		return false
	}
	for _, a := range small {
		if !slices.Contains(big, a) {
			return false
		}
	}
	return true
}

// finishOAuthSignIn holds a sign-in from a new browser for approval or
// completes the session, then audits the sign-in.
func (rt *Router) finishOAuthSignIn(w http.ResponseWriter, r *http.Request, provider string, s oauthSettled) {
	ctx := r.Context()
	hold, err := rt.holdForApproval(w, r, s.user, s.session, signInMethodOAuth)
	switch {
	case err != nil:
		rt.logger.Error("api: oauth callback: new browser approval failed", slog.String("user_id", s.user.ID), slog.String("error", err.Error()))
		rt.libSessions.Revoke(ctx, s.session)
		if s.created {
			rt.oauthRollback(ctx, s.user.ID)
		}
		rt.auditOAuthReject(r, provider, samlErrInternal)
		redirectOAuthError(w, r, samlErrInternal)
		return
	case hold != nil && hold.limited:
		redirectOAuthError(w, r, samlErrRateLimited)
		return
	case hold != nil:
		http.Redirect(w, r, "/login?approval="+url.QueryEscape(hold.approval.ID), http.StatusFound)
		return
	}
	rt.finishLibSession(w, r, s.user, s.session)
	rt.auditOAuth(r, s.user, auditActionOIDCSignedIn, provider, nil)
	http.Redirect(w, r, "/oauth/complete", http.StatusFound)
}

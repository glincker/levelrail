package api

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/GLINCKER/levelrail/internal/authengine"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	ssoAccountLinked     = "linked"
	ssoAccountByEmail    = "existing_by_email"
	ssoAccountNew        = "new_account"
	ssoAccountNotAllowed = "not_provisioned"

	ssoFixAuto   = "auto"
	ssoFixManual = "manual"

	ssoIssueNoGroups        = "no_groups_claim"
	ssoIssueWrongClaim      = "wrong_claim_name"
	ssoIssueEmailUnverified = "email_unverified"
	ssoIssueDomain          = "domain_not_allowed"
	ssoIssueNotProvisioned  = "not_provisioned"
	ssoIssueRoleIsRoot      = "role_grants_root"
	ssoIssueNoEmail         = "no_email"
)

// ssoFix is a typed correction. An auto fix names the policy field it sets, so
// the dashboard can apply it with one button.
type ssoFix struct {
	Kind   string `json:"kind"`
	Action string `json:"action,omitempty"`
	Value  string `json:"value,omitempty"`
}

type ssoIssue struct {
	Code string `json:"code"`
	Fix  ssoFix `json:"fix"`
}

// ssoEvaluation says what a sign-in with these facts would do under a
// provider's policy. Computing it changes nothing.
type ssoEvaluation struct {
	Provider        string     `json:"provider"`
	Email           string     `json:"email"`
	EmailVerified   bool       `json:"email_verified"`
	GroupsClaim     string     `json:"groups_claim"`
	GroupsFound     bool       `json:"groups_found"`
	Groups          []string   `json:"groups"`
	GroupLikeClaims []string   `json:"group_like_claims"`
	RoleID          string     `json:"role_id"`
	RoleSource      string     `json:"role_source"`
	AccountState    string     `json:"account_state"`
	SyncRoles       bool       `json:"sync_roles"`
	Allowed         bool       `json:"allowed"`
	RefusalCode     string     `json:"refusal_code,omitempty"`
	Issues          []ssoIssue `json:"issues"`
}

// evaluateOAuthSignIn mirrors the order the real sign-in applies its rules.
func (rt *Router) evaluateOAuthSignIn(ctx context.Context, provider string, policy store.OAuthSSOPolicy, f authengine.OAuthIdentityFacts) (ssoEvaluation, error) {
	ev := ssoEvaluation{
		Provider: provider, Email: f.Email, EmailVerified: f.EmailVerified, GroupsClaim: policy.GroupsClaim,
		GroupsFound: f.Claims.GroupsFound, Groups: nonNilStrings(f.Claims.Groups), GroupLikeClaims: nonNilStrings(f.Claims.GroupLikeClaims),
		SyncRoles: policy.SyncRoles, Allowed: true, Issues: []ssoIssue{},
	}
	refuse := func(code string) {
		if ev.Allowed {
			ev.Allowed, ev.RefusalCode = false, code
		}
	}
	role, source, err := rt.oauthDesiredRole(ctx, policy, f.Claims.Groups)
	if err != nil {
		return ssoEvaluation{}, err
	}
	ev.RoleID, ev.RoleSource = role, source
	if err := rt.ssoGrantableRole(ctx, role); err != nil {
		if !errors.Is(err, errSSORoleGrantsRoot) && !errors.Is(err, store.ErrRoleNotFound) {
			return ssoEvaluation{}, err
		}
		ev.Issues = append(ev.Issues, ssoIssue{Code: ssoIssueRoleIsRoot, Fix: ssoFix{Kind: ssoFixManual}})
		refuse(samlErrNoRole)
	}
	if f.Email == "" {
		ev.Issues = append(ev.Issues, ssoIssue{Code: ssoIssueNoEmail, Fix: ssoFix{Kind: ssoFixManual}})
		refuse(authengine.OAuthErrUserInfoFailed)
	} else if !f.EmailVerified {
		ev.Issues = append(ev.Issues, ssoIssue{Code: ssoIssueEmailUnverified, Fix: ssoFix{Kind: ssoFixManual}})
		refuse(authengine.OAuthErrUserInfoFailed)
	}
	if f.Email != "" && !authengine.EmailDomainAllowed(f.Email, policy.AllowedDomains) {
		domain := f.Email[strings.LastIndexByte(f.Email, '@')+1:]
		ev.Issues = append(ev.Issues, ssoIssue{Code: ssoIssueDomain, Fix: ssoFix{Kind: ssoFixAuto, Action: "add_domain", Value: strings.ToLower(domain)}})
		refuse(authengine.OAuthErrDomainNotAllowed)
	}
	if err := rt.evaluateAccount(ctx, provider, policy, f, &ev, refuse); err != nil {
		return ssoEvaluation{}, err
	}
	if !f.Claims.GroupsFound {
		issue := ssoIssue{Code: ssoIssueNoGroups, Fix: ssoFix{Kind: ssoFixManual}}
		if len(f.Claims.GroupLikeClaims) > 0 {
			issue = ssoIssue{Code: ssoIssueWrongClaim, Fix: ssoFix{Kind: ssoFixAuto, Action: "set_groups_claim", Value: f.Claims.GroupLikeClaims[0]}}
		}
		ev.Issues = append(ev.Issues, issue)
	}
	return ev, nil
}

func (rt *Router) evaluateAccount(ctx context.Context, provider string, policy store.OAuthSSOPolicy, f authengine.OAuthIdentityFacts, ev *ssoEvaluation, refuse func(string)) error {
	ev.AccountState = ssoAccountNotAllowed
	if rt.oauthIdentities != nil && f.ProviderUserID != "" {
		if _, err := rt.oauthIdentities.GetOAuthIdentity(ctx, provider, f.ProviderUserID); err == nil {
			ev.AccountState = ssoAccountLinked
			return nil
		} else if !errors.Is(err, store.ErrOAuthIdentityNotFound) {
			return err
		}
	}
	if f.Email != "" {
		_, err := rt.auth.GetUserByEmail(ctx, f.Email)
		switch {
		case err == nil:
			ev.AccountState = ssoAccountByEmail
			return nil
		case !errors.Is(err, store.ErrUserNotFound):
			return err
		}
	}
	if policy.JITProvisioning {
		ev.AccountState = ssoAccountNew
		return nil
	}
	ev.Issues = append(ev.Issues, ssoIssue{Code: ssoIssueNotProvisioned, Fix: ssoFix{Kind: ssoFixAuto, Action: "enable_jit"}})
	refuse(authengine.OAuthErrNotProvisioned)
	return nil
}

// ssoDomainOwner names the connection that already claims domain, ignoring the
// connection identified by exceptKind and exceptID.
func (rt *Router) ssoDomainOwner(ctx context.Context, domain, exceptKind, exceptID string) (string, error) {
	if rt.sso != nil {
		conns, err := rt.sso.ListSSOConnections(ctx)
		if err != nil {
			return "", err
		}
		for _, c := range conns {
			if (exceptKind != ssoKindSAML || exceptID != c.ID) && slices.Contains(c.AllowedDomains, domain) {
				return c.Name, nil
			}
		}
	}
	if rt.oauthSettings != nil {
		all, err := rt.oauthSettings.ListOAuthProviderSettings(ctx)
		if err != nil {
			return "", err
		}
		for _, s := range all {
			if (exceptKind != ssoKindOAuth || exceptID != s.Provider) && slices.Contains(s.SSOPolicy.AllowedDomains, domain) {
				return oauthDisplayName(s), nil
			}
		}
	}
	return "", nil
}

func oauthDisplayName(s store.OAuthProviderSettings) string {
	if s.DisplayName != "" {
		return s.DisplayName
	}
	return s.Provider
}

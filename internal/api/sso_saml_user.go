package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/GLINCKER/levelrail/internal/samlsso"
	"github.com/GLINCKER/levelrail/internal/store"
)

// ssoDesiredRole is the role the group mappings give, else the fallback role.
func (rt *Router) ssoDesiredRole(ctx context.Context, groups []string, fallback string) (string, error) {
	mappings, err := rt.sso.ListSSOGroupMappings(ctx)
	if err != nil {
		return "", err
	}
	rules := make([]samlsso.GroupRole, 0, len(mappings))
	for _, m := range mappings {
		rules = append(rules, samlsso.GroupRole{Group: m.Group, RoleID: m.RoleID, Priority: m.Priority})
	}
	if role, ok := samlsso.ResolveRole(groups, rules); ok {
		return role, nil
	}
	return fallback, nil
}

// samlResolveUser maps a validated identity onto a user: the linked subject
// first, then an existing account with the same email, then JIT creation when
// the connection allows it. A non-empty code means the sign-in is refused.
func (rt *Router) samlResolveUser(ctx context.Context, c store.SAMLConnection, ident samlsso.Identity) (store.User, string, error) {
	if len(c.AllowedDomains) > 0 && (ident.Email == "" || !samlsso.EmailDomainAllowed(ident.Email, c.AllowedDomains)) {
		return store.User{}, samlErrDomain, nil
	}
	desired, err := rt.ssoDesiredRole(ctx, ident.Groups, c.DefaultRoleID)
	if err != nil {
		return store.User{}, samlErrInternal, err
	}
	now := time.Now()
	uid, err := rt.sso.SAMLIdentityUser(ctx, c.ID, ident.NameID)
	switch {
	case err == nil:
	case !errors.Is(err, store.ErrSAMLIdentityNotFound):
		return store.User{}, samlErrInternal, err
	case ident.Email == "":
		return store.User{}, samlErrNoEmail, nil
	default:
		existing, gerr := rt.auth.GetUserByEmail(ctx, ident.Email)
		if errors.Is(gerr, store.ErrUserNotFound) {
			return rt.samlProvision(ctx, c, ident, desired, now)
		}
		if gerr != nil {
			return store.User{}, samlErrInternal, gerr
		}
		if lerr := rt.sso.LinkSAMLIdentity(ctx, c.ID, ident.NameID, existing.ID, now); lerr != nil {
			if errors.Is(lerr, store.ErrOAuthIdentityAlreadyLinked) {
				return store.User{}, samlErrLinked, nil
			}
			return store.User{}, samlErrInternal, lerr
		}
		uid = existing.ID
	}
	user, err := rt.auth.GetUserByID(ctx, uid)
	if err != nil {
		return store.User{}, samlErrInternal, err
	}
	if !c.SyncRoles {
		return *user, "", nil
	}
	if desired == "" {
		return store.User{}, samlErrNoRole, nil
	}
	synced, err := rt.applySSORole(ctx, *user, desired)
	if err != nil {
		return store.User{}, samlErrInternal, err
	}
	return synced, "", nil
}

func (rt *Router) samlProvision(ctx context.Context, c store.SAMLConnection, ident samlsso.Identity, roleID string, now time.Time) (store.User, string, error) {
	if !c.JITProvisioning {
		return store.User{}, samlErrNotProvisioned, nil
	}
	if roleID == "" {
		return store.User{}, samlErrNoRole, nil
	}
	id, err := randomOpaqueID("user_")
	if err != nil {
		return store.User{}, samlErrInternal, err
	}
	name := ident.DisplayName
	if name == "" {
		name = ident.Email
	}
	u := store.User{ID: id, Email: ident.Email, DisplayName: name, CreatedAt: now}
	if err := rt.sso.CreateProvisionedUser(ctx, store.ProvisionedUser{User: u, RoleID: roleID, SAMLConnectionID: c.ID, SAMLNameID: ident.NameID}); err != nil {
		return store.User{}, samlErrInternal, fmt.Errorf("provision saml user: %w", err)
	}
	created, err := rt.auth.GetUserByID(ctx, id)
	if err != nil {
		return store.User{}, samlErrInternal, err
	}
	rt.logger.Info("api: saml: provisioned user", slog.String("user_id", id), slog.String("connection_id", c.ID), slog.String("role_id", roleID))
	return *created, "", nil
}

// applySSORole moves a user to roleID unless that would strip root from the
// last root user, which is kept so directory drift can never lock out the instance.
func (rt *Router) applySSORole(ctx context.Context, user store.User, roleID string) (store.User, error) {
	current, err := rt.roles.UserRoleID(ctx, user.ID)
	if err != nil {
		return store.User{}, err
	}
	if current == roleID {
		return user, nil
	}
	role, err := rt.roles.GetRole(ctx, roleID)
	if err != nil {
		return store.User{}, fmt.Errorf("sso role %q: %w", roleID, err)
	}
	if slices.Contains(user.Abilities, AbilityRoot) && !slices.Contains(role.Abilities, AbilityRoot) {
		others, err := rt.roles.CountRootUsersExcept(ctx, user.ID)
		if err != nil {
			return store.User{}, err
		}
		if others == 0 {
			rt.logger.Warn("api: sso: kept root on the last root user", slog.String("user_id", user.ID), slog.String("role_id", roleID))
			return user, nil
		}
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

package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/GLINCKER/levelrail/internal/authengine"
	"github.com/GLINCKER/levelrail/internal/store"
)

// samlPlan is what the platform decided before the library runs: which
// library user must end up owning the session, and whether this sign-in
// created the platform user (and so must remove it if anything fails).
type samlPlan struct {
	expectEngine string
	legacyID     string
	desiredRole  string
	created      bool
}

// samlPlanSignIn applies the platform policy to a verified assertion: domain
// allowlist, disabled accounts, and JIT. The account the library will find is
// prepared here, so the library only ever links, never creates.
func (rt *Router) samlPlanSignIn(ctx context.Context, c store.SSOConnection, ident authengine.SAMLAssertion) (samlPlan, string, error) {
	if ident.Email == "" {
		return samlPlan{}, samlErrNoEmail, nil
	}
	if !emailDomainAllowed(ident.Email, c.AllowedDomains) {
		return samlPlan{}, samlErrDomain, nil
	}
	mappings, err := rt.sso.ListSSOGroupMappings(ctx)
	if err != nil {
		return samlPlan{}, samlErrInternal, err
	}
	desired, ok := resolveSSORole(ident.Groups, mappings)
	if !ok {
		desired = c.DefaultRoleID
	}
	linked, err := rt.authEngine.SAMLIdentityUser(ctx, c.LibraryID, ident.NameID)
	if err != nil {
		return samlPlan{}, samlErrInternal, err
	}
	if linked != "" {
		legacy, err := rt.authEngine.LegacyUserFor(ctx, linked)
		if err != nil {
			return samlPlan{}, samlErrInternal, err
		}
		if legacy == "" {
			return samlPlan{}, samlErrNotProvisioned, errors.New("linked library user has no platform account")
		}
		if rt.accountDisabled(ctx, legacy) {
			return samlPlan{}, samlErrDisabled, nil
		}
		return samlPlan{expectEngine: linked, legacyID: legacy, desiredRole: desired}, "", nil
	}
	existing, err := rt.auth.GetUserByEmail(ctx, ident.Email)
	switch {
	case err == nil:
		// A mailbox match must never hand a root account to the identity provider.
		if slices.Contains(existing.Abilities, AbilityRoot) {
			return samlPlan{}, samlErrRootLink, errors.New("an existing root account is not linked by email")
		}
		if rt.accountDisabled(ctx, existing.ID) {
			return samlPlan{}, samlErrDisabled, nil
		}
		engine, err := rt.libSessions.SyncUser(ctx, toLibUser(*existing))
		if err != nil {
			return samlPlan{}, samlErrInternal, err
		}
		return samlPlan{expectEngine: engine, legacyID: existing.ID, desiredRole: desired}, "", nil
	case !errors.Is(err, store.ErrUserNotFound):
		return samlPlan{}, samlErrInternal, err
	}
	return rt.samlProvision(ctx, c, ident, desired)
}

func (rt *Router) samlProvision(ctx context.Context, c store.SSOConnection, ident authengine.SAMLAssertion, roleID string) (samlPlan, string, error) {
	if !c.JITProvisioning {
		return samlPlan{}, samlErrNotProvisioned, nil
	}
	if roleID == "" {
		return samlPlan{}, samlErrNoRole, nil
	}
	if err := rt.ssoGrantableRole(ctx, roleID); errors.Is(err, errSSORoleGrantsRoot) {
		return samlPlan{}, samlErrNoRole, err
	}
	id, err := randomOpaqueID("user_")
	if err != nil {
		return samlPlan{}, samlErrInternal, err
	}
	name := ident.DisplayName
	if name == "" {
		name = ident.Email
	}
	u := store.User{ID: id, Email: ident.Email, DisplayName: name, CreatedAt: time.Now()}
	if err := rt.sso.CreateProvisionedUser(ctx, u, roleID); err != nil {
		return samlPlan{}, samlErrInternal, fmt.Errorf("provision saml user: %w", err)
	}
	plan := samlPlan{legacyID: id, desiredRole: roleID, created: true}
	engine, err := rt.libSessions.SyncUser(ctx, toLibUser(u))
	if err != nil {
		rt.samlRollback(ctx, plan)
		return samlPlan{}, samlErrInternal, err
	}
	plan.expectEngine = engine
	rt.logger.Info("api: saml: provisioned user", slog.String("user_id", id), slog.String("connection_id", c.ID), slog.String("role_id", roleID))
	return plan, "", nil
}

// samlSettleUser checks the library issued the session to the account the
// plan prepared, then applies role sync.
func (rt *Router) samlSettleUser(ctx context.Context, c store.SSOConnection, plan samlPlan, engineUser string) (store.User, string, error) {
	if engineUser != plan.expectEngine {
		return store.User{}, samlErrInternal, errors.New("library signed in a different user than planned")
	}
	user, err := rt.auth.GetUserByID(ctx, plan.legacyID)
	if err != nil {
		return store.User{}, samlErrInternal, err
	}
	if rt.accountDisabled(ctx, user.ID) {
		return store.User{}, samlErrDisabled, nil
	}
	if !c.SyncRoles || plan.created {
		return *user, "", nil
	}
	if plan.desiredRole == "" {
		return store.User{}, samlErrNoRole, nil
	}
	synced, err := rt.applySSORole(ctx, *user, plan.desiredRole)
	if errors.Is(err, errSSORoleGrantsRoot) {
		return store.User{}, samlErrNoRole, err
	}
	if err != nil {
		return store.User{}, samlErrInternal, err
	}
	return synced, "", nil
}

// samlRollback removes a platform user this sign-in created; the library side
// (user, identity, membership) goes with it.
func (rt *Router) samlRollback(ctx context.Context, plan samlPlan) {
	if !plan.created {
		return
	}
	if err := rt.auth.DeleteUser(context.WithoutCancel(ctx), plan.legacyID); err != nil {
		rt.logger.Error("api: saml: roll back provisioned user failed", slog.String("user_id", plan.legacyID), slog.String("error", err.Error()))
	}
}

package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"time"

	"github.com/GLINCKER/levelrail/internal/samlsso"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	auditActionSCIMUserCreated     = "scim.user.created"
	auditActionSCIMUserUpdated     = "scim.user.updated"
	auditActionSCIMUserDeactivated = "scim.user.deactivated"
	auditActionSCIMUserReactivated = "scim.user.reactivated"
	auditActionSCIMUserDeleted     = "scim.user.deleted"
	auditActionSCIMRoleChanged     = "scim.user.role_changed"
	auditActionSCIMGroupCreated    = "scim.group.created"
	auditActionSCIMGroupUpdated    = "scim.group.updated"
	auditActionSCIMGroupDeleted    = "scim.group.deleted"
)

var errLastRoot = errors.New("this is the last root user: grant root to someone else before deprovisioning it")

// accountDisabled reports whether a user is disabled; a lookup failure counts as disabled.
func (rt *Router) accountDisabled(ctx context.Context, userID string) bool {
	if rt.scim == nil {
		return false
	}
	disabled, err := rt.scim.UserDisabled(ctx, userID)
	if err != nil {
		rt.logger.Error("api: read disabled flag failed, refusing", slog.String("user_id", userID), slog.String("error", err.Error()))
		return true
	}
	return disabled
}

// guardLastRoot refuses to take the last root user out of service, so the
// directory can never lock every administrator out.
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

// deprovisionUser disables the account first, so every session and token of
// it fails closed at once, then revokes what it can and records the change.
func (rt *Router) deprovisionUser(r *http.Request, tok store.SCIMToken, userID string, deleted bool) error {
	ctx := r.Context()
	now := time.Now()
	var err error
	if deleted {
		err = rt.scim.MarkSCIMUserDeleted(ctx, userID, now)
	} else {
		err = rt.scim.DisableUser(ctx, userID, store.DisabledReasonSCIM, now)
	}
	if err != nil {
		return err
	}
	if rt.libSessions != nil {
		if rerr := rt.libSessions.RevokeAll(ctx, userID); rerr != nil {
			rt.logger.Error("api: scim: revoke sessions failed, account stays disabled", slog.String("user_id", userID), slog.String("error", rerr.Error()))
		}
	}
	rt.sessions.revokeAll(userID)
	rt.revokeTrustedDevices(ctx, r, signInActor{kind: auditActorTypeSCIM, id: tok.ID, name: tok.Name}, userID, "scim_deprovisioned")
	action := auditActionSCIMUserDeactivated
	if deleted {
		action = auditActionSCIMUserDeleted
	}
	rt.auditSCIM(r, tok, action, "/Users/"+userID, http.StatusOK)
	return nil
}

func (rt *Router) reactivateUser(r *http.Request, tok store.SCIMToken, userID string) error {
	if err := rt.scim.EnableUser(r.Context(), userID); err != nil {
		return err
	}
	rt.auditSCIM(r, tok, auditActionSCIMUserReactivated, "/Users/"+userID, http.StatusOK)
	return nil
}

// scimDefaultRole is the role for directory users no mapping matches.
func (rt *Router) scimDefaultRole(ctx context.Context) (string, error) {
	s, err := rt.sso.GetSSOSettings(ctx)
	if err != nil {
		return "", err
	}
	return s.SCIMDefaultRoleID, nil
}

// syncDirectoryRoles reapplies group mappings to users whose directory groups
// changed. With no mappings configured, roles stay under manual control.
func (rt *Router) syncDirectoryRoles(r *http.Request, tok store.SCIMToken, userIDs []string) error {
	ctx := r.Context()
	mappings, err := rt.sso.ListSSOGroupMappings(ctx)
	if err != nil || len(mappings) == 0 {
		return err
	}
	rules := make([]samlsso.GroupRole, 0, len(mappings))
	for _, m := range mappings {
		rules = append(rules, samlsso.GroupRole{Group: m.Group, RoleID: m.RoleID, Priority: m.Priority})
	}
	fallback, err := rt.scimDefaultRole(ctx)
	if err != nil {
		return err
	}
	for _, id := range userIDs {
		groups, err := rt.scim.SCIMGroupNamesForUser(ctx, id)
		if err != nil {
			return err
		}
		role, ok := samlsso.ResolveRole(groups, rules)
		if !ok {
			role = fallback
		}
		user, err := rt.auth.GetUserByID(ctx, id)
		if err != nil {
			return err
		}
		before, err := rt.roles.UserRoleID(ctx, id)
		if err != nil {
			return err
		}
		if _, err := rt.applySSORole(ctx, *user, role); err != nil {
			return err
		}
		if after, _ := rt.roles.UserRoleID(ctx, id); after != before {
			rt.auditSCIM(r, tok, auditActionSCIMRoleChanged, "/Users/"+id+"?role="+after, http.StatusOK)
		}
	}
	return nil
}

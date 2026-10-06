package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/GLINCKER/levelrail/internal/experimental"
	"github.com/GLINCKER/levelrail/internal/store"
)

// RoleStore is the store surface the stored-role, role-assignment and environment-grant routes need.
type RoleStore interface {
	ListRoles(ctx context.Context) ([]store.Role, error)
	GetRole(ctx context.Context, id string) (store.Role, error)
	CreateRole(ctx context.Context, r store.Role) error
	UpdateRole(ctx context.Context, r store.Role) error
	DeleteRole(ctx context.Context, id string) error
	RoleUserCounts(ctx context.Context) (map[string]int, error)
	UserRoleID(ctx context.Context, userID string) (string, error)
	SetUserRole(ctx context.Context, userID, roleID string) error
	ListUserEnvironmentGrants(ctx context.Context, userID string) ([]string, error)
	SetUserEnvironmentGrants(ctx context.Context, userID string, environmentIDs []string) error
	CountRootUsersExcept(ctx context.Context, userID string) (int, error)
	CountRootUsersOutsideRole(ctx context.Context, roleID string) (int, error)
	SyncRoleAbilities(ctx context.Context, roleID string) error
	UpdateRoleAndSync(ctx context.Context, r store.Role) error
}

const maxRoleNameLength = 64

var (
	errRoleNameRequired  = errors.New("name is required")
	errRoleNameTooLong   = fmt.Errorf("name must be at most %d characters", maxRoleNameLength)
	errBadRoleVisibility = errors.New(`visibility must be "all" or "granted"`)
	errGrantedReadOnly   = errors.New(`a role with visibility "granted" may only hold the "read" ability`)
)

// requireAccessRolesFeature answers the experimental-disabled error and reports false when the flag is off.
func requireAccessRolesFeature(w http.ResponseWriter) bool {
	if experimental.Enabled(experimental.AccessRoles) {
		return true
	}
	writeJSON(w, http.StatusNotFound, experimentalError{
		Error:   experimental.DisabledMessage(experimental.AccessRoles),
		Code:    ExperimentalDisabledCode,
		Feature: string(experimental.AccessRoles),
	})
	return false
}

func toRoleResource(r store.Role, userCount int) Role {
	return Role{
		Name:        r.Name,
		Description: r.Description,
		Abilities:   r.Abilities,
		ID:          r.ID,
		Visibility:  r.Visibility,
		Builtin:     r.Builtin,
		UserCount:   userCount,
	}
}

// validateRoleInput normalizes and checks the editable fields of a custom role.
func validateRoleInput(name, visibility string, abilities []string) (string, string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", "", errRoleNameRequired
	}
	if len(name) > maxRoleNameLength {
		return "", "", errRoleNameTooLong
	}
	if visibility == "" {
		visibility = store.RoleVisibilityAll
	}
	if visibility != store.RoleVisibilityAll && visibility != store.RoleVisibilityGranted {
		return "", "", errBadRoleVisibility
	}
	if err := validateAbilities(abilities); err != nil {
		return "", "", err
	}
	if visibility == store.RoleVisibilityGranted && slices.ContainsFunc(abilities, func(a string) bool { return a != AbilityRead }) {
		return "", "", errGrantedReadOnly
	}
	return name, visibility, nil
}

// resolveRoleSelection resolves a role_id or role name to a stored role. ok=false with a nil error means no role was named.
func (rt *Router) resolveRoleSelection(ctx context.Context, roleID, roleName string) (role store.Role, ok bool, err error) {
	switch {
	case roleID != "":
		role, err = rt.roles.GetRole(ctx, roleID)
	case roleName != "":
		var all []store.Role
		all, err = rt.roles.ListRoles(ctx)
		if err != nil {
			return store.Role{}, false, err
		}
		i := slices.IndexFunc(all, func(r store.Role) bool { return r.Name == roleName })
		if i < 0 {
			return store.Role{}, false, &unknownRoleError{role: roleName}
		}
		role = all[i]
	default:
		return store.Role{}, false, nil
	}
	if errors.Is(err, store.ErrRoleNotFound) {
		return store.Role{}, false, &unknownRoleError{role: roleID}
	}
	if err != nil {
		return store.Role{}, false, err
	}
	return role, true, nil
}

// resolveStoredAbilities returns the abilities for a create or invite request: the named stored role's set, or the explicit list.
func (rt *Router) resolveStoredAbilities(ctx context.Context, roleID, roleName string, abilities []string) ([]string, *store.Role, error) {
	role, ok, err := rt.resolveRoleSelection(ctx, roleID, roleName)
	if err != nil {
		return nil, nil, err
	}
	if ok {
		return role.Abilities, &role, nil
	}
	resolved, err := resolveAbilities("", abilities)
	return resolved, nil, err
}

// isRoleSelectionError reports whether err is the caller's fault (a 400) rather than a store failure.
func isRoleSelectionError(err error) bool {
	var unknownRole *unknownRoleError
	var unknownAbility *unknownAbilityError
	return errors.As(err, &unknownRole) || errors.As(err, &unknownAbility) ||
		errors.Is(err, errAbilityRequired) || errors.Is(err, errRootExclusive)
}

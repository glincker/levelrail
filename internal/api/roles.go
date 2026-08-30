package api

import (
	"fmt"
	"net/http"
	"slices"
)

// Role is a curated, named preset over the raw ability list
// (abilities.go): picking one sets a user's Abilities to exactly this
// set in one action, instead of hand-picking abilities individually.
// It is a convenience layer, not a second permission model: every
// existing ability check still only ever sees the resolved Abilities.
type Role struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Abilities   []string `json:"abilities"`
}

// Curated role names, roles's own Name values.
const (
	RoleAdmin    = "admin"
	RoleOperator = "operator"
	RoleViewer   = "viewer"
)

// roles is deliberately small: a permission matrix is exactly what this
// project's own six-string ability model (abilities.go) exists to avoid.
var roles = []Role{
	{
		Name:        RoleAdmin,
		Description: "Every ability, including user management, token administration, and secrets rotation.",
		Abilities:   []string{AbilityRoot},
	},
	{
		Name:        RoleOperator,
		Description: "Deploy, restart, manage domains and env vars, and view everything including sensitive reads. Cannot manage users, tokens, or roles, and cannot rotate secrets.",
		Abilities:   []string{AbilityRead, AbilityReadSensitive, AbilityWrite, AbilityDeploy},
	},
	{
		Name:        RoleViewer,
		Description: "Read-only: view apps, logs, metrics, and deploy history. Cannot change anything.",
		Abilities:   []string{AbilityRead},
	},
}

// unknownRoleError names the specific bad role string, mirroring
// unknownAbilityError's shape so the 400 response tells the caller
// exactly what to fix.
type unknownRoleError struct{ role string }

func (e *unknownRoleError) Error() string {
	return fmt.Sprintf("unknown role %q", e.role)
}

// roleAbilities returns the ability set for a curated role name, or
// ok=false if name isn't one of the presets above.
func roleAbilities(name string) ([]string, bool) {
	for _, r := range roles {
		if r.Name == name {
			return r.Abilities, true
		}
	}
	return nil, false
}

// roleForAbilities returns the curated role name whose ability set
// exactly matches abilities (order-insensitive), or ok=false if
// abilities doesn't match any preset: the "Custom" case in the UI.
func roleForAbilities(abilities []string) (string, bool) {
	for _, r := range roles {
		if abilitySetsEqual(r.Abilities, abilities) {
			return r.Name, true
		}
	}
	return "", false
}

func abilitySetsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for _, x := range a {
		if !slices.Contains(b, x) {
			return false
		}
	}
	return true
}

// resolveAbilities returns the abilities a create/update user request
// should apply: role's curated set when role is non-empty (validated
// against roleAbilities, abilities is ignored in that case), otherwise
// abilities as given (validated against validateAbilities as before).
func resolveAbilities(role string, abilities []string) ([]string, error) {
	if role == "" {
		if err := validateAbilities(abilities); err != nil {
			return nil, err
		}
		return abilities, nil
	}
	set, ok := roleAbilities(role)
	if !ok {
		return nil, &unknownRoleError{role: role}
	}
	return set, nil
}

// handleListRoles handles GET /api/v1/roles: the curated role presets
// available to apply on user create/update, AbilityRead-gated like the
// user list itself, since this is static, non-sensitive metadata.
func (rt *Router) handleListRoles(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, roles)
}

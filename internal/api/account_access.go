package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/GLINCKER/levelrail/internal/store"
)

const maxDisplayNameLen = 80

type updateProfileRequest struct {
	DisplayName string `json:"display_name"`
}

// handleUpdateProfile handles PATCH /api/v1/auth/profile: session-only, the
// caller's own display name and nothing else.
func (rt *Router) handleUpdateProfile(w http.ResponseWriter, r *http.Request) {
	userID, ok := rt.currentSessionUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, msgAuthRequired)
		return
	}
	var req updateProfileRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	name := strings.TrimSpace(req.DisplayName)
	if name == "" || utf8.RuneCountInString(name) > maxDisplayNameLen {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("display_name must be 1 to %d characters", maxDisplayNameLen))
		return
	}
	if err := rt.auth.UpdateUserDisplayName(r.Context(), userID, name); err != nil {
		if errors.Is(err, store.ErrUserNotFound) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		rt.internalError(w, "api: update profile failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"display_name": name})
}

type accessTeam struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Role string `json:"role"`
}

type accessPolicy struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type accessCapability struct {
	Key             string `json:"key"`
	Allowed         bool   `json:"allowed"`
	RequiredAbility string `json:"required_ability"`
	Reason          string `json:"reason,omitempty"`
}

type myAccessResponse struct {
	Kind            string             `json:"kind"`
	Name            string             `json:"name"`
	Tier            string             `json:"tier"`
	Abilities       []string           `json:"abilities"`
	Teams           []accessTeam       `json:"teams"`
	LinkedProviders []string           `json:"linked_providers"`
	Policies        []accessPolicy     `json:"policies"`
	Capabilities    []accessCapability `json:"capabilities"`
}

// accessAreas lists the areas My access explains, each gated by one base
// ability; IAM policies can still widen or narrow single resources.
var accessAreas = []struct{ key, ability string }{
	{"view", AbilityRead},
	{"deploy", AbilityDeploy},
	{"configure", AbilityWrite},
	{"secrets_read", AbilityReadSensitive},
	{"sensitive_write", AbilityWriteSensitive},
	{"users_and_access", AbilityRoot},
	{"platform_settings", AbilityRoot},
}

func accessTier(abilities []string) string {
	switch {
	case hasAbility(abilities, AbilityRoot):
		return "admin"
	case hasAbility(abilities, AbilityWriteSensitive):
		return "operator"
	case hasAbility(abilities, AbilityWrite) || hasAbility(abilities, AbilityDeploy):
		return "developer"
	default:
		return "viewer"
	}
}

func buildCapabilities(abilities []string) []accessCapability {
	out := make([]accessCapability, 0, len(accessAreas))
	for _, a := range accessAreas {
		c := accessCapability{Key: a.key, RequiredAbility: a.ability, Allowed: hasAbility(abilities, a.ability)}
		if !c.Allowed {
			c.Reason = fmt.Sprintf("requires the %s ability, which this account does not hold", a.ability)
		}
		out = append(out, c)
	}
	return out
}

// handleMyAccess handles GET /api/v1/auth/my-access: what the caller itself
// can reach and why, never another principal's.
func (rt *Router) handleMyAccess(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	out := myAccessResponse{Teams: []accessTeam{}, Policies: []accessPolicy{}, LinkedProviders: []string{}}
	var principalType, principalID string
	if _, pinned := rt.currentPinnedSession(r); pinned {
		writeError(w, http.StatusForbidden, "a session link has no account to report on")
		return
	}
	if userID, ok := rt.currentSessionUserID(r); ok {
		user, err := rt.auth.GetUserByID(ctx, userID)
		if err != nil {
			rt.internalError(w, "api: my access: load user failed", err)
			return
		}
		out.Kind, out.Name, out.Abilities = "session", user.Email, nonNil(user.Abilities)
		principalType, principalID = store.PrincipalTypeUser, userID
		if err := rt.fillMyTeams(ctx, userID, &out); err != nil {
			rt.internalError(w, "api: my access: teams failed", err)
			return
		}
	} else if rec, ok := ctx.Value(tokenIdentityKey{}).(*store.APIToken); ok {
		out.Kind, out.Name, out.Abilities = "token", rec.Name, nonNil(rec.Abilities)
		principalType, principalID = store.PrincipalTypeToken, rec.ID
	} else {
		writeError(w, http.StatusUnauthorized, msgAuthRequired)
		return
	}
	policies, err := rt.policies.ListPoliciesForPrincipal(ctx, principalType, principalID)
	if err != nil {
		rt.internalError(w, "api: my access: policies failed", err)
		return
	}
	for _, p := range policies {
		out.Policies = append(out.Policies, accessPolicy{ID: p.ID, Name: p.Name, Description: p.Description})
	}
	out.Tier = accessTier(out.Abilities)
	out.Capabilities = buildCapabilities(out.Abilities)
	writeJSON(w, http.StatusOK, out)
}

func (rt *Router) fillMyTeams(ctx context.Context, userID string, out *myAccessResponse) error {
	if rt.oauthIdentities != nil {
		ids, err := rt.oauthIdentities.ListOAuthIdentitiesForUser(ctx, userID)
		if err != nil {
			return err
		}
		for _, i := range ids {
			out.LinkedProviders = append(out.LinkedProviders, i.Provider)
		}
	}
	db := rt.teamAdmin()
	if db == nil || rt.organizations == nil {
		return nil
	}
	roles, err := db.UserTeamRoles(ctx, userID)
	if err != nil {
		return err
	}
	orgs, err := rt.organizations.ListOrganizations(ctx)
	if err != nil {
		return err
	}
	for _, o := range orgs {
		if role, ok := roles[o.ID]; ok {
			out.Teams = append(out.Teams, accessTeam{ID: o.ID, Name: o.Name, Role: role})
		}
	}
	return nil
}

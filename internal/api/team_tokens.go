package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/store"
)

// tokenScopeRequest limits a new token to one team, optionally one project,
// and a team role no higher than the minting user's own.
type tokenScopeRequest struct {
	TeamID    string `json:"team_id,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
	TeamRole  string `json:"team_role,omitempty"`
}

type tokenScopeResource struct {
	TeamID    string `json:"team_id"`
	ProjectID string `json:"project_id,omitempty"`
	TeamRole  string `json:"team_role"`
}

// resolveTokenScope validates a requested token scope for the minting user.
// It returns nil for an unscoped token, and false after writing an error.
func (rt *Router) resolveTokenScope(w http.ResponseWriter, r *http.Request, req createTokenRequest) (*store.APITokenScope, bool) {
	if req.TeamID == "" {
		if req.ProjectID != "" || req.TeamRole != "" {
			writeError(w, http.StatusBadRequest, "team_id is required with project_id or team_role")
			return nil, false
		}
		return nil, true
	}
	ownerID, _ := rt.currentSessionUserID(r)
	db := rt.teamDB()
	if db == nil || ownerID == "" {
		writeError(w, http.StatusBadRequest, "team scoped tokens need a signed in user")
		return nil, false
	}
	scope, err := rt.userTeamScope(r.Context(), db, ownerID)
	if err != nil {
		rt.internalError(w, "api: create token: scope", err)
		return nil, false
	}
	ownerRole := store.TeamRoleOwner
	if scope != nil {
		ownerRole = scope.roles[req.TeamID]
	}
	if ownerRole == "" {
		writeError(w, http.StatusBadRequest, "unknown team_id")
		return nil, false
	}
	role := req.TeamRole
	if role == "" {
		role = ownerRole
	}
	if !store.ValidTeamRole(role) || !store.TeamRoleAtLeast(ownerRole, role) {
		writeError(w, http.StatusBadRequest, "team_role must be one of your own role or lower")
		return nil, false
	}
	if req.ProjectID != "" {
		team, err := db.TeamOfProject(r.Context(), req.ProjectID)
		if err != nil || team != req.TeamID {
			writeError(w, http.StatusBadRequest, "project_id is not in that team")
			return nil, false
		}
	}
	for _, a := range req.Abilities {
		if !hasAbility(store.TeamRoleAbilities(role), a) {
			writeError(w, http.StatusBadRequest, "ability "+a+" is beyond the "+role+" team role")
			return nil, false
		}
	}
	return &store.APITokenScope{TeamID: req.TeamID, ProjectID: req.ProjectID, Role: role}, true
}

// saveTokenScope stores a new token's scope before its secret is returned.
// On failure the token is revoked so it can never act unscoped.
func (rt *Router) saveTokenScope(w http.ResponseWriter, r *http.Request, tokenID, engineID string, scope *store.APITokenScope) bool {
	if scope == nil {
		return true
	}
	scope.TokenID = tokenID
	db := rt.teamAdmin()
	var err error
	if db == nil {
		err = errors.New("store lacks token scopes")
	} else {
		err = db.SetAPITokenScope(r.Context(), *scope)
	}
	if err == nil {
		return true
	}
	if rerr := rt.authLib.tokens.RevokeToken(r.Context(), engineID); rerr != nil {
		rt.logger.Error("api: create token: revoke after failed scope", slog.String("token_id", tokenID), slog.String("error", rerr.Error()))
	}
	if rerr := rt.tokens.RevokeAPIToken(r.Context(), tokenID); rerr != nil {
		rt.logger.Error("api: create token: revoke mirror after failed scope", slog.String("token_id", tokenID), slog.String("error", rerr.Error()))
	}
	rt.internalError(w, "api: create token: save scope", err, slog.String("token_id", tokenID))
	return false
}

// tokenScopeOf loads a token's scope for listing; failures read as unscoped
// only for display, never for authorization.
func (rt *Router) tokenScopeOf(ctx context.Context, tokenID string) *tokenScopeResource {
	db := rt.teamDB()
	if db == nil {
		return nil
	}
	s, err := db.GetAPITokenScope(ctx, tokenID)
	if err != nil || s == nil {
		return nil
	}
	return &tokenScopeResource{TeamID: s.TeamID, ProjectID: s.ProjectID, TeamRole: s.Role}
}

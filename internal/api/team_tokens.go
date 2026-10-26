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
	// Stack binds the token to one stack by name; StackVerbs are plan and/or
	// apply and default to plan.
	Stack      string   `json:"stack,omitempty"`
	StackVerbs []string `json:"stack_verbs,omitempty"`
}

type tokenScopeResource struct {
	TeamID     string   `json:"team_id"`
	ProjectID  string   `json:"project_id,omitempty"`
	TeamRole   string   `json:"team_role"`
	StackID    string   `json:"stack_id,omitempty"`
	StackVerbs []string `json:"stack_verbs,omitempty"`
}

// resolveTokenScope validates a requested token scope for the minting user.
// It returns nil for an unscoped token, and false after writing an error.
func (rt *Router) resolveTokenScope(w http.ResponseWriter, r *http.Request, req createTokenRequest) (*store.APITokenScope, bool) {
	stack, ok := rt.resolveTokenStackScope(w, r, req)
	if !ok {
		return nil, false
	}
	scope, ok := rt.resolveTokenTeamScope(w, r, req)
	if !ok || stack == nil {
		return scope, ok
	}
	if scope == nil {
		scope = &store.APITokenScope{}
	}
	scope.Stack = stack
	return scope, true
}

func (rt *Router) resolveTokenTeamScope(w http.ResponseWriter, r *http.Request, req createTokenRequest) (*store.APITokenScope, bool) {
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
	err := rt.storeTokenScope(r.Context(), scope)
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

// storeTokenScope writes the stack scope first, so a failure there leaves
// no half-scoped token behind once the caller revokes it.
func (rt *Router) storeTokenScope(ctx context.Context, scope *store.APITokenScope) error {
	if scope.Stack != nil {
		ss := rt.tokenStackScopes()
		if ss == nil {
			return errors.New("store lacks token stack scopes")
		}
		scope.Stack.TokenID = scope.TokenID
		if err := ss.SetAPITokenStackScope(ctx, *scope.Stack); err != nil {
			return err
		}
	}
	if scope.TeamID == "" {
		return nil
	}
	db := rt.teamAdmin()
	if db == nil {
		return errors.New("store lacks token scopes")
	}
	return db.SetAPITokenScope(ctx, *scope)
}

// tokenScopeOf loads a token's scope for listing; failures read as unscoped
// only for display, never for authorization.
func (rt *Router) tokenScopeOf(ctx context.Context, tokenID string) *tokenScopeResource {
	var out *tokenScopeResource
	if db := rt.teamDB(); db != nil {
		if s, err := db.GetAPITokenScope(ctx, tokenID); err == nil && s != nil {
			out = &tokenScopeResource{TeamID: s.TeamID, ProjectID: s.ProjectID, TeamRole: s.Role}
		}
	}
	if ss := rt.tokenStackScopes(); ss != nil {
		if s, err := ss.GetAPITokenStackScope(ctx, tokenID); err == nil && s != nil {
			if out == nil {
				out = &tokenScopeResource{}
			}
			out.StackID, out.StackVerbs = s.StackID, s.Verbs
		}
	}
	return out
}

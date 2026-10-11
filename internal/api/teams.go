package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// teamAdminStore is what the team, member and invite handlers need;
// *store.DB satisfies it.
type teamAdminStore interface {
	ListTeamMembers(ctx context.Context, teamID string) ([]store.TeamMember, error)
	UserTeamRoles(ctx context.Context, userID string) (map[string]string, error)
	TeamMemberRole(ctx context.Context, teamID, userID string) (string, error)
	AddTeamMember(ctx context.Context, teamID, userID, role string) error
	SetTeamMemberRole(ctx context.Context, teamID, userID, role string) error
	RemoveTeamMember(ctx context.Context, teamID, userID string) error
	TransferTeamOwnership(ctx context.Context, teamID, from, to string) error
	CountTeamProjects(ctx context.Context, teamID string) (int, error)
	CreateTeamInvite(ctx context.Context, inv store.TeamInvite) error
	ListTeamInvites(ctx context.Context, teamID string) ([]store.TeamInvite, error)
	GetTeamInviteByHash(ctx context.Context, tokenHash string) (store.TeamInvite, error)
	RevokeTeamInvite(ctx context.Context, teamID, id string, now time.Time) error
	ClaimTeamInvite(ctx context.Context, tokenHash, userID string, now time.Time) (store.TeamInvite, error)
	RecordTeamEvent(ctx context.Context, e store.TeamEvent) error
	ListTeamEvents(ctx context.Context, teamID string, limit int) ([]store.TeamEvent, error)
	SetAPITokenScope(ctx context.Context, s store.APITokenScope) error
	GetAPITokenScope(ctx context.Context, tokenID string) (*store.APITokenScope, error)
	RenameOrganization(ctx context.Context, id, name string) error
}

func (rt *Router) teamAdmin() teamAdminStore {
	if s, ok := rt.policies.(teamAdminStore); ok {
		return s
	}
	if s, ok := rt.apps.(teamAdminStore); ok {
		return s
	}
	return nil
}

type teamResource struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Default     bool   `json:"default"`
	Role        string `json:"role"`
	MemberCount int    `json:"member_count"`
	CreatedAt   string `json:"created_at"`
}

type teamMemberResource struct {
	UserID      string `json:"user_id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
	CreatedAt   string `json:"created_at"`
}

type teamRoleResource struct {
	Name      string   `json:"name"`
	Abilities []string `json:"abilities"`
}

// teamCaller is the acting user and their role in one team.
type teamCaller struct {
	userID string
	role   string
	root   bool
}

// resolveTeamCaller finds who is acting on a team. Tokens act as their owner,
// capped by the token's own team scope through the gate's scope.
func (rt *Router) resolveTeamCaller(r *http.Request, teamID string) (teamCaller, error) {
	principalType, principalID, abilities, err := rt.callerPrincipal(r)
	if err != nil {
		return teamCaller{}, err
	}
	userID := principalID
	if principalType == store.PrincipalTypeToken {
		rec, err := rt.tokens.GetAPITokenByID(r.Context(), principalID)
		if err != nil {
			return teamCaller{}, err
		}
		userID = rec.OwnerUserID
	}
	scope, ok := callerTeamScope(r.Context())
	if !ok {
		if scope, err = rt.principalTeamScope(r.Context(), principalType, principalID); err != nil {
			return teamCaller{}, err
		}
	}
	c := teamCaller{userID: userID, root: scope == nil && hasAbility(abilities, AbilityRoot)}
	if scope == nil {
		c.role = store.TeamRoleOwner
	} else {
		c.role = scope.roleOfTeam(teamID)
	}
	return c, nil
}

// canManage reports whether the caller may change memberships of a team.
// The default team is the instance itself, so only instance admins manage it.
func (c teamCaller) canManage(teamID string) bool {
	if teamID == store.DefaultTeamID {
		return c.root
	}
	return c.root || store.TeamRoleAtLeast(c.role, store.TeamRoleAdmin)
}

// mayAssign reports whether the caller may give or take away role. Owners
// assign anything; admins only roles below their own.
func (c teamCaller) mayAssign(role string) bool {
	if c.root || c.role == store.TeamRoleOwner {
		return true
	}
	return c.role == store.TeamRoleAdmin && (role == store.TeamRoleDeveloper || role == store.TeamRoleViewer)
}

func (rt *Router) recordTeamEvent(r *http.Request, teamID, actorID, action, subject, detail string) {
	db := rt.teamAdmin()
	if db == nil {
		return
	}
	id, err := randomOpaqueID("tev_")
	if err != nil {
		rt.logger.Warn("api: team event id", slog.String("error", err.Error()))
		return
	}
	name := actorID
	if u, err := rt.auth.GetUserByID(r.Context(), actorID); err == nil {
		name = u.Email
	}
	if err := db.RecordTeamEvent(r.Context(), store.TeamEvent{
		ID: id, TeamID: teamID, ActorID: actorID, ActorName: name,
		Action: action, Subject: subject, Detail: detail, CreatedAt: time.Now(),
	}); err != nil {
		rt.logger.Warn("api: record team event failed", slog.String("team_id", teamID), slog.String("error", err.Error()))
	}
}

// handleListTeams handles GET /api/v1/teams: the teams the caller belongs to,
// every team for an instance admin.
func (rt *Router) handleListTeams(w http.ResponseWriter, r *http.Request) {
	db := rt.teamAdmin()
	if db == nil {
		rt.internalError(w, "api: list teams", errors.New("store lacks teams"))
		return
	}
	orgs, err := rt.organizations.ListOrganizations(r.Context())
	if err != nil {
		rt.internalError(w, "api: list teams", err)
		return
	}
	scope, _ := callerTeamScope(r.Context())
	out := []teamResource{}
	for _, o := range orgs {
		if !scope.allowsTeam(o.ID) {
			continue
		}
		members, err := db.ListTeamMembers(r.Context(), o.ID)
		if err != nil {
			rt.internalError(w, "api: list teams: members", err)
			return
		}
		role := store.TeamRoleOwner
		if scope != nil {
			role = scope.roleOfTeam(o.ID)
		}
		out = append(out, teamResource{ID: o.ID, Name: o.Name, Default: o.ID == store.DefaultTeamID, Role: role, MemberCount: len(members), CreatedAt: o.CreatedAt})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleListTeamRoles handles GET /api/v1/teams/roles.
func (rt *Router) handleListTeamRoles(w http.ResponseWriter, _ *http.Request) {
	out := []teamRoleResource{}
	for _, role := range []string{store.TeamRoleOwner, store.TeamRoleAdmin, store.TeamRoleDeveloper, store.TeamRoleViewer} {
		out = append(out, teamRoleResource{Name: role, Abilities: store.TeamRoleAbilities(role)})
	}
	writeJSON(w, http.StatusOK, out)
}

type createTeamRequest struct {
	Name string `json:"name"`
}

// handleCreateTeam handles POST /api/v1/teams. Instance admins only; the
// creator becomes the first owner.
func (rt *Router) handleCreateTeam(w http.ResponseWriter, r *http.Request) {
	var req createTeamRequest
	if err := decodeJSONBody(r, &req); err != nil || strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	caller, err := rt.resolveTeamCaller(r, "")
	if err != nil {
		rt.internalError(w, "api: create team: caller", err)
		return
	}
	if caller.userID == "" {
		writeError(w, http.StatusBadRequest, "a team needs a user as its first owner; create it from a user session or a user's token")
		return
	}
	id, err := randomOrganizationID()
	if err != nil {
		rt.internalError(w, "api: create team: id", err)
		return
	}
	org := store.Organization{ID: id, Name: strings.TrimSpace(req.Name), CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	if err := rt.organizations.SaveOrganization(r.Context(), org); err != nil {
		rt.internalError(w, "api: create team", err)
		return
	}
	if err := rt.teamAdmin().AddTeamMember(r.Context(), id, caller.userID, store.TeamRoleOwner); err != nil {
		rt.internalError(w, "api: create team: owner", err)
		return
	}
	rt.recordTeamEvent(r, id, caller.userID, "team.created", org.Name, "")
	writeJSON(w, http.StatusCreated, teamResource{ID: id, Name: org.Name, Role: store.TeamRoleOwner, MemberCount: 1, CreatedAt: org.CreatedAt})
}

// handleGetTeam handles GET /api/v1/teams/{id}.
func (rt *Router) handleGetTeam(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	org, err := rt.organizations.GetOrganization(r.Context(), id)
	if errors.Is(err, store.ErrOrganizationNotFound) {
		writeError(w, http.StatusNotFound, "team not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: get team", err)
		return
	}
	caller, err := rt.resolveTeamCaller(r, id)
	if err != nil {
		rt.internalError(w, "api: get team: caller", err)
		return
	}
	members, err := rt.teamAdmin().ListTeamMembers(r.Context(), id)
	if err != nil {
		rt.internalError(w, "api: get team: members", err)
		return
	}
	writeJSON(w, http.StatusOK, teamResource{ID: org.ID, Name: org.Name, Default: org.ID == store.DefaultTeamID, Role: caller.role, MemberCount: len(members), CreatedAt: org.CreatedAt})
}

// handleRenameTeam handles PATCH /api/v1/teams/{id}; owners only.
func (rt *Router) handleRenameTeam(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req createTeamRequest
	if err := decodeJSONBody(r, &req); err != nil || strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	caller, err := rt.resolveTeamCaller(r, id)
	if err != nil {
		rt.internalError(w, "api: rename team: caller", err)
		return
	}
	if (!caller.root && caller.role != store.TeamRoleOwner) || (id == store.DefaultTeamID && !caller.root) {
		writeError(w, http.StatusForbidden, "only a team owner can rename the team")
		return
	}
	if err := rt.teamAdmin().RenameOrganization(r.Context(), id, strings.TrimSpace(req.Name)); err != nil {
		rt.internalError(w, "api: rename team", err)
		return
	}
	rt.recordTeamEvent(r, id, caller.userID, "team.renamed", strings.TrimSpace(req.Name), "")
	rt.handleGetTeam(w, r)
}

// handleDeleteTeam handles DELETE /api/v1/teams/{id}. The default team and a
// team that still owns projects cannot be deleted, so nothing is orphaned.
func (rt *Router) handleDeleteTeam(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == store.DefaultTeamID {
		writeError(w, http.StatusConflict, "the default team cannot be deleted")
		return
	}
	caller, err := rt.resolveTeamCaller(r, id)
	if err != nil {
		rt.internalError(w, "api: delete team: caller", err)
		return
	}
	if !caller.root && caller.role != store.TeamRoleOwner {
		writeError(w, http.StatusForbidden, "only a team owner can delete the team")
		return
	}
	n, err := rt.teamAdmin().CountTeamProjects(r.Context(), id)
	if err != nil {
		rt.internalError(w, "api: delete team: projects", err)
		return
	}
	if n > 0 {
		writeError(w, http.StatusConflict, "move or delete the team's projects first")
		return
	}
	if err := rt.organizations.DeleteOrganization(r.Context(), id); err != nil {
		rt.internalError(w, "api: delete team", err)
		return
	}
	rt.recordTeamEvent(r, id, caller.userID, "team.deleted", id, "")
	w.WriteHeader(http.StatusNoContent)
}

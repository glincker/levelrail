package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/GLINCKER/levelrail/internal/store"
)

// handleListTeamMembers handles GET /api/v1/teams/{id}/members.
func (rt *Router) handleListTeamMembers(w http.ResponseWriter, r *http.Request) {
	members, err := rt.teamAdmin().ListTeamMembers(r.Context(), r.PathValue("id"))
	if err != nil {
		rt.internalError(w, "api: list team members", err)
		return
	}
	out := make([]teamMemberResource, 0, len(members))
	for _, m := range members {
		out = append(out, teamMemberResource{UserID: m.UserID, Email: m.Email, DisplayName: m.DisplayName, Role: m.Role, CreatedAt: m.CreatedAt})
	}
	writeJSON(w, http.StatusOK, out)
}

type setTeamMemberRoleRequest struct {
	Role string `json:"role"`
}

// writeMembershipError maps store membership errors onto HTTP answers.
func (rt *Router) writeMembershipError(w http.ResponseWriter, op string, err error) {
	switch {
	case errors.Is(err, store.ErrLastTeamOwner):
		writeError(w, http.StatusConflict, "a team must keep at least one owner; transfer ownership first")
	case errors.Is(err, store.ErrTeamMemberNotFound):
		writeError(w, http.StatusNotFound, "member not found")
	case errors.Is(err, store.ErrInvalidTeamRole):
		writeError(w, http.StatusBadRequest, "role must be owner, admin, developer or viewer")
	case errors.Is(err, store.ErrTeamMemberExists):
		writeError(w, http.StatusConflict, "already a member of this team")
	default:
		rt.internalError(w, "api: "+op, err)
	}
}

// managedMember loads the caller and target and checks the caller may manage
// the target's current role. ok is false after an error was written.
func (rt *Router) managedMember(w http.ResponseWriter, r *http.Request) (caller teamCaller, teamID, userID, current string, ok bool) {
	teamID, userID = r.PathValue("id"), r.PathValue("user")
	caller, err := rt.resolveTeamCaller(r, teamID)
	if err != nil {
		rt.internalError(w, "api: team member: caller", err)
		return caller, "", "", "", false
	}
	if !caller.canManage(teamID) {
		writeError(w, http.StatusForbidden, manageDeniedMessage(teamID))
		return caller, "", "", "", false
	}
	current, err = rt.teamAdmin().TeamMemberRole(r.Context(), teamID, userID)
	if err != nil {
		rt.writeMembershipError(w, "team member", err)
		return caller, "", "", "", false
	}
	if !caller.mayAssign(current) {
		writeError(w, http.StatusForbidden, "only an owner can change an owner or admin")
		return caller, "", "", "", false
	}
	return caller, teamID, userID, current, true
}

func manageDeniedMessage(teamID string) string {
	if teamID == store.DefaultTeamID {
		return "the default team is the instance itself; an instance admin manages its members"
	}
	return "managing members needs the admin or owner role in this team"
}

// handleSetTeamMemberRole handles PUT /api/v1/teams/{id}/members/{user}.
func (rt *Router) handleSetTeamMemberRole(w http.ResponseWriter, r *http.Request) {
	var req setTeamMemberRoleRequest
	if err := decodeJSONBody(r, &req); err != nil || !store.ValidTeamRole(req.Role) {
		writeError(w, http.StatusBadRequest, "role must be owner, admin, developer or viewer")
		return
	}
	caller, teamID, userID, current, ok := rt.managedMember(w, r)
	if !ok {
		return
	}
	if !caller.mayAssign(req.Role) {
		writeError(w, http.StatusForbidden, "an admin can only assign the developer or viewer role")
		return
	}
	if err := rt.teamAdmin().SetTeamMemberRole(r.Context(), teamID, userID, req.Role); err != nil {
		rt.writeMembershipError(w, "set team member role", err)
		return
	}
	rt.recordTeamEvent(r, teamID, caller.userID, "member.role_changed", userID, current+" -> "+req.Role)
	w.WriteHeader(http.StatusNoContent)
}

// handleRemoveTeamMember handles DELETE /api/v1/teams/{id}/members/{user}.
func (rt *Router) handleRemoveTeamMember(w http.ResponseWriter, r *http.Request) {
	caller, teamID, userID, current, ok := rt.managedMember(w, r)
	if !ok {
		return
	}
	if err := rt.teamAdmin().RemoveTeamMember(r.Context(), teamID, userID); err != nil {
		rt.writeMembershipError(w, "remove team member", err)
		return
	}
	rt.recordTeamEvent(r, teamID, caller.userID, "member.removed", userID, current)
	rt.offboardMailboxes(r.Context(), teamID, userID)
	w.WriteHeader(http.StatusNoContent)
}

// handleLeaveTeam handles DELETE /api/v1/teams/{id}/members/me for the
// signed in user, or a token's owner.
func (rt *Router) handleLeaveTeam(w http.ResponseWriter, r *http.Request) {
	teamID := r.PathValue("id")
	caller, err := rt.resolveTeamCaller(r, teamID)
	if err != nil {
		rt.internalError(w, "api: leave team: caller", err)
		return
	}
	userID := caller.userID
	if userID == "" {
		writeError(w, http.StatusBadRequest, "a system token has no team membership to leave")
		return
	}
	if err := rt.teamAdmin().RemoveTeamMember(r.Context(), teamID, userID); err != nil {
		rt.writeMembershipError(w, "leave team", err)
		return
	}
	rt.recordTeamEvent(r, teamID, userID, "member.left", userID, "")
	rt.offboardMailboxes(r.Context(), teamID, userID)
	rt.recordAuthAudit(r, userID, accountEventTeamLeft, http.StatusNoContent, map[string]string{"team_id": teamID})
	w.WriteHeader(http.StatusNoContent)
}

type transferTeamRequest struct {
	UserID string `json:"user_id"`
}

// handleTransferTeam handles POST /api/v1/teams/{id}/transfer: the calling
// owner hands ownership to another member and becomes admin.
func (rt *Router) handleTransferTeam(w http.ResponseWriter, r *http.Request) {
	teamID := r.PathValue("id")
	var req transferTeamRequest
	if err := decodeJSONBody(r, &req); err != nil || req.UserID == "" {
		writeError(w, http.StatusBadRequest, "user_id is required")
		return
	}
	caller, err := rt.resolveTeamCaller(r, teamID)
	if err != nil {
		rt.internalError(w, "api: transfer team: caller", err)
		return
	}
	if caller.role != store.TeamRoleOwner || caller.userID == "" || (teamID == store.DefaultTeamID && !caller.root) {
		writeError(w, http.StatusForbidden, "only a team owner can transfer ownership")
		return
	}
	if err := rt.teamAdmin().TransferTeamOwnership(r.Context(), teamID, caller.userID, req.UserID); err != nil {
		if errors.Is(err, store.ErrTeamMemberNotFound) {
			writeError(w, http.StatusBadRequest, "both people must be members, and you must be an owner")
			return
		}
		rt.writeMembershipError(w, "transfer team", err)
		return
	}
	rt.recordTeamEvent(r, teamID, caller.userID, "team.ownership_transferred", req.UserID, "")
	w.WriteHeader(http.StatusNoContent)
}

type teamEventResource struct {
	ID        string `json:"id"`
	ActorID   string `json:"actor_id"`
	ActorName string `json:"actor_name"`
	Action    string `json:"action"`
	Subject   string `json:"subject"`
	Detail    string `json:"detail"`
	CreatedAt string `json:"created_at"`
}

const teamEventsMax = 500

// handleListTeamEvents handles GET /api/v1/teams/{id}/events, the team's own
// membership audit trail, for admins and owners.
func (rt *Router) handleListTeamEvents(w http.ResponseWriter, r *http.Request) {
	teamID := r.PathValue("id")
	caller, err := rt.resolveTeamCaller(r, teamID)
	if err != nil {
		rt.internalError(w, "api: team events: caller", err)
		return
	}
	if !caller.root && !store.TeamRoleAtLeast(caller.role, store.TeamRoleAdmin) {
		writeError(w, http.StatusForbidden, "the team audit trail needs the admin or owner role")
		return
	}
	limit := 100
	if n, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && n > 0 {
		limit = min(n, teamEventsMax)
	}
	events, err := rt.teamAdmin().ListTeamEvents(r.Context(), teamID, limit)
	if err != nil {
		rt.internalError(w, "api: team events", err)
		return
	}
	out := make([]teamEventResource, 0, len(events))
	for _, e := range events {
		out = append(out, teamEventResource{ID: e.ID, ActorID: e.ActorID, ActorName: e.ActorName, Action: e.Action, Subject: e.Subject, Detail: e.Detail, CreatedAt: e.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00")})
	}
	writeJSON(w, http.StatusOK, out)
}

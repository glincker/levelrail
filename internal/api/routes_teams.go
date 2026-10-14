package api

import (
	"net/http"

	"github.com/GLINCKER/levelrail/internal/quota"
)

// registerTeamRoutes wires teams, members and invites. Every /teams/{id}
// route also passes the team gate (team_gate.go), so a non-member gets 404.
func (rt *Router) registerTeamRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/teams", rt.requireAbility(AbilityRead, rt.handleListTeams))
	mux.HandleFunc("POST /api/v1/teams", rt.requireAbility(AbilityRoot, rt.handleCreateTeam))
	mux.HandleFunc("GET /api/v1/teams/roles", rt.requireAbility(AbilityRead, rt.handleListTeamRoles))
	mux.HandleFunc("GET /api/v1/teams/{id}", rt.requireAbility(AbilityRead, rt.handleGetTeam))
	mux.HandleFunc("PATCH /api/v1/teams/{id}", rt.requireAbility(AbilityWrite, rt.handleRenameTeam))
	mux.HandleFunc("DELETE /api/v1/teams/{id}", rt.requireAbility(AbilityWrite, rt.handleDeleteTeam))
	mux.HandleFunc("GET /api/v1/teams/{id}/members", rt.requireAbility(AbilityRead, rt.handleListTeamMembers))
	mux.HandleFunc("PUT /api/v1/teams/{id}/members/{user}", rt.requireAbility(AbilityWrite, rt.handleSetTeamMemberRole))
	mux.HandleFunc("DELETE /api/v1/teams/{id}/members/{user}", rt.requireAbility(AbilityWrite, rt.handleRemoveTeamMember))
	// Read tier: it only ever removes the caller's own membership.
	mux.HandleFunc("DELETE /api/v1/teams/{id}/members/me", rt.requireAbility(AbilityRead, rt.handleLeaveTeam))
	mux.HandleFunc("POST /api/v1/teams/{id}/transfer", rt.requireAbility(AbilityWrite, rt.handleTransferTeam))
	mux.HandleFunc("GET /api/v1/teams/{id}/quota", rt.requireAbility(AbilityRead, rt.handleGetQuota(quota.ScopeTeam)))
	mux.HandleFunc("PUT /api/v1/teams/{id}/quota", rt.requireAbility(AbilityWrite, rt.handleSetQuota(quota.ScopeTeam)))
	mux.HandleFunc("DELETE /api/v1/teams/{id}/quota", rt.requireAbility(AbilityWrite, rt.handleDeleteQuota(quota.ScopeTeam)))
	mux.HandleFunc("GET /api/v1/projects/{id}/quota", rt.requireAbility(AbilityRead, rt.handleGetQuota(quota.ScopeProject)))
	mux.HandleFunc("PUT /api/v1/projects/{id}/quota", rt.requireAbility(AbilityWrite, rt.handleSetQuota(quota.ScopeProject)))
	mux.HandleFunc("DELETE /api/v1/projects/{id}/quota", rt.requireAbility(AbilityWrite, rt.handleDeleteQuota(quota.ScopeProject)))
	mux.HandleFunc("GET /api/v1/teams/{id}/events", rt.requireAbility(AbilityRead, rt.handleListTeamEvents))
	mux.HandleFunc("GET /api/v1/teams/{id}/invites", rt.requireAbility(AbilityRead, rt.handleListTeamInvites))
	mux.HandleFunc("POST /api/v1/teams/{id}/invites", rt.requireAbility(AbilityWrite, rt.handleCreateTeamInvite))
	mux.HandleFunc("DELETE /api/v1/teams/{id}/invites/{invite}", rt.requireAbility(AbilityWrite, rt.handleRevokeTeamInvite))
	mux.HandleFunc("GET /api/v1/team-invites/{token}", rt.requireAuth(rt.handlePreviewTeamInvite))
	mux.HandleFunc("POST /api/v1/team-invites/accept", rt.handleAcceptTeamInvite)
}

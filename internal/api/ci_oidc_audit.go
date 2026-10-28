package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	auditActionCITrustCreated = "ci_oidc_trust.created"
	auditActionCITrustDeleted = "ci_oidc_trust.deleted"
)

// ciTrustCallerMay keeps a trust from granting more than its creator holds; a self-hosted issuer can mint
// tokens offline, so naming one is a root decision.
func (rt *Router) ciTrustCallerMay(r *http.Request, app string, req ciTrustRequest) (bool, string) {
	ptype, pid, abilities, err := rt.callerPrincipal(r)
	if err != nil {
		return false, "could not resolve the caller"
	}
	policies, err := rt.policies.ListPoliciesForPrincipal(r.Context(), ptype, pid)
	if err != nil {
		return false, "could not load the caller's policies"
	}
	allowed, hidden, err := rt.authorizeResourceInEnvironment(r.Context(), ptype, pid, abilities, policies, AbilityDeploy, "app:"+app)
	if err != nil || !allowed || hidden {
		return false, "adding a CI trust needs the deploy ability on this app"
	}
	if req.Provider == ciProviderGitLab && req.Issuer != gitLabDefaultIssuer && !hasAbility(abilities, AbilityRoot) {
		return false, "a self-managed GitLab issuer can only be trusted by a root user"
	}
	return true, ""
}

type ciTrustAuditDetail struct {
	TrustID          string `json:"trust_id"`
	Provider         string `json:"provider"`
	Issuer           string `json:"issuer"`
	Repository       string `json:"repository"`
	RefPattern       string `json:"ref_pattern"`
	Environment      string `json:"environment,omitempty"`
	ProtectedRefOnly bool   `json:"protected_ref_only"`
}

// auditCITrust records a trust change with its issuer, which the generic request row does not carry.
func (rt *Router) auditCITrust(ctx context.Context, r *http.Request, action string, t store.CIOIDCTrust) {
	if rt.auditLog == nil {
		return
	}
	id, err := store.NewAuditEntryID()
	if err != nil {
		return
	}
	issuer := t.Issuer
	if ciProviderName(t.Provider) == ciProviderGitHub {
		issuer = githubActionsOIDCIssuer
	}
	detail, _ := json.Marshal(ciTrustAuditDetail{
		TrustID: t.ID, Provider: ciProviderName(t.Provider), Issuer: issuer, Repository: t.Repository,
		RefPattern: t.RefPattern, Environment: t.Environment, ProtectedRefOnly: t.ProtectedRefOnly,
	})
	entry := store.AuditEntry{
		ID: id, Ability: AbilityWriteSensitive, Method: auditMethodEvent, Path: "/api/v1/apps/" + t.ServiceName + "/ci/trusts/" + t.ID,
		StatusCode: http.StatusOK, CreatedAt: store.FormatAuditTime(time.Now()), ClientKind: clientKindFromUserAgent(r.UserAgent()),
		Action: action, RemoteAddr: clientIP(r), Detail: string(detail),
	}
	if at, aid, name, ok := rt.currentActor(r); ok {
		entry.ActorType, entry.ActorID, entry.ActorName = at, aid, name
	}
	if err := rt.auditLog.SaveAuditEntry(ctx, entry); err != nil {
		rt.logger.Warn("api: save ci trust audit entry failed", slog.String("trust_id", t.ID), slog.String("error", err.Error()))
	}
}

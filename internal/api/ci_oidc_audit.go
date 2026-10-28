package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	auditActionCITrustCreated = "ci_oidc_trust.created"
	auditActionCITrustDeleted = "ci_oidc_trust.deleted"

	auditActionCIExchangeIssued  = "ci_oidc_exchange.issued"
	auditActionCIExchangeRefused = "ci_oidc_exchange.refused"

	envCIOIDCAuditWindow        = "APP_CI_OIDC_AUDIT_WINDOW"
	envCIOIDCAuditMaxKeys       = "APP_CI_OIDC_AUDIT_MAX_KEYS"
	defaultCIOIDCAuditWindow    = time.Minute
	defaultCIOIDCAuditMaxKeys   = 1000
	ciExchangeReasonRejected    = "token_rejected"
	ciExchangeReasonNoTrust     = "no_matching_trust"
	ciExchangeReasonIssued      = "issued"
	ciExchangeSubjectHashPrefix = 12
)

// ciExchangeAuditWindow admits one refusal row per key per window and counts the rest, like the PR comment refusal window.
type ciExchangeAuditWindow struct {
	mu      sync.Mutex
	window  time.Duration
	maxKeys int
	seen    map[string]*ciExchangeWindowEntry
}

type ciExchangeWindowEntry struct {
	start      time.Time
	suppressed int
}

func newCIExchangeAuditWindow(window time.Duration, maxKeys int) *ciExchangeAuditWindow {
	return &ciExchangeAuditWindow{window: window, maxKeys: maxKeys, seen: map[string]*ciExchangeWindowEntry{}}
}

// admit reports whether key may write a row now and how many refusals were suppressed since its last row.
func (w *ciExchangeAuditWindow) admit(key string, now time.Time) (bool, int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if e := w.seen[key]; e != nil {
		if now.Sub(e.start) < w.window {
			e.suppressed++
			return false, 0
		}
		delete(w.seen, key)
		w.seen[key] = &ciExchangeWindowEntry{start: now}
		return true, e.suppressed
	}
	if len(w.seen) >= w.maxKeys {
		for k, e := range w.seen {
			if now.Sub(e.start) >= w.window {
				delete(w.seen, k)
			}
		}
		if len(w.seen) >= w.maxKeys {
			return false, 0
		}
	}
	w.seen[key] = &ciExchangeWindowEntry{start: now}
	return true, 0
}

var ciExchangeAudits = sync.OnceValue(func() *ciExchangeAuditWindow {
	return newCIExchangeAuditWindow(envDuration(envCIOIDCAuditWindow, defaultCIOIDCAuditWindow), envIntDefault(envCIOIDCAuditMaxKeys, defaultCIOIDCAuditMaxKeys))
})

type ciExchangeAuditDetail struct {
	App        string `json:"app"`
	Provider   string `json:"provider"`
	Reason     string `json:"reason"`
	Subject    string `json:"subject_sha256,omitempty"`
	TrustID    string `json:"trust_id,omitempty"`
	TokenID    string `json:"token_id,omitempty"`
	Suppressed int    `json:"suppressed,omitempty"`
}

// ciRedactedSubject lets rows from one workflow be correlated without storing its claims.
func ciRedactedSubject(sub string) string {
	if sub == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(sub))
	return hex.EncodeToString(sum[:])[:ciExchangeSubjectHashPrefix]
}

// auditCIExchange writes every issued token and, windowed per app, provider and reason, every refusal.
func (rt *Router) auditCIExchange(ctx context.Context, r *http.Request, d ciExchangeAuditDetail) {
	if rt.auditLog == nil {
		return
	}
	action, status := auditActionCIExchangeIssued, http.StatusOK
	if d.Reason != ciExchangeReasonIssued {
		action, status = auditActionCIExchangeRefused, http.StatusForbidden
		ok, suppressed := ciExchangeAudits().admit(rt.ciAuditScope()+"|"+d.App+"|"+d.Provider+"|"+d.Reason, time.Now())
		if !ok {
			return
		}
		d.Suppressed = suppressed
	}
	id, err := store.NewAuditEntryID()
	if err != nil {
		return
	}
	detail, _ := json.Marshal(d)
	entry := store.AuditEntry{
		ID: id, ActorType: "system", ActorName: "ci-oidc", Ability: AbilityDeploy, Method: auditMethodEvent,
		Path: "/api/v1/ci/oidc-exchange", StatusCode: status, CreatedAt: store.FormatAuditTime(time.Now()),
		ClientKind: clientKindFromUserAgent(r.UserAgent()), Action: action, RemoteAddr: clientIP(r), Detail: string(detail),
	}
	if err := rt.auditLog.SaveAuditEntry(ctx, entry); err != nil {
		rt.logger.Warn("api: save ci exchange audit entry failed", slog.String("app", d.App), slog.String("error", err.Error()))
	}
}

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

// ciAuditScope keys the shared window per router, so two routers in one process never suppress each other's rows.
func (rt *Router) ciAuditScope() string { return fmt.Sprintf("%p", rt) }

// ciUnverifiedProvider labels a rejected token without trusting its issuer beyond the label.
func ciUnverifiedProvider(rawToken string) string {
	if iss := unverifiedCIIssuer(rawToken); iss != "" && iss != githubActionsOIDCIssuer {
		return "unverified"
	}
	return ciProviderGitHub
}

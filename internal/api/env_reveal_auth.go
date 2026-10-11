package api

import (
	"context"
	"log/slog"
	"net/http"
	"slices"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// AbilitySecretsReveal lets an API token reveal secret values, and only
// together with read:sensitive, APP_SECRET_REVEAL_TOKENS=on and a proof of
// its owner's password. Root never implies it.
const AbilitySecretsReveal = "secrets:reveal"

const (
	auditActionEnvRevealed      = "env.revealed"
	auditActionEnvExported      = "env.exported"
	auditActionEnvSettings      = "env.settings_changed"
	auditActionEnvConverted     = "env.converted_to_secret"
	envAuditActorSession        = "session"
	envAuditActorToken          = "token"
	errCodeRevealDisabled       = "reveal_disabled"
	errCodeAppWriteOnly         = "app_write_only" //nolint:gosec // an error code, not a credential
	errCodeStepUpRequired       = "step_up_required"
	errCodeTokenRevealAbility   = "token_reveal_ability"
	errCodeTokenRevealDisabled  = "token_reveal_disabled"
	errCodeRevealSessionLink    = "reveal_session_link"
	errCodeRevealSettingsFailed = "reveal_settings_unavailable"
)

type codedError struct {
	Error string `json:"error"`
	Code  string `json:"code"`
}

func writeCodedError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, codedError{Error: msg, Code: code})
}

type envAuditActor struct {
	kind, id, name, method string
}

// saveEnvAudit writes one audit row for an env action. path names the app
// and key, never a value.
func (rt *Router) saveEnvAudit(ctx context.Context, r *http.Request, actor envAuditActor, action, path string, status int) {
	if rt.auditLog == nil {
		return
	}
	id, err := store.NewAuditEntryID()
	if err != nil {
		rt.logger.Warn("api: env audit id failed", slog.String("error", err.Error()))
		return
	}
	name := actor.name
	if name == "" {
		name = rt.auditActorName(ctx, actor.kind, actor.id, "")
	}
	if actor.method != "" {
		path += " via " + actor.method
	}
	entry := store.AuditEntry{
		ID: id, ActorType: actor.kind, ActorID: actor.id, ActorName: name,
		Ability: AbilityReadSensitive, Method: r.Method, Path: path, StatusCode: status,
		RemoteAddr: clientIP(r), CreatedAt: store.FormatAuditTime(time.Now()),
		ClientKind: clientKindFromUserAgent(r.UserAgent()), Action: action,
	}
	if err := rt.auditLog.SaveAuditEntry(ctx, entry); err != nil {
		rt.logger.Warn("api: save env audit entry failed", slog.String("action", action), slog.String("error", err.Error()))
	}
}

type revealOptions struct {
	ignoreLock   bool
	ignorePolicy bool
}

// authorizeReveal decides whether r may read real values for app under
// action. It writes the refusal itself and fails closed on any doubt.
func (rt *Router) authorizeReveal(w http.ResponseWriter, r *http.Request, app, action string, proof *stepUpProof, opts revealOptions) (envAuditActor, bool) {
	policy, _ := revealPolicy()
	if policy == revealPolicyDisabled && !opts.ignorePolicy {
		writeCodedError(w, http.StatusForbidden, errCodeRevealDisabled, "secret values are write-only on this control plane (APP_SECRET_REVEAL=disabled)")
		return envAuditActor{}, false
	}
	settings, ok := rt.envSettingsFor(r.Context(), app)
	if !ok {
		writeCodedError(w, http.StatusServiceUnavailable, errCodeRevealSettingsFailed, "could not read this app's protection settings, try again")
		return envAuditActor{}, false
	}
	if settings.SecretsWriteOnly && !opts.ignoreLock {
		writeCodedError(w, http.StatusForbidden, errCodeAppWriteOnly, "this app's owner made its secrets write-only")
		return envAuditActor{}, false
	}
	if _, pinned := rt.currentPinnedSession(r); pinned {
		writeCodedError(w, http.StatusForbidden, errCodeRevealSessionLink, "a session link cannot reveal secret values")
		return envAuditActor{}, false
	}
	if userID, ok := rt.currentSessionUserID(r); ok {
		return rt.authorizeSessionReveal(w, r, userID, app, action, policy, proof)
	}
	return rt.authorizeTokenReveal(w, r, policy, proof)
}

func (rt *Router) authorizeSessionReveal(w http.ResponseWriter, r *http.Request, userID, app, action, policy string, proof *stepUpProof) (envAuditActor, bool) {
	actor := envAuditActor{kind: envAuditActorSession, id: userID}
	if policy == revealPolicyAllow {
		actor.method = stepUpMethodPolicy
		return actor, true
	}
	c, err := r.Cookie(sessionCookieName)
	if err != nil {
		writeCodedError(w, http.StatusForbidden, errCodeStepUpRequired, "confirm it's you to reveal this value")
		return envAuditActor{}, false
	}
	if g, ok := rt.envProtect.grants.check(c.Value, userID, action, app, time.Now()); ok {
		actor.method = g.method
		return actor, true
	}
	if !proof.empty() {
		if err := rt.verifyPasswordProof(r, userID, *proof); err != nil {
			writeStepUpError(w, err)
			return envAuditActor{}, false
		}
		actor.method = stepUpMethodPassword
		return actor, true
	}
	writeCodedError(w, http.StatusForbidden, errCodeStepUpRequired, "confirm it's you to reveal this value")
	return envAuditActor{}, false
}

func (rt *Router) authorizeTokenReveal(w http.ResponseWriter, r *http.Request, policy string, proof *stepUpProof) (envAuditActor, bool) {
	token, ok := bearerToken(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return envAuditActor{}, false
	}
	rec, err := rt.lookupBearerToken(r.Context(), token)
	if err != nil || rec.RevokedAt != nil || (rec.ExpiresAt != nil && time.Now().After(*rec.ExpiresAt)) {
		writeError(w, http.StatusUnauthorized, "invalid token")
		return envAuditActor{}, false
	}
	if !slices.Contains(rec.Abilities, AbilitySecretsReveal) {
		writeCodedError(w, http.StatusForbidden, errCodeTokenRevealAbility, "API tokens cannot reveal secret values unless they carry the secrets:reveal ability")
		return envAuditActor{}, false
	}
	if !tokenRevealAllowed() {
		writeCodedError(w, http.StatusForbidden, errCodeTokenRevealDisabled, "API token reveal is off on this control plane (APP_SECRET_REVEAL_TOKENS)")
		return envAuditActor{}, false
	}
	if rec.OwnerUserID == "" {
		writeCodedError(w, http.StatusForbidden, errCodeTokenRevealAbility, "this token has no owning user to confirm a reveal against")
		return envAuditActor{}, false
	}
	if gone, oerr := rt.tokenOwnerGone(r.Context(), rec); oerr != nil || gone {
		writeError(w, http.StatusUnauthorized, "invalid token")
		return envAuditActor{}, false
	}
	actor := envAuditActor{kind: envAuditActorToken, id: rec.ID, name: rec.Name}
	if policy == revealPolicyAllow {
		actor.method = stepUpMethodPolicy
		return actor, true
	}
	if proof.empty() {
		writeCodedError(w, http.StatusForbidden, errCodeStepUpRequired, "send the token owner's password (and authenticator code) as step_up to reveal")
		return envAuditActor{}, false
	}
	if err := rt.verifyPasswordProof(r, rec.OwnerUserID, *proof); err != nil {
		writeStepUpError(w, err)
		return envAuditActor{}, false
	}
	actor.method = stepUpMethodPassword
	return actor, true
}

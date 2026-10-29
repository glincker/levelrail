package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Event names for auth routes that change who can reach the platform but
// had no audit_log row of their own.
const (
	accountEvent2FASetupStarted    = "account.2fa_setup_started"
	accountEventPasskeyRegBegun    = "account.passkey_registration_started"
	accountEventInviteAccepted     = "account.invite_accepted"
	accountEventTeamInviteAccepted = "account.team_invite_accepted"
	accountEventTeamLeft           = "account.team_left"
)

// recordAuthAudit writes one audit_log row for the acting account with ids
// only in detail (no emails, tokens or codes). Best effort, like every audit write.
func (rt *Router) recordAuthAudit(r *http.Request, userID, action string, status int, detail map[string]string) {
	if rt.auditLog == nil {
		return
	}
	ctx := context.WithoutCancel(r.Context())
	id, err := store.NewAuditEntryID()
	if err != nil {
		rt.logger.Warn("api: auth audit id failed", slog.String("error", err.Error()))
		return
	}
	actorType, actorID, actorName := auditActorSession, userID, ""
	if rec, ok := r.Context().Value(tokenIdentityKey{}).(*store.APIToken); ok {
		if _, viaSession := rt.currentSessionUserID(r); !viaSession {
			actorType, actorID, actorName = "token", rec.ID, rec.Name
		}
	}
	if actorType == auditActorSession {
		actorName = rt.auditActorName(ctx, actorType, actorID, "")
	}
	entry := store.AuditEntry{
		ID: id, ActorType: actorType, ActorID: actorID, ActorName: actorName,
		Ability: AbilityWrite, Method: r.Method, Path: r.URL.Path, StatusCode: status,
		RemoteAddr: clientIP(r), CreatedAt: store.FormatAuditTime(time.Now()),
		ClientKind: clientKindFromUserAgent(r.UserAgent()), Action: action,
	}
	if len(detail) > 0 {
		if b, err := json.Marshal(detail); err == nil {
			entry.Detail = string(b)
		}
	}
	if err := rt.auditLog.SaveAuditEntry(ctx, entry); err != nil {
		rt.logger.Warn("api: save auth audit failed", slog.String("action", action), slog.String("error", err.Error()))
	}
}

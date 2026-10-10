package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	signInAuditActor     = "Sign-in"
	loginCodeAuditPrefix = "/api/v1/auth/login-code/"
	approvalAuditPrefix  = "/api/v1/auth/login-approvals/"
	trustedAuditPrefix   = "/api/v1/auth/trusted-devices/"
	approverAuditPrefix  = "/api/v1/auth/tokens/signin-approve#"
)

// signInActor is who an audit row about a sign-in names: the deciding
// session or token, or the system for an unauthenticated requester.
type signInActor struct {
	kind, id, name string
}

func anonymousSignIn(userID string) signInActor {
	return signInActor{kind: auditActorSystem, id: userID, name: signInAuditActor}
}

// auditSignIn writes one audit row. path carries a challenge, approval or
// device id, never a code or cookie value. r may be nil for a sweep.
func (rt *Router) auditSignIn(ctx context.Context, r *http.Request, actor signInActor, action, path string, status int) {
	id, err := store.NewAuditEntryID()
	if err != nil {
		rt.logger.Warn("api: sign-in audit id failed", slog.String("error", err.Error()))
		return
	}
	entry := store.AuditEntry{
		ID: id, ActorType: actor.kind, ActorID: actor.id, ActorName: actor.name,
		Ability: action, Method: auditMethodEvent, Path: path, StatusCode: status,
		CreatedAt: store.FormatAuditTime(time.Now()), ClientKind: auditClientKindSystem, Action: action,
	}
	if r != nil {
		entry.Method = r.Method
		entry.RemoteAddr = clientIP(r)
		entry.ClientKind = clientKindFromUserAgent(r.UserAgent())
	}
	if err := rt.auditLog.SaveAuditEntry(ctx, entry); err != nil {
		rt.logger.Warn("api: save sign-in audit entry failed", slog.String("action", action), slog.String("error", err.Error()))
	}
}

// SweepSignInExpiry expires lapsed codes and approvals, auditing each once.
func (rt *Router) SweepSignInExpiry(ctx context.Context, now time.Time) error {
	if rt.loginCodes == nil {
		return nil
	}
	codes, err := rt.loginCodes.ClaimLapsedLoginCodes(ctx, now, loginSweepBatch)
	for _, c := range codes {
		rt.codeLogin.forget(c.ID)
		rt.auditSignIn(ctx, nil, anonymousSignIn(c.UserID), store.AuditActionLoginCodeExpire, loginCodeAuditPrefix+c.ID, http.StatusOK)
	}
	if err != nil {
		return err
	}
	approvals, err := rt.loginCodes.ClaimLapsedLoginApprovals(ctx, now, loginSweepBatch)
	for _, a := range approvals {
		rt.auditSignIn(ctx, nil, anonymousSignIn(a.UserID), store.AuditActionNewDeviceExpire, approvalAuditPrefix+a.ID, http.StatusOK)
	}
	if err != nil {
		return err
	}
	rt.codeLogin.purgeExpired(now)
	if err := rt.loginCodes.PruneTrustedDevices(ctx, now); err != nil {
		return err
	}
	return rt.loginCodes.PruneLoginCodes(ctx, now.Add(-deviceHistoryRetention()))
}

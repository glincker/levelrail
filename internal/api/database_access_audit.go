package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/dbaccess"
	"github.com/GLINCKER/levelrail/internal/store"
)

func decodeJSONBody(r *http.Request, dst any) error {
	if r.Body == nil || r.ContentLength == 0 {
		return nil
	}
	err := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<16)).Decode(dst)
	if errors.Is(err, io.EOF) {
		return nil
	}
	return err
}

type accessActor struct {
	kind, id, name string
}

// accessActor resolves who is calling, for created_by and audit rows.
func (rt *Router) accessActor(r *http.Request) accessActor {
	pt, pid, _, err := rt.callerPrincipal(r)
	if err != nil {
		return accessActor{kind: "session", id: "unknown", name: "unknown"}
	}
	kind := "session"
	if pt == store.PrincipalTypeToken {
		kind = "token"
	}
	name := pid
	if kind == "token" && rt.tokens != nil {
		if t, terr := rt.tokens.GetAPITokenByID(r.Context(), pid); terr == nil && t != nil {
			name = t.Name
		}
	}
	return accessActor{kind: kind, id: pid, name: rt.auditActorName(r.Context(), kind, pid, name)}
}

// auditDatabaseAccess writes an audit row named by an action constant. The
// subject is a role or principal, never a credential.
func (rt *Router) auditDatabaseAccess(r *http.Request, action, database, subject string, status int) {
	a := rt.accessActor(r)
	id, err := store.NewAuditEntryID()
	if err != nil {
		rt.logger.Warn("api: database access audit id failed", slog.String("error", err.Error()))
		return
	}
	path := "/api/v1/databases/" + database
	if subject != "" {
		path += "#" + subject
	}
	entry := store.AuditEntry{
		ID: id, ActorType: a.kind, ActorID: a.id, ActorName: a.name,
		Ability: AbilityRoot, Method: action, Path: path, StatusCode: status,
		RemoteAddr: clientIP(r), CreatedAt: store.FormatAuditTime(time.Now()),
		ClientKind: clientKindFromUserAgent(r.Header.Get("User-Agent")),
	}
	if err := rt.auditLog.SaveAuditEntry(r.Context(), entry); err != nil {
		rt.logger.Warn("api: database access audit save failed", slog.String("error", err.Error()), slog.String("database", database))
	}
}

// RecordRevoked implements dbaccess.SweepAuditor: the sweeper's own audit row.
func (rt *Router) RecordRevoked(ctx context.Context, c dbaccess.TempCredential) {
	id, err := store.NewAuditEntryID()
	if err != nil {
		rt.logger.Warn("api: temp credential audit id failed", slog.String("error", err.Error()))
		return
	}
	entry := store.AuditEntry{
		ID: id, ActorType: "system", ActorName: dbaccess.SystemActor, Ability: AbilityRoot,
		Method: dbaccess.ActionTempExpire, Path: "/api/v1/databases/" + c.Database + "#" + c.Role,
		StatusCode: http.StatusOK, CreatedAt: store.FormatAuditTime(time.Now()), ClientKind: "system",
	}
	if err := rt.auditLog.SaveAuditEntry(ctx, entry); err != nil {
		rt.logger.Warn("api: temp credential expiry audit save failed", slog.String("error", err.Error()), slog.String("database", c.Database))
	}
}

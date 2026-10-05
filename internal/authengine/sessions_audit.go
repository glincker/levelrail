package authengine

import (
	"context"
	"database/sql"
	"errors"
	"net/http"

	theauth "github.com/glincker/theauth-go/v2"
)

// AuditRecord is one auth event shaped like a platform audit log row.
type AuditRecord struct {
	ActorType  string
	ActorID    string
	ActorName  string
	Ability    string
	Method     string
	Path       string
	StatusCode int
	RemoteAddr string
}

type auditShape struct {
	method, path string
	status       int
}

// auditShapes maps library event types onto the platform routes that
// produced the same outcome before the switch, so the audit log reads the same.
var auditShapes = map[theauth.AuthEventType]auditShape{
	theauth.AuthEventLoginSuccess:           {http.MethodPost, "/api/v1/auth/login", http.StatusOK},
	theauth.AuthEventLoginFailure:           {http.MethodPost, "/api/v1/auth/login", http.StatusUnauthorized},
	theauth.AuthEventPasswordChanged:        {http.MethodPut, "/api/v1/auth/password", http.StatusNoContent},
	theauth.AuthEventPasswordResetRequested: {http.MethodPost, "/api/v1/auth/forgot-password", http.StatusNoContent},
	theauth.AuthEventPasswordResetCompleted: {http.MethodPost, "/api/v1/auth/reset-password", http.StatusNoContent},
	theauth.AuthEventSessionRevoked:         {http.MethodPost, "/api/v1/auth/logout", http.StatusNoContent},
}

func (s *Sessions) eventSink(_ context.Context, e theauth.AuthEvent) {
	if _, ok := auditShapes[e.Type]; !ok || s.hooks.Audit == nil {
		return
	}
	select {
	case s.events <- e:
	default:
		s.logger.Warn("authengine: audit queue full, dropping event", "type", string(e.Type))
	}
}

func (s *Sessions) pumpAudit() {
	defer close(s.pumped)
	for {
		select {
		case <-s.done:
			return
		case e := <-s.events:
			s.writeAudit(e)
		}
	}
}

func (s *Sessions) writeAudit(e theauth.AuthEvent) {
	shape := auditShapes[e.Type]
	ctx, cancel := context.WithTimeout(context.Background(), dispatchTimeout)
	defer cancel()
	rec := AuditRecord{
		ActorType:  "system",
		Ability:    AbilityWrite,
		Method:     shape.method,
		Path:       shape.path,
		StatusCode: shape.status,
		RemoteAddr: e.IPPrefix,
	}
	if e.UserID != "" {
		var legacy, name string
		err := s.db.QueryRowContext(ctx, `
			SELECT u.id, u.display_name FROM authengine_user_map m JOIN users u ON u.id = m.legacy_id
			WHERE m.engine_id = ?`, e.UserID).Scan(&legacy, &name)
		switch {
		case err == nil:
			rec.ActorType, rec.ActorID, rec.ActorName = "session", legacy, name
		case !errors.Is(err, sql.ErrNoRows):
			s.logger.Warn("authengine: audit actor lookup failed", "error", err.Error())
		}
	}
	s.hooks.Audit(ctx, rec)
}

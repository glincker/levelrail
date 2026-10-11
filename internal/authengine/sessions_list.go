package authengine

import (
	"context"
	"errors"
	"fmt"
	"time"

	theauth "github.com/glincker/theauth-go/v2"
)

// SessionView is one live, fully signed-in session of a platform user.
// UserAgent and IP are raw; the caller labels and masks them for display.
type SessionView struct {
	ID         string
	UserAgent  string
	IP         string
	IPPrefix   string
	CreatedAt  time.Time
	LastSeenAt time.Time
	ExpiresAt  time.Time
	Current    bool
}

// SessionIDFor returns the id of the session token names, if it is live.
func (s *Sessions) SessionIDFor(ctx context.Context, token string) (string, bool) {
	sid, _, ok := s.lookupSession(ctx, token)
	if !ok {
		return "", false
	}
	return sid.String(), true
}

// ListUserSessions lists a platform user's live sessions, newest first,
// marking the one currentToken names.
func (s *Sessions) ListUserSessions(ctx context.Context, legacyUserID, currentToken string) ([]SessionView, error) {
	engine, err := s.engineID(ctx, legacyUserID)
	if err != nil {
		return nil, err
	}
	if engine == "" {
		return []SessionView{}, nil
	}
	id, err := parseULID(engine)
	if err != nil {
		return nil, fmt.Errorf("authengine: parse engine user id: %w", err)
	}
	var current theauth.ULID
	if currentToken != "" {
		current, _, _ = s.lookupSession(ctx, currentToken)
	}
	s.mu.RLock()
	a := s.auth
	s.mu.RUnlock()
	list, err := a.ListSessions(ctx, id, current)
	if err != nil {
		return nil, fmt.Errorf("authengine: list sessions: %w", err)
	}
	raw, err := s.rawSessionContext(ctx, engine)
	if err != nil {
		return nil, err
	}
	out := make([]SessionView, 0, len(list))
	for _, si := range list {
		ctxRow := raw[si.ID.String()]
		out = append(out, SessionView{
			ID: si.ID.String(), UserAgent: ctxRow[0], IP: ctxRow[1], IPPrefix: si.IPPrefix,
			CreatedAt: si.CreatedAt, LastSeenAt: si.LastSeenAt, ExpiresAt: si.ExpiresAt, Current: si.Current,
		})
	}
	return out, nil
}

// rawSessionContext maps session id to its stored user agent and address.
func (s *Sessions) rawSessionContext(ctx context.Context, engineUserID string) (map[string][2]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, user_agent, ip FROM theauth_sessions WHERE user_id = ? AND revoked_at IS NULL`, engineUserID)
	if err != nil {
		return nil, fmt.Errorf("authengine: read session context: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string][2]string{}
	for rows.Next() {
		var id, ua, ip string
		if err := rows.Scan(&id, &ua, &ip); err != nil {
			return nil, fmt.Errorf("authengine: scan session context: %w", err)
		}
		out[id] = [2]string{ua, ip}
	}
	return out, rows.Err()
}

// RevokeUserSession ends one session of a platform user. It reports false,
// not an error, for a session that is unknown or belongs to someone else.
func (s *Sessions) RevokeUserSession(ctx context.Context, legacyUserID, sessionID string) (bool, error) {
	engine, err := s.engineID(ctx, legacyUserID)
	if err != nil {
		return false, err
	}
	if engine == "" {
		return false, nil
	}
	uid, err := parseULID(engine)
	if err != nil {
		return false, fmt.Errorf("authengine: parse engine user id: %w", err)
	}
	sid, err := parseULID(sessionID)
	if err != nil {
		return false, nil
	}
	s.mu.RLock()
	a := s.auth
	s.mu.RUnlock()
	ctx = theauth.WithAuditMetadata(ctx, theauth.AuditMetadata{ActorUserID: &uid})
	if err := a.RevokeOwnedSession(ctx, uid, sid); err != nil {
		if errors.Is(err, theauth.ErrStorageNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("authengine: revoke session: %w", err)
	}
	return true, nil
}

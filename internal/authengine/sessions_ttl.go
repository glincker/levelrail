package authengine

import (
	"context"
	"fmt"
	"time"
)

// IssueSessionTTL is IssueSession with its own lifetime. The library has one global
// session TTL, so the shorter expiry is written to the session row, where it survives restarts.
func (s *Sessions) IssueSessionTTL(ctx context.Context, u LegacyUser, userAgent, ip string, ttl time.Duration) (string, error) {
	tok, err := s.IssueSession(ctx, u, userAgent, ip)
	if err != nil {
		return "", err
	}
	sid, ok := s.SessionIDFor(ctx, tok)
	if !ok {
		s.Revoke(ctx, tok)
		return "", fmt.Errorf("authengine: new session not found")
	}
	// The library stores times as microseconds since the epoch.
	if _, err := s.db.ExecContext(ctx, `UPDATE theauth_sessions SET expires_at = ? WHERE id = ?`, time.Now().Add(ttl).UnixMicro(), sid); err != nil {
		s.Revoke(ctx, tok)
		return "", fmt.Errorf("authengine: set session expiry: %w", err)
	}
	return tok, nil
}

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Gate access rule kinds (migrations/0530).
const (
	GateRuleUser = "user"
	GateRuleTeam = "team"
	GateRuleRole = "role"
)

// Gate access log decisions.
const (
	GateDecisionAllowed        = "allowed"
	GateDecisionDenied         = "denied"
	GateDecisionLoginRedirect  = "login_redirect"
	GateDecisionTokenIssued    = "token_issued"
	GateDecisionTokenExchange  = "token_exchange"
	GateDecisionSessionRevoked = "session_revoked"
)

// Fixed width so string comparison in SQL orders timestamps correctly.
const gateTimeFormat = "2006-01-02T15:04:05.000000Z07:00"

// GateEventBucket is the minute an event coalesces into.
func GateEventBucket(t time.Time) string { return t.UTC().Format("2006-01-02T15:04") }

func gateTime(t time.Time) string { return t.UTC().Format(gateTimeFormat) }

func parseGateTime(s string) time.Time {
	t, _ := time.Parse(gateTimeFormat, s)
	return t
}

// GateRule lets one user, team or role through a built-in auth gate.
type GateRule struct {
	Kind    string
	Subject string
}

// GateToken is a single-use handoff from the dashboard to an app host.
// Only hashes are stored.
type GateToken struct {
	TokenHash       string
	Domain          string
	UserID          string
	ParentSessionID string
	StateHash       string
	ExpiresAt       time.Time
}

// GateSession is a host-bound sign-in on a gated app domain, tied to the
// dashboard session it was minted from.
type GateSession struct {
	TokenHash       string
	Domain          string
	UserID          string
	ParentSessionID string
	CreatedAt       time.Time
	ExpiresAt       time.Time
	Revoked         bool
}

// GateEvent is one access log row. Count > 1 means coalesced allows.
type GateEvent struct {
	At        time.Time
	Domain    string
	UserID    string
	UserEmail string
	Decision  string
	Reason    string
	IP        string
	Count     int
}

// ListGateRules returns domain's access rules.
func (db *DB) ListGateRules(ctx context.Context, domain string) ([]GateRule, error) {
	rows, err := db.QueryContext(ctx, `SELECT kind, subject FROM gate_access_rules WHERE domain = ? ORDER BY kind, subject`, domain)
	if err != nil {
		return nil, fmt.Errorf("store: list gate rules: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []GateRule{}
	for rows.Next() {
		var r GateRule
		if err := rows.Scan(&r.Kind, &r.Subject); err != nil {
			return nil, fmt.Errorf("store: scan gate rule: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ReplaceGateRules swaps domain's rules atomically. The gate row must exist.
func (db *DB) ReplaceGateRules(ctx context.Context, domain string, rules []GateRule) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: replace gate rules: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM gate_access_rules WHERE domain = ?`, domain); err != nil {
		return fmt.Errorf("store: clear gate rules: %w", err)
	}
	now := gateTime(time.Now())
	for _, r := range rules {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO gate_access_rules (domain, kind, subject, created_at) VALUES (?, ?, ?, ?)`,
			domain, r.Kind, r.Subject, now); err != nil {
			return fmt.Errorf("store: insert gate rule: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit gate rules: %w", err)
	}
	return nil
}

// CreateGateToken stores a handoff token hash.
func (db *DB) CreateGateToken(ctx context.Context, t GateToken) error {
	_, err := db.ExecContext(ctx, `INSERT INTO gate_tokens (token_hash, domain, user_id, parent_session_id, state_hash, created_at, expires_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		t.TokenHash, t.Domain, t.UserID, t.ParentSessionID, t.StateHash, gateTime(time.Now()), gateTime(t.ExpiresAt))
	if err != nil {
		return fmt.Errorf("store: create gate token: %w", err)
	}
	return nil
}

// ConsumeGateToken deletes and returns the token in one statement, so a
// replay finds nothing even under concurrent requests. Expiry is the caller's.
func (db *DB) ConsumeGateToken(ctx context.Context, tokenHash string) (GateToken, bool, error) {
	var t GateToken
	var expires string
	err := db.QueryRowContext(ctx, `DELETE FROM gate_tokens WHERE token_hash = ? RETURNING domain, user_id, parent_session_id, state_hash, expires_at`, tokenHash).
		Scan(&t.Domain, &t.UserID, &t.ParentSessionID, &t.StateHash, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return GateToken{}, false, nil
	}
	if err != nil {
		return GateToken{}, false, fmt.Errorf("store: consume gate token: %w", err)
	}
	t.TokenHash = tokenHash
	t.ExpiresAt = parseGateTime(expires)
	return t, true, nil
}

// CreateGateSession stores a gate session hash.
func (db *DB) CreateGateSession(ctx context.Context, s GateSession) error {
	_, err := db.ExecContext(ctx, `INSERT INTO gate_sessions (token_hash, domain, user_id, parent_session_id, created_at, expires_at) VALUES (?, ?, ?, ?, ?, ?)`,
		s.TokenHash, s.Domain, s.UserID, s.ParentSessionID, gateTime(time.Now()), gateTime(s.ExpiresAt))
	if err != nil {
		return fmt.Errorf("store: create gate session: %w", err)
	}
	return nil
}

// GetGateSession looks a gate session up by hash, revoked or not.
func (db *DB) GetGateSession(ctx context.Context, tokenHash string) (GateSession, bool, error) {
	var s GateSession
	var created, expires string
	var revoked sql.NullString
	err := db.QueryRowContext(ctx, `SELECT domain, user_id, parent_session_id, created_at, expires_at, revoked_at FROM gate_sessions WHERE token_hash = ?`, tokenHash).
		Scan(&s.Domain, &s.UserID, &s.ParentSessionID, &created, &expires, &revoked)
	if errors.Is(err, sql.ErrNoRows) {
		return GateSession{}, false, nil
	}
	if err != nil {
		return GateSession{}, false, fmt.Errorf("store: get gate session: %w", err)
	}
	s.TokenHash = tokenHash
	s.CreatedAt, s.ExpiresAt = parseGateTime(created), parseGateTime(expires)
	s.Revoked = revoked.Valid
	return s, true, nil
}

// RevokeGateSessions ends every live gate session on domain.
func (db *DB) RevokeGateSessions(ctx context.Context, domain string) (int64, error) {
	res, err := db.ExecContext(ctx, `UPDATE gate_sessions SET revoked_at = ? WHERE domain = ? AND revoked_at IS NULL`, gateTime(time.Now()), domain)
	if err != nil {
		return 0, fmt.Errorf("store: revoke gate sessions: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// CountLiveGateSessions counts unrevoked, unexpired gate sessions on domain.
func (db *DB) CountLiveGateSessions(ctx context.Context, domain string) (int, error) {
	var n int
	err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM gate_sessions WHERE domain = ? AND revoked_at IS NULL AND expires_at > ?`, domain, gateTime(time.Now())).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("store: count gate sessions: %w", err)
	}
	return n, nil
}

// DashboardSessionLive reports whether the dashboard sign-in a gate session
// came from is still valid, so signing out there ends app access too.
func (db *DB) DashboardSessionLive(ctx context.Context, sessionID string) (bool, error) {
	var one int
	err := db.QueryRowContext(ctx, `SELECT 1 FROM theauth_sessions WHERE id = ? AND revoked_at IS NULL AND auth_level = 'full' AND expires_at > ?`,
		sessionID, time.Now().UnixMicro()).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("store: check dashboard session: %w", err)
	}
	return true, nil
}

// DomainEnvironmentID returns the environment of the app owning domain, or
// "" for an untagged app or a standalone domain.
func (db *DB) DomainEnvironmentID(ctx context.Context, domain string) (string, error) {
	var env sql.NullString
	err := db.QueryRowContext(ctx, `SELECT s.environment_id FROM service_domains sd JOIN desired_services s ON s.name = sd.service_name WHERE sd.domain = ?`, domain).Scan(&env)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("store: environment of domain %q: %w", domain, err)
	}
	return env.String, nil
}

// RecordAuthGateEvent appends an access log row. Identical events in the
// same minute coalesce into one row, which bounds growth under floods.
func (db *DB) RecordAuthGateEvent(ctx context.Context, e GateEvent) error {
	at := e.At
	if at.IsZero() {
		at = time.Now()
	}
	count := e.Count
	if count < 1 {
		count = 1
	}
	bucket := GateEventBucket(at)
	_, err := db.ExecContext(ctx, `
		INSERT INTO gate_events (at, bucket, domain, user_id, user_email, decision, reason, ip, count)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (domain, user_id, decision, reason, ip, bucket)
		DO UPDATE SET count = count + excluded.count, at = MAX(at, excluded.at)
	`, gateTime(at), bucket, e.Domain, e.UserID, e.UserEmail, e.Decision, e.Reason, e.IP, count)
	if err != nil {
		return fmt.Errorf("store: record gate event: %w", err)
	}
	return nil
}

// ListAuthGateEvents returns domain's events since a time, newest first.
func (db *DB) ListAuthGateEvents(ctx context.Context, domain string, since time.Time, limit int) ([]GateEvent, error) {
	rows, err := db.QueryContext(ctx, `SELECT at, domain, user_id, user_email, decision, reason, ip, count FROM gate_events
		WHERE domain = ? AND at >= ? ORDER BY at DESC, id DESC LIMIT ?`, domain, gateTime(since), limit)
	if err != nil {
		return nil, fmt.Errorf("store: list gate events: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []GateEvent{}
	for rows.Next() {
		var e GateEvent
		var at string
		if err := rows.Scan(&at, &e.Domain, &e.UserID, &e.UserEmail, &e.Decision, &e.Reason, &e.IP, &e.Count); err != nil {
			return nil, fmt.Errorf("store: scan gate event: %w", err)
		}
		e.At = parseGateTime(at)
		out = append(out, e)
	}
	return out, rows.Err()
}

// PruneGateData deletes events older than cutoff plus expired tokens and
// sessions, returning how many rows went in total.
func (db *DB) PruneGateData(ctx context.Context, cutoff time.Time) (int64, error) {
	now := gateTime(time.Now())
	var total int64
	for _, q := range []struct {
		sql string
		arg string
	}{
		{`DELETE FROM gate_events WHERE at < ?`, gateTime(cutoff)},
		{`DELETE FROM gate_tokens WHERE expires_at < ?`, now},
		{`DELETE FROM gate_sessions WHERE expires_at < ?`, now},
	} {
		res, err := db.ExecContext(ctx, q.sql, q.arg)
		if err != nil {
			return total, fmt.Errorf("store: prune gate data: %w", err)
		}
		n, _ := res.RowsAffected()
		total += n
	}
	return total, nil
}

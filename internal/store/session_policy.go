package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// ErrSessionPolicyNotFound is returned when a team has no policy.
var ErrSessionPolicyNotFound = errors.New("store: session policy not found")

// SessionPolicy is one team's session and access rules (migration 0612).
type SessionPolicy struct {
	TeamID      string
	IdleTimeout time.Duration
	MaxLifetime time.Duration
	IPAllowlist []string
	RequireMFA  bool
	// MaxSessions caps concurrent full sessions per user; 0 means no cap.
	MaxSessions int
	// OnLimit is SessionOnLimitEvictOldest or SessionOnLimitReject.
	OnLimit   string
	UpdatedBy string
	UpdatedAt time.Time
}

// Session cap overflow modes (migration 0701).
const (
	SessionOnLimitEvictOldest = "evict_oldest"
	SessionOnLimitReject      = "reject"
)

const sessionPolicyColumns = `p.team_id, p.idle_timeout_seconds, p.max_lifetime_seconds, p.ip_allowlist, p.require_mfa, p.max_concurrent_sessions, p.on_limit, p.updated_by, p.updated_at`

func scanSessionPolicy(scan func(dest ...any) error) (SessionPolicy, error) {
	var (
		p               SessionPolicy
		idle, life, mfa int64
		maxSessions     int64
		allowlist, upd  string
	)
	err := scan(&p.TeamID, &idle, &life, &allowlist, &mfa, &maxSessions, &p.OnLimit, &p.UpdatedBy, &upd)
	if errors.Is(err, sql.ErrNoRows) {
		return SessionPolicy{}, ErrSessionPolicyNotFound
	}
	if err != nil {
		return SessionPolicy{}, fmt.Errorf("store: scan session policy: %w", err)
	}
	if err := json.Unmarshal([]byte(allowlist), &p.IPAllowlist); err != nil {
		return SessionPolicy{}, fmt.Errorf("store: session policy %q allowlist: %w", p.TeamID, err)
	}
	p.IdleTimeout, p.MaxLifetime, p.RequireMFA = time.Duration(idle)*time.Second, time.Duration(life)*time.Second, mfa == 1
	p.MaxSessions = int(maxSessions)
	if p.UpdatedAt, err = time.Parse(time.RFC3339Nano, upd); err != nil {
		return SessionPolicy{}, fmt.Errorf("store: session policy %q updated_at: %w", p.TeamID, err)
	}
	return p, nil
}

func (db *DB) querySessionPolicies(ctx context.Context, query string, args ...any) ([]SessionPolicy, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list session policies: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []SessionPolicy
	for rows.Next() {
		p, err := scanSessionPolicy(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list session policies: %w", err)
	}
	return out, nil
}

// ListSessionPolicies returns every team's policy.
func (db *DB) ListSessionPolicies(ctx context.Context) ([]SessionPolicy, error) {
	return db.querySessionPolicies(ctx, `SELECT `+sessionPolicyColumns+` FROM session_policies p ORDER BY p.team_id`)
}

// SessionPoliciesForUser returns the policies of every team the user belongs to.
func (db *DB) SessionPoliciesForUser(ctx context.Context, userID string) ([]SessionPolicy, error) {
	return db.querySessionPolicies(ctx, `SELECT `+sessionPolicyColumns+` FROM session_policies p
		JOIN team_members m ON m.team_id = p.team_id WHERE m.user_id = ? ORDER BY p.team_id`, userID)
}

// GetSessionPolicy returns one team's policy.
func (db *DB) GetSessionPolicy(ctx context.Context, teamID string) (SessionPolicy, error) {
	return scanSessionPolicy(db.QueryRowContext(ctx, `SELECT `+sessionPolicyColumns+` FROM session_policies p WHERE p.team_id = ?`, teamID).Scan)
}

// SaveSessionPolicy creates or replaces a team's policy.
func (db *DB) SaveSessionPolicy(ctx context.Context, p SessionPolicy) error {
	allowlist, err := json.Marshal(nonNilSlice(p.IPAllowlist))
	if err != nil {
		return fmt.Errorf("store: marshal allowlist: %w", err)
	}
	if p.OnLimit == "" {
		p.OnLimit = SessionOnLimitEvictOldest
	}
	_, err = db.ExecContext(ctx, `INSERT INTO session_policies (team_id, idle_timeout_seconds, max_lifetime_seconds, ip_allowlist, require_mfa, max_concurrent_sessions, on_limit, updated_by, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (team_id) DO UPDATE SET idle_timeout_seconds = excluded.idle_timeout_seconds,
			max_lifetime_seconds = excluded.max_lifetime_seconds, ip_allowlist = excluded.ip_allowlist,
			require_mfa = excluded.require_mfa, max_concurrent_sessions = excluded.max_concurrent_sessions,
			on_limit = excluded.on_limit, updated_by = excluded.updated_by, updated_at = excluded.updated_at`,
		p.TeamID, int64(p.IdleTimeout/time.Second), int64(p.MaxLifetime/time.Second), string(allowlist), boolInt(p.RequireMFA), p.MaxSessions, p.OnLimit, p.UpdatedBy, formatTime(p.UpdatedAt))
	if err != nil {
		return fmt.Errorf("store: save session policy %q: %w", p.TeamID, err)
	}
	return nil
}

// DeleteSessionPolicy removes a team's policy.
func (db *DB) DeleteSessionPolicy(ctx context.Context, teamID string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM session_policies WHERE team_id = ?`, teamID)
	if err != nil {
		return fmt.Errorf("store: delete session policy %q: %w", teamID, err)
	}
	return rowsAffectedOrNotFound(res, ErrSessionPolicyNotFound, "delete session policy %q", teamID)
}

// ClearSessionPolicies removes every policy, the host-side lockout recovery.
func (db *DB) ClearSessionPolicies(ctx context.Context) (int64, error) {
	res, err := db.ExecContext(ctx, `DELETE FROM session_policies`)
	if err != nil {
		return 0, fmt.Errorf("store: clear session policies: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// SessionActivity is what the policy needs about one live session.
type SessionActivity struct {
	CreatedAt  time.Time
	LastSeenAt time.Time
	Method     string
	Seen       bool
}

// GetSessionActivity reads the library creation time and the tracked last
// use of a session. Seen is false until the session is first touched.
func (db *DB) GetSessionActivity(ctx context.Context, sessionID string) (SessionActivity, error) {
	var (
		a       SessionActivity
		created int64
		seen    sql.NullInt64
		method  sql.NullString
	)
	err := db.QueryRowContext(ctx, `SELECT s.created_at, a.last_seen_at, a.method FROM theauth_sessions s
		LEFT JOIN session_activity a ON a.session_id = s.id WHERE s.id = ?`, sessionID).Scan(&created, &seen, &method)
	if err != nil {
		return SessionActivity{}, fmt.Errorf("store: session activity %q: %w", sessionID, err)
	}
	a.CreatedAt = time.UnixMicro(created).UTC()
	if seen.Valid {
		a.Seen, a.LastSeenAt = true, time.Unix(seen.Int64, 0).UTC()
	}
	a.Method = method.String
	return a, nil
}

// TouchSessionActivity records that the session was used at now.
func (db *DB) TouchSessionActivity(ctx context.Context, sessionID, userID string, now time.Time) error {
	_, err := db.ExecContext(ctx, `INSERT INTO session_activity (session_id, user_id, last_seen_at) VALUES (?, ?, ?)
		ON CONFLICT (session_id) DO UPDATE SET last_seen_at = excluded.last_seen_at`, sessionID, userID, now.Unix())
	if err != nil {
		return fmt.Errorf("store: touch session activity: %w", err)
	}
	return nil
}

// SetSessionMethod records how a session signed in, for policies that trust the IdP's MFA.
func (db *DB) SetSessionMethod(ctx context.Context, sessionID, userID, method string, now time.Time) error {
	_, err := db.ExecContext(ctx, `INSERT INTO session_activity (session_id, user_id, method, last_seen_at) VALUES (?, ?, ?, ?)
		ON CONFLICT (session_id) DO UPDATE SET method = excluded.method`, sessionID, userID, method, now.Unix())
	if err != nil {
		return fmt.Errorf("store: set session method: %w", err)
	}
	return nil
}

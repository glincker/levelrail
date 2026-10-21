package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Migration agent session states.
const (
	MigrationAgentPending   = "pending"
	MigrationAgentConnected = "connected"
	MigrationAgentLost      = "lost"
	MigrationAgentRevoked   = "revoked"
	MigrationAgentExpired   = "expired"
)

// Sentinel errors for migration agent sessions.
var (
	ErrMigrationAgentNotFound  = errors.New("store: migration agent session not found")
	ErrMigrationAgentTokenUsed = errors.New("store: migration agent token already used, expired or revoked")
)

// MigrationAgentSession is one read-only agent session on a source host.
// InventoryJSON and ScanJSON are owned by internal/migrateagent.
type MigrationAgentSession struct {
	ID               string
	Label            string
	TokenHash        string
	TokenExpiresAt   time.Time
	TokenUsedAt      *time.Time
	State            string
	CertFingerprint  string
	CertSerial       string
	SessionExpiresAt *time.Time
	ConnectedAt      *time.Time
	LastSeenAt       *time.Time
	RevokedAt        *time.Time
	AgentVersion     string
	AgentOS          string
	AgentArch        string
	InventoryJSON    string
	ScanJSON         string
	CreatedBy        string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

const migrationAgentColumns = `id, label, token_hash, token_expires_at, token_used_at, state, cert_fingerprint, cert_serial,
	session_expires_at, connected_at, last_seen_at, revoked_at, agent_version, agent_os, agent_arch,
	inventory_json, scan_json, created_by, created_at, updated_at`

func parseOptTime(s sql.NullString) *time.Time {
	if !s.Valid || s.String == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339Nano, s.String)
	if err != nil {
		return nil
	}
	return &t
}

func scanMigrationAgent(sc interface{ Scan(...any) error }) (MigrationAgentSession, error) {
	var s MigrationAgentSession
	var expires, used, sessExp, conn, seen, revoked sql.NullString
	var created, updated string
	err := sc.Scan(&s.ID, &s.Label, &s.TokenHash, &expires, &used, &s.State, &s.CertFingerprint, &s.CertSerial,
		&sessExp, &conn, &seen, &revoked, &s.AgentVersion, &s.AgentOS, &s.AgentArch,
		&s.InventoryJSON, &s.ScanJSON, &s.CreatedBy, &created, &updated)
	if err != nil {
		return MigrationAgentSession{}, err
	}
	if t := parseOptTime(expires); t != nil {
		s.TokenExpiresAt = *t
	}
	s.TokenUsedAt, s.SessionExpiresAt, s.ConnectedAt = parseOptTime(used), parseOptTime(sessExp), parseOptTime(conn)
	s.LastSeenAt, s.RevokedAt = parseOptTime(seen), parseOptTime(revoked)
	s.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	s.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return s, nil
}

func migTime(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

// SaveMigrationAgentSession inserts a new pending session.
func (db *DB) SaveMigrationAgentSession(ctx context.Context, s MigrationAgentSession) error {
	_, err := db.ExecContext(ctx, `INSERT INTO migration_agent_sessions (id, label, token_hash, token_expires_at, state, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, s.ID, s.Label, s.TokenHash, migTime(s.TokenExpiresAt),
		MigrationAgentPending, s.CreatedBy, migTime(s.CreatedAt), migTime(s.UpdatedAt))
	if err != nil {
		return fmt.Errorf("store: save migration agent session %q: %w", s.ID, err)
	}
	return nil
}

// GetMigrationAgentSession returns one session by id.
func (db *DB) GetMigrationAgentSession(ctx context.Context, id string) (MigrationAgentSession, error) {
	s, err := scanMigrationAgent(db.QueryRowContext(ctx, `SELECT `+migrationAgentColumns+` FROM migration_agent_sessions WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return MigrationAgentSession{}, ErrMigrationAgentNotFound
	}
	if err != nil {
		return MigrationAgentSession{}, fmt.Errorf("store: get migration agent session %q: %w", id, err)
	}
	return s, nil
}

// ListMigrationAgentSessions returns every session, newest first.
func (db *DB) ListMigrationAgentSessions(ctx context.Context) ([]MigrationAgentSession, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+migrationAgentColumns+` FROM migration_agent_sessions ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("store: list migration agent sessions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []MigrationAgentSession
	for rows.Next() {
		s, err := scanMigrationAgent(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan migration agent session: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// RedeemMigrationAgentToken spends the session's token and records the issued
// certificate in one conditional update, so two concurrent enrollments cannot both win.
func (db *DB) RedeemMigrationAgentToken(ctx context.Context, id, fingerprint, serial string, sessionExpires, now time.Time) error {
	ts := migTime(now)
	res, err := db.ExecContext(ctx, `UPDATE migration_agent_sessions SET token_used_at = ?, state = ?, cert_fingerprint = ?, cert_serial = ?,
		session_expires_at = ?, connected_at = ?, updated_at = ?
		WHERE id = ? AND token_used_at IS NULL AND state = ? AND token_expires_at > ?`,
		ts, MigrationAgentConnected, fingerprint, serial, migTime(sessionExpires), ts, ts,
		id, MigrationAgentPending, ts)
	if err != nil {
		return fmt.Errorf("store: redeem migration agent token %q: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return ErrMigrationAgentTokenUsed
	}
	return nil
}

// SetMigrationAgentState moves a session to state. A revoked session never leaves revoked.
func (db *DB) SetMigrationAgentState(ctx context.Context, id, state string, now time.Time) error {
	ts := migTime(now)
	var revoked any
	if state == MigrationAgentRevoked {
		revoked = ts
	}
	res, err := db.ExecContext(ctx, `UPDATE migration_agent_sessions SET state = ?, revoked_at = COALESCE(?, revoked_at), updated_at = ?
		WHERE id = ? AND (state <> ? OR ? = ?)`, state, revoked, ts, id, MigrationAgentRevoked, state, MigrationAgentRevoked)
	if err != nil {
		return fmt.Errorf("store: set migration agent state %q: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		if _, gerr := db.GetMigrationAgentSession(ctx, id); gerr != nil {
			return gerr
		}
	}
	return nil
}

// TouchMigrationAgentSession records agent liveness and its reported build.
func (db *DB) TouchMigrationAgentSession(ctx context.Context, id, version, goos, arch string, now time.Time) error {
	ts := migTime(now)
	_, err := db.ExecContext(ctx, `UPDATE migration_agent_sessions SET last_seen_at = ?,
		state = CASE WHEN state = ? THEN ? ELSE state END,
		agent_version = CASE WHEN ? <> '' THEN ? ELSE agent_version END,
		agent_os = CASE WHEN ? <> '' THEN ? ELSE agent_os END,
		agent_arch = CASE WHEN ? <> '' THEN ? ELSE agent_arch END, updated_at = ? WHERE id = ?`,
		ts, MigrationAgentLost, MigrationAgentConnected, version, version, goos, goos, arch, arch, ts, id)
	if err != nil {
		return fmt.Errorf("store: touch migration agent session %q: %w", id, err)
	}
	return nil
}

// SaveMigrationAgentInventory stores the latest discovery JSON.
func (db *DB) SaveMigrationAgentInventory(ctx context.Context, id, inventoryJSON string, now time.Time) error {
	return db.saveMigrationAgentBlob(ctx, "inventory_json", id, inventoryJSON, now)
}

// SaveMigrationAgentScan stores the latest volume scan JSON.
func (db *DB) SaveMigrationAgentScan(ctx context.Context, id, scanJSON string, now time.Time) error {
	return db.saveMigrationAgentBlob(ctx, "scan_json", id, scanJSON, now)
}

func (db *DB) saveMigrationAgentBlob(ctx context.Context, col, id, blob string, now time.Time) error {
	_, err := db.ExecContext(ctx, `UPDATE migration_agent_sessions SET `+col+` = ?, updated_at = ? WHERE id = ?`,
		blob, migTime(now), id)
	if err != nil {
		return fmt.Errorf("store: save migration agent %s %q: %w", col, id, err)
	}
	return nil
}

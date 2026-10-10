package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrMigrationSessionNotFound is returned for an unknown hub session id.
var ErrMigrationSessionNotFound = errors.New("migration session not found")

// Hub item statuses.
const (
	HubItemPending  = "pending"
	HubItemCopying  = "copying"
	HubItemVerified = "verified"
	HubItemFailed   = "failed"
)

// MigrationSession is one source server being migrated
// (migrations/0395_migration_hub.sql).
type MigrationSession struct {
	ID     string
	Engine string
	Host   string
	Port   int
	User   string
	TLS    bool
	NodeID string
	// HelperNetwork and SourceContainer are set when the source is a local
	// container the helper reaches over that container's own network.
	HelperNetwork   string
	SourceContainer string
	ServerVersion   string
	ServerMajor     int
	FreeBytes       int64
	Step            string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// MigrationItem is one source database within a session.
type MigrationItem struct {
	SessionID     string
	SourceDB      string
	SizeBytes     int64
	Tables        int
	Extensions    []string
	TargetName    string
	TargetVersion string
	Selected      bool
	Status        string
	Reason        string
	CreatedTarget bool
	Checked       int
	Mismatched    int
	Detail        string
	StartedAt     time.Time
	FinishedAt    time.Time
}

// CreateMigrationSession inserts a session and its inventoried items.
func (db *DB) CreateMigrationSession(ctx context.Context, s MigrationSession, items []MigrationItem) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin migration session: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	ts := s.CreatedAt.UTC().Format(time.RFC3339)
	if _, err := tx.ExecContext(ctx, `INSERT INTO migration_hub_sessions
		(id, engine, host, port, user_name, tls, node_id, helper_network, source_container, server_version, server_major, free_bytes, step, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.ID, s.Engine, s.Host, s.Port, s.User, boolInt(s.TLS), s.NodeID, s.HelperNetwork, s.SourceContainer, s.ServerVersion, s.ServerMajor, s.FreeBytes, s.Step, ts, ts); err != nil {
		return fmt.Errorf("store: insert migration session %q: %w", s.ID, err)
	}
	for _, it := range items {
		if _, err := tx.ExecContext(ctx, `INSERT INTO migration_hub_items
			(session_id, source_db, size_bytes, table_count, extensions, target_name, target_version, selected)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			s.ID, it.SourceDB, it.SizeBytes, it.Tables, strings.Join(it.Extensions, ","), it.TargetName, it.TargetVersion, boolInt(it.Selected)); err != nil {
			return fmt.Errorf("store: insert migration item %q: %w", it.SourceDB, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit migration session %q: %w", s.ID, err)
	}
	return nil
}

const hubSessionSelect = `SELECT id, engine, host, port, user_name, tls, node_id, helper_network, source_container, server_version, server_major, free_bytes, step, created_at, updated_at FROM migration_hub_sessions`

func scanHubSession(scan func(...any) error) (MigrationSession, error) {
	var s MigrationSession
	var tls int
	var created, updated string
	if err := scan(&s.ID, &s.Engine, &s.Host, &s.Port, &s.User, &tls, &s.NodeID, &s.HelperNetwork, &s.SourceContainer, &s.ServerVersion, &s.ServerMajor, &s.FreeBytes, &s.Step, &created, &updated); err != nil {
		return s, err
	}
	s.TLS = tls != 0
	s.CreatedAt, _ = time.Parse(time.RFC3339, created)
	s.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
	return s, nil
}

// GetMigrationSession returns one session.
func (db *DB) GetMigrationSession(ctx context.Context, id string) (MigrationSession, error) {
	s, err := scanHubSession(db.QueryRowContext(ctx, hubSessionSelect+` WHERE id = ?`, id).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return MigrationSession{}, ErrMigrationSessionNotFound
	}
	if err != nil {
		return MigrationSession{}, fmt.Errorf("store: get migration session %q: %w", id, err)
	}
	return s, nil
}

// ListMigrationSessions returns every session, newest first.
func (db *DB) ListMigrationSessions(ctx context.Context) ([]MigrationSession, error) {
	rows, err := db.QueryContext(ctx, hubSessionSelect+` ORDER BY created_at DESC, id`)
	if err != nil {
		return nil, fmt.Errorf("store: list migration sessions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []MigrationSession
	for rows.Next() {
		s, err := scanHubSession(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: list migration sessions: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// SetMigrationStep records the furthest step a session reached.
func (db *DB) SetMigrationStep(ctx context.Context, id, step string, now time.Time) error {
	res, err := db.ExecContext(ctx, `UPDATE migration_hub_sessions SET step = ?, updated_at = ? WHERE id = ?`,
		step, now.UTC().Format(time.RFC3339), id)
	if err != nil {
		return fmt.Errorf("store: set migration step %q: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrMigrationSessionNotFound
	}
	return nil
}

// DeleteMigrationSession forgets a session. Databases it created are kept.
func (db *DB) DeleteMigrationSession(ctx context.Context, id string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM migration_hub_items WHERE session_id = ?`, id); err != nil {
		return fmt.Errorf("store: delete migration items %q: %w", id, err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM migration_hub_sessions WHERE id = ?`, id); err != nil {
		return fmt.Errorf("store: delete migration session %q: %w", id, err)
	}
	return nil
}

const hubItemSelect = `SELECT session_id, source_db, size_bytes, table_count, extensions, target_name, target_version, selected, status, reason, created_target, checked, mismatched, detail, started_at, finished_at FROM migration_hub_items`

func scanHubItem(scan func(...any) error) (MigrationItem, error) {
	var it MigrationItem
	var ext, started, finished string
	var selected, created int
	if err := scan(&it.SessionID, &it.SourceDB, &it.SizeBytes, &it.Tables, &ext, &it.TargetName, &it.TargetVersion, &selected, &it.Status, &it.Reason, &created, &it.Checked, &it.Mismatched, &it.Detail, &started, &finished); err != nil {
		return it, err
	}
	if ext != "" {
		it.Extensions = strings.Split(ext, ",")
	}
	it.Selected, it.CreatedTarget = selected != 0, created != 0
	it.StartedAt, _ = time.Parse(time.RFC3339, started)
	it.FinishedAt, _ = time.Parse(time.RFC3339, finished)
	return it, nil
}

// ListMigrationItems returns a session's databases ordered by source name.
func (db *DB) ListMigrationItems(ctx context.Context, sessionID string) ([]MigrationItem, error) {
	rows, err := db.QueryContext(ctx, hubItemSelect+` WHERE session_id = ? ORDER BY source_db`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("store: list migration items %q: %w", sessionID, err)
	}
	defer func() { _ = rows.Close() }()
	var out []MigrationItem
	for rows.Next() {
		it, err := scanHubItem(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: list migration items %q: %w", sessionID, err)
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// SetMigrationItemPlan stores the operator's selection, name and version.
func (db *DB) SetMigrationItemPlan(ctx context.Context, sessionID, sourceDB, targetName, targetVersion string, selected bool) error {
	_, err := db.ExecContext(ctx, `UPDATE migration_hub_items SET target_name = ?, target_version = ?, selected = ? WHERE session_id = ? AND source_db = ?`,
		targetName, targetVersion, boolInt(selected), sessionID, sourceDB)
	if err != nil {
		return fmt.Errorf("store: set migration item plan %q/%q: %w", sessionID, sourceDB, err)
	}
	return nil
}

// MarkMigrationItemTargetCreated records that the hub created the managed database.
func (db *DB) MarkMigrationItemTargetCreated(ctx context.Context, sessionID, sourceDB, version string) error {
	_, err := db.ExecContext(ctx, `UPDATE migration_hub_items SET created_target = 1, target_version = ? WHERE session_id = ? AND source_db = ?`,
		version, sessionID, sourceDB)
	if err != nil {
		return fmt.Errorf("store: mark migration target created %q/%q: %w", sessionID, sourceDB, err)
	}
	return nil
}

// SetMigrationItemStatus records progress or the outcome of one item's copy.
func (db *DB) SetMigrationItemStatus(ctx context.Context, sessionID, sourceDB, status, reason string, checked, mismatched int, detail string, now time.Time) error {
	ts := now.UTC().Format(time.RFC3339)
	q := `UPDATE migration_hub_items SET status = ?, reason = ?, checked = ?, mismatched = ?, detail = ?, finished_at = ?`
	finished := ts
	args := []any{status, reason, checked, mismatched, detail}
	if status == HubItemCopying {
		finished = ""
	}
	args = append(args, finished)
	if status == HubItemCopying {
		q += `, started_at = ?`
		args = append(args, ts)
	}
	q += ` WHERE session_id = ? AND source_db = ?`
	args = append(args, sessionID, sourceDB)
	if _, err := db.ExecContext(ctx, q, args...); err != nil {
		return fmt.Errorf("store: set migration item status %q/%q: %w", sessionID, sourceDB, err)
	}
	return nil
}

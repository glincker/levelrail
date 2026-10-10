package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// ErrAppImportSessionNotFound is returned for an unknown app import session id.
var ErrAppImportSessionNotFound = errors.New("app import session not found")

// App import item states.
const (
	AppImportPlanned      = "planned"
	AppImportStaged       = "staged"
	AppImportBuilding     = "building"
	AppImportVerified     = "verified"
	AppImportVerifyFailed = "verify-failed"
	AppImportStageFailed  = "stage-failed"
	AppImportRouted       = "routed"
	AppImportRolledBack   = "rolled-back"
)

// AppImportMapping rewrites one source hostname to a new one in env values.
type AppImportMapping struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// AppImportVolume is one persistent volume and whether the operator
// confirmed its data was copied.
type AppImportVolume struct {
	Name string `json:"name"`
	// SourceName is the volume's name on the source host.
	SourceName    string `json:"source_name,omitempty"`
	ContainerPath string `json:"container_path"`
	Copied        bool   `json:"copied"`
	CopiedAt      string `json:"copied_at,omitempty"`
}

// AppImportSession is one guided app import (migrations/0408_app_import_sessions.sql).
type AppImportSession struct {
	ID        string
	Platform  string
	SourceURL string
	Step      string
	Collision string
	Mappings  []AppImportMapping
	CreatedAt time.Time
	UpdatedAt time.Time
}

// AppImportItem is one source app or service within a session.
type AppImportItem struct {
	SessionID     string
	SourceID      string
	SourceName    string
	TargetName    string
	Kind          string
	State         string
	Selected      bool
	Reason        string
	EntryJSON     string
	Domains       []string
	Volumes       []AppImportVolume
	AttemptID     string
	CreatedTarget bool
	UpdatedAt     time.Time
}

func nonNilList[T any](in []T) []T {
	if in == nil {
		return []T{}
	}
	return in
}

type appImportExecer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// CreateAppImportSession inserts a session and its inventoried items.
func (db *DB) CreateAppImportSession(ctx context.Context, s AppImportSession, items []AppImportItem) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin app import session: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	ts := s.CreatedAt.UTC().Format(time.RFC3339)
	maps, err := json.Marshal(nonNilList(s.Mappings))
	if err != nil {
		return fmt.Errorf("store: encode import mappings: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO app_import_sessions
		(id, platform, source_url, step, collision, mappings_json, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		s.ID, s.Platform, s.SourceURL, s.Step, s.Collision, string(maps), ts, ts); err != nil {
		return fmt.Errorf("store: insert app import session %q: %w", s.ID, err)
	}
	for _, it := range items {
		it.SessionID = s.ID
		if err := saveAppImportItem(ctx, tx, it, s.CreatedAt); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit app import session %q: %w", s.ID, err)
	}
	return nil
}

func saveAppImportItem(ctx context.Context, x appImportExecer, it AppImportItem, now time.Time) error {
	doms, err := json.Marshal(nonNilSlice(it.Domains))
	if err != nil {
		return fmt.Errorf("store: encode import domains: %w", err)
	}
	vols, err := json.Marshal(nonNilList(it.Volumes))
	if err != nil {
		return fmt.Errorf("store: encode import volumes: %w", err)
	}
	entry := it.EntryJSON
	if entry == "" {
		entry = "{}"
	}
	_, err = x.ExecContext(ctx, `INSERT INTO app_import_items
		(session_id, source_id, source_name, target_name, kind, state, selected, reason, entry_json, domains_json, volumes_json, attempt_id, created_target, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (session_id, source_id) DO UPDATE SET
			source_name = excluded.source_name, target_name = excluded.target_name, kind = excluded.kind, state = excluded.state,
			selected = excluded.selected, reason = excluded.reason, entry_json = excluded.entry_json, domains_json = excluded.domains_json,
			volumes_json = excluded.volumes_json, attempt_id = excluded.attempt_id, created_target = excluded.created_target, updated_at = excluded.updated_at`,
		it.SessionID, it.SourceID, it.SourceName, it.TargetName, it.Kind, it.State, boolInt(it.Selected), it.Reason, entry,
		string(doms), string(vols), it.AttemptID, boolInt(it.CreatedTarget), now.UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("store: save app import item %q/%q: %w", it.SessionID, it.SourceID, err)
	}
	return nil
}

// SaveAppImportItem inserts or replaces one item.
func (db *DB) SaveAppImportItem(ctx context.Context, it AppImportItem, now time.Time) error {
	return saveAppImportItem(ctx, db, it, now)
}

const appImportSessionSelect = `SELECT id, platform, source_url, step, collision, mappings_json, created_at, updated_at FROM app_import_sessions`

func scanAppImportSession(scan func(...any) error) (AppImportSession, error) {
	var s AppImportSession
	var maps, created, updated string
	if err := scan(&s.ID, &s.Platform, &s.SourceURL, &s.Step, &s.Collision, &maps, &created, &updated); err != nil {
		return s, err
	}
	if err := json.Unmarshal([]byte(maps), &s.Mappings); err != nil {
		return s, fmt.Errorf("decode mappings: %w", err)
	}
	s.CreatedAt, _ = time.Parse(time.RFC3339, created)
	s.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
	return s, nil
}

// GetAppImportSession returns one session.
func (db *DB) GetAppImportSession(ctx context.Context, id string) (AppImportSession, error) {
	s, err := scanAppImportSession(db.QueryRowContext(ctx, appImportSessionSelect+` WHERE id = ?`, id).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return AppImportSession{}, ErrAppImportSessionNotFound
	}
	if err != nil {
		return AppImportSession{}, fmt.Errorf("store: get app import session %q: %w", id, err)
	}
	return s, nil
}

// ListAppImportSessions returns every session, newest first.
func (db *DB) ListAppImportSessions(ctx context.Context) ([]AppImportSession, error) {
	rows, err := db.QueryContext(ctx, appImportSessionSelect+` ORDER BY created_at DESC, id`)
	if err != nil {
		return nil, fmt.Errorf("store: list app import sessions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []AppImportSession
	for rows.Next() {
		s, err := scanAppImportSession(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: list app import sessions: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// UpdateAppImportSession stores the step, collision mode and mappings.
func (db *DB) UpdateAppImportSession(ctx context.Context, s AppImportSession, now time.Time) error {
	maps, err := json.Marshal(nonNilList(s.Mappings))
	if err != nil {
		return fmt.Errorf("store: encode import mappings: %w", err)
	}
	res, err := db.ExecContext(ctx, `UPDATE app_import_sessions SET step = ?, collision = ?, mappings_json = ?, updated_at = ? WHERE id = ?`,
		s.Step, s.Collision, string(maps), now.UTC().Format(time.RFC3339), s.ID)
	if err != nil {
		return fmt.Errorf("store: update app import session %q: %w", s.ID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrAppImportSessionNotFound
	}
	return nil
}

// DeleteAppImportSession forgets a session. Apps it created are kept.
func (db *DB) DeleteAppImportSession(ctx context.Context, id string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM app_import_items WHERE session_id = ?`, id); err != nil {
		return fmt.Errorf("store: delete app import items %q: %w", id, err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM app_import_sessions WHERE id = ?`, id); err != nil {
		return fmt.Errorf("store: delete app import session %q: %w", id, err)
	}
	return nil
}

// ListAppImportItems returns a session's items ordered by source name.
func (db *DB) ListAppImportItems(ctx context.Context, sessionID string) ([]AppImportItem, error) {
	rows, err := db.QueryContext(ctx, `SELECT session_id, source_id, source_name, target_name, kind, state, selected, reason, entry_json, domains_json, volumes_json, attempt_id, created_target, updated_at
		FROM app_import_items WHERE session_id = ? ORDER BY source_name, source_id`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("store: list app import items %q: %w", sessionID, err)
	}
	defer func() { _ = rows.Close() }()
	var out []AppImportItem
	for rows.Next() {
		var it AppImportItem
		var selected, created int
		var doms, vols, updated string
		if err := rows.Scan(&it.SessionID, &it.SourceID, &it.SourceName, &it.TargetName, &it.Kind, &it.State, &selected, &it.Reason, &it.EntryJSON, &doms, &vols, &it.AttemptID, &created, &updated); err != nil {
			return nil, fmt.Errorf("store: scan app import item: %w", err)
		}
		if err := json.Unmarshal([]byte(doms), &it.Domains); err != nil {
			return nil, fmt.Errorf("store: decode import domains: %w", err)
		}
		if err := json.Unmarshal([]byte(vols), &it.Volumes); err != nil {
			return nil, fmt.Errorf("store: decode import volumes: %w", err)
		}
		it.Selected, it.CreatedTarget = selected != 0, created != 0
		it.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
		out = append(out, it)
	}
	return out, rows.Err()
}

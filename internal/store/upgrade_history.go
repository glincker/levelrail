package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"time"
)

// ErrUpgradeHistoryNotFound is returned when no history row has the id.
var ErrUpgradeHistoryNotFound = errors.New("store: upgrade history entry not found")

// Notes states of an UpgradeHistoryEntry: the snapshot is filled once.
const (
	UpgradeNotesPending     = "pending"
	UpgradeNotesFetched     = "fetched"
	UpgradeNotesUnavailable = "unavailable"
)

// UpgradeHistoryEntry is one recorded control plane version transition
// (migrations/0400). Rows are append-only apart from the notes snapshot,
// filled once, and the acknowledgement.
type UpgradeHistoryEntry struct {
	ID           string
	Kind         string
	FromVersion  string
	ToVersion    string
	Channel      string
	SchemaBefore int
	SchemaAfter  int
	OccurredAt   time.Time
	Initiator    string
	Method       string
	BackupName   string
	Health       string
	Notes        string
	NotesState   string
	AckedByType  string
	AckedByID    string
	AckedByName  string
	AckedAt      *time.Time
}

// Acknowledged reports whether an operator has acknowledged the transition.
func (e UpgradeHistoryEntry) Acknowledged() bool { return e.AckedAt != nil }

const upgradeHistoryColumns = `id, kind, from_version, to_version, channel, schema_before, schema_after,
	occurred_at, initiator, method, backup_name, health, notes, notes_state,
	acked_by_type, acked_by_id, acked_by_name, acked_at`

func newUpgradeHistoryID() (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("store: generate upgrade history id: %w", err)
	}
	return "uh_" + base64.RawURLEncoding.EncodeToString(buf), nil
}

// InsertUpgradeHistory appends e unless the newest row already records a
// transition to e.ToVersion, so a crash loop or a double start records one
// row. The check and insert are one statement. It reports whether a row was
// written and returns the stored entry (with its new id) when it was.
func (db *DB) InsertUpgradeHistory(ctx context.Context, e UpgradeHistoryEntry) (UpgradeHistoryEntry, bool, error) {
	id, err := newUpgradeHistoryID()
	if err != nil {
		return UpgradeHistoryEntry{}, false, err
	}
	e.ID = id
	if e.NotesState == "" {
		e.NotesState = UpgradeNotesPending
	}
	res, err := db.ExecContext(ctx, `
		INSERT INTO upgrade_history (id, kind, from_version, to_version, channel, schema_before, schema_after,
			occurred_at, initiator, method, backup_name, health, notes, notes_state)
		SELECT ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
		WHERE NOT EXISTS (
			SELECT 1 FROM (SELECT to_version FROM upgrade_history ORDER BY seq DESC LIMIT 1) WHERE to_version = ?
		)
	`, e.ID, e.Kind, e.FromVersion, e.ToVersion, e.Channel, e.SchemaBefore, e.SchemaAfter,
		formatTime(e.OccurredAt), e.Initiator, e.Method, e.BackupName, e.Health, e.Notes, e.NotesState, e.ToVersion)
	if err != nil {
		return UpgradeHistoryEntry{}, false, fmt.Errorf("store: insert upgrade history %q: %w", e.ToVersion, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return UpgradeHistoryEntry{}, false, fmt.Errorf("store: insert upgrade history %q: rows affected: %w", e.ToVersion, err)
	}
	return e, n == 1, nil
}

// LatestUpgradeHistory returns the newest recorded transition, ok false when
// none exists yet.
func (db *DB) LatestUpgradeHistory(ctx context.Context) (UpgradeHistoryEntry, bool, error) {
	row := db.QueryRowContext(ctx, `SELECT `+upgradeHistoryColumns+` FROM upgrade_history ORDER BY seq DESC LIMIT 1`)
	e, err := scanUpgradeHistory(row)
	if errors.Is(err, sql.ErrNoRows) {
		return UpgradeHistoryEntry{}, false, nil
	}
	if err != nil {
		return UpgradeHistoryEntry{}, false, fmt.Errorf("store: latest upgrade history: %w", err)
	}
	return e, true, nil
}

// ListUpgradeHistory returns transitions newest first, at most limit rows.
func (db *DB) ListUpgradeHistory(ctx context.Context, limit int) ([]UpgradeHistoryEntry, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+upgradeHistoryColumns+` FROM upgrade_history ORDER BY seq DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list upgrade history: %w", err)
	}
	return collectUpgradeHistory(rows)
}

// ListUnacknowledgedUpgrades returns transitions nobody has acknowledged,
// newest first. This is the query the attention center consumes.
func (db *DB) ListUnacknowledgedUpgrades(ctx context.Context) ([]UpgradeHistoryEntry, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+upgradeHistoryColumns+` FROM upgrade_history WHERE acked_at IS NULL ORDER BY seq DESC`)
	if err != nil {
		return nil, fmt.Errorf("store: list unacknowledged upgrades: %w", err)
	}
	return collectUpgradeHistory(rows)
}

func collectUpgradeHistory(rows *sql.Rows) ([]UpgradeHistoryEntry, error) {
	defer func() { _ = rows.Close() }()
	var out []UpgradeHistoryEntry
	for rows.Next() {
		e, err := scanUpgradeHistory(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan upgrade history: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// CompleteUpgradeNotes stores the release-notes snapshot once. A row whose
// snapshot is already settled is left alone, never overwritten.
func (db *DB) CompleteUpgradeNotes(ctx context.Context, id, notes, state string) error {
	_, err := db.ExecContext(ctx, `
		UPDATE upgrade_history SET notes = ?, notes_state = ? WHERE id = ? AND notes_state = ?
	`, notes, state, id, UpgradeNotesPending)
	if err != nil {
		return fmt.Errorf("store: complete upgrade notes %q: %w", id, err)
	}
	return nil
}

// AcknowledgeUpgrade records who acknowledged the transition. The first
// acknowledgement wins; a repeat returns the stored entry unchanged and
// changed false.
func (db *DB) AcknowledgeUpgrade(ctx context.Context, id, byType, byID, byName string, now time.Time) (e UpgradeHistoryEntry, changed bool, err error) {
	res, err := db.ExecContext(ctx, `
		UPDATE upgrade_history SET acked_by_type = ?, acked_by_id = ?, acked_by_name = ?, acked_at = ?
		WHERE id = ? AND acked_at IS NULL
	`, byType, byID, byName, formatTime(now), id)
	if err != nil {
		return UpgradeHistoryEntry{}, false, fmt.Errorf("store: acknowledge upgrade %q: %w", id, err)
	}
	n, _ := res.RowsAffected()
	row := db.QueryRowContext(ctx, `SELECT `+upgradeHistoryColumns+` FROM upgrade_history WHERE id = ?`, id)
	e, err = scanUpgradeHistory(row)
	if errors.Is(err, sql.ErrNoRows) {
		return UpgradeHistoryEntry{}, false, ErrUpgradeHistoryNotFound
	}
	if err != nil {
		return UpgradeHistoryEntry{}, false, fmt.Errorf("store: read upgrade %q: %w", id, err)
	}
	return e, n == 1, nil
}

type upgradeRowScanner interface{ Scan(dest ...any) error }

func scanUpgradeHistory(s upgradeRowScanner) (UpgradeHistoryEntry, error) {
	var e UpgradeHistoryEntry
	var occurred string
	var ackType, ackID, ackName, ackAt sql.NullString
	if err := s.Scan(&e.ID, &e.Kind, &e.FromVersion, &e.ToVersion, &e.Channel, &e.SchemaBefore, &e.SchemaAfter,
		&occurred, &e.Initiator, &e.Method, &e.BackupName, &e.Health, &e.Notes, &e.NotesState,
		&ackType, &ackID, &ackName, &ackAt); err != nil {
		return UpgradeHistoryEntry{}, err
	}
	t, err := time.Parse(time.RFC3339Nano, occurred)
	if err != nil {
		return UpgradeHistoryEntry{}, fmt.Errorf("parse occurred_at %q: %w", occurred, err)
	}
	e.OccurredAt = t
	e.AckedByType, e.AckedByID, e.AckedByName = ackType.String, ackID.String, ackName.String
	if ackAt.Valid {
		at, err := time.Parse(time.RFC3339Nano, ackAt.String)
		if err != nil {
			return UpgradeHistoryEntry{}, fmt.Errorf("parse acked_at %q: %w", ackAt.String, err)
		}
		e.AckedAt = &at
	}
	return e, nil
}

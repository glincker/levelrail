package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ResourceNotes is the operator description and markdown notes of an app or database.
type ResourceNotes struct {
	Description string `json:"description"`
	Notes       string `json:"notes"`
	UpdatedBy   string `json:"updated_by,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
}

// GetResourceNotes returns the notes of one resource, zero valued when none were saved.
func (db *DB) GetResourceNotes(ctx context.Context, kind, name string) (ResourceNotes, error) {
	var n ResourceNotes
	err := db.reader().QueryRowContext(ctx, `SELECT description, notes, updated_by, updated_at FROM resource_notes WHERE kind = ? AND name = ?`, kind, name).
		Scan(&n.Description, &n.Notes, &n.UpdatedBy, &n.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ResourceNotes{}, nil
	}
	if err != nil {
		return ResourceNotes{}, fmt.Errorf("store: get %s notes: %w", kind, err)
	}
	return n, nil
}

// SetResourceNotes replaces both fields; two empty fields delete the row so nothing stale lingers.
func (db *DB) SetResourceNotes(ctx context.Context, kind, name string, n ResourceNotes) error {
	var err error
	if n.Description == "" && n.Notes == "" {
		_, err = db.ExecContext(ctx, `DELETE FROM resource_notes WHERE kind = ? AND name = ?`, kind, name)
	} else {
		_, err = db.ExecContext(ctx, `
			INSERT INTO resource_notes (kind, name, description, notes, updated_by, updated_at)
			VALUES (?, ?, ?, ?, ?, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
			ON CONFLICT (kind, name) DO UPDATE SET
				description = excluded.description,
				notes = excluded.notes,
				updated_by = excluded.updated_by,
				updated_at = excluded.updated_at
		`, kind, name, n.Description, n.Notes, n.UpdatedBy)
	}
	if err != nil {
		return fmt.Errorf("store: set %s notes: %w", kind, err)
	}
	return nil
}

// ResourceNotesUpdate sets only the non-nil fields, so two edits of different fields never undo each other.
type ResourceNotesUpdate struct {
	Description *string
	Notes       *string
	UpdatedBy   string
}

// UpdateResourceNotes merges the present fields in one upsert, then drops a row left with both fields empty.
func (db *DB) UpdateResourceNotes(ctx context.Context, kind, name string, u ResourceNotesUpdate) error {
	var desc, notes sql.NullString
	if u.Description != nil {
		desc = sql.NullString{String: *u.Description, Valid: true}
	}
	if u.Notes != nil {
		notes = sql.NullString{String: *u.Notes, Valid: true}
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO resource_notes (kind, name, description, notes, updated_by, updated_at)
		VALUES (?, ?, COALESCE(?, ''), COALESCE(?, ''), ?, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		ON CONFLICT (kind, name) DO UPDATE SET
			description = COALESCE(?, description),
			notes = COALESCE(?, notes),
			updated_by = excluded.updated_by,
			updated_at = excluded.updated_at
	`, kind, name, desc, notes, u.UpdatedBy, desc, notes); err != nil {
		return fmt.Errorf("store: update %s notes: %w", kind, err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM resource_notes WHERE kind = ? AND name = ? AND description = '' AND notes = ''`, kind, name); err != nil {
		return fmt.Errorf("store: prune %s notes: %w", kind, err)
	}
	return nil
}

// NotesSearchPrefixBytes bounds how much of each notes row the apps list search scans.
// LIKE folds ASCII case only, so non ASCII text matches its exact case.
const NotesSearchPrefixBytes = 4096

// ListResourceDescriptions returns every non-empty description of kind keyed by name, for list rows.
func (db *DB) ListResourceDescriptions(ctx context.Context, kind string) (map[string]string, error) {
	rows, err := db.reader().QueryContext(ctx, `SELECT name, description FROM resource_notes WHERE kind = ? AND description != ''`, kind)
	if err != nil {
		return nil, fmt.Errorf("store: list %s descriptions: %w", kind, err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]string{}
	for rows.Next() {
		var name, d string
		if err := rows.Scan(&name, &d); err != nil {
			return nil, fmt.Errorf("store: scan %s description: %w", kind, err)
		}
		out[name] = d
	}
	return out, rows.Err()
}

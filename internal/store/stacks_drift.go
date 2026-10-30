package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Stack drift statuses.
const (
	StackDriftNone    = "none"
	StackDriftDrifted = "drifted"
)

// StackDriftItem is one drifted resource. Fields hold field paths only, never
// values, so env, labels and commands stay hidden.
type StackDriftItem struct {
	Kind   string   `json:"kind"`
	Name   string   `json:"name"`
	Scope  string   `json:"scope,omitempty"`
	Action string   `json:"action"`
	Fields []string `json:"fields,omitempty"`
}

// StackDrift is the latest drift check of one stack.
type StackDrift struct {
	StackID   string
	Status    string
	Revision  int
	Summary   []StackDriftItem
	CheckedAt time.Time
}

// SetStackDrift stores the latest check of a stack, replacing the previous one.
func (db *DB) SetStackDrift(ctx context.Context, d StackDrift) error {
	if d.Status != StackDriftNone && d.Status != StackDriftDrifted {
		return fmt.Errorf("store: unknown stack drift status %q", d.Status)
	}
	if d.Summary == nil {
		d.Summary = []StackDriftItem{}
	}
	raw, err := json.Marshal(d.Summary)
	if err != nil {
		return fmt.Errorf("store: encode stack drift: %w", err)
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO stack_drift (stack_id, status, revision, summary_json, checked_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(stack_id) DO UPDATE SET status = excluded.status, revision = excluded.revision,
			summary_json = excluded.summary_json, checked_at = excluded.checked_at`,
		d.StackID, d.Status, d.Revision, string(raw), stackTime(d.CheckedAt))
	if err != nil {
		return fmt.Errorf("store: set stack drift: %w", err)
	}
	return nil
}

func scanStackDrift(r rowScanner) (StackDrift, error) {
	var d StackDrift
	var summary, checked string
	if err := r.Scan(&d.StackID, &d.Status, &d.Revision, &summary, &checked); err != nil {
		return StackDrift{}, err
	}
	if err := json.Unmarshal([]byte(summary), &d.Summary); err != nil {
		return StackDrift{}, fmt.Errorf("decode stack drift summary: %w", err)
	}
	d.CheckedAt, _ = time.Parse(time.RFC3339Nano, checked)
	return d, nil
}

// GetStackDrift returns the latest check of a stack; ok is false before the
// first check.
func (db *DB) GetStackDrift(ctx context.Context, stackID string) (StackDrift, bool, error) {
	d, err := scanStackDrift(db.QueryRowContext(ctx, `SELECT stack_id, status, revision, summary_json, checked_at FROM stack_drift WHERE stack_id = ?`, stackID))
	if errors.Is(err, sql.ErrNoRows) {
		return StackDrift{}, false, nil
	}
	if err != nil {
		return StackDrift{}, false, fmt.Errorf("store: get stack drift: %w", err)
	}
	return d, true, nil
}

// ListStackDrift returns every stack's latest check, drifted ones first.
func (db *DB) ListStackDrift(ctx context.Context) ([]StackDrift, error) {
	rows, err := db.QueryContext(ctx, `SELECT stack_id, status, revision, summary_json, checked_at FROM stack_drift ORDER BY status DESC, stack_id`)
	if err != nil {
		return nil, fmt.Errorf("store: list stack drift: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []StackDrift{}
	for rows.Next() {
		d, err := scanStackDrift(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan stack drift: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

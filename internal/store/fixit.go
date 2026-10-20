package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// FixitAttempt is one persisted fix apply or re-check.
type FixitAttempt struct {
	FixID       string
	Subject     string
	At          time.Time
	Actor       string
	Outcome     string
	Detail      string
	Fingerprint string
}

// RecentFixitAttempts returns the newest attempts for a fix and subject.
func (db *DB) RecentFixitAttempts(ctx context.Context, fixID, subject string, limit int) ([]FixitAttempt, error) {
	rows, err := db.QueryContext(ctx, `SELECT fix_id, subject, at, actor, outcome, detail, fingerprint
		FROM fixit_attempts WHERE fix_id = ? AND subject = ? ORDER BY id DESC LIMIT ?`, fixID, subject, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list fixit attempts: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []FixitAttempt
	for rows.Next() {
		var a FixitAttempt
		var at string
		if err := rows.Scan(&a.FixID, &a.Subject, &at, &a.Actor, &a.Outcome, &a.Detail, &a.Fingerprint); err != nil {
			return nil, fmt.Errorf("store: scan fixit attempt: %w", err)
		}
		a.At, _ = time.Parse(time.RFC3339Nano, at)
		out = append(out, a)
	}
	return out, rows.Err()
}

// CountFixitAttemptsSince counts a fix's attempts at or after since.
func (db *DB) CountFixitAttemptsSince(ctx context.Context, fixID string, since time.Time) (int, error) {
	var n int
	err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM fixit_attempts WHERE fix_id = ? AND at >= ?`,
		fixID, since.UTC().Format(time.RFC3339Nano)).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("store: count fixit attempts: %w", err)
	}
	return n, nil
}

// RecordFixitAttempt appends one attempt.
func (db *DB) RecordFixitAttempt(ctx context.Context, a FixitAttempt) error {
	_, err := db.ExecContext(ctx, `INSERT INTO fixit_attempts (fix_id, subject, at, actor, outcome, detail, fingerprint)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, a.FixID, a.Subject, a.At.UTC().Format(time.RFC3339Nano), a.Actor, a.Outcome, a.Detail, a.Fingerprint)
	if err != nil {
		return fmt.Errorf("store: record fixit attempt: %w", err)
	}
	return nil
}

// GetFixitSetting returns a stored setting value.
func (db *DB) GetFixitSetting(ctx context.Context, key string) (string, bool, error) {
	var v string
	err := db.QueryRowContext(ctx, `SELECT value FROM fixit_settings WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("store: get fixit setting: %w", err)
	}
	return v, true, nil
}

// SetFixitSetting upserts a setting value.
func (db *DB) SetFixitSetting(ctx context.Context, key, value, actor string, now time.Time) error {
	_, err := db.ExecContext(ctx, `INSERT INTO fixit_settings (key, value, updated_at, updated_by) VALUES (?, ?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at, updated_by = excluded.updated_by`,
		key, value, now.UTC().Format(time.RFC3339Nano), actor)
	if err != nil {
		return fmt.Errorf("store: set fixit setting: %w", err)
	}
	return nil
}

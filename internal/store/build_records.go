package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrBuildRecordNotFound means no signed build record has that id or attempt.
var ErrBuildRecordNotFound = errors.New("store: build record not found")

// BuildRecord is one stored signed record; Claims is the signed JSON as issued.
type BuildRecord struct {
	ID        string
	App       string
	AttemptID string
	Claims    string
	Signature string
	KeyID     string
	CreatedAt time.Time
}

// SaveBuildRecord stores a freshly signed record.
func (db *DB) SaveBuildRecord(ctx context.Context, r BuildRecord) error {
	_, err := db.ExecContext(ctx, `INSERT INTO build_records (id, app_name, attempt_id, claims, signature, key_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, r.ID, r.App, r.AttemptID, r.Claims, r.Signature, r.KeyID, r.CreatedAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("store: save build record: %w", err)
	}
	return nil
}

// LinkBuildRecordAttempt ties a record to the deploy attempt that used the image.
func (db *DB) LinkBuildRecordAttempt(ctx context.Context, id, attemptID string) error {
	res, err := db.ExecContext(ctx, `UPDATE build_records SET attempt_id = ? WHERE id = ?`, attemptID, id)
	if err != nil {
		return fmt.Errorf("store: link build record: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrBuildRecordNotFound
	}
	return nil
}

const buildRecordCols = `id, app_name, attempt_id, claims, signature, key_id, created_at`

func scanBuildRecord(row *sql.Row) (BuildRecord, error) {
	var r BuildRecord
	var at string
	err := row.Scan(&r.ID, &r.App, &r.AttemptID, &r.Claims, &r.Signature, &r.KeyID, &at)
	if errors.Is(err, sql.ErrNoRows) {
		return BuildRecord{}, ErrBuildRecordNotFound
	}
	if err != nil {
		return BuildRecord{}, fmt.Errorf("store: read build record: %w", err)
	}
	r.CreatedAt, _ = time.Parse(time.RFC3339Nano, at)
	return r, nil
}

// GetBuildRecord returns a record by id.
func (db *DB) GetBuildRecord(ctx context.Context, id string) (BuildRecord, error) {
	return scanBuildRecord(db.QueryRowContext(ctx, `SELECT `+buildRecordCols+` FROM build_records WHERE id = ?`, id))
}

// GetBuildRecordByAttempt returns the record linked to a deploy attempt (the build id).
func (db *DB) GetBuildRecordByAttempt(ctx context.Context, attemptID string) (BuildRecord, error) {
	return scanBuildRecord(db.QueryRowContext(ctx, `SELECT `+buildRecordCols+` FROM build_records WHERE attempt_id = ? ORDER BY created_at DESC LIMIT 1`, attemptID))
}

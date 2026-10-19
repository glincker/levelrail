package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrDEKRotationConflict means a row changed between the pre-image read
// and the rewrite, so the whole service rotation was rolled back.
var ErrDEKRotationConflict = errors.New("store: secret rows changed during DEK rotation")

// ServiceDEKStatus is one service's key age and value count, no key material.
type ServiceDEKStatus struct {
	ServiceName string
	CreatedAt   time.Time
	// RotatedAt is zero until the DEK has been rotated at least once.
	RotatedAt  time.Time
	ValueCount int
}

// KeyAgeAnchor is when the DEK last changed: its rotation, else its creation.
func (s ServiceDEKStatus) KeyAgeAnchor() time.Time {
	if !s.RotatedAt.IsZero() {
		return s.RotatedAt
	}
	return s.CreatedAt
}

func parseStoredTime(raw string) time.Time {
	if raw == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}

// ListServiceDEKStatus lists every service DEK with its age and value count.
func (db *DB) ListServiceDEKStatus(ctx context.Context) ([]ServiceDEKStatus, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT s.service_name, s.created_at, s.rotated_at,
		       (SELECT COUNT(*) FROM service_secret_values v WHERE v.service_name = s.service_name)
		FROM service_secrets s ORDER BY s.service_name
	`)
	if err != nil {
		return nil, fmt.Errorf("store: list service DEK status: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []ServiceDEKStatus
	for rows.Next() {
		var st ServiceDEKStatus
		var created, rotated string
		if err := rows.Scan(&st.ServiceName, &created, &rotated, &st.ValueCount); err != nil {
			return nil, fmt.Errorf("store: scan service DEK status: %w", err)
		}
		st.CreatedAt, st.RotatedAt = parseStoredTime(created), parseStoredTime(rotated)
		out = append(out, st)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list service DEK status: %w", err)
	}
	return out, nil
}

// RotateServiceDEK swaps one service's wrapped DEK and re-encrypts all its
// values in one transaction. rewrite gets the current wrapped DEK and the
// current ciphertexts and returns the new wrapped DEK plus one replacement
// per row, in order. Every UPDATE is guarded by its pre-image, so a row
// changed by someone else rolls the whole rotation back.
func (db *DB) RotateServiceDEK(ctx context.Context, serviceName string, at time.Time,
	rewrite func(wrapped []byte, rows []SecretCiphertext) (newWrapped []byte, replacements [][]byte, err error)) (int, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("store: rotate DEK for %q: begin: %w", serviceName, err)
	}
	defer func() { _ = tx.Rollback() }()

	var wrapped []byte
	err = tx.QueryRowContext(ctx, `SELECT wrapped_dek FROM service_secrets WHERE service_name = ?`, serviceName).Scan(&wrapped)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrServiceDEKNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("store: rotate DEK for %q: read DEK: %w", serviceName, err)
	}

	rows, err := tx.QueryContext(ctx, `
		SELECT env_key, ciphertext FROM service_secret_values WHERE service_name = ? ORDER BY env_key
	`, serviceName)
	if err != nil {
		return 0, fmt.Errorf("store: rotate DEK for %q: list values: %w", serviceName, err)
	}
	var current []SecretCiphertext
	for rows.Next() {
		row := SecretCiphertext{ServiceName: serviceName}
		if err := rows.Scan(&row.EnvKey, &row.Ciphertext); err != nil {
			_ = rows.Close()
			return 0, fmt.Errorf("store: rotate DEK for %q: scan value: %w", serviceName, err)
		}
		current = append(current, row)
	}
	rowsErr := rows.Err()
	_ = rows.Close()
	if rowsErr != nil {
		return 0, fmt.Errorf("store: rotate DEK for %q: list values: %w", serviceName, rowsErr)
	}

	newWrapped, replacements, err := rewrite(wrapped, current)
	if err != nil {
		return 0, err
	}
	if len(replacements) != len(current) {
		return 0, fmt.Errorf("store: rotate DEK for %q: got %d replacements for %d values", serviceName, len(replacements), len(current))
	}

	res, err := tx.ExecContext(ctx, `
		UPDATE service_secrets SET wrapped_dek = ?, rotated_at = ? WHERE service_name = ? AND wrapped_dek = ?
	`, newWrapped, at.UTC().Format(time.RFC3339Nano), serviceName, wrapped)
	if err != nil {
		return 0, fmt.Errorf("store: rotate DEK for %q: update DEK: %w", serviceName, err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return 0, ErrDEKRotationConflict
	}
	for i, row := range current {
		res, err := tx.ExecContext(ctx, `
			UPDATE service_secret_values SET ciphertext = ?
			WHERE service_name = ? AND env_key = ? AND ciphertext = ?
		`, replacements[i], serviceName, row.EnvKey, row.Ciphertext)
		if err != nil {
			return 0, fmt.Errorf("store: rotate DEK for %q: update value: %w", serviceName, err)
		}
		if n, _ := res.RowsAffected(); n != 1 {
			return 0, ErrDEKRotationConflict
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("store: rotate DEK for %q: commit: %w", serviceName, err)
	}
	return len(current), nil
}

// SecretKeyPolicy is the operator's key age policy. MaxKeyAgeDays 0 means off.
type SecretKeyPolicy struct {
	MaxKeyAgeDays   int
	RemindEveryDays int
	LastRemindedAt  time.Time
	UpdatedAt       time.Time
	UpdatedBy       string
}

// DefaultRemindEveryDays is how often an overdue reminder repeats.
const DefaultRemindEveryDays = 7

// GetSecretKeyPolicy returns the stored policy, or the default (off) one.
func (db *DB) GetSecretKeyPolicy(ctx context.Context) (SecretKeyPolicy, error) {
	p := SecretKeyPolicy{RemindEveryDays: DefaultRemindEveryDays}
	var reminded, updated string
	err := db.QueryRowContext(ctx, `
		SELECT max_key_age_days, remind_every_days, last_reminded_at, updated_at, updated_by
		FROM secret_key_policy WHERE id = 1
	`).Scan(&p.MaxKeyAgeDays, &p.RemindEveryDays, &reminded, &updated, &p.UpdatedBy)
	if errors.Is(err, sql.ErrNoRows) {
		return p, nil
	}
	if err != nil {
		return p, fmt.Errorf("store: get secret key policy: %w", err)
	}
	p.LastRemindedAt, p.UpdatedAt = parseStoredTime(reminded), parseStoredTime(updated)
	return p, nil
}

// SaveSecretKeyPolicy stores the policy and keeps the last reminder time.
func (db *DB) SaveSecretKeyPolicy(ctx context.Context, p SecretKeyPolicy, now time.Time) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO secret_key_policy (id, max_key_age_days, remind_every_days, updated_at, updated_by)
		VALUES (1, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET max_key_age_days = excluded.max_key_age_days,
			remind_every_days = excluded.remind_every_days, updated_at = excluded.updated_at, updated_by = excluded.updated_by
	`, p.MaxKeyAgeDays, p.RemindEveryDays, now.UTC().Format(time.RFC3339Nano), p.UpdatedBy)
	if err != nil {
		return fmt.Errorf("store: save secret key policy: %w", err)
	}
	return nil
}

// MarkSecretKeyReminded records when the last overdue reminder went out.
func (db *DB) MarkSecretKeyReminded(ctx context.Context, at time.Time) error {
	stamp := at.UTC().Format(time.RFC3339Nano)
	_, err := db.ExecContext(ctx, `
		INSERT INTO secret_key_policy (id, updated_at, last_reminded_at) VALUES (1, ?, ?)
		ON CONFLICT (id) DO UPDATE SET last_reminded_at = excluded.last_reminded_at
	`, stamp, stamp)
	if err != nil {
		return fmt.Errorf("store: mark secret key reminded: %w", err)
	}
	return nil
}

// DEKRotationRun tracks one rotate-all pass so an interrupted one resumes.
type DEKRotationRun struct {
	ID         string
	StartedAt  time.Time
	FinishedAt time.Time
	Total      int
	Done       int
	Failed     int
	Actor      string
}

// Finished reports whether the run completed.
func (r DEKRotationRun) Finished() bool { return !r.FinishedAt.IsZero() }

const dekRunColumns = `id, started_at, finished_at, total, done, failed, actor`

// LatestDEKRotationRun returns the newest run, ok=false if none.
func (db *DB) LatestDEKRotationRun(ctx context.Context) (DEKRotationRun, bool, error) {
	var r DEKRotationRun
	var started, finished string
	err := db.QueryRowContext(ctx, `SELECT `+dekRunColumns+` FROM dek_rotation_runs ORDER BY started_at DESC, id DESC LIMIT 1`).
		Scan(&r.ID, &started, &finished, &r.Total, &r.Done, &r.Failed, &r.Actor)
	if errors.Is(err, sql.ErrNoRows) {
		return DEKRotationRun{}, false, nil
	}
	if err != nil {
		return DEKRotationRun{}, false, fmt.Errorf("store: latest DEK rotation run: %w", err)
	}
	r.StartedAt, r.FinishedAt = parseStoredTime(started), parseStoredTime(finished)
	return r, true, nil
}

// CreateDEKRotationRun inserts a new run.
func (db *DB) CreateDEKRotationRun(ctx context.Context, r DEKRotationRun) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO dek_rotation_runs (id, started_at, total, actor) VALUES (?, ?, ?, ?)
	`, r.ID, r.StartedAt.UTC().Format(time.RFC3339Nano), r.Total, r.Actor)
	if err != nil {
		return fmt.Errorf("store: create DEK rotation run: %w", err)
	}
	return nil
}

// UpdateDEKRotationRun saves progress, and finished_at when finished.
func (db *DB) UpdateDEKRotationRun(ctx context.Context, r DEKRotationRun) error {
	finished := ""
	if !r.FinishedAt.IsZero() {
		finished = r.FinishedAt.UTC().Format(time.RFC3339Nano)
	}
	_, err := db.ExecContext(ctx, `
		UPDATE dek_rotation_runs SET total = ?, done = ?, failed = ?, finished_at = ? WHERE id = ?
	`, r.Total, r.Done, r.Failed, finished, r.ID)
	if err != nil {
		return fmt.Errorf("store: update DEK rotation run: %w", err)
	}
	return nil
}

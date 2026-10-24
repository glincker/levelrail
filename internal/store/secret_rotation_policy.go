package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Secret rotation policy states. Anything but idle needs a person or a retry.
const (
	SecretRotationIdle          = "idle"
	SecretRotationRestartFailed = "restart_failed"
	SecretRotationFailed        = "failed"
	SecretRotationPending       = "restart_pending"
)

// ErrSecretRotationPolicyNotFound means no policy exists for the secret.
var ErrSecretRotationPolicyNotFound = errors.New("store: secret rotation policy not found")

// SecretRotationPolicy is the schedule and progress of one secret's
// rotation. It never carries a value.
type SecretRotationPolicy struct {
	ServiceName       string
	EnvKey            string
	IntervalDays      int
	GraceHours        int
	Owner             string
	AutoRotate        bool
	LastRotatedAt     time.Time
	State             string
	LastError         string
	PreviousExpiresAt time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

const secretRotationColumns = `service_name, env_key, interval_days, grace_hours, owner, auto_rotate,
	last_rotated_at, state, last_error, previous_expires_at, created_at, updated_at`

func formatRotationTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func parseRotationTime(raw string) (time.Time, error) {
	if raw == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse time %q: %w", raw, err)
	}
	return t, nil
}

type rotationScanner interface {
	Scan(dest ...any) error
}

func scanSecretRotationPolicy(s rotationScanner) (SecretRotationPolicy, error) {
	var p SecretRotationPolicy
	var auto int
	var last, prev, created, updated string
	err := s.Scan(&p.ServiceName, &p.EnvKey, &p.IntervalDays, &p.GraceHours, &p.Owner, &auto,
		&last, &p.State, &p.LastError, &prev, &created, &updated)
	if err != nil {
		return SecretRotationPolicy{}, err
	}
	p.AutoRotate = auto != 0
	for _, f := range []struct {
		raw string
		dst *time.Time
	}{{last, &p.LastRotatedAt}, {prev, &p.PreviousExpiresAt}, {created, &p.CreatedAt}, {updated, &p.UpdatedAt}} {
		if *f.dst, err = parseRotationTime(f.raw); err != nil {
			return SecretRotationPolicy{}, err
		}
	}
	return p, nil
}

// UpsertSecretRotationPolicy writes the operator-set fields of a policy and
// leaves progress (last rotated, state, previous expiry) untouched on update.
func (db *DB) UpsertSecretRotationPolicy(ctx context.Context, p SecretRotationPolicy, now time.Time) error {
	auto := 0
	if p.AutoRotate {
		auto = 1
	}
	ts := formatRotationTime(now)
	_, err := db.ExecContext(ctx, `
		INSERT INTO secret_rotation_policies
			(service_name, env_key, interval_days, grace_hours, owner, auto_rotate, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (service_name, env_key) DO UPDATE SET
			interval_days = excluded.interval_days, grace_hours = excluded.grace_hours,
			owner = excluded.owner, auto_rotate = excluded.auto_rotate, updated_at = excluded.updated_at
	`, p.ServiceName, p.EnvKey, p.IntervalDays, p.GraceHours, p.Owner, auto, ts, ts)
	if err != nil {
		return fmt.Errorf("store: upsert secret rotation policy %q/%q: %w", p.ServiceName, p.EnvKey, err)
	}
	return nil
}

// GetSecretRotationPolicy returns one policy or ErrSecretRotationPolicyNotFound.
func (db *DB) GetSecretRotationPolicy(ctx context.Context, serviceName, envKey string) (SecretRotationPolicy, error) {
	p, err := scanSecretRotationPolicy(db.QueryRowContext(ctx,
		`SELECT `+secretRotationColumns+` FROM secret_rotation_policies WHERE service_name = ? AND env_key = ?`,
		serviceName, envKey))
	if errors.Is(err, sql.ErrNoRows) {
		return SecretRotationPolicy{}, ErrSecretRotationPolicyNotFound
	}
	if err != nil {
		return SecretRotationPolicy{}, fmt.Errorf("store: get secret rotation policy %q/%q: %w", serviceName, envKey, err)
	}
	return p, nil
}

// ListSecretRotationPolicies lists policies for one app, or every app when
// serviceName is empty.
func (db *DB) ListSecretRotationPolicies(ctx context.Context, serviceName string) ([]SecretRotationPolicy, error) {
	query := `SELECT ` + secretRotationColumns + ` FROM secret_rotation_policies`
	var args []any
	if serviceName != "" {
		query += ` WHERE service_name = ?`
		args = append(args, serviceName)
	}
	rows, err := db.reader().QueryContext(ctx, query+` ORDER BY service_name, env_key`, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list secret rotation policies: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []SecretRotationPolicy
	for rows.Next() {
		p, err := scanSecretRotationPolicy(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan secret rotation policy: %w", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list secret rotation policies: %w", err)
	}
	return out, nil
}

// SaveSecretRotationProgress writes the progress fields of an existing policy.
func (db *DB) SaveSecretRotationProgress(ctx context.Context, p SecretRotationPolicy, now time.Time) error {
	res, err := db.ExecContext(ctx, `
		UPDATE secret_rotation_policies
		SET last_rotated_at = ?, state = ?, last_error = ?, previous_expires_at = ?, updated_at = ?
		WHERE service_name = ? AND env_key = ?
	`, formatRotationTime(p.LastRotatedAt), p.State, p.LastError, formatRotationTime(p.PreviousExpiresAt),
		formatRotationTime(now), p.ServiceName, p.EnvKey)
	if err != nil {
		return fmt.Errorf("store: save secret rotation progress %q/%q: %w", p.ServiceName, p.EnvKey, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrSecretRotationPolicyNotFound
	}
	return nil
}

// DeleteSecretRotationPolicy removes a policy, reporting whether one existed.
func (db *DB) DeleteSecretRotationPolicy(ctx context.Context, serviceName, envKey string) (bool, error) {
	res, err := db.ExecContext(ctx, `DELETE FROM secret_rotation_policies WHERE service_name = ? AND env_key = ?`, serviceName, envKey)
	if err != nil {
		return false, fmt.Errorf("store: delete secret rotation policy %q/%q: %w", serviceName, envKey, err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

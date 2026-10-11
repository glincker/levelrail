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

// Audit action names for mobile pairing.
const (
	AuditActionFamilyMobile     = "mobile"
	AuditActionMobilePairCode   = AuditActionFamilyMobile + ".pairing_created"
	AuditActionMobilePaired     = AuditActionFamilyMobile + ".paired"
	AuditActionMobileRevoked    = AuditActionFamilyMobile + ".revoked"
	AuditActionMobilePairFailed = AuditActionFamilyMobile + ".pair_failed"
)

// ErrMobilePairingNotFound covers every reason a pairing code cannot be used.
var ErrMobilePairingNotFound = errors.New("store: mobile pairing code not usable")

// ErrMobileDeviceNotFound is returned when no live device has the id.
var ErrMobileDeviceNotFound = errors.New("store: mobile device not found")

// MobilePairingCode is a one-time code row. CodeHash is hex SHA-256.
type MobilePairingCode struct {
	ID          string
	Lookup      string
	CodeHash    string
	OwnerUserID string
	CreatedAt   time.Time
	ExpiresAt   time.Time
	UsedAt      *time.Time
	FailedTries int
}

// MobileDevice is a paired phone and the API token it holds.
type MobileDevice struct {
	ID          string
	TokenID     string
	OwnerUserID string
	Name        string
	Platform    string
	CreatedAt   time.Time
	RevokedAt   *time.Time
}

// NewMobileID returns a fresh id with prefix.
func NewMobileID(prefix string) (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("store: generate mobile id: %w", err)
	}
	return prefix + base64.RawURLEncoding.EncodeToString(buf), nil
}

// CreateMobilePairingCode inserts c and drops codes that expired an hour ago.
func (db *DB) CreateMobilePairingCode(ctx context.Context, c MobilePairingCode) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM mobile_pairing_codes WHERE expires_at < ?`,
		c.CreatedAt.Add(-time.Hour).UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("store: prune mobile pairing codes: %w", err)
	}
	_, err := db.ExecContext(ctx, `INSERT INTO mobile_pairing_codes
		(id, lookup, code_hash, owner_user_id, created_at, expires_at) VALUES (?, ?, ?, ?, ?, ?)`,
		c.ID, c.Lookup, c.CodeHash, c.OwnerUserID,
		c.CreatedAt.UTC().Format(time.RFC3339Nano), c.ExpiresAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("store: create mobile pairing code: %w", err)
	}
	return nil
}

// GetMobilePairingCodeByLookup returns the unused, unexpired code with lookup.
func (db *DB) GetMobilePairingCodeByLookup(ctx context.Context, lookup string, now time.Time, maxTries int) (MobilePairingCode, error) {
	var (
		c                MobilePairingCode
		created, expires string
	)
	err := db.QueryRowContext(ctx, `SELECT id, lookup, code_hash, owner_user_id, created_at, expires_at, failed_tries
		FROM mobile_pairing_codes WHERE lookup = ? AND used_at IS NULL AND expires_at > ? AND failed_tries < ?`,
		lookup, now.UTC().Format(time.RFC3339Nano), maxTries).
		Scan(&c.ID, &c.Lookup, &c.CodeHash, &c.OwnerUserID, &created, &expires, &c.FailedTries)
	if errors.Is(err, sql.ErrNoRows) {
		return MobilePairingCode{}, ErrMobilePairingNotFound
	}
	if err != nil {
		return MobilePairingCode{}, fmt.Errorf("store: get mobile pairing code: %w", err)
	}
	if c.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
		return MobilePairingCode{}, fmt.Errorf("store: parse pairing created_at: %w", err)
	}
	if c.ExpiresAt, err = time.Parse(time.RFC3339Nano, expires); err != nil {
		return MobilePairingCode{}, fmt.Errorf("store: parse pairing expires_at: %w", err)
	}
	return c, nil
}

// RecordMobilePairingFailure counts one wrong guess against the code.
func (db *DB) RecordMobilePairingFailure(ctx context.Context, id string) error {
	if _, err := db.ExecContext(ctx, `UPDATE mobile_pairing_codes SET failed_tries = failed_tries + 1 WHERE id = ?`, id); err != nil {
		return fmt.Errorf("store: record mobile pairing failure: %w", err)
	}
	return nil
}

// ConsumeMobilePairingCode marks the code used. Exactly one caller wins a race.
func (db *DB) ConsumeMobilePairingCode(ctx context.Context, id string, now time.Time) error {
	ts := now.UTC().Format(time.RFC3339Nano)
	res, err := db.ExecContext(ctx, `UPDATE mobile_pairing_codes SET used_at = ?
		WHERE id = ? AND used_at IS NULL AND expires_at > ?`, ts, id, ts)
	if err != nil {
		return fmt.Errorf("store: consume mobile pairing code: %w", err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return ErrMobilePairingNotFound
	}
	return nil
}

// CreateMobileDevice inserts d.
func (db *DB) CreateMobileDevice(ctx context.Context, d MobileDevice) error {
	_, err := db.ExecContext(ctx, `INSERT INTO mobile_devices (id, token_id, owner_user_id, name, platform, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`, d.ID, d.TokenID, d.OwnerUserID, d.Name, d.Platform, d.CreatedAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("store: create mobile device: %w", err)
	}
	return nil
}

// ListMobileDevices returns live devices, newest first; ownerID "" means all.
func (db *DB) ListMobileDevices(ctx context.Context, ownerID string) ([]MobileDevice, error) {
	q := `SELECT id, token_id, owner_user_id, name, platform, created_at FROM mobile_devices WHERE revoked_at IS NULL`
	args := []any{}
	if ownerID != "" {
		q += ` AND owner_user_id = ?`
		args = append(args, ownerID)
	}
	rows, err := db.QueryContext(ctx, q+` ORDER BY created_at DESC`, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list mobile devices: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []MobileDevice{}
	for rows.Next() {
		var (
			d       MobileDevice
			created string
		)
		if err := rows.Scan(&d.ID, &d.TokenID, &d.OwnerUserID, &d.Name, &d.Platform, &created); err != nil {
			return nil, fmt.Errorf("store: scan mobile device: %w", err)
		}
		if d.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
			return nil, fmt.Errorf("store: parse mobile device created_at: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// GetMobileDevice returns the live device with id.
func (db *DB) GetMobileDevice(ctx context.Context, id string) (MobileDevice, error) {
	var (
		d       MobileDevice
		created string
	)
	err := db.QueryRowContext(ctx, `SELECT id, token_id, owner_user_id, name, platform, created_at
		FROM mobile_devices WHERE id = ? AND revoked_at IS NULL`, id).
		Scan(&d.ID, &d.TokenID, &d.OwnerUserID, &d.Name, &d.Platform, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return MobileDevice{}, ErrMobileDeviceNotFound
	}
	if err != nil {
		return MobileDevice{}, fmt.Errorf("store: get mobile device: %w", err)
	}
	if d.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
		return MobileDevice{}, fmt.Errorf("store: parse mobile device created_at: %w", err)
	}
	return d, nil
}

// RevokeMobileDevice marks the device revoked.
func (db *DB) RevokeMobileDevice(ctx context.Context, id string, now time.Time) error {
	res, err := db.ExecContext(ctx, `UPDATE mobile_devices SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL`,
		now.UTC().Format(time.RFC3339Nano), id)
	if err != nil {
		return fmt.Errorf("store: revoke mobile device: %w", err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return ErrMobileDeviceNotFound
	}
	return nil
}

// LatestDeploymentStatus is the newest deploy of one app.
type LatestDeploymentStatus struct {
	Status     string
	StartedAt  time.Time
	FinishedAt *time.Time
}

// LatestDeploymentsFor returns each named app's newest deploy in one query.
func (db *DB) LatestDeploymentsFor(ctx context.Context, apps []string) (map[string]LatestDeploymentStatus, error) {
	out := map[string]LatestDeploymentStatus{}
	if len(apps) == 0 {
		return out, nil
	}
	ph, args := inPlaceholders(apps)
	rows, err := db.QueryContext(ctx, `SELECT d.service_name, `+deploymentStatusSQL+`, d.started_at, d.finished_at
		FROM deploy_attempts d WHERE d.service_name IN (`+ph+`)
		AND d.started_at = (SELECT MAX(m.started_at) FROM deploy_attempts m WHERE m.service_name = d.service_name)`, args...)
	if err != nil {
		return nil, fmt.Errorf("store: latest deployments: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var (
			name, status, started string
			finished              sql.NullString
		)
		if err := rows.Scan(&name, &status, &started, &finished); err != nil {
			return nil, fmt.Errorf("store: scan latest deployment: %w", err)
		}
		l := LatestDeploymentStatus{Status: status}
		if l.StartedAt, err = time.Parse(time.RFC3339Nano, started); err != nil {
			return nil, fmt.Errorf("store: parse latest deployment started_at: %w", err)
		}
		if finished.Valid {
			if t, perr := time.Parse(time.RFC3339Nano, finished.String); perr == nil {
				l.FinishedAt = &t
			}
		}
		out[name] = l
	}
	return out, rows.Err()
}

func inPlaceholders(vals []string) (string, []any) {
	ph := make([]byte, 0, len(vals)*2)
	args := make([]any, len(vals))
	for i, v := range vals {
		if i > 0 {
			ph = append(ph, ',')
		}
		ph = append(ph, '?')
		args[i] = v
	}
	return string(ph), args
}

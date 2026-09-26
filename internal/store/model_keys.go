package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrModelKeyNotFound is returned when a model has no key with the given id.
var ErrModelKeyNotFound = errors.New("store: model key not found")

// ErrModelKeyExists is returned when a live key already has the name.
var ErrModelKeyExists = errors.New("store: model key name already in use")

// DefaultModelKeyName is the name of the key every model is created with.
const DefaultModelKeyName = "default"

// DefaultModelKeyID is the id of a model's original key.
func DefaultModelKeyID(model string) string { return "default-" + model }

// ModelKey is one named virtual API key of a model. Only the hash of the
// key is stored. A limit of 0 means unlimited; empty allow lists mean any.
type ModelKey struct {
	ID          string
	ModelName   string
	Name        string
	KeyHash     string
	KeyPrefix   string
	RPM         int
	TPM         int
	MaxParallel int
	AllowPaths  []string
	AllowModels []string
	ReplacedBy  string
	CreatedAt   time.Time
	ExpiresAt   *time.Time
	RevokedAt   *time.Time
	LastUsedAt  *time.Time
}

const modelKeyColumns = `id, model_name, name, key_hash, key_prefix, rpm, tpm, max_parallel, allow_paths, allow_models, replaced_by, created_at, expires_at, revoked_at, last_used_at`

type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func insertModelKey(ctx context.Context, ex execer, k ModelKey) error {
	paths, err := json.Marshal(nonNilStrings(k.AllowPaths))
	if err != nil {
		return fmt.Errorf("marshal allow paths: %w", err)
	}
	models, err := json.Marshal(nonNilStrings(k.AllowModels))
	if err != nil {
		return fmt.Errorf("marshal allow models: %w", err)
	}
	_, err = ex.ExecContext(ctx, `INSERT INTO model_keys (`+modelKeyColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		k.ID, k.ModelName, k.Name, k.KeyHash, k.KeyPrefix, k.RPM, k.TPM, k.MaxParallel, string(paths), string(models),
		k.ReplacedBy, formatTime(k.CreatedAt), timeOrEmpty(k.ExpiresAt), timeOrEmpty(k.RevokedAt), timeOrEmpty(k.LastUsedAt))
	if err != nil {
		if strings.Contains(err.Error(), "model_keys.model_name") {
			return ErrModelKeyExists
		}
		return err
	}
	return nil
}

func timeOrEmpty(t *time.Time) string {
	if t == nil {
		return ""
	}
	return formatTime(*t)
}

func parseTimeOrNil(s string) (*time.Time, error) {
	if s == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func scanModelKey(scan func(dest ...any) error) (*ModelKey, error) {
	var (
		k                                       ModelKey
		paths, models                           string
		createdAt, expiresAt, revokedAt, usedAt string
	)
	if err := scan(&k.ID, &k.ModelName, &k.Name, &k.KeyHash, &k.KeyPrefix, &k.RPM, &k.TPM, &k.MaxParallel, &paths, &models,
		&k.ReplacedBy, &createdAt, &expiresAt, &revokedAt, &usedAt); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(paths), &k.AllowPaths); err != nil {
		return nil, fmt.Errorf("parse allow_paths: %w", err)
	}
	if err := json.Unmarshal([]byte(models), &k.AllowModels); err != nil {
		return nil, fmt.Errorf("parse allow_models: %w", err)
	}
	var err error
	if k.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	if k.ExpiresAt, err = parseTimeOrNil(expiresAt); err != nil {
		return nil, fmt.Errorf("parse expires_at: %w", err)
	}
	if k.RevokedAt, err = parseTimeOrNil(revokedAt); err != nil {
		return nil, fmt.Errorf("parse revoked_at: %w", err)
	}
	if k.LastUsedAt, err = parseTimeOrNil(usedAt); err != nil {
		return nil, fmt.Errorf("parse last_used_at: %w", err)
	}
	return &k, nil
}

func (db *DB) queryModelKeys(ctx context.Context, where string, args ...any) ([]ModelKey, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+modelKeyColumns+` FROM model_keys `+where+` ORDER BY model_name, created_at, id`, args...)
	if err != nil {
		return nil, fmt.Errorf("store: query model keys: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []ModelKey
	for rows.Next() {
		k, err := scanModelKey(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan model key: %w", err)
		}
		out = append(out, *k)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate model keys: %w", err)
	}
	return out, nil
}

// ListModelKeys returns every key of a model, revoked ones included.
func (db *DB) ListModelKeys(ctx context.Context, model string) ([]ModelKey, error) {
	return db.queryModelKeys(ctx, `WHERE model_name = ?`, model)
}

// ListActiveModelKeys returns the non-revoked keys of every model, for the
// gateway. Expiry is checked by the caller against the request time.
func (db *DB) ListActiveModelKeys(ctx context.Context) ([]ModelKey, error) {
	return db.queryModelKeys(ctx, `WHERE revoked_at = ''`)
}

// GetModelKey returns one key of a model, or ErrModelKeyNotFound.
func (db *DB) GetModelKey(ctx context.Context, model, id string) (*ModelKey, error) {
	k, err := scanModelKey(db.QueryRowContext(ctx, `SELECT `+modelKeyColumns+` FROM model_keys WHERE model_name = ? AND id = ?`, model, id).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrModelKeyNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: get model key %q: %w", id, err)
	}
	return k, nil
}

// CreateModelKey inserts a key for an existing model.
func (db *DB) CreateModelKey(ctx context.Context, k ModelKey) error {
	if _, err := db.GetModel(ctx, k.ModelName); err != nil {
		return err
	}
	if err := insertModelKey(ctx, db, k); err != nil {
		if errors.Is(err, ErrModelKeyExists) {
			return err
		}
		return fmt.Errorf("store: create model key %q: %w", k.Name, err)
	}
	return nil
}

// RevokeModelKey stops a key from authenticating immediately. Idempotent.
func (db *DB) RevokeModelKey(ctx context.Context, model, id string, now time.Time) error {
	res, err := db.ExecContext(ctx, `UPDATE model_keys SET revoked_at = CASE WHEN revoked_at = '' THEN ? ELSE revoked_at END WHERE model_name = ? AND id = ?`,
		formatTime(now), model, id)
	if err != nil {
		return fmt.Errorf("store: revoke model key %q: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrModelKeyNotFound
	}
	return nil
}

// RotateModelKey inserts next and retires the key oldID: it is revoked at
// once when graceUntil is nil, otherwise it keeps working until then.
func (db *DB) RotateModelKey(ctx context.Context, model, oldID string, next ModelKey, graceUntil *time.Time, now time.Time) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin rotate model key: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	expires, revoked := "", ""
	if graceUntil == nil {
		revoked = formatTime(now)
	} else {
		expires = formatTime(*graceUntil)
	}
	res, err := tx.ExecContext(ctx, `UPDATE model_keys SET replaced_by = ?, revoked_at = CASE WHEN ? <> '' THEN ? ELSE revoked_at END,
		expires_at = CASE WHEN ? <> '' AND (expires_at = '' OR expires_at > ?) THEN ? ELSE expires_at END
		WHERE model_name = ? AND id = ? AND revoked_at = '' AND replaced_by = ''`,
		next.ID, revoked, revoked, expires, expires, expires, model, oldID)
	if err != nil {
		return fmt.Errorf("store: retire model key %q: %w", oldID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrModelKeyNotFound
	}
	if err := insertModelKey(ctx, tx, next); err != nil {
		if errors.Is(err, ErrModelKeyExists) {
			return err
		}
		return fmt.Errorf("store: insert rotated model key: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit rotate model key: %w", err)
	}
	return nil
}

// TouchModelKeys records last-use times, keeping the later of the stored
// and given value.
func (db *DB) TouchModelKeys(ctx context.Context, used map[string]time.Time) error {
	if len(used) == 0 {
		return nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin touch model keys: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for id, at := range used {
		ts := formatTime(at)
		if _, err := tx.ExecContext(ctx, `UPDATE model_keys SET last_used_at = ? WHERE id = ? AND last_used_at < ?`, ts, id, ts); err != nil {
			return fmt.Errorf("store: touch model key %q: %w", id, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit touch model keys: %w", err)
	}
	return nil
}

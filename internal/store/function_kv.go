package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Errors returned by the function key-value store.
var (
	ErrKVNotFound      = errors.New("store: kv key not found")
	ErrKVKeyInvalid    = errors.New("store: kv key is empty or too long")
	ErrKVValueTooLarge = errors.New("store: kv value exceeds the size limit")
	ErrKVTooManyKeys   = errors.New("store: kv namespace is at its key limit")
	ErrKVNamespaceFull = errors.New("store: kv namespace is at its byte limit")
	ErrKVTokenInvalid  = errors.New("store: kv token is not valid")
)

// KVLimits bound one namespace; a zero field means unlimited.
type KVLimits struct {
	MaxKeyBytes   int
	MaxValueBytes int
	MaxKeys       int
	MaxTotalBytes int64
}

// KVEntry is one stored key. Value is empty in listings.
type KVEntry struct {
	Key       string
	Value     []byte
	Size      int
	ExpiresAt time.Time
	UpdatedAt time.Time
}

// KVUsage is what a namespace currently holds.
type KVUsage struct {
	Keys  int
	Bytes int64
}

const kvTokenPrefix = "lrkv_"

// HashKVToken returns the lookup hash stored for a token.
func HashKVToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// RotateFunctionKVToken issues a new token for the namespace and invalidates the old one.
// The plaintext is returned once; only its hash is stored here.
func (db *DB) RotateFunctionKVToken(ctx context.Context, namespace string, now time.Time) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("store: generate kv token for %q: %w", namespace, err)
	}
	token := kvTokenPrefix + hex.EncodeToString(raw)
	_, err := db.ExecContext(ctx, `INSERT INTO function_kv_tokens (namespace, token_hash, rotated_at) VALUES (?, ?, ?)
ON CONFLICT(namespace) DO UPDATE SET token_hash = excluded.token_hash, rotated_at = excluded.rotated_at`,
		namespace, HashKVToken(token), now.Unix())
	if err != nil {
		return "", fmt.Errorf("store: save kv token for %q: %w", namespace, err)
	}
	return token, nil
}

// ResolveFunctionKVToken maps a presented token to the one namespace it is scoped to.
func (db *DB) ResolveFunctionKVToken(ctx context.Context, token string) (string, error) {
	if !strings.HasPrefix(token, kvTokenPrefix) {
		return "", ErrKVTokenInvalid
	}
	var ns string
	err := db.QueryRowContext(ctx, `SELECT namespace FROM function_kv_tokens WHERE token_hash = ?`, HashKVToken(token)).Scan(&ns)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrKVTokenInvalid
	}
	if err != nil {
		return "", fmt.Errorf("store: resolve kv token: %w", err)
	}
	return ns, nil
}

// HasFunctionKVToken reports whether the namespace has a token, meaning the store is enabled for it.
func (db *DB) HasFunctionKVToken(ctx context.Context, namespace string) (bool, error) {
	var n int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM function_kv_tokens WHERE namespace = ?`, namespace).Scan(&n); err != nil {
		return false, fmt.Errorf("store: check kv token for %q: %w", namespace, err)
	}
	return n > 0, nil
}

// DeleteFunctionKV removes a namespace's token and every key, for when the function is deleted.
func (db *DB) DeleteFunctionKV(ctx context.Context, namespace string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin kv delete for %q: %w", namespace, err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM function_kv WHERE namespace = ?`, namespace); err != nil {
		return fmt.Errorf("store: delete kv keys of %q: %w", namespace, err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM function_kv_tokens WHERE namespace = ?`, namespace); err != nil {
		return fmt.Errorf("store: delete kv token of %q: %w", namespace, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit kv delete for %q: %w", namespace, err)
	}
	return nil
}

const kvLive = `(expires_at IS NULL OR expires_at > ?)`

// PutFunctionKV stores a value in the namespace. A ttl of zero or less keeps the key until it is deleted.
func (db *DB) PutFunctionKV(ctx context.Context, namespace, key string, value []byte, ttl time.Duration, lim KVLimits, now time.Time) error {
	if key == "" || (lim.MaxKeyBytes > 0 && len(key) > lim.MaxKeyBytes) {
		return ErrKVKeyInvalid
	}
	if lim.MaxValueBytes > 0 && len(value) > lim.MaxValueBytes {
		return ErrKVValueTooLarge
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin kv put in %q: %w", namespace, err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM function_kv WHERE namespace = ? AND expires_at IS NOT NULL AND expires_at <= ?`, namespace, now.Unix()); err != nil {
		return fmt.Errorf("store: purge expired kv in %q: %w", namespace, err)
	}
	var oldSize sql.NullInt64
	err = tx.QueryRowContext(ctx, `SELECT length(CAST(key AS BLOB)) + length(value) FROM function_kv WHERE namespace = ? AND key = ?`, namespace, key).Scan(&oldSize)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("store: read kv key in %q: %w", namespace, err)
	}
	var keys int
	var bytes int64
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(SUM(length(CAST(key AS BLOB)) + length(value)), 0) FROM function_kv WHERE namespace = ?`, namespace).Scan(&keys, &bytes); err != nil {
		return fmt.Errorf("store: measure kv namespace %q: %w", namespace, err)
	}
	if !oldSize.Valid && lim.MaxKeys > 0 && keys >= lim.MaxKeys {
		return ErrKVTooManyKeys
	}
	if lim.MaxTotalBytes > 0 && bytes-oldSize.Int64+int64(len(key)+len(value)) > lim.MaxTotalBytes {
		return ErrKVNamespaceFull
	}
	var expires sql.NullInt64
	if ttl > 0 {
		expires = sql.NullInt64{Int64: now.Add(ttl).Unix(), Valid: true}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO function_kv (namespace, key, value, expires_at, updated_at) VALUES (?, ?, ?, ?, ?)
ON CONFLICT(namespace, key) DO UPDATE SET value = excluded.value, expires_at = excluded.expires_at, updated_at = excluded.updated_at`,
		namespace, key, value, expires, now.Unix()); err != nil {
		return fmt.Errorf("store: put kv key in %q: %w", namespace, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit kv put in %q: %w", namespace, err)
	}
	return nil
}

// GetFunctionKV returns a live key, or ErrKVNotFound.
func (db *DB) GetFunctionKV(ctx context.Context, namespace, key string, now time.Time) (KVEntry, error) {
	e := KVEntry{Key: key}
	var expires sql.NullInt64
	var updated int64
	err := db.QueryRowContext(ctx, `SELECT value, expires_at, updated_at FROM function_kv WHERE namespace = ? AND key = ? AND `+kvLive, namespace, key, now.Unix()).Scan(&e.Value, &expires, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return KVEntry{}, ErrKVNotFound
	}
	if err != nil {
		return KVEntry{}, fmt.Errorf("store: get kv key in %q: %w", namespace, err)
	}
	e.Size = len(e.Value)
	e.UpdatedAt = time.Unix(updated, 0).UTC()
	if expires.Valid {
		e.ExpiresAt = time.Unix(expires.Int64, 0).UTC()
	}
	return e, nil
}

// DeleteFunctionKVKey removes one key and reports whether it existed.
func (db *DB) DeleteFunctionKVKey(ctx context.Context, namespace, key string) (bool, error) {
	res, err := db.ExecContext(ctx, `DELETE FROM function_kv WHERE namespace = ? AND key = ?`, namespace, key)
	if err != nil {
		return false, fmt.Errorf("store: delete kv key in %q: %w", namespace, err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// ListFunctionKV lists live keys with the prefix in key order. withValue also loads values.
func (db *DB) ListFunctionKV(ctx context.Context, namespace, prefix string, limit int, withValue bool, now time.Time) ([]KVEntry, error) {
	if limit <= 0 {
		limit = 100
	}
	esc := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(prefix)
	col := "NULL"
	if withValue {
		col = "value"
	}
	rows, err := db.QueryContext(ctx, `SELECT key, length(value), `+col+`, expires_at, updated_at FROM function_kv
WHERE namespace = ? AND key LIKE ? ESCAPE '\' AND `+kvLive+` ORDER BY key LIMIT ?`, namespace, esc+"%", now.Unix(), limit)
	if err != nil {
		return nil, fmt.Errorf("store: list kv keys in %q: %w", namespace, err)
	}
	defer func() { _ = rows.Close() }()
	out := []KVEntry{}
	for rows.Next() {
		var e KVEntry
		var expires sql.NullInt64
		var updated int64
		if err := rows.Scan(&e.Key, &e.Size, &e.Value, &expires, &updated); err != nil {
			return nil, fmt.Errorf("store: scan kv key in %q: %w", namespace, err)
		}
		e.UpdatedAt = time.Unix(updated, 0).UTC()
		if expires.Valid {
			e.ExpiresAt = time.Unix(expires.Int64, 0).UTC()
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: read kv keys in %q: %w", namespace, err)
	}
	return out, nil
}

// FunctionKVUsage counts live keys and bytes in the namespace.
func (db *DB) FunctionKVUsage(ctx context.Context, namespace string, now time.Time) (KVUsage, error) {
	var u KVUsage
	err := db.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(SUM(length(CAST(key AS BLOB)) + length(value)), 0) FROM function_kv WHERE namespace = ? AND `+kvLive, namespace, now.Unix()).Scan(&u.Keys, &u.Bytes)
	if err != nil {
		return KVUsage{}, fmt.Errorf("store: measure kv namespace %q: %w", namespace, err)
	}
	return u, nil
}

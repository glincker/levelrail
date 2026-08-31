package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// This file is pure byte storage for TASKS.md 1.7's envelope encryption:
// it never sees a plaintext secret value or a raw DEK,
// only a wrapped DEK and ciphertext, both opaque []byte as far as this
// package is concerned. internal/secrets.Manager is what actually
// encrypts, decrypts, wraps, and unwraps; this is its storage layer,
// the same separation reconcile_status.go keeps between "what a
// controller decided" and "what persists it".

// ErrServiceDEKNotFound is returned by GetServiceDEK when no DEK has
// been generated for a service yet, i.e. no secret value has ever been
// set for it.
var ErrServiceDEKNotFound = errors.New("store: service DEK not found")

// SaveServiceDEK creates a service's wrapped DEK. Called once per
// service, the first time a secret value is set for it: internal/secrets.
// Manager checks GetServiceDEK first and only calls this on
// ErrServiceDEKNotFound, so this intentionally has no upsert path. A
// service's DEK, once generated, is never replaced: replacing it would
// silently orphan every value already encrypted under the old one.
func (db *DB) SaveServiceDEK(ctx context.Context, serviceName string, wrappedDEK []byte) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO service_secrets (service_name, wrapped_dek)
		VALUES (?, ?)
	`, serviceName, wrappedDEK)
	if err != nil {
		return fmt.Errorf("store: save service DEK for %q: %w", serviceName, err)
	}
	return nil
}

// GetServiceDEK returns the wrapped DEK for serviceName, or
// ErrServiceDEKNotFound if none has been generated yet.
func (db *DB) GetServiceDEK(ctx context.Context, serviceName string) ([]byte, error) {
	var wrapped []byte
	err := db.QueryRowContext(ctx, `
		SELECT wrapped_dek FROM service_secrets WHERE service_name = ?
	`, serviceName).Scan(&wrapped)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrServiceDEKNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: get service DEK for %q: %w", serviceName, err)
	}
	return wrapped, nil
}

// ErrSecretValueNotFound is returned by GetSecretValue when no value has
// been set for that (service, key) pair.
var ErrSecretValueNotFound = errors.New("store: secret value not found")

// SaveSecretValue creates or replaces the ciphertext for one env var of
// one service. Unlike SaveServiceDEK this is a real upsert: updating a
// secret's value (rotating it) is the normal case, updating its DEK is
// not.
func (db *DB) SaveSecretValue(ctx context.Context, serviceName, envKey string, ciphertext []byte) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO service_secret_values (service_name, env_key, ciphertext, updated_at)
		VALUES (?, ?, ?, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		ON CONFLICT (service_name, env_key) DO UPDATE SET
			ciphertext = excluded.ciphertext,
			updated_at = excluded.updated_at
	`, serviceName, envKey, ciphertext)
	if err != nil {
		return fmt.Errorf("store: save secret value for %q/%q: %w", serviceName, envKey, err)
	}
	return nil
}

// GetSecretValue returns the ciphertext for one env var of one service,
// or ErrSecretValueNotFound if no value has been set.
func (db *DB) GetSecretValue(ctx context.Context, serviceName, envKey string) ([]byte, error) {
	var ciphertext []byte
	err := db.QueryRowContext(ctx, `
		SELECT ciphertext FROM service_secret_values WHERE service_name = ? AND env_key = ?
	`, serviceName, envKey).Scan(&ciphertext)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrSecretValueNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: get secret value for %q/%q: %w", serviceName, envKey, err)
	}
	return ciphertext, nil
}

// HasSecretValue reports whether a value has been set for (serviceName,
// envKey), without decrypting or even fetching the ciphertext. Used by
// internal/deploy to fail a deploy loudly when a { secret: true,
// required: true } env var has no value yet, matching
// spec.EnvVar.Required's documented meaning, rather than deferring that
// check to container-create time where a missing secret would only
// surface as a confusing runtime failure.
func (db *DB) HasSecretValue(ctx context.Context, serviceName, envKey string) (bool, error) {
	var exists int
	err := db.QueryRowContext(ctx, `
		SELECT 1 FROM service_secret_values WHERE service_name = ? AND env_key = ? LIMIT 1
	`, serviceName, envKey).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("store: check secret value for %q/%q: %w", serviceName, envKey, err)
	}
	return true, nil
}

// SecretKeyInfo is one secret key known for a service, with its locked
// state, never a value.
type SecretKeyInfo struct {
	Key    string
	Locked bool
}

// ListSecretKeys returns every secret key set for serviceName, ordered
// by key, each with its locked state. Never touches ciphertext: the
// caller (internal/api's GET /apps/{name}/secrets) can safely return
// this straight to a browser.
func (db *DB) ListSecretKeys(ctx context.Context, serviceName string) ([]SecretKeyInfo, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT env_key, locked FROM service_secret_values
		WHERE service_name = ? ORDER BY env_key
	`, serviceName)
	if err != nil {
		return nil, fmt.Errorf("store: list secret keys for %q: %w", serviceName, err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var keys []SecretKeyInfo
	for rows.Next() {
		var info SecretKeyInfo
		var locked int
		if err := rows.Scan(&info.Key, &locked); err != nil {
			return nil, fmt.Errorf("store: scan secret key for %q: %w", serviceName, err)
		}
		info.Locked = locked != 0
		keys = append(keys, info)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list secret keys for %q: %w", serviceName, err)
	}
	return keys, nil
}

// GetSecretKeyLocked reports whether (serviceName, envKey) has a value
// set and, if so, whether it's locked. exists is false when no value
// has ever been set for that key; locked is only meaningful when exists
// is true.
func (db *DB) GetSecretKeyLocked(ctx context.Context, serviceName, envKey string) (exists, locked bool, err error) {
	var lockedInt int
	scanErr := db.QueryRowContext(ctx, `
		SELECT locked FROM service_secret_values WHERE service_name = ? AND env_key = ?
	`, serviceName, envKey).Scan(&lockedInt)
	if errors.Is(scanErr, sql.ErrNoRows) {
		return false, false, nil
	}
	if scanErr != nil {
		return false, false, fmt.Errorf("store: get secret key locked state for %q/%q: %w", serviceName, envKey, scanErr)
	}
	return true, lockedInt != 0, nil
}

// SetSecretLocked sets (serviceName, envKey)'s locked flag, either
// direction: unlike Coolify's permanent lock, this is meant to be
// reversible. Returns ErrSecretValueNotFound if no value has been set
// for that key yet, matching GetSecretValue's own not-found sentinel:
// locking is a property of a value that already exists, not something
// pre-settable before one does.
func (db *DB) SetSecretLocked(ctx context.Context, serviceName, envKey string, locked bool) error {
	lockedInt := 0
	if locked {
		lockedInt = 1
	}
	res, err := db.ExecContext(ctx, `
		UPDATE service_secret_values SET locked = ? WHERE service_name = ? AND env_key = ?
	`, lockedInt, serviceName, envKey)
	if err != nil {
		return fmt.Errorf("store: set secret locked for %q/%q: %w", serviceName, envKey, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: set secret locked for %q/%q: %w", serviceName, envKey, err)
	}
	if n == 0 {
		return ErrSecretValueNotFound
	}
	return nil
}

// DeleteServiceSecrets removes every value and the wrapped DEK for
// serviceName, in a transaction. Deleting the DEK itself (not just the
// value rows) matters: any ciphertext that somehow survives elsewhere
// (a backup, a replication lag) becomes permanently unrecoverable once
// its DEK is gone, rather than merely inaccessible through this API.
// Idempotent: a serviceName with no rows in either table is not an
// error, matching DeleteDesiredService's own "delete is idempotent"
// convention.
func (db *DB) DeleteServiceSecrets(ctx context.Context, serviceName string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: delete service secrets for %q: begin transaction: %w", serviceName, err)
	}
	defer func() {
		_ = tx.Rollback() // no-op if Commit already succeeded
	}()

	if _, err := tx.ExecContext(ctx, `DELETE FROM service_secret_values WHERE service_name = ?`, serviceName); err != nil {
		return fmt.Errorf("store: delete service secret values for %q: %w", serviceName, err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM service_secrets WHERE service_name = ?`, serviceName); err != nil {
		return fmt.Errorf("store: delete service DEK for %q: %w", serviceName, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: delete service secrets for %q: commit: %w", serviceName, err)
	}
	return nil
}

// RotateServiceDEKs reads every row in service_secrets, calls rewrap
// once per row, writes back whatever it returns, and records the
// rotation timestamp, all in one transaction: if rewrap returns an
// error for any row, nothing is written, matching DeleteServiceSecrets's
// own defer-rollback pattern. Callers (internal/secrets.RotateStoredDEKs)
// supply rewrap to do the actual unwrap-under-old-key/wrap-under-new-key
// work; this file only ever sees opaque bytes.
func (db *DB) RotateServiceDEKs(ctx context.Context, rewrap func(serviceName string, wrapped []byte) ([]byte, error)) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: rotate service DEKs: begin transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback() // no-op if Commit already succeeded
	}()

	rows, err := tx.QueryContext(ctx, `SELECT service_name, wrapped_dek FROM service_secrets`)
	if err != nil {
		return fmt.Errorf("store: rotate service DEKs: list: %w", err)
	}
	type serviceDEK struct {
		serviceName string
		wrapped     []byte
	}
	var deks []serviceDEK
	for rows.Next() {
		var d serviceDEK
		if err := rows.Scan(&d.serviceName, &d.wrapped); err != nil {
			_ = rows.Close()
			return fmt.Errorf("store: rotate service DEKs: scan: %w", err)
		}
		deks = append(deks, d)
	}
	rowsErr := rows.Err()
	_ = rows.Close()
	if rowsErr != nil {
		return fmt.Errorf("store: rotate service DEKs: %w", rowsErr)
	}

	for _, d := range deks {
		newWrapped, err := rewrap(d.serviceName, d.wrapped)
		if err != nil {
			return fmt.Errorf("store: rotate service DEKs: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE service_secrets SET wrapped_dek = ? WHERE service_name = ?
		`, newWrapped, d.serviceName); err != nil {
			return fmt.Errorf("store: rotate service DEKs: update %q: %w", d.serviceName, err)
		}
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO master_key_rotation (id, rotated_at) VALUES (1, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		ON CONFLICT (id) DO UPDATE SET rotated_at = excluded.rotated_at
	`); err != nil {
		return fmt.Errorf("store: rotate service DEKs: record rotation timestamp: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: rotate service DEKs: commit: %w", err)
	}
	return nil
}

// GetMasterKeyRotatedAt returns the last time RotateServiceDEKs
// committed successfully, or ok=false if it never has.
func (db *DB) GetMasterKeyRotatedAt(ctx context.Context) (rotatedAt time.Time, ok bool, err error) {
	var raw string
	scanErr := db.QueryRowContext(ctx, `SELECT rotated_at FROM master_key_rotation WHERE id = 1`).Scan(&raw)
	if errors.Is(scanErr, sql.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if scanErr != nil {
		return time.Time{}, false, fmt.Errorf("store: get master key rotated at: %w", scanErr)
	}
	t, parseErr := time.Parse(time.RFC3339Nano, raw)
	if parseErr != nil {
		return time.Time{}, false, fmt.Errorf("store: get master key rotated at: parse timestamp: %w", parseErr)
	}
	return t, true, nil
}

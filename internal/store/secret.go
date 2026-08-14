package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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

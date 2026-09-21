package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
)

// instanceIDPrefix mirrors NewDeployAttemptID/NewAuditEntryID's own
// "short, greppable tag on an otherwise-opaque random ID" convention.
const instanceIDPrefix = "inst_"

// NewInstanceID generates an opaque, URL-safe control-plane instance
// identifier, minted the same way NewAuditEntryID mints its own (fixed-
// length crypto/rand bytes, base64 URL encoding, a short prefix).
func NewInstanceID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("store: generate instance id: %w", err)
	}
	return instanceIDPrefix + base64.RawURLEncoding.EncodeToString(buf), nil
}

// GetOrCreateInstanceID returns this control-plane instance's own
// persistent, random identity, minting and persisting one on first call
// if none exists yet. Every subsequent call, across restarts, returns
// the exact same value: this is the identity internal/spec.InstanceLabelKey
// stamps onto every Docker resource this instance creates, so two
// separate instances sharing one Docker daemon can tell their own
// resources apart during cleanup (see InstanceLabelKey's own doc
// comment).
//
// Race-safe against a concurrent first call (two processes racing to
// seed the singleton row, or a caller invoking this more than once
// during startup): INSERT OR IGNORE means at most one insert ever wins,
// and the final SELECT always reads back whichever one did, so every
// caller converges on the same ID regardless of who inserted it.
func (db *DB) GetOrCreateInstanceID(ctx context.Context) (string, error) {
	if id, err := db.instanceID(ctx); err == nil {
		return id, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}

	newID, err := NewInstanceID()
	if err != nil {
		return "", err
	}
	if _, err := db.ExecContext(ctx, `
		INSERT OR IGNORE INTO instance_identity (id, instance_id) VALUES (1, ?)
	`, newID); err != nil {
		return "", fmt.Errorf("store: create instance id: %w", err)
	}

	id, err := db.instanceID(ctx)
	if err != nil {
		return "", fmt.Errorf("store: read instance id after create: %w", err)
	}
	return id, nil
}

func (db *DB) instanceID(ctx context.Context) (string, error) {
	var id string
	err := db.QueryRowContext(ctx, `SELECT instance_id FROM instance_identity WHERE id = 1`).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("store: get instance id: %w", err)
	}
	return id, nil
}

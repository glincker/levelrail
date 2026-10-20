package store

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// InstanceMarkerFile sits in the data dir beside the database; a restored DB without it was moved.
const InstanceMarkerFile = "instance.marker"

const tokenSeenWriteInterval = time.Minute

// ErrNoPreviousInstance is returned by ConfirmSameInstance when no move is pending.
var ErrNoPreviousInstance = errors.New("store: no previous instance identity to restore")

// InstanceInfo is what the public identity endpoint publishes. The fingerprint is an
// HMAC of the instance id under a per-instance key that never leaves the database.
type InstanceInfo struct {
	InstanceID         string
	Fingerprint        string
	MovedAt            string
	PreviousInstanceID string
}

func newRandomKey() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("store: generate key: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// FormatFingerprint renders a hex digest as dash-separated groups of four for reading aloud.
func FormatFingerprint(hexDigest string) string {
	var sb strings.Builder
	for i := 0; i < len(hexDigest); i += 4 {
		if i > 0 {
			sb.WriteByte('-')
		}
		sb.WriteString(hexDigest[i:min(i+4, len(hexDigest))])
	}
	return sb.String()
}

func computeFingerprint(key, instanceID string) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(instanceID))
	return FormatFingerprint(hex.EncodeToString(mac.Sum(nil)[:16]))
}

// GetInstanceInfo returns the instance identity with its fingerprint, minting the
// fingerprint key on first call.
func (db *DB) GetInstanceInfo(ctx context.Context) (InstanceInfo, error) {
	id, err := db.GetOrCreateInstanceID(ctx)
	if err != nil {
		return InstanceInfo{}, err
	}
	var key string
	var info InstanceInfo
	err = db.QueryRowContext(ctx, `SELECT fingerprint_key, moved_at, previous_instance_id FROM instance_identity WHERE id = 1`).
		Scan(&key, &info.MovedAt, &info.PreviousInstanceID)
	if err != nil {
		return InstanceInfo{}, fmt.Errorf("store: read instance identity: %w", err)
	}
	if key == "" {
		fresh, kerr := newRandomKey()
		if kerr != nil {
			return InstanceInfo{}, kerr
		}
		if _, err := db.ExecContext(ctx, `UPDATE instance_identity SET fingerprint_key = ? WHERE id = 1 AND fingerprint_key = ''`, fresh); err != nil {
			return InstanceInfo{}, fmt.Errorf("store: save fingerprint key: %w", err)
		}
		if err := db.QueryRowContext(ctx, `SELECT fingerprint_key FROM instance_identity WHERE id = 1`).Scan(&key); err != nil {
			return InstanceInfo{}, fmt.Errorf("store: read fingerprint key: %w", err)
		}
	}
	info.InstanceID = id
	info.Fingerprint = computeFingerprint(key, id)
	return info, nil
}

// ReconcileInstanceHost compares the DB's host marker with the marker file in dataDir.
// A DB restored into a fresh data dir (marker missing or different) gets a new identity,
// so tokens bound to the old one stop working until an owner confirms ConfirmSameInstance.
func (db *DB) ReconcileInstanceHost(ctx context.Context, dataDir string) (bool, error) {
	if _, err := db.GetOrCreateInstanceID(ctx); err != nil {
		return false, err
	}
	path := filepath.Join(dataDir, InstanceMarkerFile)
	fileMarker := ""
	if b, rerr := os.ReadFile(path); rerr == nil { //nolint:gosec // fixed file name under the operator's data dir
		fileMarker = strings.TrimSpace(string(b))
	}
	var dbMarker string
	if err := db.QueryRowContext(ctx, `SELECT host_marker FROM instance_identity WHERE id = 1`).Scan(&dbMarker); err != nil {
		return false, fmt.Errorf("store: read host marker: %w", err)
	}

	switch {
	case dbMarker == "" && fileMarker == "":
		return false, db.writeMarker(ctx, path)
	case dbMarker == "":
		_, err := db.ExecContext(ctx, `UPDATE instance_identity SET host_marker = ? WHERE id = 1`, fileMarker)
		return false, err
	case dbMarker == fileMarker:
		return false, nil
	}
	if err := db.rotateInstance(ctx); err != nil {
		return false, err
	}
	return true, db.writeMarker(ctx, path)
}

func (db *DB) writeMarker(ctx context.Context, path string) error {
	marker, err := newRandomKey()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("store: create data dir for instance marker: %w", err)
	}
	if err := os.WriteFile(path, []byte(marker+"\n"), 0o600); err != nil {
		return fmt.Errorf("store: write instance marker: %w", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE instance_identity SET host_marker = ? WHERE id = 1`, marker); err != nil {
		return fmt.Errorf("store: save host marker: %w", err)
	}
	return nil
}

func (db *DB) rotateInstance(ctx context.Context) error {
	newID, err := NewInstanceID()
	if err != nil {
		return err
	}
	newKey, err := newRandomKey()
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `
		UPDATE instance_identity SET
			previous_instance_id = instance_id, previous_fingerprint_key = fingerprint_key,
			instance_id = ?, fingerprint_key = ?, moved_at = ?
		WHERE id = 1
	`, newID, newKey, formatTime(time.Now()))
	if err != nil {
		return fmt.Errorf("store: rotate instance identity: %w", err)
	}
	return nil
}

// ConfirmSameInstance restores the pre-move identity after an owner confirms this is the
// same instance, so tokens bound to it work again.
func (db *DB) ConfirmSameInstance(ctx context.Context) error {
	res, err := db.ExecContext(ctx, `
		UPDATE instance_identity SET
			instance_id = previous_instance_id, fingerprint_key = previous_fingerprint_key,
			previous_instance_id = '', previous_fingerprint_key = '', moved_at = ''
		WHERE id = 1 AND previous_instance_id <> ''
	`)
	if err != nil {
		return fmt.Errorf("store: confirm same instance: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNoPreviousInstance
	}
	return nil
}

// TokenBinding is the server a token was first used against, plus where it was last seen.
type TokenBinding struct {
	TokenID    string
	InstanceID string
	BoundAt    string
	LastIP     string
	LastSeenAt string
}

// BindOrCheckToken binds an unbound token to instanceID on first use (so existing tokens
// keep working) and reports whether the token belongs to this instance.
func (db *DB) BindOrCheckToken(ctx context.Context, tokenID, instanceID, ip string) (bool, error) {
	now := time.Now()
	if _, err := db.ExecContext(ctx, `
		INSERT OR IGNORE INTO api_token_bindings (token_id, instance_id, bound_at, last_ip, last_seen_at)
		VALUES (?, ?, ?, ?, ?)
	`, tokenID, instanceID, formatTime(now), ip, formatTime(now)); err != nil {
		return false, fmt.Errorf("store: bind token %q: %w", tokenID, err)
	}
	var bound, lastIP, lastSeen string
	err := db.QueryRowContext(ctx, `SELECT instance_id, last_ip, last_seen_at FROM api_token_bindings WHERE token_id = ?`, tokenID).
		Scan(&bound, &lastIP, &lastSeen)
	if err != nil {
		return false, fmt.Errorf("store: read token binding %q: %w", tokenID, err)
	}
	if bound != instanceID {
		return false, nil
	}
	seen, perr := time.Parse(time.RFC3339Nano, lastSeen)
	if lastIP != ip || perr != nil || now.Sub(seen) > tokenSeenWriteInterval {
		if _, err := db.ExecContext(ctx, `UPDATE api_token_bindings SET last_ip = ?, last_seen_at = ? WHERE token_id = ?`, ip, formatTime(now), tokenID); err != nil {
			return true, fmt.Errorf("store: touch token binding %q: %w", tokenID, err)
		}
	}
	return true, nil
}

// ListTokenBindings returns every binding keyed by token id.
func (db *DB) ListTokenBindings(ctx context.Context) (map[string]TokenBinding, error) {
	rows, err := db.QueryContext(ctx, `SELECT token_id, instance_id, bound_at, last_ip, last_seen_at FROM api_token_bindings`)
	if err != nil {
		return nil, fmt.Errorf("store: list token bindings: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]TokenBinding{}
	for rows.Next() {
		var b TokenBinding
		if err := rows.Scan(&b.TokenID, &b.InstanceID, &b.BoundAt, &b.LastIP, &b.LastSeenAt); err != nil {
			return nil, fmt.Errorf("store: scan token binding: %w", err)
		}
		out[b.TokenID] = b
	}
	return out, rows.Err()
}

package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// TokenPurgeKind picks which inactive tokens a purge may remove.
type TokenPurgeKind string

// Token purge kinds. Expired covers only tokens that were never revoked.
const (
	TokenPurgeRevoked TokenPurgeKind = "revoked"
	TokenPurgeExpired TokenPurgeKind = "expired"
)

type purgeTarget struct{ engineID, legacyID string }

type purgeSelect struct{ library, mirror string }

// Whole statements, one per kind, so no SQL is built from pieces. Library
// timestamps are unix microseconds; mirror timestamps are RFC 3339 text.
var purgeSelects = map[TokenPurgeKind]purgeSelect{
	TokenPurgeRevoked: {
		library: `SELECT e.id, COALESCE(m.legacy_id, '') FROM theauth_api_tokens e
			LEFT JOIN authengine_token_map m ON m.engine_id = e.id
			WHERE e.revoked_at IS NOT NULL AND e.revoked_at < ? ORDER BY e.created_at LIMIT ?`,
		mirror: `SELECT a.id FROM api_tokens a
			WHERE NOT EXISTS (SELECT 1 FROM authengine_token_map m WHERE m.legacy_id = a.id)
			AND a.revoked_at IS NOT NULL AND julianday(a.revoked_at) < julianday(?) ORDER BY a.created_at LIMIT ?`,
	},
	TokenPurgeExpired: {
		library: `SELECT e.id, COALESCE(m.legacy_id, '') FROM theauth_api_tokens e
			LEFT JOIN authengine_token_map m ON m.engine_id = e.id
			WHERE e.revoked_at IS NULL AND e.expires_at IS NOT NULL AND e.expires_at < ? ORDER BY e.created_at LIMIT ?`,
		mirror: `SELECT a.id FROM api_tokens a
			WHERE NOT EXISTS (SELECT 1 FROM authengine_token_map m WHERE m.legacy_id = a.id)
			AND a.revoked_at IS NULL AND a.expires_at IS NOT NULL AND julianday(a.expires_at) < julianday(?) ORDER BY a.created_at LIMIT ?`,
	},
}

type txRunner interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// PurgeInactiveAPITokens deletes up to limit tokens of kind whose revoked or
// expiry time is before cutoff, from the library table and the legacy mirror.
// With dryRun it only counts. An active token never matches.
func (db *DB) PurgeInactiveAPITokens(ctx context.Context, kind TokenPurgeKind, cutoff time.Time, limit int, dryRun bool) (int, error) {
	if limit < 1 {
		return 0, nil
	}
	q, ok := purgeSelects[kind]
	if !ok {
		return 0, fmt.Errorf("store: purge tokens: unknown kind %q", kind)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("store: purge tokens: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	targets, err := selectPurgeTargets(ctx, tx, q, cutoff, limit)
	if err != nil {
		return 0, err
	}
	if dryRun || len(targets) == 0 {
		return len(targets), nil
	}
	for _, t := range targets {
		if err := deleteTokenRows(ctx, tx, t); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("store: purge tokens: commit: %w", err)
	}
	return len(targets), nil
}

func selectPurgeTargets(ctx context.Context, tx *sql.Tx, q purgeSelect, cutoff time.Time, limit int) ([]purgeTarget, error) {
	var targets []purgeTarget
	rows, err := tx.QueryContext(ctx, q.library, cutoff.UnixMicro(), limit)
	if err != nil {
		return nil, fmt.Errorf("store: purge tokens: select library rows: %w", err)
	}
	for rows.Next() {
		var t purgeTarget
		if err := rows.Scan(&t.engineID, &t.legacyID); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("store: purge tokens: scan library row: %w", err)
		}
		targets = append(targets, t)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("store: purge tokens: close library rows: %w", err)
	}
	left := limit - len(targets)
	if left < 1 {
		return targets, nil
	}
	// Rows with an id-map entry were handled through the library side above.
	lrows, err := tx.QueryContext(ctx, q.mirror, formatTime(cutoff), left)
	if err != nil {
		return nil, fmt.Errorf("store: purge tokens: select mirror rows: %w", err)
	}
	for lrows.Next() {
		var t purgeTarget
		if err := lrows.Scan(&t.legacyID); err != nil {
			_ = lrows.Close()
			return nil, fmt.Errorf("store: purge tokens: scan mirror row: %w", err)
		}
		targets = append(targets, t)
	}
	if err := lrows.Close(); err != nil {
		return nil, fmt.Errorf("store: purge tokens: close mirror rows: %w", err)
	}
	return targets, nil
}

// deleteTokenRows removes a token and the side rows keyed by its ids. The
// audit log holds actor names only, so no history row points at these.
func deleteTokenRows(ctx context.Context, tx txRunner, t purgeTarget) error {
	for _, table := range []string{"api_token_scopes", "api_token_bindings", "token_hygiene_notices"} {
		for _, id := range []string{t.engineID, t.legacyID} {
			if id == "" {
				continue
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE token_id = ?`, id); err != nil {
				return fmt.Errorf("store: purge tokens: delete %s: %w", table, err)
			}
		}
	}
	if t.engineID != "" {
		if _, err := tx.ExecContext(ctx, `DELETE FROM authengine_token_map WHERE engine_id = ?`, t.engineID); err != nil {
			return fmt.Errorf("store: purge tokens: delete id map: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM theauth_api_tokens WHERE id = ?`, t.engineID); err != nil {
			return fmt.Errorf("store: purge tokens: delete library row: %w", err)
		}
	}
	if t.legacyID != "" {
		if _, err := tx.ExecContext(ctx, `DELETE FROM api_tokens WHERE id = ?`, t.legacyID); err != nil {
			return fmt.Errorf("store: purge tokens: delete mirror row: %w", err)
		}
	}
	return nil
}

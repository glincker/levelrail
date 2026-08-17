package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"fmt"
	"time"
)

// AuditEntry is one recorded request that passed internal/api's
// requireAbility at more than AbilityRead (migrations/0043): who did
// what, closing the gap docs/comparison.md's own "no audit log exists
// anywhere in the codebase" line calls out. CreatedAt is a pre-formatted
// RFC3339Nano string, not a time.Time, the same "caller controls the
// wire format" convention BackupHistory's own StartedAt/FinishedAt
// fields already establish in this package.
type AuditEntry struct {
	ID         string
	ActorType  string // "session" or "token"
	ActorID    string
	ActorName  string
	Ability    string
	Method     string
	Path       string
	StatusCode int
	RemoteAddr string
	CreatedAt  string
}

// auditTimeLayout formats a CreatedAt value with a fixed 9-digit
// fractional second (0s, not 9s, in the layout). time.RFC3339Nano
// strips trailing zeros, which would make two entries created at
// different times produce different-length strings; ListAuditEntries'
// before-cursor filter is a plain string comparison
// (`created_at < ?`), which is only correct when every stored value has
// identical width.
const auditTimeLayout = "2006-01-02T15:04:05.000000000Z"

// FormatAuditTime renders t as this package's audit_log.created_at
// format. Exported so internal/api's requireAbility, the only writer of
// AuditEntry.CreatedAt, produces values in the exact format
// ListAuditEntries' own cursor comparison assumes.
func FormatAuditTime(t time.Time) string {
	return t.UTC().Format(auditTimeLayout)
}

// auditEntryIDPrefix mirrors NewDeployAttemptID's own "short, greppable
// tag on an otherwise-opaque random ID" convention.
const auditEntryIDPrefix = "aud_"

// NewAuditEntryID generates an opaque, URL-safe audit entry identifier,
// minted the same way NewDeployAttemptID mints its own (fixed-length
// crypto/rand bytes, base64 URL encoding, a short prefix).
func NewAuditEntryID() (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("store: generate audit entry id: %w", err)
	}
	return auditEntryIDPrefix + base64.RawURLEncoding.EncodeToString(buf), nil
}

// SaveAuditEntry inserts a new audit log row. Insert-only, like
// SaveDeployAttempt: an audit log has no update or delete path through
// the app, an entry ID is minted fresh by every caller, and a duplicate
// ID would only ever indicate a caller bug, left to fail on the primary
// key constraint rather than silently overwriting history.
func (db *DB) SaveAuditEntry(ctx context.Context, e AuditEntry) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO audit_log (id, actor_type, actor_id, actor_name, ability, method, path, status_code, remote_addr, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, e.ID, e.ActorType, e.ActorID, e.ActorName, e.Ability, e.Method, e.Path, e.StatusCode, e.RemoteAddr, e.CreatedAt)
	if err != nil {
		return fmt.Errorf("store: save audit entry %q: %w", e.ID, err)
	}
	return nil
}

// ListAuditEntries returns up to limit audit log rows, newest first.
// before, when non-nil, restricts the result to entries strictly older
// than that timestamp, so a caller pages backward through a large table
// by re-issuing this call with the last returned row's CreatedAt: cursor
// pagination, not offset pagination, the same reasoning this function's
// own doc comment in the task spec calls for (an OFFSET query degrades
// linearly as the table grows; this one doesn't).
func (db *DB) ListAuditEntries(ctx context.Context, limit int, before *time.Time) ([]AuditEntry, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if before != nil {
		rows, err = db.QueryContext(ctx, `
			SELECT id, actor_type, actor_id, actor_name, ability, method, path, status_code, remote_addr, created_at
			FROM audit_log
			WHERE created_at < ?
			ORDER BY created_at DESC
			LIMIT ?
		`, FormatAuditTime(*before), limit)
	} else {
		rows, err = db.QueryContext(ctx, `
			SELECT id, actor_type, actor_id, actor_name, ability, method, path, status_code, remote_addr, created_at
			FROM audit_log
			ORDER BY created_at DESC
			LIMIT ?
		`, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("store: list audit entries: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []AuditEntry
	for rows.Next() {
		var e AuditEntry
		if err := rows.Scan(&e.ID, &e.ActorType, &e.ActorID, &e.ActorName, &e.Ability, &e.Method, &e.Path, &e.StatusCode, &e.RemoteAddr, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("store: scan audit entry row: %w", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate audit entry rows: %w", err)
	}
	return out, nil
}

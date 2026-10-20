package store

import (
	"context"
	"fmt"
	"time"
)

// Audit action names for the CLI device login lifecycle. Family is the
// prefix the audit log filter matches.
const (
	AuditActionFamilyDeviceLogin    = "device_login"
	AuditActionDeviceLoginApproved  = AuditActionFamilyDeviceLogin + ".approved"
	AuditActionDeviceLoginDenied    = AuditActionFamilyDeviceLogin + ".denied"
	AuditActionDeviceLoginExpired   = AuditActionFamilyDeviceLogin + ".expired"
	AuditActionDeviceLoginDismissed = AuditActionFamilyDeviceLogin + ".dismissed"
)

// Device login states as the dashboard, CLI and MCP name them.
const (
	DeviceLoginStateWaiting  = "waiting"
	DeviceLoginStateExpired  = "expired"
	DeviceLoginStateDenied   = "denied"
	DeviceLoginStateApproved = "approved"
)

// DeviceLoginAuditPath is the audit path shared by every entry about one
// request, so a single filter returns its whole history. The id is not a
// credential.
func DeviceLoginAuditPath(requestID string) string {
	return "/api/v1/auth/device/requests/" + requestID
}

// DeviceLoginItemKey identifies one state of one request for dismissal:
// a request that changes state gets a new key, so it reappears.
func DeviceLoginItemKey(requestID, state string) string {
	return AuditActionFamilyDeviceLogin + ":" + requestID + ":" + state
}

// ClaimDeviceLoginExpiry records that a request lapsed and writes its audit
// entry in one transaction. It reports false when the expiry was already
// claimed, so the audit entry exists exactly once however often this runs.
func (db *DB) ClaimDeviceLoginExpiry(ctx context.Context, requestID string, at time.Time, entry AuditEntry) (bool, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("store: claim device login expiry %q: begin: %w", requestID, err)
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx, `
		INSERT OR IGNORE INTO device_login_expiries (request_id, expired_at, audit_id) VALUES (?, ?, ?)
	`, requestID, FormatAuditTime(at), entry.ID)
	if err != nil {
		return false, fmt.Errorf("store: claim device login expiry %q: %w", requestID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("store: claim device login expiry %q: rows: %w", requestID, err)
	}
	if n == 0 {
		return false, nil
	}
	if err := insertAuditEntry(ctx, tx, entry); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("store: claim device login expiry %q: commit: %w", requestID, err)
	}
	return true, nil
}

// DeviceLoginExpiryClaimed reports whether a request's expiry was recorded.
func (db *DB) DeviceLoginExpiryClaimed(ctx context.Context, requestID string) (bool, error) {
	var n int
	err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM device_login_expiries WHERE request_id = ?`, requestID).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("store: check device login expiry %q: %w", requestID, err)
	}
	return n > 0, nil
}

// DismissAttentionItem stores a per-user dismissal and, when entry is
// non-nil, its audit entry in one transaction. It reports false when the
// item was already dismissed, writing nothing the second time.
func (db *DB) DismissAttentionItem(ctx context.Context, userID, itemKey string, at time.Time, entry *AuditEntry) (bool, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("store: dismiss attention item %q: begin: %w", itemKey, err)
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx, `
		INSERT OR IGNORE INTO attention_dismissals (user_id, item_key, dismissed_at) VALUES (?, ?, ?)
	`, userID, itemKey, FormatAuditTime(at))
	if err != nil {
		return false, fmt.Errorf("store: dismiss attention item %q: %w", itemKey, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("store: dismiss attention item %q: rows: %w", itemKey, err)
	}
	if n == 0 {
		return false, nil
	}
	if entry != nil {
		if err := insertAuditEntry(ctx, tx, *entry); err != nil {
			return false, err
		}
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("store: dismiss attention item %q: commit: %w", itemKey, err)
	}
	return true, nil
}

// ListAttentionDismissals returns the item keys a user dismissed.
func (db *DB) ListAttentionDismissals(ctx context.Context, userID string) (map[string]bool, error) {
	rows, err := db.QueryContext(ctx, `SELECT item_key FROM attention_dismissals WHERE user_id = ?`, userID)
	if err != nil {
		return nil, fmt.Errorf("store: list attention dismissals: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]bool{}
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, fmt.Errorf("store: scan attention dismissal: %w", err)
		}
		out[k] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate attention dismissals: %w", err)
	}
	return out, nil
}

// PruneAttentionState removes dismissals and expiry markers older than
// cutoff. Audit entries are never touched.
func (db *DB) PruneAttentionState(ctx context.Context, cutoff time.Time) error {
	c := FormatAuditTime(cutoff)
	if _, err := db.ExecContext(ctx, `DELETE FROM attention_dismissals WHERE dismissed_at < ?`, c); err != nil {
		return fmt.Errorf("store: prune attention dismissals: %w", err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM device_login_expiries WHERE expired_at < ?`, c); err != nil {
		return fmt.Errorf("store: prune device login expiries: %w", err)
	}
	return nil
}

package alerting

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// inboxBulkCap bounds how many items one bulk action or sweep touches per principal.
const inboxBulkCap = 5000

// Bulk scopes and actions accepted by ClearInbox.
const (
	InboxScopeRead     = "read"
	InboxScopeResolved = "resolved"
	InboxScopeArchived = "archived"
	InboxActionArchive = "archive"
	InboxActionDelete  = "delete"
)

// ErrInboxNotDeletable marks ids that are derived live (approvals, failed deploys, failed upgrades) and can only be archived.
var ErrInboxNotDeletable = fmt.Errorf("%w: only stored alert items can be deleted", ErrInboxInvalid)

// InboxSelector picks a principal's items for a bulk action.
type InboxSelector struct {
	Scope     string
	OlderThan time.Duration
	Now       time.Time
}

func archiveTx(ctx context.Context, tx *sql.Tx, principal string, ids []string, archived bool, now time.Time) error {
	for _, id := range ids {
		var err error
		if archived {
			_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO inbox_archive (principal, item_id, archived_at) VALUES (?, ?, ?)`, principal, id, fmtTime(now))
		} else {
			_, err = tx.ExecContext(ctx, `DELETE FROM inbox_archive WHERE principal = ? AND item_id = ?`, principal, id)
		}
		if err != nil {
			return fmt.Errorf("alerting: archive inbox: %w", err)
		}
	}
	return nil
}

func deleteTx(ctx context.Context, tx *sql.Tx, principal string, ids []string, restore bool, now time.Time) error {
	for _, id := range ids {
		var err error
		if restore {
			_, err = tx.ExecContext(ctx, `DELETE FROM inbox_deleted WHERE principal = ? AND item_id = ?`, principal, id)
		} else {
			_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO inbox_deleted (principal, item_id, deleted_at) VALUES (?, ?, ?)`, principal, id, fmtTime(now))
		}
		if err != nil {
			return fmt.Errorf("alerting: delete inbox: %w", err)
		}
	}
	return nil
}

// DeleteInbox hides stored alert items from the principal's inbox, or brings them back when restore is true.
// Alert history itself is kept; ids of derived items are rejected so they can only be archived.
func (db *DB) DeleteInbox(ctx context.Context, principal string, ids []string, restore bool, now time.Time) error {
	if len(ids) == 0 || len(ids) > inboxMaxMarkIDs {
		return fmt.Errorf("%w: ids must hold 1 to %d entries", ErrInboxInvalid, inboxMaxMarkIDs)
	}
	stored, err := db.storedAlertIDs(ctx, ids)
	if err != nil {
		return err
	}
	if len(stored) != len(ids) {
		return ErrInboxNotDeletable
	}
	return db.inTx(ctx, "delete inbox", func(tx *sql.Tx) error { return deleteTx(ctx, tx, principal, ids, restore, now) })
}

func (db *DB) inTx(ctx context.Context, what string, fn func(*sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("alerting: %s: %w", what, err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("alerting: %s: %w", what, err)
	}
	return nil
}

// SelectInbox returns the ids of the principal's visible items that match sel.
func (db *DB) SelectInbox(ctx context.Context, principal string, sel InboxSelector) ([]string, error) {
	switch sel.Scope {
	case InboxScopeRead, InboxScopeResolved, InboxScopeArchived:
	default:
		return nil, fmt.Errorf("%w: scope must be read, resolved or archived", ErrInboxInvalid)
	}
	items, err := db.listInbox(ctx, principal, InboxFilter{Archived: sel.Scope == InboxScopeArchived}, inboxBulkCap)
	if err != nil {
		return nil, err
	}
	ids := []string{}
	for _, it := range items {
		if sel.OlderThan > 0 && sel.Now.Sub(it.At) < sel.OlderThan {
			continue
		}
		if (sel.Scope == InboxScopeRead && !it.Read) || (sel.Scope == InboxScopeResolved && !it.Resolved) {
			continue
		}
		ids = append(ids, it.ID)
	}
	return ids, nil
}

// ClearInbox archives or deletes every item sel matches and returns how many it touched. With dryRun it only counts.
func (db *DB) ClearInbox(ctx context.Context, principal, action string, sel InboxSelector, dryRun bool) (int, error) {
	if action != InboxActionArchive && action != InboxActionDelete {
		return 0, fmt.Errorf("%w: action must be archive or delete", ErrInboxInvalid)
	}
	if action == InboxActionArchive && sel.Scope == InboxScopeArchived {
		return 0, fmt.Errorf("%w: archived items are already archived", ErrInboxInvalid)
	}
	ids, err := db.SelectInbox(ctx, principal, sel)
	if err != nil {
		return 0, err
	}
	if action == InboxActionDelete {
		if ids, err = db.storedAlertIDs(ctx, ids); err != nil {
			return 0, err
		}
	}
	if dryRun || len(ids) == 0 {
		return len(ids), nil
	}
	err = db.inTx(ctx, "clear inbox", func(tx *sql.Tx) error {
		if action == InboxActionArchive {
			return archiveTx(ctx, tx, principal, ids, true, sel.Now)
		}
		return deleteTx(ctx, tx, principal, ids, false, sel.Now)
	})
	if err != nil {
		return 0, err
	}
	return len(ids), nil
}

// storedAlertIDs keeps the ids that have an alert history row; derived items have none.
func (db *DB) storedAlertIDs(ctx context.Context, ids []string) ([]string, error) {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		var one int
		err := db.QueryRowContext(ctx, `SELECT 1 FROM alert_history WHERE id = ?`, id).Scan(&one)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("alerting: check inbox item: %w", err)
		}
		out = append(out, id)
	}
	return out, nil
}

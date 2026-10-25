package store

import (
	"context"
	"fmt"
	"time"
)

// SetGitSourcePRCommentCommands toggles pull request comment commands for the
// app's connected git source; ErrGitSourceNotFound if none is connected.
func (db *DB) SetGitSourcePRCommentCommands(ctx context.Context, serviceName string, enabled bool) error {
	return db.setGitSourceBoolColumn(ctx, "pr_comment_commands", serviceName, enabled)
}

// SetGitSourcePRCommentCommandsForks toggles whether those commands are also
// accepted on pull requests from forks.
func (db *DB) SetGitSourcePRCommentCommandsForks(ctx context.Context, serviceName string, enabled bool) error {
	return db.setGitSourceBoolColumn(ctx, "pr_comment_commands_forks", serviceName, enabled)
}

// ClaimPRCommentCommand records that commentID was handled for serviceName and
// reports whether this call was the first to do so. Rows older than retain are
// pruned on the way in, which keeps the table bounded.
func (db *DB) ClaimPRCommentCommand(ctx context.Context, serviceName string, commentID int64, actor, command string, prNumber int, now time.Time, retain time.Duration) (bool, error) {
	if retain > 0 {
		if _, err := db.ExecContext(ctx, `DELETE FROM pr_comment_commands WHERE created_at < ?`, now.Add(-retain).UTC().Format(time.RFC3339)); err != nil {
			return false, fmt.Errorf("store: prune pr comment commands: %w", err)
		}
	}
	res, err := db.ExecContext(ctx, `
		INSERT OR IGNORE INTO pr_comment_commands (service_name, comment_id, actor, command, pr_number, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, serviceName, commentID, actor, command, prNumber, now.UTC().Format(time.RFC3339))
	if err != nil {
		return false, fmt.Errorf("store: claim pr comment command %d for %q: %w", commentID, serviceName, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("store: claim pr comment command rows affected: %w", err)
	}
	return n == 1, nil
}

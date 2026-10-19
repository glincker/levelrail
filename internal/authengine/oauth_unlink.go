package authengine

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// UnlinkOAuthIdentity removes a provider from a user's library account, the counterpart of
// LinkOAuthIdentity, so an unlinked provider can no longer sign in. Users not yet copied are skipped.
func (e *Engine) UnlinkOAuthIdentity(ctx context.Context, legacyUserID, provider string) error {
	rt := e.oauth
	if rt == nil {
		return nil
	}
	var engineID string
	err := rt.db.QueryRowContext(ctx, `SELECT engine_id FROM authengine_user_map WHERE legacy_id = ?`, legacyUserID).Scan(&engineID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("authengine: look up user map: %w", err)
	}
	if _, err := rt.db.ExecContext(ctx, `DELETE FROM theauth_oauth_accounts WHERE user_id = ? AND provider = ?`, engineID, provider); err != nil {
		return fmt.Errorf("authengine: unlink identity: %w", err)
	}
	return nil
}

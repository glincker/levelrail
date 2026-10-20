package authengine

import (
	"context"
	"database/sql"
	"fmt"
)

// DisableUser blocks every sign-in of a platform user and, in the same
// library call, revokes their sessions, API tokens and trusted devices.
func (e *Engine) DisableUser(ctx context.Context, legacyID string) error {
	engine, err := e.dir.EnsureEngineUser(ctx, legacyID)
	if err != nil {
		return err
	}
	id, err := parseULID(engine)
	if err != nil {
		return err
	}
	if err := e.auth.DisableUser(ctx, id); err != nil {
		return fmt.Errorf("authengine: disable user %s: %w", legacyID, err)
	}
	return nil
}

// EnableUser lifts DisableUser. Revoked sessions and tokens stay revoked.
func (e *Engine) EnableUser(ctx context.Context, legacyID string) error {
	engine, ok, err := e.dir.EngineUserID(ctx, legacyID)
	if err != nil || !ok {
		return err
	}
	id, err := parseULID(engine)
	if err != nil {
		return err
	}
	if err := e.auth.EnableUser(ctx, id); err != nil {
		return fmt.Errorf("authengine: enable user %s: %w", legacyID, err)
	}
	return nil
}

// SetMustChangePassword flags a platform user so every sign-in is refused
// until a password reset completes. Setting it revokes the user's sessions.
func (e *Engine) SetMustChangePassword(ctx context.Context, legacyID string, required bool) error {
	engine, err := e.dir.EnsureEngineUser(ctx, legacyID)
	if err != nil {
		return err
	}
	id, err := parseULID(engine)
	if err != nil {
		return err
	}
	if err := e.auth.SetMustChangePassword(ctx, id, required); err != nil {
		return fmt.Errorf("authengine: set must change password for %s: %w", legacyID, err)
	}
	return nil
}

// AccountState is a platform user's lifecycle flags.
type AccountState struct {
	Disabled           bool
	MustChangePassword bool
}

// AccountStates returns the flags of every platform user that has any set.
func (e *Engine) AccountStates(ctx context.Context) (map[string]AccountState, error) {
	rows, err := e.db.QueryContext(ctx, `
		SELECT m.legacy_id, s.disabled_at IS NOT NULL, s.must_change_password FROM theauth_account_states s
		JOIN authengine_user_map m ON m.engine_id = s.user_id`)
	if err != nil {
		return nil, fmt.Errorf("authengine: account states: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]AccountState{}
	for rows.Next() {
		var id string
		var st AccountState
		if err := rows.Scan(&id, &st.Disabled, &st.MustChangePassword); err != nil {
			return nil, fmt.Errorf("authengine: account states: %w", err)
		}
		out[id] = st
	}
	return out, rows.Err()
}

// UserDisabled reports whether a platform user is disabled.
func (e *Engine) UserDisabled(ctx context.Context, legacyID string) (bool, error) {
	return userDisabled(ctx, e.db, legacyID)
}

// userDisabled reads the library's account state through the id map.
func userDisabled(ctx context.Context, db *sql.DB, legacyID string) (bool, error) {
	var n int
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM theauth_account_states s
		JOIN authengine_user_map m ON m.engine_id = s.user_id
		WHERE m.legacy_id = ? AND s.disabled_at IS NOT NULL`, legacyID).Scan(&n); err != nil {
		return false, fmt.Errorf("authengine: disabled lookup: %w", err)
	}
	return n > 0, nil
}

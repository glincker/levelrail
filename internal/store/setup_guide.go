package store

import (
	"context"
	"fmt"
	"time"
)

// Setup guide override states an operator can record for a step.
const (
	SetupGuideOverrideSkipped = "skipped"
	SetupGuideOverrideDone    = "done"
)

// SetupGuideState is the persisted part of the Get started hub: whether it
// is hidden and which steps the operator overrode.
type SetupGuideState struct {
	Hidden    bool
	Overrides map[string]string
}

// GetSetupGuideState returns the hub's hidden flag and every step override.
func (db *DB) GetSetupGuideState(ctx context.Context) (SetupGuideState, error) {
	out := SetupGuideState{Overrides: map[string]string{}}
	var hidden int
	if err := db.QueryRowContext(ctx, `SELECT hidden FROM setup_guide_settings WHERE id = 1`).Scan(&hidden); err != nil {
		return out, fmt.Errorf("store: get setup guide settings: %w", err)
	}
	out.Hidden = hidden != 0
	rows, err := db.QueryContext(ctx, `SELECT step_id, state FROM setup_guide_step_overrides`)
	if err != nil {
		return out, fmt.Errorf("store: list setup guide overrides: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id, state string
		if err := rows.Scan(&id, &state); err != nil {
			return out, fmt.Errorf("store: scan setup guide override: %w", err)
		}
		out.Overrides[id] = state
	}
	if err := rows.Err(); err != nil {
		return out, fmt.Errorf("store: iterate setup guide overrides: %w", err)
	}
	return out, nil
}

// SetSetupGuideHidden records whether the hub is hidden for the instance.
func (db *DB) SetSetupGuideHidden(ctx context.Context, hidden bool, by string) error {
	v := 0
	if hidden {
		v = 1
	}
	_, err := db.ExecContext(ctx, `
		UPDATE setup_guide_settings SET hidden = ?, updated_by = ?, updated_at = ? WHERE id = 1
	`, v, by, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("store: set setup guide hidden: %w", err)
	}
	return nil
}

// SetSetupGuideOverride records an operator override for one step; an empty
// state clears it so the step goes back to its derived state.
func (db *DB) SetSetupGuideOverride(ctx context.Context, stepID, state, by string) error {
	if state == "" {
		if _, err := db.ExecContext(ctx, `DELETE FROM setup_guide_step_overrides WHERE step_id = ?`, stepID); err != nil {
			return fmt.Errorf("store: clear setup guide override: %w", err)
		}
		return nil
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO setup_guide_step_overrides (step_id, state, updated_by, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(step_id) DO UPDATE SET state = excluded.state, updated_by = excluded.updated_by, updated_at = excluded.updated_at
	`, stepID, state, by, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("store: set setup guide override: %w", err)
	}
	return nil
}

// ResetSetupGuide clears every override and un-hides the hub.
func (db *DB) ResetSetupGuide(ctx context.Context) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin reset setup guide: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM setup_guide_step_overrides`); err != nil {
		return fmt.Errorf("store: clear setup guide overrides: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE setup_guide_settings SET hidden = 0, updated_by = '', updated_at = ? WHERE id = 1`, time.Now().UTC().Format(time.RFC3339)); err != nil {
		return fmt.Errorf("store: unhide setup guide: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit reset setup guide: %w", err)
	}
	return nil
}

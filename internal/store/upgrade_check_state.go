package store

import (
	"context"
	"fmt"
)

// UpgradeCheckState is the persisted result of the scheduler's release lookups.
type UpgradeCheckState struct {
	LastCheckAt string
	LastOK      bool
	LastError   string
	Failures    int
	NextCheckAt string
	// LastGoodAt is when a lookup last returned a fresh release list.
	LastGoodAt string
	// LastSource is "api" or the name of the fallback source that answered last.
	LastSource string
}

// GetUpgradeCheckState returns the singleton row.
func (db *DB) GetUpgradeCheckState(ctx context.Context) (UpgradeCheckState, error) {
	var s UpgradeCheckState
	var ok int
	err := db.QueryRowContext(ctx, `SELECT last_check_at, last_ok, last_error, failures, next_check_at, last_good_at, last_source
		FROM upgrade_check_state WHERE id = 1`).Scan(&s.LastCheckAt, &ok, &s.LastError, &s.Failures, &s.NextCheckAt, &s.LastGoodAt, &s.LastSource)
	if err != nil {
		return UpgradeCheckState{}, fmt.Errorf("store: get upgrade check state: %w", err)
	}
	s.LastOK = ok != 0
	return s, nil
}

// SaveUpgradeCheckState replaces the singleton row.
func (db *DB) SaveUpgradeCheckState(ctx context.Context, s UpgradeCheckState) error {
	ok := 0
	if s.LastOK {
		ok = 1
	}
	_, err := db.ExecContext(ctx, `UPDATE upgrade_check_state SET last_check_at = ?, last_ok = ?, last_error = ?,
		failures = ?, next_check_at = ?, last_good_at = ?, last_source = ? WHERE id = 1`,
		s.LastCheckAt, ok, s.LastError, s.Failures, s.NextCheckAt, s.LastGoodAt, s.LastSource)
	if err != nil {
		return fmt.Errorf("store: save upgrade check state: %w", err)
	}
	return nil
}

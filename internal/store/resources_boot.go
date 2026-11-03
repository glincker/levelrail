package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strconv"
)

// ResourceBootGapFromEnv is how far a boot jumps the version counter.
func ResourceBootGapFromEnv() int64 {
	if n, err := strconv.ParseInt(os.Getenv(EnvResourceBootGap), 10, 64); err == nil && n > 0 {
		return n
	}
	return defaultResourceBootGap
}

// AdvanceResourceBootOnce runs AdvanceResourceBoot the first time it is called
// on this handle and does nothing afterwards, so building the router twice
// cannot expire the versions this process has already handed out.
func (db *DB) AdvanceResourceBootOnce(ctx context.Context, gap int64) error {
	var err error
	db.resourceBoot.Do(func() { err = db.AdvanceResourceBoot(ctx, gap) })
	return err
}

// AdvanceResourceBoot jumps the version counter by gap and raises the
// compaction watermark to the new base. After a restore or standby promote the
// counter can sit below versions clients already saw; without the jump those
// clients would resume silently and miss the reverted changes. Every watch
// from before the boot now gets 410 and relists, and a list taken after the
// boot returns the new base, which is not below the watermark. A log that has
// never issued a version has nothing to protect and is left alone, so a watch
// from 0 keeps meaning "from the beginning".
func (db *DB) AdvanceResourceBoot(ctx context.Context, gap int64) error {
	if gap < 0 {
		return fmt.Errorf("store: resource boot gap must not be negative")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin resource boot: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var seq int64
	err = tx.QueryRowContext(ctx, `SELECT seq FROM sqlite_sequence WHERE name = 'resource_changes'`).Scan(&seq)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("store: read resource sequence: %w", err)
	}
	if seq == 0 {
		return nil
	}
	next := seq + gap
	if _, err := tx.ExecContext(ctx, `UPDATE sqlite_sequence SET seq = ? WHERE name = 'resource_changes'`, next); err != nil {
		return fmt.Errorf("store: advance resource sequence: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE resource_compaction SET compacted_through = MAX(compacted_through, ?) WHERE id = 1`, next); err != nil {
		return fmt.Errorf("store: raise resource watermark: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit resource boot: %w", err)
	}
	return nil
}

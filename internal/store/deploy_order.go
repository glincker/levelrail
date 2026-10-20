package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// DeployOrder is where one deploy sits in its service's history: the
// trigger-time Sequence plus, for a git push, the commit ordering the
// provider reported.
type DeployOrder struct {
	Sequence int64
	// Automatic marks a webhook, schedule or pipeline deploy. Manual deploys
	// and explicit rollbacks are never rejected as stale.
	Automatic bool
	CommitSHA string
	// Before is the commit the push moved the branch from.
	Before   string
	CommitAt time.Time
}

// DeployCursor is the ordering state of the last deploy that actually wrote
// a service's desired state.
type DeployCursor struct {
	AppliedSequence int64
	CommitSHA       string
	Before          string
	CommitAt        time.Time
}

// DeployOrderCheck decides whether order may still write desired state given
// cursor; a non-nil error aborts the write.
type DeployOrderCheck func(cursor DeployCursor, order DeployOrder) error

// NextDeploySequence hands out serviceName's next trigger sequence, starting
// at 1. Sequences only grow, so a later trigger always sorts after an earlier one.
func (db *DB) NextDeploySequence(ctx context.Context, serviceName string) (int64, error) {
	var seq int64
	err := db.QueryRowContext(ctx, `
		INSERT INTO deploy_cursors (service_name, next_sequence) VALUES (?, 1)
		ON CONFLICT (service_name) DO UPDATE SET next_sequence = next_sequence + 1,
			updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		RETURNING next_sequence
	`, serviceName).Scan(&seq)
	if err != nil {
		return 0, fmt.Errorf("store: next deploy sequence for %q: %w", serviceName, err)
	}
	return seq, nil
}

// GetDeployCursor returns serviceName's cursor, the zero value if it has none.
func (db *DB) GetDeployCursor(ctx context.Context, serviceName string) (DeployCursor, error) {
	return getDeployCursor(ctx, db.QueryRowContext, serviceName)
}

func getDeployCursor(ctx context.Context, query func(ctx context.Context, q string, args ...any) *sql.Row, serviceName string) (DeployCursor, error) {
	var (
		c        DeployCursor
		commitAt string
	)
	err := query(ctx, `
		SELECT applied_sequence, applied_commit, applied_before, applied_commit_at
		FROM deploy_cursors WHERE service_name = ?
	`, serviceName).Scan(&c.AppliedSequence, &c.CommitSHA, &c.Before, &commitAt)
	if errors.Is(err, sql.ErrNoRows) {
		return DeployCursor{}, nil
	}
	if err != nil {
		return DeployCursor{}, fmt.Errorf("store: get deploy cursor for %q: %w", serviceName, err)
	}
	if commitAt != "" {
		t, err := time.Parse(time.RFC3339Nano, commitAt)
		if err != nil {
			return DeployCursor{}, fmt.Errorf("store: get deploy cursor for %q: parse commit time: %w", serviceName, err)
		}
		c.CommitAt = t
	}
	return c, nil
}

// SaveDesiredServiceOrdered is SaveDesiredService guarded by check, in one
// transaction: check sees the cursor as of this write, and on success the
// cursor advances to order. Manual orders keep the last automatic commit so
// a later stale webhook is still judged against real commit history.
func (db *DB) SaveDesiredServiceOrdered(ctx context.Context, svc DesiredService, order DeployOrder, check DeployOrderCheck) error {
	return db.saveDesiredService(ctx, svc, func(tx *sql.Tx) error {
		cursor, err := getDeployCursor(ctx, tx.QueryRowContext, svc.Name)
		if err != nil {
			return err
		}
		if check != nil {
			if err := check(cursor, order); err != nil {
				return err
			}
		}
		return advanceDeployCursor(ctx, tx, svc.Name, cursor, order)
	})
}

func advanceDeployCursor(ctx context.Context, tx *sql.Tx, serviceName string, cursor DeployCursor, order DeployOrder) error {
	next := cursor
	if order.Sequence > next.AppliedSequence {
		next.AppliedSequence = order.Sequence
	}
	if order.Automatic && order.CommitSHA != "" {
		next.CommitSHA, next.Before, next.CommitAt = order.CommitSHA, order.Before, order.CommitAt
	}
	commitAt := ""
	if !next.CommitAt.IsZero() {
		commitAt = next.CommitAt.UTC().Format(time.RFC3339Nano)
	}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO deploy_cursors (service_name, next_sequence, applied_sequence, applied_commit, applied_before, applied_commit_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (service_name) DO UPDATE SET
			next_sequence = MAX(next_sequence, excluded.applied_sequence),
			applied_sequence = excluded.applied_sequence,
			applied_commit = excluded.applied_commit,
			applied_before = excluded.applied_before,
			applied_commit_at = excluded.applied_commit_at,
			updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
	`, serviceName, next.AppliedSequence, next.AppliedSequence, next.CommitSHA, next.Before, commitAt)
	if err != nil {
		return fmt.Errorf("store: advance deploy cursor for %q: %w", serviceName, err)
	}
	return nil
}

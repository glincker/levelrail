package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// envRevisionTimeLayout is fixed width so stored times compare as text.
const envRevisionTimeLayout = "2006-01-02T15:04:05.000000000Z"

// ErrEnvRevisionNotFound means the app has no revision with that number.
var ErrEnvRevisionNotFound = errors.New("store: env revision not found")

// EnvRevision is one saved env state of an app. Snapshot is ciphertext, or nil when no master key was loaded.
type EnvRevision struct {
	AppName  string
	Rev      int
	At       time.Time
	Actor    string
	Names    []string
	Snapshot []byte
}

// AddEnvRevision stores a revision as the app's newest, then prunes: only the newest keep revisions
// survive, and older than maxAge (when positive) go too, but the newest one always stays.
func (db *DB) AddEnvRevision(ctx context.Context, app, actor string, names []string, snapshot []byte, keep int, maxAge time.Duration) (int, error) {
	if names == nil {
		names = []string{}
	}
	namesJSON, err := json.Marshal(names)
	if err != nil {
		return 0, fmt.Errorf("store: add env revision: %w", err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("store: add env revision: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var rev int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(rev), 0) + 1 FROM env_revisions WHERE app_name = ?`, app).Scan(&rev); err != nil {
		return 0, fmt.Errorf("store: add env revision: %w", err)
	}
	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO env_revisions (app_name, rev, at, actor, names, snapshot) VALUES (?, ?, ?, ?, ?, ?)`,
		app, rev, now.Format(envRevisionTimeLayout), actor, string(namesJSON), snapshot); err != nil {
		return 0, fmt.Errorf("store: add env revision: %w", err)
	}
	if keep < 1 {
		keep = 1
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM env_revisions WHERE app_name = ? AND rev <= ?`, app, rev-keep); err != nil {
		return 0, fmt.Errorf("store: prune env revisions: %w", err)
	}
	if maxAge > 0 {
		cutoff := now.Add(-maxAge).Format(envRevisionTimeLayout)
		if _, err := tx.ExecContext(ctx, `DELETE FROM env_revisions WHERE app_name = ? AND rev < ? AND at < ?`, app, rev, cutoff); err != nil {
			return 0, fmt.Errorf("store: prune env revisions: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("store: add env revision: %w", err)
	}
	return rev, nil
}

// ListEnvRevisions returns an app's revisions newest first, without snapshots.
func (db *DB) ListEnvRevisions(ctx context.Context, app string, limit int) ([]EnvRevision, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT rev, at, actor, names FROM env_revisions WHERE app_name = ? ORDER BY rev DESC LIMIT ?`, app, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list env revisions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []EnvRevision
	for rows.Next() {
		r := EnvRevision{AppName: app}
		var at, names string
		if err := rows.Scan(&r.Rev, &at, &r.Actor, &names); err != nil {
			return nil, fmt.Errorf("store: scan env revision: %w", err)
		}
		r.At, _ = time.Parse(envRevisionTimeLayout, at)
		_ = json.Unmarshal([]byte(names), &r.Names)
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate env revisions: %w", err)
	}
	return out, nil
}

// GetEnvRevision returns one revision including its encrypted snapshot.
func (db *DB) GetEnvRevision(ctx context.Context, app string, rev int) (EnvRevision, error) {
	r := EnvRevision{AppName: app, Rev: rev}
	var at, names string
	err := db.QueryRowContext(ctx, `SELECT at, actor, names, snapshot FROM env_revisions WHERE app_name = ? AND rev = ?`, app, rev).
		Scan(&at, &r.Actor, &names, &r.Snapshot)
	if errors.Is(err, sql.ErrNoRows) {
		return EnvRevision{}, ErrEnvRevisionNotFound
	}
	if err != nil {
		return EnvRevision{}, fmt.Errorf("store: get env revision: %w", err)
	}
	r.At, _ = time.Parse(envRevisionTimeLayout, at)
	_ = json.Unmarshal([]byte(names), &r.Names)
	return r, nil
}

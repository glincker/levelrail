package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrGitPushRemoteNotFound means the app has no git push remote.
var ErrGitPushRemoteNotFound = errors.New("store: git push remote not found")

// GitPushRemote is an app's git push deploy remote. TokenHash is the SHA-256 of
// the push token; the token itself is never stored.
type GitPushRemote struct {
	ServiceName    string
	TokenHash      string
	Branch         string
	CreatedAt      time.Time
	RotatedAt      time.Time
	LastPushAt     time.Time
	LastPushCommit string
	LastPushResult string
}

func parseOptionalTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// GetGitPushRemote loads the remote for an app.
func (db *DB) GetGitPushRemote(ctx context.Context, name string) (*GitPushRemote, error) {
	var g GitPushRemote
	var created, rotated, lastAt string
	err := db.QueryRowContext(ctx, `
		SELECT service_name, token_hash, branch, created_at, rotated_at, last_push_at, last_push_commit, last_push_result
		FROM git_push_remotes WHERE service_name = ?`, name).
		Scan(&g.ServiceName, &g.TokenHash, &g.Branch, &created, &rotated, &lastAt, &g.LastPushCommit, &g.LastPushResult)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrGitPushRemoteNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: get git push remote for %q: %w", name, err)
	}
	g.CreatedAt, g.RotatedAt, g.LastPushAt = parseOptionalTime(created), parseOptionalTime(rotated), parseOptionalTime(lastAt)
	return &g, nil
}

// SaveGitPushRemoteToken creates the remote or replaces its token hash,
// keeping the branch and push history on a rotation.
func (db *DB) SaveGitPushRemoteToken(ctx context.Context, name, tokenHash, defaultBranch string, now time.Time) error {
	ts := formatOptionalTime(now)
	_, err := db.ExecContext(ctx, `
		INSERT INTO git_push_remotes (service_name, token_hash, branch, created_at, rotated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(service_name) DO UPDATE SET token_hash = excluded.token_hash, rotated_at = excluded.rotated_at`,
		name, tokenHash, defaultBranch, ts, ts)
	if err != nil {
		return fmt.Errorf("store: save git push remote for %q: %w", name, err)
	}
	return nil
}

// SetGitPushRemoteBranch changes the branch that deploys.
func (db *DB) SetGitPushRemoteBranch(ctx context.Context, name, branch string) error {
	res, err := db.ExecContext(ctx, `UPDATE git_push_remotes SET branch = ? WHERE service_name = ?`, branch, name)
	if err != nil {
		return fmt.Errorf("store: set git push branch for %q: %w", name, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrGitPushRemoteNotFound
	}
	return nil
}

// RecordGitPush stores the outcome of the latest push.
func (db *DB) RecordGitPush(ctx context.Context, name, commit, result string, at time.Time) error {
	_, err := db.ExecContext(ctx, `
		UPDATE git_push_remotes SET last_push_at = ?, last_push_commit = ?, last_push_result = ? WHERE service_name = ?`,
		formatOptionalTime(at), commit, result, name)
	if err != nil {
		return fmt.Errorf("store: record git push for %q: %w", name, err)
	}
	return nil
}

// DeleteGitPushRemote removes the remote, which revokes its token.
func (db *DB) DeleteGitPushRemote(ctx context.Context, name string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM git_push_remotes WHERE service_name = ?`, name); err != nil {
		return fmt.Errorf("store: delete git push remote for %q: %w", name, err)
	}
	return nil
}

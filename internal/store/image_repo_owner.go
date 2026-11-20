package store

import (
	"context"
	"fmt"
)

// ClaimImageRepo records app as the owner of repo unless another app already
// holds it, and returns the owner. The primary key makes concurrent claims atomic.
func (db *DB) ClaimImageRepo(ctx context.Context, repo, app string) (string, error) {
	if _, err := db.ExecContext(ctx, `INSERT INTO image_repo_owners (repo, app_name) VALUES (?, ?) ON CONFLICT(repo) DO NOTHING`, repo, app); err != nil {
		return "", fmt.Errorf("store: claim image repo: %w", err)
	}
	return db.ImageRepoOwner(ctx, repo)
}

// ImageRepoOwner returns the app that owns repo, or "" when unclaimed.
func (db *DB) ImageRepoOwner(ctx context.Context, repo string) (string, error) {
	var owner string
	rows, err := db.QueryContext(ctx, `SELECT app_name FROM image_repo_owners WHERE repo = ?`, repo)
	if err != nil {
		return "", fmt.Errorf("store: image repo owner: %w", err)
	}
	defer func() { _ = rows.Close() }()
	if rows.Next() {
		if err := rows.Scan(&owner); err != nil {
			return "", fmt.Errorf("store: image repo owner: %w", err)
		}
	}
	return owner, rows.Err()
}

// ReleaseImageRepos frees every repo an app owned.
func (db *DB) ReleaseImageRepos(ctx context.Context, app string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM image_repo_owners WHERE app_name = ?`, app); err != nil {
		return fmt.Errorf("store: release image repos: %w", err)
	}
	return nil
}
